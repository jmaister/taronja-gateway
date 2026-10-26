package repositories

import (
	"github.com/jmaister/taronja-gateway/db"
	"gorm.io/gorm"
)

// AuthEventRepository stores each user's security activity log.
type AuthEventRepository interface {
	Create(event *db.AuthEvent) error
	// ListRecent returns userID's newest events first, at most limit.
	ListRecent(userID string, limit int) ([]db.AuthEvent, error)
	// Exists reports whether userID has any event of eventType for provider.
	Exists(userID, eventType, provider string) (bool, error)
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

func (r *AuthEventRepositoryDB) ListRecent(userID string, limit int) ([]db.AuthEvent, error) {
	var events []db.AuthEvent
	err := r.db.Where("user_id = ?", userID).Order("created_at DESC, id DESC").Limit(limit).Find(&events).Error
	if err != nil {
		return nil, err
	}
	return events, nil
}

func (r *AuthEventRepositoryDB) Exists(userID, eventType, provider string) (bool, error) {
	var count int64
	err := r.db.Model(&db.AuthEvent{}).Where("user_id = ? AND type = ? AND provider = ?", userID, eventType, provider).Count(&count).Error
	return count > 0, err
}
