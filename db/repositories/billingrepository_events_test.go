package repositories

import (
	"testing"
	"time"

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

func TestBillingRepositoryDB_StuckEvents(t *testing.T) {
	db.SetupTestDB(t.Name())
	repo := NewBillingRepositoryDB(db.GetConnection())
	t0 := time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC)

	require.NoError(t, repo.RecordStuckEvent("evt_2", "invoice.paid", "sub_2", "first", t0.Add(time.Minute)))
	require.NoError(t, repo.RecordStuckEvent("evt_1", "invoice.payment_failed", "sub_1", "first", t0))
	require.NoError(t, repo.RecordStuckEvent("evt_1", "invoice.payment_failed", "sub_1", "second", t0.Add(time.Hour)))

	events, err := repo.StuckEvents()
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, "evt_1", events[0].EventID, "oldest first")
	assert.Equal(t, 2, events[0].Attempts)
	assert.Equal(t, "second", events[0].Reason)
	assert.True(t, events[0].FirstSeenAt.Equal(t0))
	assert.True(t, events[0].LastSeenAt.Equal(t0.Add(time.Hour)))

	require.NoError(t, repo.ClearStuckEvent("evt_1"))
	require.NoError(t, repo.ClearStuckEvent("evt_unknown"))
	events, err = repo.StuckEvents()
	require.NoError(t, err)
	require.Len(t, events, 1)
}
