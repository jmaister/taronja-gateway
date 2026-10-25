package repositories

import (
	"testing"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBillingRepositoryDB_RecordStripeEvent_New(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	isNew, err := repo.RecordStripeEvent("evt_123", "invoice.paid")
	require.NoError(t, err)
	assert.True(t, isNew)
}

func TestBillingRepositoryDB_RecordStripeEvent_Duplicate(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	// Record once
	isNew, err := repo.RecordStripeEvent("evt_123", "invoice.paid")
	require.NoError(t, err)
	assert.True(t, isNew)

	// Record again - should return false (not new)
	isNew, err = repo.RecordStripeEvent("evt_123", "invoice.paid")
	require.NoError(t, err)
	assert.False(t, isNew)
}

func TestBillingRepositoryDB_RecordStripeEvent_Multiple(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	// Record different events
	isNew1, err := repo.RecordStripeEvent("evt_1", "invoice.paid")
	require.NoError(t, err)
	assert.True(t, isNew1)

	isNew2, err := repo.RecordStripeEvent("evt_2", "invoice.created")
	require.NoError(t, err)
	assert.True(t, isNew2)

	// Record first event again
	isNew3, err := repo.RecordStripeEvent("evt_1", "invoice.paid")
	require.NoError(t, err)
	assert.False(t, isNew3)
}
