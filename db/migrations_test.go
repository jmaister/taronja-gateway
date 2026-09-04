package db

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/middleware/fingerprint"
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

// addLegacyFingerprintColumns adds the three pre-a559a93 fingerprint
// columns to an already-current-schema table via raw ALTER TABLE, then
// seeds one row's values for them via raw UPDATE — simulating "this row's
// data has been sitting in these columns since before the consolidation,
// and the columns themselves have survived every AutoMigrate since (which
// only ever adds columns, never drops one a struct stopped declaring)"
// without needing to actually replay that whole history through
// AutoMigrate itself twice, which is its own source of migrator quirks
// unrelated to what this test is checking.
func addLegacyFingerprintColumns(t *testing.T, table string) {
	t.Helper()
	for _, col := range []string{"ja4_fingerprint", "ja4_tls_fingerprint", "stable_fingerprint"} {
		require.NoError(t, GetConnection().Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s TEXT", table, col)).Error)
	}
}

func seedLegacyFingerprintColumns(t *testing.T, table, pkColumn, pkValue, ja4, ja4tls, stable string) {
	t.Helper()
	require.NoError(t, GetConnection().Exec(
		fmt.Sprintf("UPDATE %s SET ja4_fingerprint = ?, ja4_tls_fingerprint = ?, stable_fingerprint = ? WHERE %s = ?", table, pkColumn),
		ja4, ja4tls, stable, pkValue,
	).Error)
}

// TestMigrateLegacyFingerprintColumns_BackfillsByPriority covers
// migrateLegacyFingerprintColumns against a "sessions" table with the old
// fingerprint columns added back on top of the current schema, with rows
// exercising every priority case — ja4_tls_fingerprint winning over the
// other two when several are present, stable_fingerprint next, and
// ja4_fingerprint (JA4H) only when it's the sole one present — plus a row
// that already has Fingerprint set (should be left alone, not overwritten
// from a stale old column, even a decoy one).
func TestMigrateLegacyFingerprintColumns_BackfillsByPriority(t *testing.T) {
	SetupTestDB(t.Name())
	gdb := GetConnection()
	addLegacyFingerprintColumns(t, "sessions")

	repo := NewSessionRepositoryDB(gdb)
	for _, token := range []string{"all-three-present", "stable-and-ja4h", "ja4h-only", "none-present"} {
		require.NoError(t, repo.CreateSession(token, &Session{UserID: "test-user", ValidUntil: time.Now().Add(time.Hour)}))
	}

	seedLegacyFingerprintColumns(t, "sessions", "token", "all-three-present", "ja4h-value", "ja4tls-value", "stable-value")
	seedLegacyFingerprintColumns(t, "sessions", "token", "stable-and-ja4h", "ja4h-value-2", "", "stable-value-2")
	seedLegacyFingerprintColumns(t, "sessions", "token", "ja4h-only", "ja4h-value-3", "", "")
	seedLegacyFingerprintColumns(t, "sessions", "token", "none-present", "should-not-win", "", "")
	require.NoError(t, gdb.Exec(
		`UPDATE sessions SET fingerprint = ?, fingerprint_type = ? WHERE token = ?`,
		"already-set-value", "ja4h", "none-present",
	).Error)

	require.NoError(t, migrateLegacyFingerprintColumns(gdb))

	type result struct {
		Fingerprint     string
		FingerprintType string
	}
	get := func(token string) result {
		var r result
		require.NoError(t, gdb.Raw("SELECT fingerprint, fingerprint_type FROM sessions WHERE token = ?", token).Row().Scan(&r.Fingerprint, &r.FingerprintType))
		return r
	}

	assert.Equal(t, result{"ja4tls-value", fingerprint.TypeJA4TLS}, get("all-three-present"), "ja4_tls_fingerprint should win when all three are present")
	assert.Equal(t, result{"stable-value-2", fingerprint.TypeStable}, get("stable-and-ja4h"), "stable_fingerprint should win over ja4_fingerprint")
	assert.Equal(t, result{"ja4h-value-3", fingerprint.TypeJA4H}, get("ja4h-only"), "ja4_fingerprint (JA4H) used only when it's the sole source")
	assert.Equal(t, result{"already-set-value", "ja4h"}, get("none-present"), "a row that already has Fingerprint set must not be overwritten from an old column, even a decoy one (\"should-not-win\")")
}

// TestMigrateLegacyFingerprintColumns_NoOpWhenColumnsNeverExisted covers a
// table created entirely after the a559a93 consolidation — the normal case
// for db/db.go's real Init/SetupTestDB tables, which never declare the old
// columns at all. migrateLegacyFingerprintColumns must not error trying to
// reference a column that was never there.
func TestMigrateLegacyFingerprintColumns_NoOpWhenColumnsNeverExisted(t *testing.T) {
	SetupTestDB(t.Name())
	require.NoError(t, migrateLegacyFingerprintColumns(GetConnection()))
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
