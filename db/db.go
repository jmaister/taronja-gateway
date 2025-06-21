package db

import (
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var conn *gorm.DB

func Init() {
	db, err := gorm.Open(sqlite.Open("taronja-gateway.db"), &gorm.Config{})
	if err != nil {
		panic("Failed to connect database: " + err.Error())
	}

	// Migrate the schema
	err2 := db.AutoMigrate(&User{}, &Session{}, &TrafficMetric{})
	if err2 != nil {
		panic("Failed to migration DB: " + err2.Error())
	}

	conn = db
}

func InitForTest() {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		panic("Failed to connect database: " + err.Error())
	}

	// Migrate the schema
	err2 := db.AutoMigrate(&User{}, &Session{}, &TrafficMetric{})
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
