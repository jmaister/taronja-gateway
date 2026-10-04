package db

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	"github.com/jmaister/taronja-gateway/middleware/fingerprint"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// One-time bridge for databases that predate golang-migrate: AutoMigrate plus
// three data repairs that plain SQL files cannot express (they need live schema
// introspection). Future changes are numbered SQL files in db/migrations/.

// autoMigrateModels lists every model applyLegacyDataMigrations passes to
// AutoMigrate — a single source of truth so a test that needs to
// AutoMigrate a database outside of it (db/migration_v0_fixture_test.go,
// exercising this same bridge function directly against a real historical
// fixture) uses the exact same model list rather than a second hand-copied
// one that could silently drift from the real one.
var autoMigrateModels = []interface{}{
	&User{},
	&Session{},
	&TrafficMetric{},
	&Token{},
	&Counter{},
	&BlockedClient{},
	&Notification{},
	&NotificationDelivery{},
	&NotificationChannelLink{},
	&NotificationLinkCode{},
	&NotificationPreference{},
}

// applyLegacyDataMigrations runs AutoMigrate (after 0001_initial_schema) and then
// the data repairs, once per database. It is a safe no-op on a fresh database:
// each repair guards itself.
func applyLegacyDataMigrations(sqlDB *sql.DB) error {
	gdb, err := gorm.Open(sqlite.Dialector{DriverName: "sqlite", Conn: sqlDB}, &gorm.Config{
		Logger:  logger.Default.LogMode(logger.Silent),
		NowFunc: utcNowFunc,
	})
	if err != nil {
		return fmt.Errorf("wrapping connection for legacy data migrations: %w", err)
	}

	log.Printf("db: applying legacy migration: AutoMigrate (adds any columns/tables an old database is still missing)")
	if err := gdb.AutoMigrate(autoMigrateModels...); err != nil {
		return fmt.Errorf("legacy migration (AutoMigrate): %w", err)
	}
	log.Printf("db: applying legacy migration: normalize existing timestamps to UTC")
	if err := migrateTimestampsToUTC(gdb); err != nil {
		return fmt.Errorf("legacy migration (UTC timestamps): %w", err)
	}
	log.Printf("db: applying legacy migration: backfill Fingerprint/FingerprintType from the old three-column scheme")
	if err := migrateLegacyFingerprintColumns(gdb); err != nil {
		return fmt.Errorf("legacy migration (fingerprint backfill): %w", err)
	}
	log.Printf("db: applying legacy migration: backfill sessions.token_hash from the old plaintext token column, and drop it")
	if err := migrateSessionTokensToHashed(gdb); err != nil {
		return fmt.Errorf("legacy migration (session token hashing): %w", err)
	}
	return nil
}

// utcTimestampColumn names one timestamp column migrateTimestampsToUTC
// should normalize, and the column identifying which row to write back to.
type utcTimestampColumn struct {
	table  string
	column string
	pk     string
}

// utcTimestampColumns lists the timestamp columns that existed when this
// migration was written. New columns need no entry: db.utcNowFunc and the
// BeforeSave hooks already write UTC.
var utcTimestampColumns = []utcTimestampColumn{
	{"traffic_metrics", "timestamp", "id"},
	{"traffic_metrics", "created_at", "id"},
	{"traffic_metrics", "updated_at", "id"},
	{"traffic_metrics", "deleted_at", "id"},

	// "id", not "token": Session's primary key used to be its (plaintext)
	// token column, but security/session-token-hashing made that column
	// gorm:"-" (never persisted) and moved the real primary key to
	// token_hash — see db.Session.TokenHash's doc comment. gorm.Model's
	// own "id" column was always present and stable regardless, so it's
	// the identifier this backfill's per-row UPDATE targets now.
	{"sessions", "valid_until", "id"},
	{"sessions", "last_activity", "id"},
	{"sessions", "closed_on", "id"},
	{"sessions", "created_at", "id"},
	{"sessions", "updated_at", "id"},
	{"sessions", "deleted_at", "id"},

	{"users", "password_reset_expires", "id"},
	{"users", "email_confirmation_expires", "id"},
	{"users", "created_at", "id"},
	{"users", "updated_at", "id"},
	{"users", "deleted_at", "id"},

	{"tokens", "expires_at", "id"},
	{"tokens", "last_used_at", "id"},
	{"tokens", "revoked_at", "id"},
	{"tokens", "created_at", "id"},
	{"tokens", "updated_at", "id"},

	{"counters", "created_at", "id"},
	{"counters", "updated_at", "id"},
	{"counters", "deleted_at", "id"},
}

// migrateTimestampsToUTC rewrites non-UTC values in utcTimestampColumns to UTC, once.
// BETWEEN queries compare raw TEXT, so mixed offsets would compare wrongly.
//
// Values are read as strings: the sqlite driver normalizes them to RFC3339 text
// first, which parseStoredTimestamp handles (plus the raw form). An unparseable
// row is logged and skipped rather than aborting the migration.
func migrateTimestampsToUTC(gdb *gorm.DB) error {
	for _, col := range utcTimestampColumns {
		err := backfillColumnToUTC(gdb, col)
		if err != nil {
			return err
		}
	}
	return nil
}

// backfillColumnToUTC converts one column to UTC, writing back a time.Time so
// it is serialized like any other UTC write.
func backfillColumnToUTC(gdb *gorm.DB, col utcTimestampColumn) error {
	type scannedRow struct {
		PK    string
		Value sql.NullString
	}
	var rows []scannedRow
	selectSQL := fmt.Sprintf("SELECT %s AS pk, %s AS value FROM %s WHERE %s IS NOT NULL", col.pk, col.column, col.table, col.column)
	err := gdb.Raw(selectSQL).Scan(&rows).Error
	if err != nil {
		return fmt.Errorf("reading %s.%s for UTC backfill: %w", col.table, col.column, err)
	}

	updateSQL := fmt.Sprintf("UPDATE %s SET %s = ? WHERE %s = ?", col.table, col.column, col.pk)
	for _, row := range rows {
		if !row.Value.Valid {
			continue
		}
		parsed, err := parseStoredTimestamp(row.Value.String)
		if err != nil {
			log.Printf("db: skipping unparseable %s.%s value %q for %s=%s during UTC backfill: %v", col.table, col.column, row.Value.String, col.pk, row.PK, err)
			continue
		}
		if _, offset := parsed.Zone(); offset == 0 {
			continue // already UTC, nothing to rewrite
		}
		err = gdb.Exec(updateSQL, parsed.UTC(), row.PK).Error
		if err != nil {
			return fmt.Errorf("rewriting %s.%s to UTC for %s=%s: %w", col.table, col.column, col.pk, row.PK, err)
		}
	}
	return nil
}

// legacyFingerprintSource maps a pre-consolidation fingerprint column to its
// FingerprintType. Order matches fingerprint.SelectFingerprint's priority.
type legacyFingerprintSource struct {
	column string
	typ    string
}

var legacyFingerprintSources = []legacyFingerprintSource{
	{"ja4_tls_fingerprint", "ja4_tls"},
	{"stable_fingerprint", fingerprint.TypeStable},
	{"ja4_fingerprint", fingerprint.TypeJA4H},
}

// legacyFingerprintTables lists every table ClientInfo has ever been
// embedded in that could plausibly carry data in the old columns: Session
// and TrafficMetric since JA4Fingerprint was first introduced, and Token
// too — it started embedding ClientInfo (see auth.TokenService.
// GenerateToken's clientInfo parameter) before the a559a93 consolidation.
var legacyFingerprintTables = []string{"sessions", "traffic_metrics", "tokens"}

// migrateLegacyFingerprintColumns copies the old ja4_fingerprint,
// ja4_tls_fingerprint and stable_fingerprint columns into
// Fingerprint/FingerprintType for rows where Fingerprint is empty (AutoMigrate
// adds columns but never moves values). One SQL UPDATE per table, guarded with
// HasColumn because newer tables never had the old columns.
func migrateLegacyFingerprintColumns(gdb *gorm.DB) error {
	for _, table := range legacyFingerprintTables {
		err := backfillLegacyFingerprintColumn(gdb, table)
		if err != nil {
			return err
		}
	}
	return nil
}

// backfillLegacyFingerprintColumn handles one table for
// migrateLegacyFingerprintColumns — see that function's comment.
func backfillLegacyFingerprintColumn(gdb *gorm.DB, table string) error {
	var present []legacyFingerprintSource
	for _, src := range legacyFingerprintSources {
		if gdb.Migrator().HasColumn(table, src.column) {
			present = append(present, src)
		}
	}
	if len(present) == 0 {
		return nil // this table never had the old columns — nothing to backfill
	}

	var fingerprintCases, typeCases, anyPresentConds strings.Builder
	for _, src := range present {
		cond := fmt.Sprintf("%s IS NOT NULL AND %s != ''", src.column, src.column)
		fmt.Fprintf(&fingerprintCases, "WHEN %s THEN %s ", cond, src.column)
		fmt.Fprintf(&typeCases, "WHEN %s THEN '%s' ", cond, src.typ)
		if anyPresentConds.Len() > 0 {
			anyPresentConds.WriteString(" OR ")
		}
		fmt.Fprintf(&anyPresentConds, "(%s)", cond)
	}

	updateSQL := fmt.Sprintf(
		`UPDATE %s SET
			fingerprint = CASE %s ELSE fingerprint END,
			fingerprint_type = CASE %s ELSE fingerprint_type END
		WHERE (fingerprint IS NULL OR fingerprint = '') AND (%s)`,
		table, fingerprintCases.String(), typeCases.String(), anyPresentConds.String(),
	)
	err := gdb.Exec(updateSQL).Error
	if err != nil {
		return fmt.Errorf("backfilling fingerprint/fingerprint_type on %s from legacy columns: %w", table, err)
	}
	return nil
}

// migrateSessionTokensToHashed backfills token_hash from the raw token column so
// existing sessions stay valid. It is a no-op (guarded with HasColumn) on
// databases that never had that column.
func migrateSessionTokensToHashed(gdb *gorm.DB) error {
	if !gdb.Migrator().HasColumn(&Session{}, "token") {
		return nil // fresh schema, no legacy plaintext column to backfill from
	}

	type scannedRow struct {
		ID    uint
		Token sql.NullString
	}
	var rows []scannedRow
	// Selected via raw SQL, not the model: db.Session no longer declares a
	// Go field mapped to the "token" column (it's gorm:"-" now), so GORM
	// itself has no way to read it.
	err := gdb.Raw(`SELECT id, token FROM sessions WHERE token IS NOT NULL AND token != '' AND (token_hash IS NULL OR token_hash = '')`).Scan(&rows).Error
	if err != nil {
		return fmt.Errorf("reading sessions.token for token_hash backfill: %w", err)
	}

	for _, row := range rows {
		if !row.Token.Valid || row.Token.String == "" {
			continue
		}
		err = gdb.Exec(`UPDATE sessions SET token_hash = ? WHERE id = ?`, hashSessionToken(row.Token.String), row.ID).Error
		if err != nil {
			return fmt.Errorf("backfilling sessions.token_hash for id=%d: %w", row.ID, err)
		}
	}

	// The physical NOT NULL "token" column must be dropped: AutoMigrate never drops
	// columns, and Session.Token is no longer written, so new sessions would fail
	// the constraint. Raw SQL, because Migrator().DropColumn silently no-ops once
	// the model has no matching column. SQLite supports DROP COLUMN since 3.35.
	if err := gdb.Exec(`ALTER TABLE sessions DROP COLUMN token`).Error; err != nil {
		return fmt.Errorf("dropping legacy sessions.token column: %w", err)
	}
	return nil
}
