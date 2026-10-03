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

// utcNowFunc replaces GORM's clock so timestamps it sets (gorm.Model fields,
// autoCreateTime/autoUpdateTime) are UTC. Fields the application sets itself, such
// as Session.ValidUntil, are normalized in BeforeSave hooks or at the call site.
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

	// Without cache=shared, each pooled connection to "file::memory:" gets its own
	// empty database, so extra connections fail with "no such table". Use one
	// connection, set before migrating: the database dies when its last connection closes.
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
