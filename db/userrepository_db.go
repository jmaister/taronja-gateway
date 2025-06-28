package db

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// UserRepositoryDB implements UserRepository with a GORM database connection
type UserRepositoryDB struct {
	db *gorm.DB
}

// NewDBUserRepository creates a new database-backed user repository
func NewDBUserRepository(db *gorm.DB) *UserRepositoryDB {
	return &UserRepositoryDB{
		db: db,
	}
}

// FindUserByIdOrUsername finds a user by ID, username, or email
func (r *UserRepositoryDB) FindUserByIdOrUsername(id, username, email string) (*User, error) {
	var user User

	if id != "" {
		result := r.db.First(&user, "id = ?", id)
		if result.Error == nil {
			return &user, nil
		}
		if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, result.Error
		}
	}

	if email != "" {
		result := r.db.First(&user, "email = ?", email)
		if result.Error == nil {
			return &user, nil
		}
		if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, result.Error
		}
	}

	if username != "" {
		result := r.db.First(&user, "username = ?", username)
		if result.Error == nil {
			return &user, nil
		}
		if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, result.Error
		}
	}

	return nil, gorm.ErrRecordNotFound
}

// CreateUser adds a new user to the repository
func (r *UserRepositoryDB) CreateUser(user *User) error {
	// GORM will use the BeforeCreate hook that generates a CUID
	// Password hashing is handled automatically by the BeforeSave hook
	result := r.db.Create(user)
	return result.Error
}

// UpdateUser updates an existing user in the repository
func (r *UserRepositoryDB) UpdateUser(user *User) error {
	// Make sure the user exists
	var count int64
	r.db.Model(&User{}).Where("id = ?", user.ID).Count(&count)
	if count == 0 {
		return errors.New("user not found")
	}

	// Update the user - save all fields except primary key
	result := r.db.Save(user)
	return result.Error
}

// DeleteUser removes a user from the repository
func (r *UserRepositoryDB) DeleteUser(id string) error {
	result := r.db.Delete(&User{}, "id = ?", id)
	if result.RowsAffected == 0 {
		return errors.New("user not found")
	}
	return result.Error
}

// GetAllUsers retrieves all users from the database.
func (r *UserRepositoryDB) GetAllUsers() ([]*User, error) {
	users := []*User{}
	result := r.db.Find(&users)
	if result.Error != nil {
		return nil, fmt.Errorf("error getting all users: %w", result.Error)
	}
	return users, nil
}

// EnsureAdminUser creates or updates an admin user based on config
func (r *UserRepositoryDB) EnsureAdminUser(username, email, password string) error {
	if username == "" || password == "" {
		return fmt.Errorf("admin username and password cannot be empty")
	}

	// Try to find existing user by username or email
	existingUser, err := r.FindUserByIdOrUsername("", username, email)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("error checking for existing admin user: %w", err)
	}

	if existingUser != nil {
		// User exists, update password if it's different
		existingUser.Password = password // This will be hashed by the BeforeSave hook
		if email != "" {
			existingUser.Email = email
		}
		existingUser.EmailConfirmed = true // Admin users are always confirmed

		err = r.UpdateUser(existingUser)
		if err != nil {
			return fmt.Errorf("error updating admin user: %w", err)
		}
		return nil
	}

	// User doesn't exist, create new admin user
	adminUser := &User{
		Username:       username,
		Email:          email,
		Password:       password,      // Will be hashed by BeforeSave hook
		EmailConfirmed: true,          // Admin users are always confirmed
		Provider:       AdminProvider, // Mark as config-based user
	}

	err = r.CreateUser(adminUser)
	if err != nil {
		return fmt.Errorf("error creating admin user: %w", err)
	}

	return nil
}
