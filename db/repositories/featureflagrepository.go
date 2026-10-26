package repositories

import (
	"github.com/jmaister/taronja-gateway/db"
	"gorm.io/gorm"
)

// FeatureFlagRepository persists feature flags. Lookups of a missing key return
// gorm.ErrRecordNotFound.
type FeatureFlagRepository interface {
	List() ([]db.FeatureFlag, error)
	Get(key string) (*db.FeatureFlag, error)
	Create(flag *db.FeatureFlag) error
	Update(flag *db.FeatureFlag) error
	Delete(key string) error
}

// FeatureFlagRepositoryDB is the GORM-backed implementation.
type FeatureFlagRepositoryDB struct {
	db *gorm.DB
}

// NewFeatureFlagRepositoryDB creates a new database-backed FeatureFlagRepository.
func NewFeatureFlagRepositoryDB(db *gorm.DB) *FeatureFlagRepositoryDB {
	return &FeatureFlagRepositoryDB{db: db}
}

func (r *FeatureFlagRepositoryDB) List() ([]db.FeatureFlag, error) {
	var flags []db.FeatureFlag
	err := r.db.Order("`key` ASC").Find(&flags).Error
	if err != nil {
		return nil, err
	}
	return flags, nil
}

func (r *FeatureFlagRepositoryDB) Get(key string) (*db.FeatureFlag, error) {
	var flag db.FeatureFlag
	err := r.db.Where("`key` = ?", key).First(&flag).Error
	if err != nil {
		return nil, err
	}
	return &flag, nil
}

func (r *FeatureFlagRepositoryDB) Create(flag *db.FeatureFlag) error {
	return r.db.Create(flag).Error
}

// Update saves every field of flag, including Enabled=false, which a
// struct-based Updates would skip as a zero value.
func (r *FeatureFlagRepositoryDB) Update(flag *db.FeatureFlag) error {
	result := r.db.Model(&db.FeatureFlag{}).Where("`key` = ?", flag.Key).Updates(map[string]any{
		"description": flag.Description,
		"type":        flag.Type,
		"enabled":     flag.Enabled,
		"updated_at":  db.NowUTC(),
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *FeatureFlagRepositoryDB) Delete(key string) error {
	result := r.db.Where("`key` = ?", key).Delete(&db.FeatureFlag{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
