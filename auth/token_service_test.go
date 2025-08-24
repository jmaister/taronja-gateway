package auth

import (
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenService(t *testing.T) {
	// Setup repositories
	userRepo := db.NewMemoryUserRepository()
	tokenRepo := db.NewTokenRepositoryMemory()
	tokenService := NewTokenService(tokenRepo, userRepo)

	// Create a test user
	user := &db.User{
		ID:       "user-123",
		Username: "testuser",
		Email:    "test@example.com",
		Name:     "Test User",
	}
	err := userRepo.CreateUser(user)
	require.NoError(t, err)

	t.Run("GenerateToken", func(t *testing.T) {
		tokenString, token, err := tokenService.GenerateToken(
			user.ID,
			"Test Token",
			nil, // no expiration
			[]string{"read", "write"},
			"test",
			nil,
		)

		require.NoError(t, err)
		assert.NotEmpty(t, tokenString)
		assert.Contains(t, tokenString, TokenPrefix)
		assert.Equal(t, user.ID, token.UserID)
		assert.Equal(t, "Test Token", token.Name)
		assert.True(t, token.IsActive)
		assert.Contains(t, token.Scopes, "read")
		assert.Contains(t, token.Scopes, "write")
	})

	t.Run("ValidateToken", func(t *testing.T) {
		// Generate a token first
		tokenString, originalToken, err := tokenService.GenerateToken(
			user.ID,
			"Validation Test Token",
			nil,
			nil,
			"test",
			nil,
		)
		require.NoError(t, err)

		// Validate the token
		validatedUser, validatedToken, err := tokenService.ValidateToken(tokenString)
		require.NoError(t, err)

		assert.Equal(t, user.ID, validatedUser.ID)
		assert.Equal(t, user.Username, validatedUser.Username)
		assert.Equal(t, originalToken.ID, validatedToken.ID)
		assert.Equal(t, int64(1), validatedToken.UsageCount) // Should be incremented
	})

	t.Run("ValidateToken_InvalidFormat", func(t *testing.T) {
		_, _, err := tokenService.ValidateToken("invalid-token")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid token format")
	})

	t.Run("ValidateToken_NotFound", func(t *testing.T) {
		// Create a token with the right format but wrong content
		invalidToken := TokenPrefix + "invalidtokendata"
		_, _, err := tokenService.ValidateToken(invalidToken)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "token not found")
	})

	t.Run("GenerateToken_ExpiredToken", func(t *testing.T) {
		// Generate a token that expires in the past
		expiredTime := time.Now().Add(-1 * time.Hour)
		tokenString, _, err := tokenService.GenerateToken(
			user.ID,
			"Expired Token",
			&expiredTime,
			nil,
			"test",
			nil,
		)
		require.NoError(t, err)

		// Try to validate the expired token
		_, _, err = tokenService.ValidateToken(tokenString)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "token is expired")
	})

	t.Run("GetUserTokens", func(t *testing.T) {
		// Generate multiple tokens for the user
		_, _, err := tokenService.GenerateToken(user.ID, "Token 1", nil, nil, "test", nil)
		require.NoError(t, err)

		_, _, err = tokenService.GenerateToken(user.ID, "Token 2", nil, nil, "test", nil)
		require.NoError(t, err)

		// Get all tokens for the user
		tokens, err := tokenService.GetUserTokens(user.ID)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(tokens), 2) // At least 2 tokens (might be more from other tests)
	})



	t.Run("RevokeToken", func(t *testing.T) {
		// Generate a token to revoke
		_, token, err := tokenService.GenerateToken(user.ID, "Token to Revoke", nil, nil, "test", nil)
		require.NoError(t, err)
		assert.True(t, token.IsActive)

		// Revoke the token
		err = tokenService.RevokeToken(token.ID, user.ID, user.ID)
		require.NoError(t, err)

		// Verify token is revoked
		updatedToken, err := tokenRepo.GetTokenByID(token.ID)
		require.NoError(t, err)
		assert.False(t, updatedToken.IsActive)
		assert.NotNil(t, updatedToken.RevokedAt)
		assert.Equal(t, user.ID, updatedToken.RevokedBy)

		// Try to revoke again - should fail
		err = tokenService.RevokeToken(token.ID, user.ID, user.ID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already revoked")
	})

	t.Run("RevokeToken_NotOwner", func(t *testing.T) {
		// Generate a token for user1
		_, token, err := tokenService.GenerateToken(user.ID, "Token owned by user1", nil, nil, "test", nil)
		require.NoError(t, err)

		// Try to revoke with different user - should fail
		otherUserID := "other-user-id"
		err = tokenService.RevokeToken(token.ID, otherUserID, otherUserID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "does not belong to user")
	})

	t.Run("RevokeToken_NotFound", func(t *testing.T) {
		// Try to revoke non-existent token
		err := tokenService.RevokeToken("non-existent-token", user.ID, user.ID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "token not found")
	})
}
