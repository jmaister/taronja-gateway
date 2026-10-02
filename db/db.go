package db

import (
	"database/sql"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

var conn *gorm.DB

// utcNowFunc replaces GORM's default clock (a bare time.Now(), which carries
// the server process's local zone) so every timestamp GORM sets on our
// behalf — gorm.Model's CreatedAt/UpdatedAt/DeletedAt on every model that
// embeds it, and any autoCreateTime/autoUpdateTime-tagged field (Token's
// CreatedAt/UpdatedAt) — is stored as UTC, consistently, regardless of what
// timezone the host machine happens to be running in. This doesn't cover
// fields the application sets explicitly outside GORM's own create/update
// bookkeeping (e.g. Session.ValidUntil, Token.ExpiresAt) — those are
// normalized individually, at the model's BeforeSave hook or the call site
// that constructs them, since GORM's clock never touches them.
func utcNowFunc() time.Time {
	return time.Now().UTC()
}

func Init() {
	// Don't re-initialize if already done
	if conn != nil {
		return
	}

	// Use modernc.org/sqlite driver (pure Go, no CGO required)
	// Configure SQLite for better concurrent access and performance
	dsn := "taronja-gateway.db?" +
		"_pragma=foreign_keys(1)&" +
		"_pragma=journal_mode(WAL)&" +
		"_pragma=synchronous(NORMAL)&" +
		"_pragma=cache_size(1000)&" +
		"_pragma=busy_timeout(30000)&" +
		"_pragma=temp_store(memory)"

	// Migrated on a short-lived connection of its own, fully closed before
	// the real, long-lived, pooled serving connection below ever opens —
	// the gateway never has a connection open against this database for
	// anything else while runMigrations runs. See runMigrations' own doc
	// comment for why this project uses golang-migrate here instead of
	// GORM's AutoMigrate.
	migrationConn, err := sql.Open("sqlite", dsn)
	if err != nil {
		panic("Failed to open database for migrations: " + err.Error())
	}
	if err := runMigrations(migrationConn); err != nil {
		panic("Failed to migrate database: " + err.Error())
	}
	if err := migrationConn.Close(); err != nil {
		panic("Failed to close migration connection: " + err.Error())
	}

	db, err := gorm.Open(sqlite.Dialector{
		DriverName: "sqlite",
		DSN:        dsn,
	}, &gorm.Config{
		NowFunc: utcNowFunc,
	})
	if err != nil {
		panic("Failed to connect database: " + err.Error())
	}

	// Configure connection pool
	sqlDB, err := db.DB()
	if err != nil {
		panic("Failed to get underlying sql.DB: " + err.Error())
	}

	// Set connection pool settings
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(0) // No limit for SQLite

	conn = db
}

// SetupTestDB creates a new in-memory test database with all necessary tables
func SetupTestDB(testName string) {
	// Use a unique database name for each test to ensure isolation
	// Remove cache=shared to ensure each test gets its own database
	dbName := "file::memory:?_" + testName +
		"&_pragma=foreign_keys(1)&" +
		"_pragma=journal_mode(WAL)&" +
		"_pragma=synchronous(NORMAL)&" +
		"_pragma=cache_size(1000)&" +
		"_pragma=busy_timeout(30000)&" +
		"_pragma=temp_store(memory)"

	db, err := gorm.Open(sqlite.Dialector{
		DriverName: "sqlite",
		DSN:        dbName,
	}, &gorm.Config{
		Logger:  logger.Default.LogMode(logger.Silent), // Suppress logging during tests
		NowFunc: utcNowFunc,
	})
	if err != nil {
		panic("Failed to connect to test database: " + err.Error())
	}

	// Configure connection pool
	sqlDB, err := db.DB()
	if err != nil {
		panic("Failed to get underlying sql.DB: " + err.Error())
	}

	// A "file::memory:" DSN without cache=shared gives every pooled
	// connection its own independent, empty in-memory database — only the
	// one connection migrations ran on has the schema. With MaxOpenConns >
	// 1, any query the pool hands to a different connection then fails
	// with "no such table: ...", intermittently and only under enough
	// concurrent load to actually check out a second connection (e.g. the
	// async traffic-metrics write racing a session lookup). A single
	// connection makes that impossible: there is only ever one in-memory
	// database for this test to talk to. (cache=shared would be the other
	// fix, but is deliberately not used here — see the comment above on
	// dbName.) Set before migrating, not after: the in-memory database
	// only exists for as long as at least one connection to it stays
	// open, so this must already be the one and only connection
	// runMigrations below uses too, not a second, throwaway one that's
	// closed afterward (fine for Init's real file, where the schema
	// persists on disk regardless of which connection wrote it, but fatal
	// here — closing the only connection to a "file::memory:" database
	// destroys it, schema included).
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0) // No limit for SQLite

	if err := runMigrations(sqlDB); err != nil {
		panic("Failed to migrate test database: " + err.Error())
	}

	conn = db
}

func GetConnection() *gorm.DB {
	if conn == nil {
		panic("Connection not initialized. Call db.Init() first.")
	}
	return conn
}

// ResetConnection forces a reset of the global connection
// This is useful for testing to ensure a fresh database
func ResetConnection() {
	if conn != nil {
		if sqlDB, err := conn.DB(); err == nil {
			sqlDB.Close()
		}
	}
	conn = nil
}
