package db

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenRepositoryMemory(t *testing.T) {
	repo := NewTokenRepositoryMemory()
	testTokenRepository(t, repo)
}

func testTokenRepository(t *testing.T, repo TokenRepository) {
	// Create a test token
	token := &Token{
		ID:          "test-token-1",
		UserID:      "user-123",
		TokenHash:   "hash123",
		Name:        "Test Token",
		IsActive:    true,
		UsageCount:  0,
		CreatedFrom: "test",
	}

	// Test CreateToken
	err := repo.CreateToken(token)
	require.NoError(t, err)

	// Test FindTokenByHash
	foundToken, err := repo.FindTokenByHash("hash123")
	require.NoError(t, err)
	assert.Equal(t, token.ID, foundToken.ID)
	assert.Equal(t, token.UserID, foundToken.UserID)
	assert.Equal(t, token.Name, foundToken.Name)

	// Test FindTokensByUserID
	tokens, err := repo.FindTokensByUserID("user-123")
	require.NoError(t, err)
	assert.Len(t, tokens, 1)
	assert.Equal(t, token.ID, tokens[0].ID)

	// Test IncrementUsageCount
	now := time.Now()
	err = repo.IncrementUsageCount(token.ID, now)
	require.NoError(t, err)

	updatedToken, err := repo.FindTokenByHash("hash123")
	require.NoError(t, err)
	assert.Equal(t, int64(1), updatedToken.UsageCount)
	assert.NotNil(t, updatedToken.LastUsedAt)

	// Test RevokeToken
	err = repo.RevokeToken(token.ID, "user-123")
	require.NoError(t, err)

	// Verify token is revoked
	revokedToken, err := repo.GetTokenByID(token.ID)
	require.NoError(t, err)
	assert.False(t, revokedToken.IsActive)
	assert.NotNil(t, revokedToken.RevokedAt)
	assert.Equal(t, "user-123", revokedToken.RevokedBy)

}

func TestTokenRepositoryUserIsolation(t *testing.T) {
	repo := NewTokenRepositoryMemory()

	// Create tokens for different users
	user1Token := &Token{
		ID:          "token-user1",
		UserID:      "user-1",
		TokenHash:   "hash-user1",
		Name:        "User 1 Token",
		IsActive:    true,
		UsageCount:  0,
		CreatedFrom: "test",
	}

	user2Token := &Token{
		ID:          "token-user2",
		UserID:      "user-2",
		TokenHash:   "hash-user2",
		Name:        "User 2 Token",
		IsActive:    true,
		UsageCount:  0,
		CreatedFrom: "test",
	}

	// Create tokens
	err := repo.CreateToken(user1Token)
	require.NoError(t, err)

	err = repo.CreateToken(user2Token)
	require.NoError(t, err)

	// Test that each user only sees their own tokens
	user1Tokens, err := repo.FindTokensByUserID("user-1")
	require.NoError(t, err)
	assert.Len(t, user1Tokens, 1)
	assert.Equal(t, "token-user1", user1Tokens[0].ID)
	assert.Equal(t, "user-1", user1Tokens[0].UserID)

	user2Tokens, err := repo.FindTokensByUserID("user-2")
	require.NoError(t, err)
	assert.Len(t, user2Tokens, 1)
	assert.Equal(t, "token-user2", user2Tokens[0].ID)
	assert.Equal(t, "user-2", user2Tokens[0].UserID)

}

func TestTokenRepositoryMemory_ExpirationHandling(t *testing.T) {
	repo := NewTokenRepositoryMemory()

	// Create expired token
	expiredTime := time.Now().Add(-1 * time.Hour)
	expiredToken := &Token{
		ID:        "expired-token",
		UserID:    "user-123",
		TokenHash: "expired-hash",
		Name:      "Expired Token",
		IsActive:  true,
		ExpiresAt: &expiredTime,
	}

	err := repo.CreateToken(expiredToken)
	require.NoError(t, err)

	// Create active token
	futureTime := time.Now().Add(1 * time.Hour)
	activeToken := &Token{
		ID:        "active-token",
		UserID:    "user-123",
		TokenHash: "active-hash",
		Name:      "Active Token",
		IsActive:  true,
		ExpiresAt: &futureTime,
	}

	err = repo.CreateToken(activeToken)
	require.NoError(t, err)

	// Verify that both tokens still exist (expired tokens are kept)
	expiredToken, err = repo.FindTokenByHash("expired-hash")
	require.NoError(t, err)
	assert.Equal(t, "expired-token", expiredToken.ID)

	activeToken, err = repo.FindTokenByHash("active-hash")
	require.NoError(t, err)
	assert.Equal(t, "active-token", activeToken.ID)
}
