package repositories

import (
	"errors"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBillingRepositoryDB_Transaction_Commits(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	err := repo.Transaction(func(txRepo BillingRepository) error {
		plan := &BillingPlanWithFeatures{
			BillingPlan: db.BillingPlan{
				Key:         "test",
				Name:        "Test Plan",
				Kind:        db.PlanKindSubscription,
				AmountCents: 1000,
				Currency:    "eur",
			},
			Features: []string{"f1"},
		}
		return txRepo.SavePlan(plan)
	})

	require.NoError(t, err)

	// Verify the plan was saved
	plan, err := repo.GetPlan("test")
	require.NoError(t, err)
	assert.Equal(t, "test", plan.Key)
}

func TestBillingRepositoryDB_Transaction_Rollback(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	err := repo.Transaction(func(txRepo BillingRepository) error {
		plan := &BillingPlanWithFeatures{
			BillingPlan: db.BillingPlan{
				Key:         "test",
				Name:        "Test Plan",
				Kind:        db.PlanKindSubscription,
				AmountCents: 1000,
				Currency:    "eur",
			},
			Features: []string{"f1"},
		}
		err := txRepo.SavePlan(plan)
		if err != nil {
			return err
		}

		// Return an error to trigger rollback
		return errors.New("intentional error")
	})

	require.Error(t, err)

	// Verify the plan was NOT saved
	_, err = repo.GetPlan("test")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestBillingRepositoryDB_Transaction_MultipleOperations(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	now := time.Now().UTC()
	err := repo.Transaction(func(txRepo BillingRepository) error {
		// Create a plan
		plan := &BillingPlanWithFeatures{
			BillingPlan: db.BillingPlan{
				Key:         "basic",
				Name:        "Basic Plan",
				Kind:        db.PlanKindSubscription,
				AmountCents: 1000,
				Currency:    "eur",
			},
			Features: []string{"f1"},
		}
		err := txRepo.SavePlan(plan)
		if err != nil {
			return err
		}

		// Create a customer
		customer := &db.BillingCustomer{
			Subject:          "user123",
			StripeCustomerID: "cus_123",
			Name:             "Test User",
		}
		err = txRepo.SaveCustomer(customer)
		if err != nil {
			return err
		}

		// Create a subscription
		stripeID := "sub_123"
		subscription := &db.BillingSubscription{
			Subject:              "user123",
			PlanKey:              "basic",
			Status:               db.SubscriptionActive,
			StripeSubscriptionID: &stripeID,
		}
		err = txRepo.CreateSubscription(subscription)
		if err != nil {
			return err
		}

		// Create a purchase
		purchase := &db.BillingPurchase{
			Subject:               "user123",
			PlanKey:               "basic",
			Status:                db.PurchaseStatusPaid,
			AmountCents:           1000,
			Currency:              "eur",
			StripePaymentIntentID: "pi_123",
			PaidAt:                now,
		}
		return txRepo.CreatePurchase(purchase)
	})

	require.NoError(t, err)

	// Verify all objects were saved
	plan, err := repo.GetPlan("basic")
	require.NoError(t, err)
	assert.Equal(t, "basic", plan.Key)

	customer, err := repo.GetCustomer("user123")
	require.NoError(t, err)
	assert.Equal(t, "user123", customer.Subject)

	stripeID := "sub_123"
	subscription, err := repo.GetSubscriptionByStripeID(stripeID)
	require.NoError(t, err)
	assert.Equal(t, "user123", subscription.Subject)

	purchase, err := repo.GetPurchaseByPaymentIntent("pi_123")
	require.NoError(t, err)
	assert.Equal(t, int64(1000), purchase.AmountCents)
}

func TestBillingRepositoryDB_Transaction_RollbackMultiple(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	err := repo.Transaction(func(txRepo BillingRepository) error {
		// Create a plan
		plan := &BillingPlanWithFeatures{
			BillingPlan: db.BillingPlan{
				Key:         "test",
				Name:        "Test Plan",
				Kind:        db.PlanKindSubscription,
				AmountCents: 1000,
				Currency:    "eur",
			},
			Features: []string{},
		}
		err := txRepo.SavePlan(plan)
		if err != nil {
			return err
		}

		// Create a customer
		customer := &db.BillingCustomer{
			Subject:          "user123",
			StripeCustomerID: "cus_123",
		}
		err = txRepo.SaveCustomer(customer)
		if err != nil {
			return err
		}

		// Return error to rollback
		return errors.New("rollback test")
	})

	require.Error(t, err)

	// Verify neither the plan nor customer were saved
	_, err = repo.GetPlan("test")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)

	_, err = repo.GetCustomer("user123")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}
