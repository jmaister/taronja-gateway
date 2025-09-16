package db

import (
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

var conn *gorm.DB

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
	}, &gorm.Config{})
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
	err2 := db.AutoMigrate(&User{}, &Session{}, &TrafficMetric{}, &Token{}, &Credit{})
	if err2 != nil {
		panic("Failed to migration DB: " + err2.Error())
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
		Logger: logger.Default.LogMode(logger.Silent), // Suppress logging during tests
	})
	if err != nil {
		panic("Failed to connect to test database: " + err.Error())
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

	// Migrate all schemas
	err = db.AutoMigrate(
		&User{},
		&Session{},
		&TrafficMetric{},
		&Token{},
		&Credit{},
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
