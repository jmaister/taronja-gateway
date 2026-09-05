package db

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/middleware/fingerprint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestMigrateRealV0024Database opens a real, historical SQLite database —
// db/testdata/v0.0.24.db, produced by actually running the v0.0.24 tagged
// release (the last tag before this project's v1 schema work — the UTC
// timestamp normalization and the JA4H/TLS-JA4/stable fingerprint
// consolidation in db/migrations.go were both written after it) and
// generating real traffic against it — see db/testdata/README.md for
// exactly how and why a real fixture, not just a synthetic reproduction of
// the old schema, is worth keeping. This runs the exact same sequence
// db.Init runs on every startup (AutoMigrate, then applyDBMigrations)
// against that file and checks the result, end to end, rather than just
// exercising one migration function in isolation the way
// migrations_test.go's other tests do.
func TestMigrateRealV0024Database(t *testing.T) {
	fixture, err := os.ReadFile("testdata/v0.0.24.db")
	require.NoError(t, err, "db/testdata/v0.0.24.db must exist — see db/testdata/README.md")

	// Work on a throwaway copy in a temp dir — AutoMigrate/applyDBMigrations
	// write to this file, and the checked-in fixture must never be mutated
	// by running the test suite.
	dbPath := filepath.Join(t.TempDir(), "v0.0.24.db")
	require.NoError(t, os.WriteFile(dbPath, fixture, 0o644))

	gdb, err := gorm.Open(sqlite.Dialector{
		DriverName: "sqlite",
		DSN:        dbPath,
	}, &gorm.Config{
		Logger:  logger.Default.LogMode(logger.Silent),
		NowFunc: utcNowFunc,
	})
	require.NoError(t, err)

	// The core regression check: AutoMigrate must not error against a real
	// pre-v1 database. In particular, TrafficMetric.IsStaticAsset (`not
	// null`, added after v0.0.24, on a table this fixture already has rows
	// in) needs its `default:false` tag, or SQLite refuses the ALTER TABLE
	// outright — see TestAutoMigrate_AddingNotNullColumnToExistingRows_
	// NeedsADefault for the synthetic version of this same regression, now
	// exercised here against genuine historical data instead.
	require.NoError(t, gdb.AutoMigrate(autoMigrateModels...))
	require.NoError(t, applyDBMigrations(gdb))

	// Existing data survived, untouched in substance.
	var userCount, sessionCount, metricCount int64
	require.NoError(t, gdb.Model(&User{}).Count(&userCount).Error)
	require.NoError(t, gdb.Model(&Session{}).Count(&sessionCount).Error)
	require.NoError(t, gdb.Model(&TrafficMetric{}).Count(&metricCount).Error)
	assert.EqualValues(t, 1, userCount)
	assert.EqualValues(t, 1, sessionCount)
	assert.EqualValues(t, 11, metricCount)

	var user User
	require.NoError(t, gdb.First(&user).Error)
	assert.Equal(t, "admin", user.Username)
	assert.Equal(t, AdminProvider, user.Provider)

	var metrics []TrafficMetric
	require.NoError(t, gdb.Order("id").Find(&metrics).Error)
	require.Len(t, metrics, 11)

	// New-since-v0.0.24 column: every one of these rows predates
	// IsStaticAsset entirely, so it must default to false rather than fail
	// the migration or come back true.
	for _, m := range metrics {
		assert.False(t, m.IsStaticAsset, "row %d predates IsStaticAsset entirely; should default to false", m.ID)
	}

	// UTC normalization: this fixture was generated on a host running
	// Europe/London (BST, +01:00) — genuinely non-UTC — so this is a real
	// exercise of migrateTimestampsToUTC, not a no-op against
	// already-UTC data.
	for _, m := range metrics {
		_, offset := m.Timestamp.Zone()
		assert.Equal(t, 0, offset, "traffic_metrics.timestamp (id=%d) should be normalized to UTC, got %v", m.ID, m.Timestamp)
	}
	var session Session
	require.NoError(t, gdb.First(&session).Error)
	_, sessionOffset := session.ValidUntil.Zone()
	assert.Equal(t, 0, sessionOffset, "sessions.valid_until should be normalized to UTC, got %v", session.ValidUntil)
	// The instant itself must be preserved, not just the zone: the fixture's
	// raw stored value is "2026-09-06 10:25:34.705910518 +0100 BST", which
	// is 2026-09-06 09:25:34.705910518 UTC.
	assert.True(t, session.ValidUntil.Equal(time.Date(2026, 9, 6, 9, 25, 34, 705910518, time.UTC)),
		"got %v", session.ValidUntil)

	// Legacy fingerprint backfill: v0.0.24 only ever had the JA4H column
	// (ja4_tls_fingerprint/stable_fingerprint were added later), so every
	// backfilled row here should land as TypeJA4H specifically.
	//
	// metrics[0] is id=1, the very first "GET /" — no session cookie yet.
	assert.Equal(t, "ge11nn020000_a00508f53a24_000000000000_000000000000", metrics[0].Fingerprint)
	assert.Equal(t, fingerprint.TypeJA4H, metrics[0].FingerprintType)
	// metrics[6] is id=7, "GET /secret" made *with* the session cookie from
	// the login just before it — a genuinely different JA4H value (JA4H
	// encodes cookie presence), confirming the backfill reads per-row
	// rather than reusing a single cached value.
	assert.Equal(t, "ge11cn020000_87deabb5c334_e21f69ebe701_e21f69ebe701", metrics[6].Fingerprint)
	assert.Equal(t, fingerprint.TypeJA4H, metrics[6].FingerprintType)
	assert.Equal(t, "po11nn040000_be74a2312c86_000000000000_000000000000", session.Fingerprint)
	assert.Equal(t, fingerprint.TypeJA4H, session.FingerprintType)

	// blocked_clients didn't exist at all in v0.0.24 (this session's work,
	// long after this fixture's tag) — it must exist post-migration and be
	// queryable, even though nothing backfills rows into it: v0.0.24 never
	// recorded a block anywhere durable, only the in-memory rate limiter's
	// live (and by now long-expired) state.
	assert.True(t, gdb.Migrator().HasTable(&BlockedClient{}))
	var blockedCount int64
	require.NoError(t, gdb.Model(&BlockedClient{}).Count(&blockedCount).Error)
	assert.EqualValues(t, 0, blockedCount)
}
