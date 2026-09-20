package db

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	"github.com/jmaister/taronja-gateway/middleware/fingerprint"
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
	{2, "backfill Fingerprint/FingerprintType from the old three-column scheme", migrateLegacyFingerprintColumns},
	{3, "backfill sessions.token_hash from the old plaintext token column", migrateSessionTokensToHashed},
	// Same function as migration 3, run again. A shipped version of that
	// migration backfilled token_hash but never actually dropped the
	// legacy token column afterward — its own surviving NOT NULL
	// constraint (from when it was the primary key) then broke every
	// subsequent CreateSession outright, i.e. every login, on any
	// database that had already recorded migration 3 as applied under
	// that version. Since PRAGMA user_version already reads 3 for those
	// databases, migration 3 itself never runs again for them — this
	// entry exists purely so they still get the column actually dropped,
	// on the very next startup, without needing manual intervention. A
	// database that never hit the bug (migrated fresh under the already-
	// fixed function, or never had a legacy token column at all) finds
	// the column already gone and every row's token_hash already
	// populated, so this is a correct no-op for it — see
	// migrateSessionTokensToHashed's own HasColumn guard.
	{4, "drop the legacy sessions.token column left behind by a buggy version of migration 3", migrateSessionTokensToHashed},
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
	err = sqlDB.QueryRow("PRAGMA user_version").Scan(&current)
	if err != nil {
		return fmt.Errorf("reading PRAGMA user_version: %w", err)
	}
	for _, m := range dbMigrations {
		if m.version <= current {
			continue
		}
		log.Printf("db: applying migration %d: %s", m.version, m.description)
		err = m.apply(gdb)
		if err != nil {
			return fmt.Errorf("db migration %d (%s): %w", m.version, m.description, err)
		}
		// PRAGMA doesn't support bound parameters in SQLite — safe here
		// regardless, since m.version is a compile-time constant from
		// dbMigrations above, never external input.
		_, err = sqlDB.Exec(fmt.Sprintf("PRAGMA user_version = %d", m.version))
		if err != nil {
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
		err := backfillColumnToUTC(gdb, col)
		if err != nil {
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

// legacyFingerprintSource names one of the three fingerprint columns that
// existed before commit a559a93 consolidated them into a single
// Fingerprint + FingerprintType pair, and the FingerprintType value that
// column corresponds to. Order matters: it matches
// fingerprint.SelectFingerprint's own priority (most reliable signal
// wins) — see that function's doc comment — so a row that happened to
// have more than one of the old columns populated backfills to the same
// choice the application would have made had it processed that request
// after the consolidation.
type legacyFingerprintSource struct {
	column string
	typ    string
}

var legacyFingerprintSources = []legacyFingerprintSource{
	{"ja4_tls_fingerprint", fingerprint.TypeJA4TLS},
	{"stable_fingerprint", fingerprint.TypeStable},
	{"ja4_fingerprint", fingerprint.TypeJA4H},
}

// legacyFingerprintTables lists every table ClientInfo has ever been
// embedded in that could plausibly carry data in the old columns: Session
// and TrafficMetric since JA4Fingerprint was first introduced, and Token
// too — it started embedding ClientInfo (see auth.TokenService.
// GenerateToken's clientInfo parameter) before the a559a93 consolidation.
var legacyFingerprintTables = []string{"sessions", "traffic_metrics", "tokens"}

// migrateLegacyFingerprintColumns backfills Fingerprint/FingerprintType
// from whichever of the pre-a559a93 columns (ja4_fingerprint,
// ja4_tls_fingerprint, stable_fingerprint) a table still has, for rows
// where Fingerprint is still empty. AutoMigrate added the new columns for
// every pre-existing row for free, but it only ever adds columns — it
// never carries a value between them — so a row from before the
// consolidation keeps its historical fingerprint sitting in a column the
// application stopped reading entirely: invisible to fingerprint-based
// grouping stats and the new-vs-returning-visitor calculation, even
// though the data is still physically there. Confirmed against a real
// database built and run from before the consolidation ever existed.
//
// One UPDATE per table, not a row-by-row Go loop like
// migrateTimestampsToUTC: this is purely picking among already-stored
// strings by priority, none of the timezone-parsing concerns that
// migration has to deal with, so SQL can express the whole thing itself
// via CASE expressions built from whichever old columns are actually
// present.
//
// Guarded per table and per column with Migrator().HasColumn: a table
// created after a559a93 never had any of the three old columns at all —
// AutoMigrate only ever adds columns a struct currently declares, never
// resurrects one it stopped declaring — and referencing a nonexistent
// column in the generated SQL would error, not silently skip, without
// this check.
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

// migrateSessionTokensToHashed backfills token_hash for every session row
// that still needs it — see db.Session.TokenHash's doc comment for why
// session tokens are looked up by hash now, instead of stored and matched
// against verbatim as they used to be. AutoMigrate only ever adds the new
// token_hash column; it never populates it or drops the pre-existing raw
// token column, so without this, every session created before this upgrade
// would simply become unfindable the moment FindSessionByToken starts
// looking up by token_hash (NULL for every such row) — forcing every
// currently-logged-in user of an upgraded deployment to log in again, an
// avoidable cost for a schema change that has a perfectly deterministic
// backfill (the raw token this codebase already has on hand for each row
// hashes to exactly one value).
//
// Guarded with Migrator().HasColumn, same as backfillLegacyFingerprintColumn
// above: a database that never had the pre-hashing schema at all — every
// one created fresh by this or a later version — never had a `token`
// column to read from in the first place, so this is a correct no-op for
// it rather than a raw-SQL error over a column that was never there.
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

	// Drop the legacy column outright — this is not optional cleanup.
	// AutoMigrate never drops a column, so this table's real, physical
	// "token" column — NOT NULL at the SQLite level, from when it was the
	// primary key (see Session.TokenHash's doc comment) — silently
	// survived Session.Token becoming gorm:"-" (never written by
	// Create/Save). Every session created after that point on an
	// in-place-migrated database therefore omitted "token" from its
	// INSERT entirely, which SQLite rejected outright as a NOT NULL
	// violation: a real regression that made login fail on any database
	// that carried this migration, caught only by an actual login attempt
	// against one, not by TestMigrateRealV0024Database — that test reads
	// the migrated row back, it never writes a new one afterward.
	// Raw SQL, not gdb.Migrator().DropColumn(&Session{}, "token"): that
	// silently no-ops here rather than dropping anything — presumably
	// because GORM's migrator resolves the target column from the
	// model's own parsed schema, and Session.Token no longer maps to any
	// column at all (gorm:"-"), so it has nothing to resolve "token" to.
	// Confirmed directly: DropColumn returns a nil error while
	// PRAGMA table_info(sessions) still lists the column afterward.
	// SQLite has supported ALTER TABLE DROP COLUMN natively since 3.35.0
	// (2021); modernc.org/sqlite, this project's driver, bundles a far
	// newer version.
	if err := gdb.Exec(`ALTER TABLE sessions DROP COLUMN token`).Error; err != nil {
		return fmt.Errorf("dropping legacy sessions.token column: %w", err)
	}
	return nil
}
