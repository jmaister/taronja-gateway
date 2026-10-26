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
	ListByUserID(userID string) ([]db.UserIdentity, error)
	TouchLastLogin(id string, now time.Time) error
	// Delete removes one of userID's identities; it returns
	// gorm.ErrRecordNotFound if the identity does not belong to userID.
	Delete(userID, id string) error
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

func (r *UserIdentityRepositoryDB) ListByUserID(userID string) ([]db.UserIdentity, error) {
	var identities []db.UserIdentity
	err := r.db.Where("user_id = ?", userID).Order("created_at ASC").Find(&identities).Error
	if err != nil {
		return nil, err
	}
	return identities, nil
}

func (r *UserIdentityRepositoryDB) TouchLastLogin(id string, now time.Time) error {
	return r.db.Model(&db.UserIdentity{}).Where("id = ?", id).Update("last_login_at", now.UTC()).Error
}

func (r *UserIdentityRepositoryDB) Delete(userID, id string) error {
	result := r.db.Where("id = ? AND user_id = ?", id, userID).Delete(&db.UserIdentity{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
