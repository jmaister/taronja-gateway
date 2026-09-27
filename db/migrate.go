package db

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log"

	"github.com/golang-migrate/migrate/v4"
	migratesqlite "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// migrationFiles embeds every numbered SQL migration under db/migrations/
// into the compiled binary — same convention this project already uses for
// webapp/dist and static/ (see main.go/static/static.go), so a migration
// ships with the binary and needs no separate file to deploy alongside it.
//
//go:embed migrations/*.sql
var migrationFiles embed.FS

// legacyBridgeDoneVersion is the PRAGMA user_version value runMigrations
// writes once applyLegacyDataMigrations has completed successfully — see
// needsLegacyBridge's doc comment for why this reuses that PRAGMA (the old
// system's own version counter) rather than, say, a fresh marker table:
// deliberately a value no version the old system ever wrote (1-4) could be
// confused with, so there's no ambiguity between "an old database that
// finished under the previous system" and "a database this bridge has
// finished with."
const legacyBridgeDoneVersion = 9999

// runMigrations brings sqlDB's schema up to date via golang-migrate,
// replacing this project's previous approach: GORM's AutoMigrate
// (schema-as-code from Go struct tags, applied unconditionally on every
// startup) plus a hand-written, PRAGMA-user_version-tracked list of Go
// functions for anything AutoMigrate couldn't do (db/legacy_migrations.go,
// what this file used to be called db/migrations.go). AutoMigrate no
// longer runs on every startup — every *future* schema change is a plain,
// reviewable SQL file under db/migrations/, applied and tracked by
// golang-migrate's own schema_migrations table instead of a bespoke
// version counter. It isn't gone completely, though: it still runs, once,
// as part of the legacy bridge below, for a database old enough to need
// it — see applyLegacyDataMigrations.
//
// The bridge runs *before* golang-migrate's own Up() call, deliberately,
// even though that means db/migrations/0001_initial_schema.up.sql never
// actually does real work against any database this code has ever been
// run against (the bridge already brings a never-before-bridged database,
// new or old, fully up to date first; a database that's already bridged
// needs neither step to do anything) — it exists as an explicit, readable
// baseline of the schema at the point this project adopted golang-migrate,
// for the same reason a legacy codebase migrating onto a real migration
// tool usually writes one, not because this code path depends on it doing
// anything. The order isn't optional the other way around: SQLite has no
// "ALTER TABLE ADD COLUMN IF NOT EXISTS", and 0001's CREATE INDEX IF NOT
// EXISTS statements reference columns (e.g. sessions.fingerprint) that
// simply don't exist yet on a table already created in some older shape —
// confirmed directly, this fails outright ("no such column") the moment
// db/testdata/v0.0.24.db's genuinely old sessions table reaches it before
// AutoMigrate has added that column. AutoMigrate, inside the bridge, is
// the one thing here that actually knows how to add a missing column to
// an existing table; running it first means 0001's IF NOT EXISTS clauses
// only ever see a schema that's already exactly current, table and column
// and index alike, whichever path got it there.
//
// Beyond schema, this project's old system also existed for exactly what
// no SQL migration can express at all: rewriting values already on disk
// in a way that depends on runtime schema introspection (does this table
// still have a column the current release doesn't declare?) rather than
// a fixed SQL statement — see db/legacy_migrations.go's own doc comment.
// Rather than half-porting that logic into fragile SQL, the three
// existing, already-proven Go functions run unchanged as the rest of the
// same bridge.
func runMigrations(sqlDB *sql.DB) error {
	needsBridge, err := needsLegacyBridge(sqlDB)
	if err != nil {
		return fmt.Errorf("checking whether the legacy migration bridge has already run: %w", err)
	}
	if needsBridge {
		log.Printf("db: applying one-time legacy migration bridge (AutoMigrate + data repairs) for a database that predates golang-migrate")
		if err := applyLegacyDataMigrations(sqlDB); err != nil {
			return fmt.Errorf("applying legacy migrations: %w", err)
		}
		// Recorded only after every step above actually succeeds — a
		// process killed mid-bridge, or a step that errors outright,
		// leaves user_version exactly where it was found (0, or an old
		// 1-3 partial value — see needsLegacyBridge), so the *entire*
		// bridge retries on the next startup rather than silently being
		// considered "done" with some of its work never having happened.
		// Safe to retry wholesale: every step in applyLegacyDataMigrations
		// already guards itself against work it's already done.
		if _, err := sqlDB.Exec(fmt.Sprintf("PRAGMA user_version = %d", legacyBridgeDoneVersion)); err != nil {
			return fmt.Errorf("recording legacy migration bridge as applied: %w", err)
		}
	}

	src, err := iofs.New(migrationFiles, "migrations")
	if err != nil {
		return fmt.Errorf("loading embedded migrations: %w", err)
	}
	driver, err := migratesqlite.WithInstance(sqlDB, &migratesqlite.Config{})
	if err != nil {
		return fmt.Errorf("creating sqlite migration driver: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "sqlite", driver)
	if err != nil {
		return fmt.Errorf("creating migrator: %w", err)
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("running schema migrations: %w", err)
	}

	return nil
}

// needsLegacyBridge reports whether sqlDB still needs
// applyLegacyDataMigrations run against it: its PRAGMA user_version isn't
// already legacyBridgeDoneVersion. A brand-new database reads 0 and runs
// the bridge once (a fast no-op, nothing to repair, but still the thing
// that actually creates its schema — see runMigrations' doc comment for
// why that's AutoMigrate's job here, not 0001_initial_schema.up.sql's); a
// genuinely old database (anywhere from completely untouched to fully
// migrated under the old system, PRAGMA values 0 through 4) runs it once
// for real; any database already at legacyBridgeDoneVersion skips it,
// permanently.
func needsLegacyBridge(sqlDB *sql.DB) (bool, error) {
	var current int
	if err := sqlDB.QueryRow("PRAGMA user_version").Scan(&current); err != nil {
		return false, fmt.Errorf("reading PRAGMA user_version: %w", err)
	}
	return current != legacyBridgeDoneVersion, nil
}
