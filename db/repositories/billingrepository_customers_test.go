package repositories

import (
	"testing"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBillingRepositoryDB_SaveCustomer_Create(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	customer := &db.BillingCustomer{
		Subject:          "user123",
		StripeCustomerID: "cus_123",
		Name:             "John Doe",
		Email:            "john@example.com",
	}

	err := repo.SaveCustomer(customer)
	require.NoError(t, err)

	retrieved, err := repo.GetCustomer("user123")
	require.NoError(t, err)
	assert.Equal(t, "user123", retrieved.Subject)
	assert.Equal(t, "cus_123", retrieved.StripeCustomerID)
}

func TestBillingRepositoryDB_SaveCustomer_Update(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	customer := &db.BillingCustomer{
		Subject:          "user123",
		StripeCustomerID: "cus_123",
		Name:             "John Doe",
	}
	require.NoError(t, repo.SaveCustomer(customer))

	// Update the customer
	customer.Email = "john.updated@example.com"
	err := repo.SaveCustomer(customer)
	require.NoError(t, err)

	retrieved, err := repo.GetCustomer("user123")
	require.NoError(t, err)
	assert.Equal(t, "john.updated@example.com", retrieved.Email)
}

func TestBillingRepositoryDB_GetCustomer(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	customer := &db.BillingCustomer{
		Subject:          "org456",
		StripeCustomerID: "cus_456",
		Name:             "Acme Corp",
		Email:            "billing@acme.com",
	}
	require.NoError(t, repo.SaveCustomer(customer))

	retrieved, err := repo.GetCustomer("org456")
	require.NoError(t, err)
	assert.Equal(t, "org456", retrieved.Subject)
	assert.Equal(t, "Acme Corp", retrieved.Name)
}

func TestBillingRepositoryDB_GetCustomer_NotFound(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	_, err := repo.GetCustomer("nonexistent")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestBillingRepositoryDB_GetCustomerByStripeID(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	customer := &db.BillingCustomer{
		Subject:          "user789",
		StripeCustomerID: "cus_789",
		Name:             "Jane Doe",
	}
	require.NoError(t, repo.SaveCustomer(customer))

	retrieved, err := repo.GetCustomerByStripeID("cus_789")
	require.NoError(t, err)
	assert.Equal(t, "user789", retrieved.Subject)
}

func TestBillingRepositoryDB_GetCustomerByStripeID_NotFound(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	_, err := repo.GetCustomerByStripeID("cus_nonexistent")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}
