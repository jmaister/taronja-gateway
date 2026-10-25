package repositories

import (
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBillingRepositoryDB_CreatePurchase(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	purchase := &db.BillingPurchase{
		Subject:               "user123",
		PlanKey:               "basic",
		Status:                db.PurchaseStatusPaid,
		AmountCents:           1000,
		Currency:              "eur",
		StripePaymentIntentID: "pi_123",
		PaidAt:                time.Now().UTC(),
	}

	err := repo.CreatePurchase(purchase)
	require.NoError(t, err)
	assert.NotEmpty(t, purchase.ID)
}

func TestBillingRepositoryDB_CreatePurchase_DuplicatePaymentIntent(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	purchase1 := &db.BillingPurchase{
		Subject:               "user123",
		PlanKey:               "basic",
		Status:                db.PurchaseStatusPaid,
		AmountCents:           1000,
		Currency:              "eur",
		StripePaymentIntentID: "pi_123",
		PaidAt:                time.Now().UTC(),
	}
	require.NoError(t, repo.CreatePurchase(purchase1))

	// Try to create with the same payment intent
	purchase2 := &db.BillingPurchase{
		Subject:               "user456",
		PlanKey:               "pro",
		Status:                db.PurchaseStatusPaid,
		AmountCents:           5000,
		Currency:              "eur",
		StripePaymentIntentID: "pi_123",
		PaidAt:                time.Now().UTC(),
	}
	err := repo.CreatePurchase(purchase2)
	assert.ErrorIs(t, err, ErrDuplicate)
}

func TestBillingRepositoryDB_GetPurchase(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	purchase := &db.BillingPurchase{
		Subject:               "user123",
		PlanKey:               "basic",
		Status:                db.PurchaseStatusPaid,
		AmountCents:           1000,
		Currency:              "eur",
		StripePaymentIntentID: "pi_456",
		PaidAt:                time.Now().UTC(),
	}
	require.NoError(t, repo.CreatePurchase(purchase))

	retrieved, err := repo.GetPurchase(purchase.ID)
	require.NoError(t, err)
	assert.Equal(t, purchase.ID, retrieved.ID)
	assert.Equal(t, int64(1000), retrieved.AmountCents)
}

func TestBillingRepositoryDB_GetPurchase_NotFound(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	_, err := repo.GetPurchase("nonexistent")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestBillingRepositoryDB_GetPurchaseByPaymentIntent(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	purchase := &db.BillingPurchase{
		Subject:               "user123",
		PlanKey:               "pro",
		Status:                db.PurchaseStatusPaid,
		AmountCents:           5000,
		Currency:              "eur",
		StripePaymentIntentID: "pi_789",
		PaidAt:                time.Now().UTC(),
	}
	require.NoError(t, repo.CreatePurchase(purchase))

	retrieved, err := repo.GetPurchaseByPaymentIntent("pi_789")
	require.NoError(t, err)
	assert.Equal(t, purchase.ID, retrieved.ID)
}

func TestBillingRepositoryDB_UpdatePurchase(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	purchase := &db.BillingPurchase{
		Subject:               "user123",
		PlanKey:               "basic",
		Status:                db.PurchaseStatusPaid,
		AmountCents:           1000,
		Currency:              "eur",
		StripePaymentIntentID: "pi_123",
		PaidAt:                time.Now().UTC(),
	}
	require.NoError(t, repo.CreatePurchase(purchase))

	purchase.RefundedCents = 500
	err := repo.UpdatePurchase(purchase)
	require.NoError(t, err)

	retrieved, err := repo.GetPurchase(purchase.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(500), retrieved.RefundedCents)
}

func TestBillingRepositoryDB_ListPurchases_NoFilter(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	now := time.Now().UTC()
	purchase1 := &db.BillingPurchase{
		Subject:               "user1",
		Status:                db.PurchaseStatusPaid,
		AmountCents:           1000,
		StripePaymentIntentID: "pi_1",
		PaidAt:                now,
	}
	purchase2 := &db.BillingPurchase{
		Subject:               "user2",
		Status:                db.PurchaseStatusPaid,
		AmountCents:           2000,
		StripePaymentIntentID: "pi_2",
		PaidAt:                now.Add(time.Second),
	}
	require.NoError(t, repo.CreatePurchase(purchase1))
	require.NoError(t, repo.CreatePurchase(purchase2))

	purchases, err := repo.ListPurchases(PurchaseFilter{})
	require.NoError(t, err)
	assert.Len(t, purchases, 2)
	// Newest paid first
	assert.Equal(t, purchase2.ID, purchases[0].ID)
	assert.Equal(t, purchase1.ID, purchases[1].ID)
}

func TestBillingRepositoryDB_ListPurchases_FilterBySince(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	now := time.Now().UTC()
	purchase1 := &db.BillingPurchase{
		Subject:               "user1",
		Status:                db.PurchaseStatusPaid,
		AmountCents:           1000,
		StripePaymentIntentID: "pi_1",
		PaidAt:                now.Add(-time.Hour),
	}
	purchase2 := &db.BillingPurchase{
		Subject:               "user2",
		Status:                db.PurchaseStatusPaid,
		AmountCents:           2000,
		StripePaymentIntentID: "pi_2",
		PaidAt:                now,
	}
	require.NoError(t, repo.CreatePurchase(purchase1))
	require.NoError(t, repo.CreatePurchase(purchase2))

	since := now.Add(-30 * time.Minute)
	purchases, err := repo.ListPurchases(PurchaseFilter{Since: &since})
	require.NoError(t, err)
	assert.Len(t, purchases, 1)
	assert.Equal(t, purchase2.ID, purchases[0].ID)
}

func TestBillingRepositoryDB_ListPurchasesBySubjects(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	now := time.Now().UTC()
	purchase1 := &db.BillingPurchase{
		Subject:               "user1",
		Status:                db.PurchaseStatusPaid,
		AmountCents:           1000,
		StripePaymentIntentID: "pi_1",
		PaidAt:                now,
	}
	purchase2 := &db.BillingPurchase{
		Subject:               "user2",
		Status:                db.PurchaseStatusPaid,
		AmountCents:           2000,
		StripePaymentIntentID: "pi_2",
		PaidAt:                now,
	}
	purchase3 := &db.BillingPurchase{
		Subject:               "user3",
		Status:                db.PurchaseStatusPaid,
		AmountCents:           3000,
		StripePaymentIntentID: "pi_3",
		PaidAt:                now,
	}
	require.NoError(t, repo.CreatePurchase(purchase1))
	require.NoError(t, repo.CreatePurchase(purchase2))
	require.NoError(t, repo.CreatePurchase(purchase3))

	purchases, err := repo.ListPurchasesBySubjects([]string{"user1", "user3"})
	require.NoError(t, err)
	assert.Len(t, purchases, 2)
}
