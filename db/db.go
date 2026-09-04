package db

import (
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

	// Migrate the schema
	err2 := db.AutoMigrate(&User{}, &Session{}, &TrafficMetric{}, &Token{}, &Counter{}, &BlockedClient{})
	if err2 != nil {
		panic("Failed to migration DB: " + err2.Error())
	}

	// Data-level migrations AutoMigrate itself can't do (see
	// applyDBMigrations' comment) — after AutoMigrate, since a migration may
	// depend on a column/table it only just added.
	if err := applyDBMigrations(db); err != nil {
		panic("Failed to apply DB migrations: " + err.Error())
	}

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
	// one connection AutoMigrate happened to run on has the schema. With
	// MaxOpenConns > 1, any query the pool hands to a different connection
	// then fails with "no such table: ...", intermittently and only under
	// enough concurrent load to actually check out a second connection
	// (e.g. the async traffic-metrics write racing a session lookup). A
	// single connection makes that impossible: there is only ever one
	// in-memory database for this test to talk to. (cache=shared would be
	// the other fix, but is deliberately not used here — see the comment
	// above on dbName.)
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0) // No limit for SQLite

	// Migrate all schemas
	err = db.AutoMigrate(
		&User{},
		&Session{},
		&TrafficMetric{},
		&Token{},
		&Counter{},
		&BlockedClient{},
	)
	if err != nil {
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
