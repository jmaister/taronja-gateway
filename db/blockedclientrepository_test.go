package db_test

import (
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlockedClientRepository_CreateAndList(t *testing.T) {
	db.SetupTestDB(t.Name())
	repo := db.NewBlockedClientRepositoryDB(db.GetConnection())

	now := time.Now()
	seed := []db.BlockedClient{
		{
			Reason:       db.BlockReasonRateLimit,
			TriggerCount: 105,
			BlockedAt:    now.Add(-2 * time.Hour),
			BlockedUntil: now.Add(-1 * time.Hour),
			ClientInfo:   db.ClientInfo{IPAddress: "203.0.113.1", UserAgent: "curl/8.0"},
		},
		{
			Reason:       db.BlockReasonVulnerabilityScan,
			Path:         "/admin.php",
			TriggerCount: 4,
			BlockedAt:    now.Add(-time.Hour),
			BlockedUntil: now,
			ClientInfo:   db.ClientInfo{IPAddress: "203.0.113.2", UserAgent: "scanner/1.0"},
		},
		{
			Reason:       db.BlockReasonMaxErrors,
			TriggerCount: 21,
			BlockedAt:    now,
			BlockedUntil: now.Add(time.Hour),
			ClientInfo:   db.ClientInfo{IPAddress: "203.0.113.1", UserAgent: "curl/8.0"},
		},
	}
	for i := range seed {
		require.NoError(t, repo.Create(&seed[i]))
	}

	t.Run("lists everything, most recent first, with total count", func(t *testing.T) {
		items, total, err := repo.List("", 10, 0)
		require.NoError(t, err)
		assert.EqualValues(t, 3, total)
		require.Len(t, items, 3)
		assert.Equal(t, db.BlockReasonMaxErrors, items[0].Reason, "most recently blocked_at should come first")
		assert.Equal(t, db.BlockReasonVulnerabilityScan, items[1].Reason)
		assert.Equal(t, db.BlockReasonRateLimit, items[2].Reason)
	})

	t.Run("filters by IP", func(t *testing.T) {
		items, total, err := repo.List("203.0.113.1", 10, 0)
		require.NoError(t, err)
		assert.EqualValues(t, 2, total)
		require.Len(t, items, 2)
		for _, item := range items {
			assert.Equal(t, "203.0.113.1", item.IPAddress)
		}
	})

	t.Run("respects limit and offset", func(t *testing.T) {
		items, total, err := repo.List("", 1, 1)
		require.NoError(t, err)
		assert.EqualValues(t, 3, total, "total count ignores limit/offset")
		require.Len(t, items, 1)
		assert.Equal(t, db.BlockReasonVulnerabilityScan, items[0].Reason, "second most recent, since offset=1 skips the first")
	})

	t.Run("empty result for an IP with no blocks", func(t *testing.T) {
		items, total, err := repo.List("203.0.113.99", 10, 0)
		require.NoError(t, err)
		assert.EqualValues(t, 0, total)
		assert.Empty(t, items)
	})
}

func TestBlockedClient_BeforeCreate_NormalizesToUTC(t *testing.T) {
	db.SetupTestDB(t.Name())
	repo := db.NewBlockedClientRepositoryDB(db.GetConnection())

	nonUTC := time.FixedZone("CEST", 2*60*60)
	bc := &db.BlockedClient{
		Reason:       db.BlockReasonRateLimit,
		TriggerCount: 1,
		BlockedAt:    time.Date(2026, 6, 1, 10, 0, 0, 0, nonUTC),
		BlockedUntil: time.Date(2026, 6, 1, 11, 0, 0, 0, nonUTC),
		ClientInfo:   db.ClientInfo{IPAddress: "203.0.113.5"},
	}
	require.NoError(t, repo.Create(bc))

	var stored time.Time
	require.NoError(t, db.GetConnection().Raw("SELECT blocked_at FROM blocked_clients WHERE id = ?", bc.ID).Row().Scan(&stored))
	_, offset := stored.Zone()
	assert.Equal(t, 0, offset, "blocked_at should be stored as UTC")
}
