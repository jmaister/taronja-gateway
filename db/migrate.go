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

// legacyBridgeDoneVersion is the PRAGMA user_version written once the legacy
// bridge completes. It reuses the old system's counter with a value that system
// never wrote (1-4).
const legacyBridgeDoneVersion = 9999

// runMigrations brings sqlDB up to date. Databases that predate golang-migrate
// first go through the legacy bridge (AutoMigrate plus the Go data repairs in
// legacy_migrations.go); after that every schema change is a numbered SQL file in
// db/migrations/, tracked in schema_migrations.
//
// The bridge runs before Up(): 0001_initial_schema is only a readable baseline,
// and its CREATE INDEX IF NOT EXISTS statements fail ("no such column") on a
// table in an older shape until AutoMigrate has added the missing columns.
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
		// Recorded only after every step succeeds, so a failure or crash retries the
		// whole bridge on next startup. Each step guards against repeated work.
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

// needsLegacyBridge reports whether PRAGMA user_version is not yet
// legacyBridgeDoneVersion. A brand-new database runs the bridge too: it is what
// creates its schema.
func needsLegacyBridge(sqlDB *sql.DB) (bool, error) {
	var current int
	if err := sqlDB.QueryRow("PRAGMA user_version").Scan(&current); err != nil {
		return false, fmt.Errorf("reading PRAGMA user_version: %w", err)
	}
	return current != legacyBridgeDoneVersion, nil
}
