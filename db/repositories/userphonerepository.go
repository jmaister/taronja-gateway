package repositories

import (
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UserPhoneRepository stores each user's phone number. A user without one
// returns gorm.ErrRecordNotFound from Get.
type UserPhoneRepository interface {
	Get(userID string) (*db.UserPhone, error)
	// GetMany returns the phones of the given users, keyed by user ID; users
	// without one are absent.
	GetMany(userIDs []string) (map[string]db.UserPhone, error)
	// Set stores phone for userID, replacing any previous one. verifiedAt is
	// nil for a number nobody has proven yet.
	Set(userID, phone string, verifiedAt *time.Time) error
	Delete(userID string) error
}

// UserPhoneRepositoryDB is the GORM-backed implementation.
type UserPhoneRepositoryDB struct {
	db *gorm.DB
}

// NewUserPhoneRepositoryDB creates a new database-backed UserPhoneRepository.
func NewUserPhoneRepositoryDB(db *gorm.DB) *UserPhoneRepositoryDB {
	return &UserPhoneRepositoryDB{db: db}
}

func (r *UserPhoneRepositoryDB) Get(userID string) (*db.UserPhone, error) {
	var phone db.UserPhone
	err := r.db.Where("user_id = ?", userID).First(&phone).Error
	if err != nil {
		return nil, err
	}
	return &phone, nil
}

func (r *UserPhoneRepositoryDB) GetMany(userIDs []string) (map[string]db.UserPhone, error) {
	out := make(map[string]db.UserPhone, len(userIDs))
	if len(userIDs) == 0 {
		return out, nil
	}
	var phones []db.UserPhone
	err := r.db.Where("user_id IN ?", userIDs).Find(&phones).Error
	if err != nil {
		return nil, err
	}
	for _, p := range phones {
		out[p.UserID] = p
	}
	return out, nil
}

func (r *UserPhoneRepositoryDB) Set(userID, phone string, verifiedAt *time.Time) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"phone", "verified_at", "updated_at"}),
	}).Create(&db.UserPhone{UserID: userID, Phone: phone, VerifiedAt: verifiedAt}).Error
}

func (r *UserPhoneRepositoryDB) Delete(userID string) error {
	return r.db.Where("user_id = ?", userID).Delete(&db.UserPhone{}).Error
}
