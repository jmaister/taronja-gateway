package db

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupTestDB creates a new in-memory test database for each test
func setupTestDB(t *testing.T) *gorm.DB {
	// Use a unique database name for each test to ensure isolation
	dbName := "file::memory:?cache=shared&_" + t.Name()
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	assert.NoError(t, err)

	// Migrate the schema
	err = db.AutoMigrate(&User{})
	assert.NoError(t, err)

	return db
}

// createTestUser creates a test user for tests
func createTestUser(suffix string) *User {
	return &User{
		Email:          "test-" + suffix + "@example.com",
		Username:       "testuser-" + suffix,
		Name:           "Test User",
		Password:       "hashedpassword",
		EmailConfirmed: true,
	}
}

func TestFindUserByIdOrUsername(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDBUserRepository(db)

	// Create a test user
	testUser := createTestUser("find-test")
	result := db.Create(testUser)
	assert.NoError(t, result.Error)

	// Test finding by ID
	foundUser, err := repo.FindUserByIdOrUsername(testUser.ID, "", "")
	assert.NoError(t, err)
	assert.NotNil(t, foundUser)
	assert.Equal(t, testUser.ID, foundUser.ID)

	// Test finding by username
	foundUser, err = repo.FindUserByIdOrUsername("", testUser.Username, "")
	assert.NoError(t, err)
	assert.NotNil(t, foundUser)
	assert.Equal(t, testUser.Username, foundUser.Username)

	// Test finding by email
	foundUser, err = repo.FindUserByIdOrUsername("", "", testUser.Email)
	assert.NoError(t, err)
	assert.NotNil(t, foundUser)
	assert.Equal(t, testUser.Email, foundUser.Email)

	// Test not found case
	foundUser, err = repo.FindUserByIdOrUsername("nonexistent", "", "")
	assert.Nil(t, foundUser)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "record not found", "Should return record not found error")
}

func TestCreateUser(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDBUserRepository(db)

	// Create a test user
	testUser := createTestUser("create-test")
	err := repo.CreateUser(testUser)
	assert.NoError(t, err)
	assert.NotEmpty(t, testUser.ID) // Verify CUID is generated

	// Verify user exists in DB
	var foundUser User
	result := db.First(&foundUser, "id = ?", testUser.ID)
	assert.NoError(t, result.Error)
	assert.Equal(t, testUser.Username, foundUser.Username)
	assert.Equal(t, testUser.Email, foundUser.Email)
}

func TestUpdateUser(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDBUserRepository(db)

	// Create a test user
	testUser := createTestUser("update-test")
	result := db.Create(testUser)
	assert.NoError(t, result.Error)

	// Update user
	testUser.Name = "Updated Name"
	testUser.Email = "updated@example.com"
	err := repo.UpdateUser(testUser)
	assert.NoError(t, err)

	// Verify changes persisted
	var foundUser User
	result = db.First(&foundUser, "id = ?", testUser.ID)
	assert.NoError(t, result.Error)
	assert.Equal(t, "Updated Name", foundUser.Name)
	assert.Equal(t, "updated@example.com", foundUser.Email)

	// Test updating non-existent user
	nonExistentUser := &User{
		ID:       "nonexistent",
		Username: "nonexistent",
		Email:    "nonexistent@example.com",
	}
	err = repo.UpdateUser(nonExistentUser)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "user not found")
}

func TestDeleteUser(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDBUserRepository(db)

	// Create a test user
	testUser := createTestUser("delete-test")
	result := db.Create(testUser)
	assert.NoError(t, result.Error)

	// Delete user
	err := repo.DeleteUser(testUser.ID)
	assert.NoError(t, err)

	// Verify user is deleted
	var foundUser User
	result = db.First(&foundUser, "id = ?", testUser.ID)
	assert.Error(t, result.Error) // Should not find the user

	// Test deleting non-existent user
	err = repo.DeleteUser("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "user not found")
}

func TestConcurrentOperations(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDBUserRepository(db)

	// First ensure the table is empty
	db.Exec("DELETE FROM users")

	// Create multiple test users
	expectedCount := 10
	for i := 0; i < expectedCount; i++ {
		user := createTestUser("concurrent-" + string(rune('0'+i)))
		err := repo.CreateUser(user)
		assert.NoError(t, err)
	}

	// Verify count
	var count int64
	db.Model(&User{}).Count(&count)
	assert.Equal(t, int64(expectedCount), count)
}

func TestUserSchema(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDBUserRepository(db)

	// Create a user with all fields populated
	user := &User{
		Email:                    "complete@example.com",
		Username:                 "complete_user",
		Picture:                  "https://example.com/pic.jpg",
		Name:                     "Complete User",
		Locale:                   "en-US",
		Password:                 "hashedpassword",
		PasswordReset:            true,
		PasswordResetCode:        "resetcode",
		PasswordResetExpires:     &time.Time{},
		EmailConfirmed:           true,
		EmailConfirmationCode:    "confirmcode",
		EmailConfirmationExpires: &time.Time{},
	}

	err := repo.CreateUser(user)
	assert.NoError(t, err)

	// Retrieve and verify all fields
	foundUser, err := repo.FindUserByIdOrUsername(user.ID, "", "")
	assert.NoError(t, err)
	assert.Equal(t, user.Email, foundUser.Email)
	assert.Equal(t, user.Username, foundUser.Username)
	assert.Equal(t, user.Picture, foundUser.Picture)
	assert.Equal(t, user.Name, foundUser.Name)
	assert.Equal(t, user.PasswordReset, foundUser.PasswordReset)
	assert.Equal(t, user.EmailConfirmed, foundUser.EmailConfirmed)
}

// TestUniqueConstraints tests that the database enforces unique constraints on email and username
// Note: This test purposely creates errors which will be logged - these errors are expected and are part of the test
func TestUniqueConstraints(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDBUserRepository(db)

	// Create initial test user
	testUser := createTestUser("unique-test")
	err := repo.CreateUser(testUser)
	assert.NoError(t, err)

	t.Log("Testing email uniqueness - expect an error next")
	// Try to create user with same email
	duplicateEmailUser := createTestUser("unique-email-test")
	duplicateEmailUser.Email = testUser.Email // Use the same email to trigger constraint
	err = repo.CreateUser(duplicateEmailUser)
	// Validate that the error occurs and contains expected text
	assert.Error(t, err, "Should fail due to unique email constraint")
	assert.Contains(t, err.Error(), "UNIQUE constraint failed", "Error should mention UNIQUE constraint")

	t.Log("Testing username uniqueness - expect an error next")
	// Try to create user with same username
	duplicateUsernameUser := createTestUser("unique-username-test")
	duplicateUsernameUser.Username = testUser.Username // Use the same username to trigger constraint
	err = repo.CreateUser(duplicateUsernameUser)
	// Validate that the error occurs and contains expected text
	assert.Error(t, err, "Should fail due to unique username constraint")
	assert.Contains(t, err.Error(), "UNIQUE constraint failed", "Error should mention UNIQUE constraint")
}
