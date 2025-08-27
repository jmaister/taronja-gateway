package db

import (
	"testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserLoginRepositoryMemory_CreateAndFind(t *testing.T) {
	userRepo := NewMemoryUserRepository()
	userLoginRepo := NewUserLoginRepositoryMemory(userRepo)

	// Create a test user first
	user := &User{
		Email:    "test@example.com",
		Username: "testuser",
		Name:     "Test User",
	}
	err := userRepo.CreateUser(user)
	require.NoError(t, err)

	// Create a user login
	userLogin := &UserLogin{
		UserID:     user.ID,
		Provider:   "google",
		ProviderId: "google123",
		Email:      "test@gmail.com",
		Username:   "testuser",
		IsActive:   true,
	}

	// Test CreateUserLogin
	err = userLoginRepo.CreateUserLogin(userLogin)
	require.NoError(t, err)
	assert.NotEmpty(t, userLogin.ID)

	// Test FindUserLoginByProvider
	foundLogin, err := userLoginRepo.FindUserLoginByProvider("google", "google123")
	require.NoError(t, err)
	assert.Equal(t, userLogin.ID, foundLogin.ID)
	assert.Equal(t, "google", foundLogin.Provider)
	assert.Equal(t, "google123", foundLogin.ProviderId)

	// Test FindUserLoginsByUserID
	userLogins, err := userLoginRepo.FindUserLoginsByUserID(user.ID)
	require.NoError(t, err)
	assert.Len(t, userLogins, 1)
	assert.Equal(t, userLogin.ID, userLogins[0].ID)

	// Test FindUserByProviderLogin
	foundUser, err := userLoginRepo.FindUserByProviderLogin("google", "google123")
	require.NoError(t, err)
	assert.Equal(t, user.ID, foundUser.ID)
	assert.Equal(t, user.Email, foundUser.Email)
}

func TestUserLoginRepositoryMemory_MultipleLogins(t *testing.T) {
	userRepo := NewMemoryUserRepository()
	userLoginRepo := NewUserLoginRepositoryMemory(userRepo)

	// Create a test user
	user := &User{
		Email:    "test@example.com",
		Username: "testuser",
		Name:     "Test User",
	}
	err := userRepo.CreateUser(user)
	require.NoError(t, err)

	// Create multiple user logins for the same user
	googleLogin := &UserLogin{
		UserID:     user.ID,
		Provider:   "google",
		ProviderId: "google123",
		Email:      "test@gmail.com",
	}
	
	githubLogin := &UserLogin{
		UserID:     user.ID,
		Provider:   "github",
		ProviderId: "github456",
		Email:      "test@example.com",
	}

	err = userLoginRepo.CreateUserLogin(googleLogin)
	require.NoError(t, err)

	err = userLoginRepo.CreateUserLogin(githubLogin)
	require.NoError(t, err)

	// Test finding all logins for the user
	userLogins, err := userLoginRepo.FindUserLoginsByUserID(user.ID)
	require.NoError(t, err)
	assert.Len(t, userLogins, 2)

	// Test finding each login by provider
	foundGoogle, err := userLoginRepo.FindUserLoginByProvider("google", "google123")
	require.NoError(t, err)
	assert.Equal(t, googleLogin.ID, foundGoogle.ID)

	foundGithub, err := userLoginRepo.FindUserLoginByProvider("github", "github456")
	require.NoError(t, err)
	assert.Equal(t, githubLogin.ID, foundGithub.ID)

	// Test that both logins point to the same user
	userFromGoogle, err := userLoginRepo.FindUserByProviderLogin("google", "google123")
	require.NoError(t, err)
	
	userFromGithub, err := userLoginRepo.FindUserByProviderLogin("github", "github456")
	require.NoError(t, err)
	
	assert.Equal(t, user.ID, userFromGoogle.ID)
	assert.Equal(t, user.ID, userFromGithub.ID)
}

func TestUserLoginRepositoryMemory_DuplicateProviderLogin(t *testing.T) {
	userRepo := NewMemoryUserRepository()
	userLoginRepo := NewUserLoginRepositoryMemory(userRepo)

	// Create two users
	user1 := &User{
		Email:    "user1@example.com",
		Username: "user1",
		Name:     "User One",
	}
	user2 := &User{
		Email:    "user2@example.com",
		Username: "user2",
		Name:     "User Two",
	}
	
	err := userRepo.CreateUser(user1)
	require.NoError(t, err)
	err = userRepo.CreateUser(user2)
	require.NoError(t, err)

	// Create first login
	login1 := &UserLogin{
		UserID:     user1.ID,
		Provider:   "google",
		ProviderId: "google123",
		Email:      "test@gmail.com",
	}
	err = userLoginRepo.CreateUserLogin(login1)
	require.NoError(t, err)

	// Try to create duplicate provider login - should fail
	login2 := &UserLogin{
		UserID:     user2.ID,
		Provider:   "google",
		ProviderId: "google123", // Same provider ID
		Email:      "test@gmail.com",
	}
	err = userLoginRepo.CreateUserLogin(login2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestUserLoginRepositoryMemory_DeactivateLogin(t *testing.T) {
	userRepo := NewMemoryUserRepository()
	userLoginRepo := NewUserLoginRepositoryMemory(userRepo)

	// Create a test user and login
	user := &User{
		Email:    "test@example.com",
		Username: "testuser",
		Name:     "Test User",
	}
	err := userRepo.CreateUser(user)
	require.NoError(t, err)

	userLogin := &UserLogin{
		UserID:     user.ID,
		Provider:   "google",
		ProviderId: "google123",
		Email:      "test@gmail.com",
	}
	err = userLoginRepo.CreateUserLogin(userLogin)
	require.NoError(t, err)

	// Verify login is active and findable
	foundLogin, err := userLoginRepo.FindUserLoginByProvider("google", "google123")
	require.NoError(t, err)
	assert.True(t, foundLogin.IsActive)

	// Deactivate the login
	err = userLoginRepo.DeactivateUserLogin(userLogin.ID)
	require.NoError(t, err)

	// Verify login is no longer findable by provider
	_, err = userLoginRepo.FindUserLoginByProvider("google", "google123")
	assert.Error(t, err)

	// But should still be in memory (just inactive)
	userLogins, err := userLoginRepo.FindUserLoginsByUserID(user.ID)
	require.NoError(t, err)
	assert.Len(t, userLogins, 0) // Should be 0 because we only return active logins
}
