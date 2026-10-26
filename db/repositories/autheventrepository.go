package repositories

import (
	"github.com/jmaister/taronja-gateway/db"
	"gorm.io/gorm"
)

// AuthEventRepository stores each user's security activity log.
type AuthEventRepository interface {
	Create(event *db.AuthEvent) error
}

// AuthEventRepositoryDB is the GORM-backed implementation.
type AuthEventRepositoryDB struct {
	db *gorm.DB
}

// NewAuthEventRepositoryDB creates a new database-backed AuthEventRepository.
func NewAuthEventRepositoryDB(db *gorm.DB) *AuthEventRepositoryDB {
	return &AuthEventRepositoryDB{db: db}
}

func (r *AuthEventRepositoryDB) Create(event *db.AuthEvent) error {
	return r.db.Create(event).Error
}
