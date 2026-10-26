package repositories

import (
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"gorm.io/gorm"
)

// UserIdentityRepository stores the external logins linked to each user.
// Lookups of a missing identity return gorm.ErrRecordNotFound.
type UserIdentityRepository interface {
	Create(identity *db.UserIdentity) error
	FindByProviderSubject(provider, subject string) (*db.UserIdentity, error)
	TouchLastLogin(id string, now time.Time) error
}

// UserIdentityRepositoryDB is the GORM-backed implementation.
type UserIdentityRepositoryDB struct {
	db *gorm.DB
}

// NewUserIdentityRepositoryDB creates a new database-backed UserIdentityRepository.
func NewUserIdentityRepositoryDB(db *gorm.DB) *UserIdentityRepositoryDB {
	return &UserIdentityRepositoryDB{db: db}
}

func (r *UserIdentityRepositoryDB) Create(identity *db.UserIdentity) error {
	return r.db.Create(identity).Error
}

func (r *UserIdentityRepositoryDB) FindByProviderSubject(provider, subject string) (*db.UserIdentity, error) {
	var identity db.UserIdentity
	err := r.db.Where("provider = ? AND subject = ?", provider, subject).First(&identity).Error
	if err != nil {
		return nil, err
	}
	return &identity, nil
}

func (r *UserIdentityRepositoryDB) TouchLastLogin(id string, now time.Time) error {
	return r.db.Model(&db.UserIdentity{}).Where("id = ?", id).Update("last_login_at", now.UTC()).Error
}
