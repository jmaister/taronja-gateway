package db

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// migrationsNonUTC is a fixed +01:00 zone standing in for "whatever the
// host's local zone happened to be" before db.utcNowFunc and the
// BeforeSave hooks existed — deliberately not UTC. Named "CEST" (a real
// zone abbreviation) rather than something synthetic: this test writes the
// raw text directly (bypassing the normal write path, to simulate a
// pre-upgrade row), and the driver's own decode of a stored zone
// abbreviation only recognizes plain alphabetic text, same as every real
// zone name — a synthetic name like "TEST+0100" would fail to round-trip
// for reasons that have nothing to do with what this test is checking.
var migrationsNonUTC = time.FixedZone("CEST", 60*60)

// seedLegacyRow creates a real row through the normal (already
// UTC-normalizing) path, then overwrites one column with raw legacy-format
// text via direct SQL — simulating a row written before db.utcNowFunc and
// the BeforeSave hooks existed, without needing to satisfy every other
// NOT NULL constraint by hand.
func seedLegacyTimestampColumn(t *testing.T, table, column, pkColumn, pkValue string, legacyValue time.Time) {
	t.Helper()
	require.NoError(t, GetConnection().Exec(
		"UPDATE "+table+" SET "+column+" = ? WHERE "+pkColumn+" = ?", legacyValue, pkValue,
	).Error)
}

// readTimestampColumn re-reads a single timestamp value straight out of
// SQLite via a fresh query.
func readTimestampColumn(t *testing.T, table, column, pkColumn, pkValue string) time.Time {
	t.Helper()
	var stored time.Time
	require.NoError(t, GetConnection().Raw(
		"SELECT "+column+" FROM "+table+" WHERE "+pkColumn+" = ?", pkValue,
	).Row().Scan(&stored))
	return stored
}

// TestMigrateTimestampsToUTC_BackfillsLegacyRows is the core regression
// test for the scenario db.migrateTimestampsToUTC exists to fix: a
// database with rows already written in a non-UTC zone (from before
// db.utcNowFunc and the BeforeSave hooks existed) should end up with those
// rows normalized to UTC once this migration runs — covering one column
// from each of the five tables utcTimestampColumns lists.
func TestMigrateTimestampsToUTC_BackfillsLegacyRows(t *testing.T) {
	SetupTestDB(t.Name())

	user := &User{Username: "legacy-user", Email: "legacy@example.com"}
	require.NoError(t, GetConnection().Create(user).Error)
	legacyResetExpires := time.Date(2026, 6, 1, 8, 0, 0, 0, migrationsNonUTC)
	seedLegacyTimestampColumn(t, "users", "password_reset_expires", "id", user.ID, legacyResetExpires)

	session := &Session{Token: "legacy-session", UserID: user.ID, ValidUntil: time.Now().Add(time.Hour)}
	require.NoError(t, NewSessionRepositoryDB(GetConnection()).CreateSession("legacy-session", session))
	legacyValidUntil := time.Date(2026, 6, 2, 9, 30, 0, 0, migrationsNonUTC)
	seedLegacyTimestampColumn(t, "sessions", "valid_until", "token", "legacy-session", legacyValidUntil)

	metric := &TrafficMetric{HttpMethod: "GET", Path: "/legacy", HttpStatus: 200}
	require.NoError(t, GetConnection().Create(metric).Error)
	metricID := strconv.FormatUint(uint64(metric.ID), 10)
	legacyTimestamp := time.Date(2026, 6, 3, 10, 0, 0, 0, migrationsNonUTC)
	seedLegacyTimestampColumn(t, "traffic_metrics", "timestamp", "id", metricID, legacyTimestamp)

	token := &Token{UserID: user.ID, TokenHash: "legacy-hash", Name: "legacy-token"}
	require.NoError(t, NewTokenRepositoryDB(GetConnection()).CreateToken(token))
	legacyExpiresAt := time.Date(2026, 6, 4, 11, 0, 0, 0, migrationsNonUTC)
	seedLegacyTimestampColumn(t, "tokens", "expires_at", "id", token.ID, legacyExpiresAt)

	// Sanity check: before migrating, these really are stored non-UTC (the
	// point of this test would be moot if seeding hadn't worked).
	before := readTimestampColumn(t, "users", "password_reset_expires", "id", user.ID)
	_, beforeOffset := before.Zone()
	require.NotEqual(t, 0, beforeOffset, "seeding should have produced a non-UTC stored value")

	require.NoError(t, migrateTimestampsToUTC(GetConnection()))

	for _, tc := range []struct {
		table, column, pkColumn, pkValue string
		want                             time.Time
	}{
		{"users", "password_reset_expires", "id", user.ID, legacyResetExpires},
		{"sessions", "valid_until", "token", "legacy-session", legacyValidUntil},
		{"traffic_metrics", "timestamp", "id", metricID, legacyTimestamp},
		{"tokens", "expires_at", "id", token.ID, legacyExpiresAt},
	} {
		got := readTimestampColumn(t, tc.table, tc.column, tc.pkColumn, tc.pkValue)
		_, offset := got.Zone()
		assert.Equal(t, 0, offset, "%s.%s should be UTC after migration, got %v", tc.table, tc.column, got)
		assert.True(t, tc.want.Equal(got), "%s.%s should still represent the same instant, want %v got %v", tc.table, tc.column, tc.want, got)
	}
}

// TestApplyDBMigrations_IsIdempotent covers the PRAGMA user_version
// gating: running applyDBMigrations twice should only do the work once —
// simulated here by seeding a legacy row, migrating, manually re-seeding
// the same legacy value, and confirming a second applyDBMigrations call
// leaves it untouched (proving it short-circuited on user_version, not
// that migrateTimestampsToUTC is itself a no-op the second time around,
// which it also would be, just not what this test is isolating).
func TestApplyDBMigrations_IsIdempotent(t *testing.T) {
	SetupTestDB(t.Name())

	user := &User{Username: "idempotent-user", Email: "idempotent@example.com"}
	require.NoError(t, GetConnection().Create(user).Error)
	legacy := time.Date(2026, 6, 1, 8, 0, 0, 0, migrationsNonUTC)
	seedLegacyTimestampColumn(t, "users", "password_reset_expires", "id", user.ID, legacy)

	require.NoError(t, applyDBMigrations(GetConnection()))
	migrated := readTimestampColumn(t, "users", "password_reset_expires", "id", user.ID)
	_, offset := migrated.Zone()
	require.Equal(t, 0, offset)

	// Re-seed the same legacy (non-UTC) value directly, bypassing the
	// migration, then run applyDBMigrations again. If user_version gating
	// works, this value is left alone (still non-UTC) — the migration
	// never re-runs, since the database's user_version already records it
	// as applied.
	seedLegacyTimestampColumn(t, "users", "password_reset_expires", "id", user.ID, legacy)
	require.NoError(t, applyDBMigrations(GetConnection()))
	untouched := readTimestampColumn(t, "users", "password_reset_expires", "id", user.ID)
	_, untouchedOffset := untouched.Zone()
	assert.NotEqual(t, 0, untouchedOffset, "second applyDBMigrations call should have skipped the already-applied migration, not re-normalized it")
}
