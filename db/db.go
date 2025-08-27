package db

import (
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

var conn *gorm.DB

func Init() {
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
	err2 := db.AutoMigrate(&User{}, &UserLogin{}, &Session{}, &TrafficMetric{}, &Token{})
	if err2 != nil {
		panic("Failed to migration DB: " + err2.Error())
	}

	conn = db
}

func InitForTest() {
	// Use modernc.org/sqlite driver for in-memory testing
	// Configure SQLite for better concurrent access and performance
	dsn := "file::memory:?cache=shared&" +
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
	err2 := db.AutoMigrate(&User{}, &UserLogin{}, &Session{}, &TrafficMetric{}, &Token{})
	if err2 != nil {
		panic("Failed to migration DB: " + err2.Error())
	}

	conn = db
}

func GetConnection() *gorm.DB {
	if conn == nil {
		panic("Connection not initialized. Call db.Init() first.")
	}
	return conn
}
