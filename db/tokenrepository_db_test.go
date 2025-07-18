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

	// Test GetActiveTokensByUserID
	activeTokens, err := repo.GetActiveTokensByUserID("user-123")
	require.NoError(t, err)
	assert.Len(t, activeTokens, 1)
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

	// Test GetActiveTokensByUserID includes all active tokens (even expired ones)
	// Tokens only expire when accessed, not when queried
	activeTokens, err := repo.GetActiveTokensByUserID("user-123")
	require.NoError(t, err)
	assert.Len(t, activeTokens, 2) // Both tokens are still active until accessed

	// Verify that both tokens still exist (expired tokens are kept)
	expiredToken, err = repo.FindTokenByHash("expired-hash")
	require.NoError(t, err)
	assert.Equal(t, "expired-token", expiredToken.ID)

	activeToken, err = repo.FindTokenByHash("active-hash")
	require.NoError(t, err)
	assert.Equal(t, "active-token", activeToken.ID)
}
