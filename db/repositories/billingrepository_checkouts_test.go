package repositories

import (
	"testing"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBillingRepositoryDB_CreateCheckout(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	checkout := &db.BillingCheckout{
		Subject:         "user123",
		PlanKey:         "basic",
		UserID:          "user123",
		StripeSessionID: "sess_123",
		Status:          db.CheckoutStatusOpen,
	}

	err := repo.CreateCheckout(checkout)
	require.NoError(t, err)
	assert.NotEmpty(t, checkout.ID)
}

func TestBillingRepositoryDB_GetCheckout(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	checkout := &db.BillingCheckout{
		Subject:         "user123",
		PlanKey:         "basic",
		UserID:          "user123",
		StripeSessionID: "sess_123",
		Status:          db.CheckoutStatusOpen,
	}
	require.NoError(t, repo.CreateCheckout(checkout))

	retrieved, err := repo.GetCheckout(checkout.ID)
	require.NoError(t, err)
	assert.Equal(t, checkout.ID, retrieved.ID)
	assert.Equal(t, "user123", retrieved.Subject)
}

func TestBillingRepositoryDB_GetCheckout_NotFound(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	_, err := repo.GetCheckout("nonexistent")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestBillingRepositoryDB_GetCheckoutByStripeSession(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	checkout := &db.BillingCheckout{
		Subject:         "user123",
		PlanKey:         "basic",
		UserID:          "user123",
		StripeSessionID: "sess_456",
		Status:          db.CheckoutStatusOpen,
	}
	require.NoError(t, repo.CreateCheckout(checkout))

	retrieved, err := repo.GetCheckoutByStripeSession("sess_456")
	require.NoError(t, err)
	assert.Equal(t, checkout.ID, retrieved.ID)
}

func TestBillingRepositoryDB_GetCheckoutByStripeSession_NotFound(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	_, err := repo.GetCheckoutByStripeSession("sess_nonexistent")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestBillingRepositoryDB_UpdateCheckout(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	checkout := &db.BillingCheckout{
		Subject:         "user123",
		PlanKey:         "basic",
		UserID:          "user123",
		StripeSessionID: "sess_123",
		Status:          db.CheckoutStatusOpen,
	}
	require.NoError(t, repo.CreateCheckout(checkout))

	checkout.Status = db.CheckoutStatusCompleted
	err := repo.UpdateCheckout(checkout)
	require.NoError(t, err)

	retrieved, err := repo.GetCheckout(checkout.ID)
	require.NoError(t, err)
	assert.Equal(t, db.CheckoutStatusCompleted, retrieved.Status)
}
