package db

import (
	"errors"
	"gorm.io/gorm"
)

// UserLoginRepositoryDB is the GORM-based implementation of UserLoginRepository
type UserLoginRepositoryDB struct {
	db *gorm.DB
}

// NewUserLoginRepositoryDB creates a new UserLoginRepositoryDB instance
func NewUserLoginRepositoryDB(database *gorm.DB) *UserLoginRepositoryDB {
	return &UserLoginRepositoryDB{db: database}
}

// FindUserLoginByProvider finds a login method by provider and provider ID
func (repo *UserLoginRepositoryDB) FindUserLoginByProvider(provider, providerId string) (*UserLogin, error) {
	var userLogin UserLogin
	err := repo.db.Where("provider = ? AND provider_id = ? AND is_active = ?", provider, providerId, true).First(&userLogin).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return nil, err
	}
	return &userLogin, nil
}

// FindUserLoginsByUserID finds all login methods for a user
func (repo *UserLoginRepositoryDB) FindUserLoginsByUserID(userID string) ([]*UserLogin, error) {
	var userLogins []*UserLogin
	err := repo.db.Where("user_id = ? AND is_active = ?", userID, true).Find(&userLogins).Error
	if err != nil {
		return nil, err
	}
	return userLogins, nil
}

// CreateUserLogin creates a new login method for a user
func (repo *UserLoginRepositoryDB) CreateUserLogin(userLogin *UserLogin) error {
	return repo.db.Create(userLogin).Error
}

// UpdateUserLogin updates an existing login method
func (repo *UserLoginRepositoryDB) UpdateUserLogin(userLogin *UserLogin) error {
	return repo.db.Save(userLogin).Error
}

// DeleteUserLogin deletes a login method (hard delete)
func (repo *UserLoginRepositoryDB) DeleteUserLogin(id string) error {
	return repo.db.Delete(&UserLogin{}, "id = ?", id).Error
}

// DeactivateUserLogin deactivates a login method instead of deleting
func (repo *UserLoginRepositoryDB) DeactivateUserLogin(id string) error {
	return repo.db.Model(&UserLogin{}).Where("id = ?", id).Update("is_active", false).Error
}

// FindUserByProviderLogin finds the user associated with a provider login
func (repo *UserLoginRepositoryDB) FindUserByProviderLogin(provider, providerId string) (*User, error) {
	var user User
	err := repo.db.Joins("JOIN user_logins ON users.id = user_logins.user_id").
		Where("user_logins.provider = ? AND user_logins.provider_id = ? AND user_logins.is_active = ?", 
			provider, providerId, true).
		First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return nil, err
	}
	return &user, nil
}
