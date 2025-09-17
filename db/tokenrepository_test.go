package db

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenRepository(t *testing.T) {
	// Create test database with unique name for isolation
	testName := fmt.Sprintf("tokenrepository_test_%d", time.Now().UnixNano())
	SetupTestDB(testName)
	testDB := GetConnection()

	repo := NewTokenRepositoryDB(testDB)
	testTokenRepository(t, repo)
}

func testTokenRepository(t *testing.T, repo TokenRepository) {
	// Create a test token (ID will be auto-generated)
	token := &Token{
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
	require.NotEmpty(t, token.ID) // ID should be auto-generated

	// Test FindTokenByHash
	foundToken, err := repo.FindTokenByHash("hash123")
	require.NoError(t, err)
	assert.Equal(t, token.ID, foundToken.ID) // Use the auto-generated ID
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
	// Create test database with unique name for isolation
	testName := fmt.Sprintf("tokenrepository_user_isolation_test_%d", time.Now().UnixNano())
	SetupTestDB(testName)
	testDB := GetConnection()
	repo := NewTokenRepositoryDB(testDB)

	// Create tokens for different users (IDs will be auto-generated)
	user1Token := &Token{
		UserID:      "user-1",
		TokenHash:   "hash-user1",
		Name:        "User 1 Token",
		IsActive:    true,
		UsageCount:  0,
		CreatedFrom: "test",
	}

	user2Token := &Token{
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
	require.NotEmpty(t, user1Token.ID) // ID should be auto-generated

	err = repo.CreateToken(user2Token)
	require.NoError(t, err)
	require.NotEmpty(t, user2Token.ID) // ID should be auto-generated

	// Test that each user only sees their own tokens
	user1Tokens, err := repo.FindTokensByUserID("user-1")
	require.NoError(t, err)
	assert.Len(t, user1Tokens, 1)
	assert.Equal(t, user1Token.ID, user1Tokens[0].ID) // Use auto-generated ID
	assert.Equal(t, "user-1", user1Tokens[0].UserID)

	user2Tokens, err := repo.FindTokensByUserID("user-2")
	require.NoError(t, err)
	assert.Len(t, user2Tokens, 1)
	assert.Equal(t, user2Token.ID, user2Tokens[0].ID) // Use auto-generated ID
	assert.Equal(t, "user-2", user2Tokens[0].UserID)

}

func TestTokenRepository_ExpirationHandling(t *testing.T) {
	// Create test database with unique name for isolation
	testName := fmt.Sprintf("tokenrepository_expiration_test_%d", time.Now().UnixNano())
	SetupTestDB(testName)
	testDB := GetConnection()
	repo := NewTokenRepositoryDB(testDB)

	// Create expired token (ID will be auto-generated)
	expiredTime := time.Now().Add(-1 * time.Hour)
	expiredToken := &Token{
		UserID:    "user-123",
		TokenHash: "expired-hash",
		Name:      "Expired Token",
		IsActive:  true,
		ExpiresAt: &expiredTime,
	}

	err := repo.CreateToken(expiredToken)
	require.NoError(t, err)
	require.NotEmpty(t, expiredToken.ID) // ID should be auto-generated

	// Create active token (ID will be auto-generated)
	futureTime := time.Now().Add(1 * time.Hour)
	activeToken := &Token{
		UserID:    "user-123",
		TokenHash: "active-hash",
		Name:      "Active Token",
		IsActive:  true,
		ExpiresAt: &futureTime,
	}

	err = repo.CreateToken(activeToken)
	require.NoError(t, err)
	require.NotEmpty(t, activeToken.ID) // ID should be auto-generated

	// Verify that both tokens still exist (expired tokens are kept)
	foundExpiredToken, err := repo.FindTokenByHash("expired-hash")
	require.NoError(t, err)
	assert.Equal(t, expiredToken.ID, foundExpiredToken.ID) // Use auto-generated ID

	foundActiveToken, err := repo.FindTokenByHash("active-hash")
	require.NoError(t, err)
	assert.Equal(t, activeToken.ID, foundActiveToken.ID) // Use auto-generated ID
}
