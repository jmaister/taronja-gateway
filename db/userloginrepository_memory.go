package db

import (
	"crypto/rand"
	"errors"
	"sync"
	"gorm.io/gorm"
	"github.com/lucsky/cuid"
)

// UserLoginRepositoryMemory is the in-memory implementation of UserLoginRepository for testing
type UserLoginRepositoryMemory struct {
	userLogins map[string]*UserLogin // id -> UserLogin
	mu         sync.RWMutex
	userRepo   UserRepository // Reference to user repository for finding users
}

// NewUserLoginRepositoryMemory creates a new UserLoginRepositoryMemory instance
func NewUserLoginRepositoryMemory(userRepo UserRepository) *UserLoginRepositoryMemory {
	return &UserLoginRepositoryMemory{
		userLogins: make(map[string]*UserLogin),
		userRepo:   userRepo,
	}
}

// FindUserLoginByProvider finds a login method by provider and provider ID
func (repo *UserLoginRepositoryMemory) FindUserLoginByProvider(provider, providerId string) (*UserLogin, error) {
	repo.mu.RLock()
	defer repo.mu.RUnlock()
	
	for _, userLogin := range repo.userLogins {
		if userLogin.Provider == provider && userLogin.ProviderId == providerId && userLogin.IsActive {
			return userLogin, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

// FindUserLoginsByUserID finds all login methods for a user
func (repo *UserLoginRepositoryMemory) FindUserLoginsByUserID(userID string) ([]*UserLogin, error) {
	repo.mu.RLock()
	defer repo.mu.RUnlock()
	
	var userLogins []*UserLogin
	for _, userLogin := range repo.userLogins {
		if userLogin.UserID == userID && userLogin.IsActive {
			userLogins = append(userLogins, userLogin)
		}
	}
	return userLogins, nil
}

// CreateUserLogin creates a new login method for a user
func (repo *UserLoginRepositoryMemory) CreateUserLogin(userLogin *UserLogin) error {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	
	// Generate ID if not set
	if userLogin.ID == "" {
		newId, err := cuid.NewCrypto(rand.Reader)
		if err != nil {
			return err
		}
		userLogin.ID = newId
	}
	
	// Check if this provider login already exists
	for _, existing := range repo.userLogins {
		if existing.Provider == userLogin.Provider && existing.ProviderId == userLogin.ProviderId && existing.IsActive {
			return errors.New("user login with this provider and provider ID already exists")
		}
	}
	
	userLogin.IsActive = true
	repo.userLogins[userLogin.ID] = userLogin
	return nil
}

// UpdateUserLogin updates an existing login method
func (repo *UserLoginRepositoryMemory) UpdateUserLogin(userLogin *UserLogin) error {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	
	if _, exists := repo.userLogins[userLogin.ID]; !exists {
		return gorm.ErrRecordNotFound
	}
	
	repo.userLogins[userLogin.ID] = userLogin
	return nil
}

// DeleteUserLogin deletes a login method (hard delete)
func (repo *UserLoginRepositoryMemory) DeleteUserLogin(id string) error {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	
	if _, exists := repo.userLogins[id]; !exists {
		return gorm.ErrRecordNotFound
	}
	
	delete(repo.userLogins, id)
	return nil
}

// DeactivateUserLogin deactivates a login method instead of deleting
func (repo *UserLoginRepositoryMemory) DeactivateUserLogin(id string) error {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	
	userLogin, exists := repo.userLogins[id]
	if !exists {
		return gorm.ErrRecordNotFound
	}
	
	userLogin.IsActive = false
	return nil
}

// FindUserByProviderLogin finds the user associated with a provider login
func (repo *UserLoginRepositoryMemory) FindUserByProviderLogin(provider, providerId string) (*User, error) {
	userLogin, err := repo.FindUserLoginByProvider(provider, providerId)
	if err != nil {
		return nil, err
	}
	
	return repo.userRepo.FindUserByIdOrUsername(userLogin.UserID, "", "")
}
