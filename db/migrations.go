package db

import (
	"database/sql"
	"fmt"
	"log"

	"gorm.io/gorm"
)

// dbMigration is one versioned, one-time data-repair step applied after
// AutoMigrate, tracked via SQLite's built-in PRAGMA user_version (an
// integer the database file itself carries, defaulting to 0 for both a
// brand-new database and any database that predates this mechanism — the
// two are indistinguishable, but that's fine: a migration re-reading an
// empty/absent table is a correct no-op either way).
//
// This exists because AutoMigrate — which runs unconditionally on every
// startup, in Init and SetupTestDB — only ever adds tables, columns, and
// indexes; it never renames a column, changes an existing column's stored
// values, or otherwise transforms data already on disk. Anything beyond
// "the new schema also has this column, defaulted to zero/NULL for
// existing rows" needs an explicit migration like this, run once and
// tracked so it's never repeated. This is this project's answer to "how
// does an existing database behave when opened by a newer version of the
// gateway": schema changes apply themselves for free (AutoMigrate), but a
// changed convention for existing *data* needs a migration entry here —
// modeled on config/'s own versioned migration convention (see main.go's
// "migrate" command and doc/CONFIG.md) applied to the database instead of
// the config file. Add a new entry, at the next version number, when a
// future change needs one; each step can assume every earlier one has
// already run.
type dbMigration struct {
	version     int
	description string
	apply       func(*gorm.DB) error
}

// dbMigrations lists every migration in order. version numbers must be
// consecutive starting at 1 and never reused or reordered once released —
// applyDBMigrations runs every entry whose version is greater than the
// database's current PRAGMA user_version, so a database migrated under an
// older gateway version picks up everything it missed, in order, the next
// time it's opened.
var dbMigrations = []dbMigration{
	{1, "normalize existing timestamps to UTC", migrateTimestampsToUTC},
}

// applyDBMigrations runs every dbMigrations entry newer than the database's
// current PRAGMA user_version, in order, bumping user_version after each
// one succeeds — so a failure partway through (or a process killed mid-
// migration) leaves user_version at the last *fully applied* step, and the
// next startup resumes from there rather than skipping or repeating work.
// Called from Init, after AutoMigrate (a migration may need columns/tables
// AutoMigrate only just added) and before the connection is published via
// conn. Not called from SetupTestDB: a fresh test database never has
// pre-existing data for a migration to act on, so running these against it
// would only be wasted work — tests that need to exercise a migration call
// it directly instead (see migrations_test.go).
func applyDBMigrations(gdb *gorm.DB) error {
	sqlDB, err := gdb.DB()
	if err != nil {
		return fmt.Errorf("getting underlying sql.DB for migrations: %w", err)
	}
	var current int
	if err := sqlDB.QueryRow("PRAGMA user_version").Scan(&current); err != nil {
		return fmt.Errorf("reading PRAGMA user_version: %w", err)
	}
	for _, m := range dbMigrations {
		if m.version <= current {
			continue
		}
		log.Printf("db: applying migration %d: %s", m.version, m.description)
		if err := m.apply(gdb); err != nil {
			return fmt.Errorf("db migration %d (%s): %w", m.version, m.description, err)
		}
		// PRAGMA doesn't support bound parameters in SQLite — safe here
		// regardless, since m.version is a compile-time constant from
		// dbMigrations above, never external input.
		if _, err := sqlDB.Exec(fmt.Sprintf("PRAGMA user_version = %d", m.version)); err != nil {
			return fmt.Errorf("recording db migration %d as applied: %w", m.version, err)
		}
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

// utcTimestampColumns lists every timestamp column across the schema as it
// stood when this migration was written (db/schema.go's User, Session,
// TrafficMetric, Token, and Counter) — see migrateTimestampsToUTC's comment
// for why each of these needed a runtime fix (db.go's NowFunc, the
// BeforeSave hooks, or a call-site fix) that only takes effect on rows
// written after upgrading, leaving pre-upgrade rows on whatever the host's
// local zone was at the time. A future new timestamp field doesn't need
// adding here — db.utcNowFunc and the BeforeSave hooks already make sure
// nothing written from here on needs backfilling in the first place.
var utcTimestampColumns = []utcTimestampColumn{
	{"traffic_metrics", "timestamp", "id"},
	{"traffic_metrics", "created_at", "id"},
	{"traffic_metrics", "updated_at", "id"},
	{"traffic_metrics", "deleted_at", "id"},

	{"sessions", "valid_until", "token"},
	{"sessions", "last_activity", "token"},
	{"sessions", "closed_on", "token"},
	{"sessions", "created_at", "token"},
	{"sessions", "updated_at", "token"},
	{"sessions", "deleted_at", "token"},

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

// migrateTimestampsToUTC rewrites every existing non-UTC value in
// utcTimestampColumns to the equivalent UTC instant, in place.
//
// Before db.utcNowFunc and db/schema.go's BeforeSave hooks (added the same
// time as this migration), every one of these columns was written using
// whatever zone the host's time.Now() returned at the time — the server's
// local zone, for any deployment that wasn't already running in UTC. Those
// fixes only change what gets written *from now on*; a database that
// already has rows from before upgrading keeps them exactly as they were
// unless something rewrites them, which is what this does, once.
//
// This matters beyond tidiness: WHERE timestamp BETWEEN ? AND ? (see
// db/trafficmetricrepository.go and db/timeseries.go) is a raw TEXT
// comparison, not a chronological one, and query bounds are now always
// UTC-normalized. Left un-migrated, a pre-upgrade row's local-offset text
// and a post-upgrade row's UTC text would compare inconsistently again —
// the exact bug class the earlier UTC fixes exist to prevent, just
// reintroduced at the old/new data boundary instead of within a single
// call site.
//
// Reads each column via a plain SELECT into a string rather than scanning
// straight into time.Time: modernc.org/sqlite's driver decodes a
// successfully-recognized DATETIME-affinity value before database/sql ever
// sees the destination type, and — confirmed by hand — normalizes it to
// RFC3339(Nano) text (numeric offset, no zone abbreviation) even when the
// destination is a string, regardless of what zone-abbreviated text was
// originally stored. parseStoredTimestamp's layout list covers that
// normalized form (and, defensively, the raw pre-normalization form, for
// any value that reaches here without having gone through that decode
// step). A row that fails to parse is logged and left as-is rather than
// aborting the whole migration over one bad value — this only ever
// improves data that's already there, so skipping the unparseable rest of
// a table isn't a regression.
func migrateTimestampsToUTC(gdb *gorm.DB) error {
	for _, col := range utcTimestampColumns {
		if err := backfillColumnToUTC(gdb, col); err != nil {
			return err
		}
	}
	return nil
}

// backfillColumnToUTC handles one (table, column) pair for
// migrateTimestampsToUTC: read every non-NULL value, parse it, and — for
// any value not already UTC — write back the equivalent UTC instant. The
// UPDATE binds an actual time.Time (via col.apply below), not a
// hand-formatted string, so the rewritten value is serialized exactly the
// way any other UTC-normalized write in this codebase is — the same driver
// code path, not a hand-rolled imitation of it.
func backfillColumnToUTC(gdb *gorm.DB, col utcTimestampColumn) error {
	type scannedRow struct {
		PK    string
		Value sql.NullString
	}
	var rows []scannedRow
	selectSQL := fmt.Sprintf("SELECT %s AS pk, %s AS value FROM %s WHERE %s IS NOT NULL", col.pk, col.column, col.table, col.column)
	if err := gdb.Raw(selectSQL).Scan(&rows).Error; err != nil {
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
		if err := gdb.Exec(updateSQL, parsed.UTC(), row.PK).Error; err != nil {
			return fmt.Errorf("rewriting %s.%s to UTC for %s=%s: %w", col.table, col.column, col.pk, row.PK, err)
		}
	}
	return nil
}
