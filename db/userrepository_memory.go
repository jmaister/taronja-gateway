package db

import (
	"crypto/rand"
	"errors"
	"sort"
	"sync"

	"github.com/jmaister/taronja-gateway/encryption"
	"github.com/lucsky/cuid"
)

// MemoryUserRepository implements UserRepository with an in-memory store
type MemoryUserRepository struct {
	users  map[string]*User  // Key is user ID
	emails map[string]string // Maps email to user ID
	mutex  sync.RWMutex
}

// NewMemoryUserRepository creates a new in-memory user repository
func NewMemoryUserRepository() *MemoryUserRepository {
	return &MemoryUserRepository{
		users:  make(map[string]*User),
		emails: make(map[string]string),
	}
}

// FindUserByIdOrUsername finds a user by ID, username, or email
func (r *MemoryUserRepository) FindUserByIdOrUsername(id, username, email string) (*User, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	if id != "" {
		if user, found := r.users[id]; found {
			return user, nil
		}
	}

	if email != "" {
		if userId, found := r.emails[email]; found {
			if user, found := r.users[userId]; found {
				return user, nil
			}
		}
	}

	if username != "" {
		// Search through all users for username match (less efficient)
		for _, user := range r.users {
			if user.Username == username {
				return user, nil
			}
		}
	}

	return nil, nil // Not found but not an error
}

// CreateUser adds a new user to the repository
func (r *MemoryUserRepository) CreateUser(user *User) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Check if user ID is already taken
	if user.ID == "" {
		// Generate a simple ID for demo purposes
		newId, err := cuid.NewCrypto(rand.Reader)
		if err != nil {
			return err
		}
		user.ID = newId
	}

	if _, exists := r.users[user.ID]; exists {
		return errors.New("user ID already exists")
	}

	// Check if email is already taken
	if user.Email != "" {
		if _, exists := r.emails[user.Email]; exists {
			return errors.New("email already exists")
		}
		r.emails[user.Email] = user.ID
	}

	// Hash the password if it's not already hashed
	if user.Password != "" && !encryption.IsPasswordHashed(user.Password) {
		hashedPassword, err := encryption.GeneratePasswordHash(user.Password)
		if err != nil {
			return err
		}
		user.Password = hashedPassword
	}

	// Store the user
	r.users[user.ID] = user

	return nil
}

// UpdateUser updates an existing user in the repository
func (r *MemoryUserRepository) UpdateUser(user *User) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Check if user exists
	oldUser, exists := r.users[user.ID]
	if !exists {
		return errors.New("user not found")
	}

	// Handle email change
	if oldUser.Email != user.Email {
		delete(r.emails, oldUser.Email)
		if user.Email != "" {
			// Check if new email is already taken by another user
			if existingID, taken := r.emails[user.Email]; taken && existingID != user.ID {
				return errors.New("email already in use by another user")
			}
			r.emails[user.Email] = user.ID
		}
	}

	// Hash the password if it has changed and is not already hashed
	if user.Password != "" && user.Password != oldUser.Password && !encryption.IsPasswordHashed(user.Password) {
		hashedPassword, err := encryption.GeneratePasswordHash(user.Password)
		if err != nil {
			return err
		}
		user.Password = hashedPassword
	}

	// Update the user
	r.users[user.ID] = user
	return nil
}

// DeleteUser removes a user from the repository
func (r *MemoryUserRepository) DeleteUser(id string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	user, exists := r.users[id]
	if !exists {
		return errors.New("user not found")
	}

	// Remove email mapping
	if user.Email != "" {
		delete(r.emails, user.Email)
	}

	// Remove user
	delete(r.users, id)
	return nil
}

// GetAllUsers retrieves all users from the in-memory store.
func (r *MemoryUserRepository) GetAllUsers() ([]*User, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	users := make([]*User, 0, len(r.users))
	for _, user := range r.users {
		users = append(users, user)
	}
	// Optionally sort users, e.g., by username for consistency
	sort.Slice(users, func(i, j int) bool {
		return users[i].Username < users[j].Username
	})
	return users, nil
}

// EnsureAdminUser creates or updates an admin user based on config
func (r *MemoryUserRepository) EnsureAdminUser(username, email, password string) error {
	if username == "" || password == "" {
		return errors.New("admin username and password cannot be empty")
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Try to find existing user by username or email
	var existingUser *User
	for _, user := range r.users {
		if user.Username == username || (email != "" && user.Email == email) {
			existingUser = user
			break
		}
	}

	if existingUser != nil {
		// User exists, update password
		if !encryption.IsPasswordHashed(password) {
			hashedPassword, err := encryption.GeneratePasswordHash(password)
			if err != nil {
				return err
			}
			existingUser.Password = hashedPassword
		} else {
			existingUser.Password = password
		}
		if email != "" {
			// Update email mapping if email changed
			if existingUser.Email != "" {
				delete(r.emails, existingUser.Email)
			}
			existingUser.Email = email
			r.emails[email] = existingUser.ID
		}
		existingUser.EmailConfirmed = true // Admin users are always confirmed
		return nil
	}

	// User doesn't exist, create new admin user
	newId, err := cuid.NewCrypto(rand.Reader)
	if err != nil {
		return err
	}

	hashedPassword := password
	if !encryption.IsPasswordHashed(password) {
		hashedPassword, err = encryption.GeneratePasswordHash(password)
		if err != nil {
			return err
		}
	}

	adminUser := &User{
		ID:             newId,
		Username:       username,
		Email:          email,
		Password:       hashedPassword,
		EmailConfirmed: true,          // Admin users are always confirmed
		Provider:       AdminProvider, // Mark as config-based user
	}

	r.users[adminUser.ID] = adminUser
	if email != "" {
		r.emails[email] = adminUser.ID
	}

	return nil
}
