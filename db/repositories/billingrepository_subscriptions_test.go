package repositories

import (
	"testing"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBillingRepositoryDB_CreateSubscription(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	stripeID := "sub_123"
	sub := &db.BillingSubscription{
		Subject:               "user123",
		PlanKey:               "pro",
		Status:                db.SubscriptionActive,
		StripeSubscriptionID:  &stripeID,
		StripePaymentMethodID: "pm_123",
	}

	err := repo.CreateSubscription(sub)
	require.NoError(t, err)
	assert.NotEmpty(t, sub.ID)
}

func TestBillingRepositoryDB_CreateSubscription_DuplicateStripeID(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	stripeID := "sub_123"
	sub1 := &db.BillingSubscription{
		Subject:              "user123",
		PlanKey:              "pro",
		Status:               db.SubscriptionActive,
		StripeSubscriptionID: &stripeID,
	}
	require.NoError(t, repo.CreateSubscription(sub1))

	// Try to create with the same Stripe ID
	sub2 := &db.BillingSubscription{
		Subject:              "user456",
		PlanKey:              "pro",
		Status:               db.SubscriptionActive,
		StripeSubscriptionID: &stripeID,
	}
	err := repo.CreateSubscription(sub2)
	assert.ErrorIs(t, err, ErrDuplicate)
}

func TestBillingRepositoryDB_GetSubscription(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	stripeID := "sub_456"
	sub := &db.BillingSubscription{
		Subject:              "user123",
		PlanKey:              "basic",
		Status:               db.SubscriptionTrialing,
		StripeSubscriptionID: &stripeID,
	}
	require.NoError(t, repo.CreateSubscription(sub))

	retrieved, err := repo.GetSubscription(sub.ID)
	require.NoError(t, err)
	assert.Equal(t, sub.ID, retrieved.ID)
	assert.Equal(t, db.SubscriptionTrialing, retrieved.Status)
}

func TestBillingRepositoryDB_GetSubscription_NotFound(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	_, err := repo.GetSubscription("nonexistent")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestBillingRepositoryDB_GetSubscriptionByStripeID(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	stripeID := "sub_789"
	sub := &db.BillingSubscription{
		Subject:              "user123",
		PlanKey:              "pro",
		Status:               db.SubscriptionActive,
		StripeSubscriptionID: &stripeID,
	}
	require.NoError(t, repo.CreateSubscription(sub))

	retrieved, err := repo.GetSubscriptionByStripeID(stripeID)
	require.NoError(t, err)
	assert.Equal(t, sub.ID, retrieved.ID)
}

func TestBillingRepositoryDB_UpdateSubscription(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	stripeID := "sub_123"
	sub := &db.BillingSubscription{
		Subject:              "user123",
		PlanKey:              "pro",
		Status:               db.SubscriptionTrialing,
		StripeSubscriptionID: &stripeID,
	}
	require.NoError(t, repo.CreateSubscription(sub))

	sub.Status = db.SubscriptionActive
	err := repo.UpdateSubscription(sub)
	require.NoError(t, err)

	retrieved, err := repo.GetSubscription(sub.ID)
	require.NoError(t, err)
	assert.Equal(t, db.SubscriptionActive, retrieved.Status)
}

func TestBillingRepositoryDB_ListSubscriptions_NoFilter(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	stripeID1 := "sub_1"
	sub1 := &db.BillingSubscription{
		Subject:              "user1",
		PlanKey:              "pro",
		Status:               db.SubscriptionActive,
		StripeSubscriptionID: &stripeID1,
	}
	stripeID2 := "sub_2"
	sub2 := &db.BillingSubscription{
		Subject:              "user2",
		PlanKey:              "basic",
		Status:               db.SubscriptionTrialing,
		StripeSubscriptionID: &stripeID2,
	}
	require.NoError(t, repo.CreateSubscription(sub1))
	require.NoError(t, repo.CreateSubscription(sub2))

	subs, err := repo.ListSubscriptions(SubscriptionFilter{})
	require.NoError(t, err)
	assert.Len(t, subs, 2)
	// Newest first
	assert.Equal(t, sub2.ID, subs[0].ID)
	assert.Equal(t, sub1.ID, subs[1].ID)
}

func TestBillingRepositoryDB_ListSubscriptions_FilterBySubject(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	stripeID1 := "sub_1"
	sub1 := &db.BillingSubscription{
		Subject:              "user1",
		PlanKey:              "pro",
		Status:               db.SubscriptionActive,
		StripeSubscriptionID: &stripeID1,
	}
	stripeID2 := "sub_2"
	sub2 := &db.BillingSubscription{
		Subject:              "user2",
		PlanKey:              "basic",
		Status:               db.SubscriptionTrialing,
		StripeSubscriptionID: &stripeID2,
	}
	require.NoError(t, repo.CreateSubscription(sub1))
	require.NoError(t, repo.CreateSubscription(sub2))

	subs, err := repo.ListSubscriptions(SubscriptionFilter{Subject: "user1"})
	require.NoError(t, err)
	assert.Len(t, subs, 1)
	assert.Equal(t, "user1", subs[0].Subject)
}

func TestBillingRepositoryDB_ListSubscriptions_FilterByStatus(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	stripeID1 := "sub_1"
	sub1 := &db.BillingSubscription{
		Subject:              "user1",
		Status:               db.SubscriptionActive,
		StripeSubscriptionID: &stripeID1,
	}
	stripeID2 := "sub_2"
	sub2 := &db.BillingSubscription{
		Subject:              "user2",
		Status:               db.SubscriptionTrialing,
		StripeSubscriptionID: &stripeID2,
	}
	require.NoError(t, repo.CreateSubscription(sub1))
	require.NoError(t, repo.CreateSubscription(sub2))

	subs, err := repo.ListSubscriptions(SubscriptionFilter{Status: db.SubscriptionActive})
	require.NoError(t, err)
	assert.Len(t, subs, 1)
	assert.Equal(t, db.SubscriptionActive, subs[0].Status)
}

func TestBillingRepositoryDB_ListSubscriptionsBySubjects(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	stripeID1 := "sub_1"
	sub1 := &db.BillingSubscription{
		Subject:              "user1",
		Status:               db.SubscriptionActive,
		StripeSubscriptionID: &stripeID1,
	}
	stripeID2 := "sub_2"
	sub2 := &db.BillingSubscription{
		Subject:              "user2",
		Status:               db.SubscriptionActive,
		StripeSubscriptionID: &stripeID2,
	}
	stripeID3 := "sub_3"
	sub3 := &db.BillingSubscription{
		Subject:              "user3",
		Status:               db.SubscriptionActive,
		StripeSubscriptionID: &stripeID3,
	}
	require.NoError(t, repo.CreateSubscription(sub1))
	require.NoError(t, repo.CreateSubscription(sub2))
	require.NoError(t, repo.CreateSubscription(sub3))

	subs, err := repo.ListSubscriptionsBySubjects([]string{"user1", "user3"})
	require.NoError(t, err)
	assert.Len(t, subs, 2)
}
