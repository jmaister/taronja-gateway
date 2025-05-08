// filepath: c:\dev\workspace\taronja-gateway\db\mockdb.go
package db

import (
	"crypto/rand"
	"errors"
	"math/big"
	"sync"
)

// User represents a user in the system
type User struct {
	ID             string
	Username       string
	Email          string
	Name           string
	GivenName      string
	FamilyName     string
	Picture        string
	Locale         string
	Roles          string
	EmailConfirmed bool
	Provider       string
	ProviderId     string
}

// UserRepository interface for abstracting user database operations
type UserRepository interface {
	FindUserByIdOrUsername(id, username, email string) (*User, error)
	CreateUser(user *User) error
	UpdateUser(user *User) error
	DeleteUser(id string) error
}

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
		user.ID = GenerateRandomID()
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

// GenerateRandomID generates a simple random ID
func GenerateRandomID() string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 16)
	for i := range b {
		max := big.NewInt(int64(len(letters)))
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			// Fallback to a simple counter in case of failure
			b[i] = letters[i%len(letters)]
			continue
		}
		b[i] = letters[n.Int64()]
	}
	return string(b)
}
