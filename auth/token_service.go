package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/lucsky/cuid"
)

const (
	TokenLength = 32    // Length of the random token in bytes
	TokenPrefix = "tg_" // Prefix for tokens to identify them
)

// TokenService handles token generation, validation and management
type TokenService struct {
	tokenRepo db.TokenRepository
	userRepo  db.UserRepository
}

// NewTokenService creates a new token service
func NewTokenService(tokenRepo db.TokenRepository, userRepo db.UserRepository) *TokenService {
	return &TokenService{
		tokenRepo: tokenRepo,
		userRepo:  userRepo,
	}
}

// GenerateToken creates a new token for a user
func (s *TokenService) GenerateToken(userID, name string, expiresAt *time.Time, scopes []string, createdFrom string, clientInfo *db.ClientInfo) (string, *db.Token, error) {
	// Validate user exists
	user, err := s.userRepo.FindUserByIdOrUsername(userID, "", "")
	if err != nil {
		return "", nil, fmt.Errorf("user not found: %w", err)
	}

	// Generate random token
	randomBytes := make([]byte, TokenLength)
	_, err = rand.Read(randomBytes)
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate random token: %w", err)
	}

	// Create the actual token string with prefix
	tokenString := TokenPrefix + base64.URLEncoding.EncodeToString(randomBytes)

	// Hash the token for storage
	hasher := sha256.New()
	hasher.Write([]byte(tokenString))
	tokenHash := fmt.Sprintf("%x", hasher.Sum(nil))

	// Create token record
	tokenID, err := cuid.NewCrypto(rand.Reader)
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate token ID: %w", err)
	}

	token := &db.Token{
		ID:          tokenID,
		UserID:      user.ID,
		TokenHash:   tokenHash,
		Name:        name,
		IsActive:    true,
		ExpiresAt:   expiresAt,
		UsageCount:  0,
		CreatedFrom: createdFrom,
	}

	// Set scopes if provided
	if len(scopes) > 0 {
		// In a real implementation, you might want to use JSON marshaling
		token.Scopes = fmt.Sprintf("[%s]", joinStrings(scopes, ","))
	}

	// Set client info if provided
	if clientInfo != nil {
		token.ClientInfo = *clientInfo
	}

	// Save token to repository
	err = s.tokenRepo.CreateToken(token)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create token: %w", err)
	}

	return tokenString, token, nil
}

// ValidateToken validates a token and returns the associated user and token info
func (s *TokenService) ValidateToken(tokenString string) (*db.User, *db.Token, error) {
	// Check if token has the correct prefix
	if len(tokenString) < len(TokenPrefix) || tokenString[:len(TokenPrefix)] != TokenPrefix {
		return nil, nil, fmt.Errorf("invalid token format")
	}

	// Hash the token for lookup
	hasher := sha256.New()
	hasher.Write([]byte(tokenString))
	tokenHash := fmt.Sprintf("%x", hasher.Sum(nil))

	// Find token in repository
	token, err := s.tokenRepo.FindTokenByHash(tokenHash)
	if err != nil {
		return nil, nil, fmt.Errorf("token not found: %w", err)
	}

	// Check if token is active
	if !token.IsActive {
		return nil, nil, fmt.Errorf("token is deactivated")
	}

	// Check if token is expired and expire it if so
	if token.ExpiresAt != nil && token.ExpiresAt.Before(time.Now()) {
		// Expire the token when accessed after expiration
		err := s.tokenRepo.ExpireToken(token.ID)
		if err != nil {
			fmt.Printf("Warning: failed to expire token: %v\n", err)
		}
		return nil, nil, fmt.Errorf("token is expired")
	}

	// Get user associated with token
	user, err := s.userRepo.FindUserByIdOrUsername(token.UserID, "", "")
	if err != nil {
		return nil, nil, fmt.Errorf("user associated with token not found: %w", err)
	}

	// Update usage count
	err = s.tokenRepo.IncrementUsageCount(token.ID, time.Now())
	if err != nil {
		// Log error but don't fail the validation
		// In a production system, you might want to log this properly
		fmt.Printf("Warning: failed to increment token usage count: %v\n", err)
	} else {
		// Reload token to get updated usage count for return value
		updatedToken, err := s.tokenRepo.GetTokenByID(token.ID)
		if err == nil {
			token = updatedToken
		}
	}

	return user, token, nil
}

// GetUserTokens returns all tokens for a user
func (s *TokenService) GetUserTokens(userID string) ([]*db.Token, error) {
	return s.tokenRepo.FindTokensByUserID(userID)
}

// RevokeToken revokes a user's token, ensuring only the owner can revoke their own tokens
func (s *TokenService) RevokeToken(tokenID string, userID string, revokedBy string) error {
	// First, verify the token exists and belongs to the user
	token, err := s.tokenRepo.GetTokenByID(tokenID)
	if err != nil {
		return fmt.Errorf("token not found: %w", err)
	}

	if token.UserID != userID {
		return fmt.Errorf("token does not belong to user")
	}

	if token.RevokedAt != nil {
		return fmt.Errorf("token is already revoked")
	}

	if !token.IsActive {
		return fmt.Errorf("token is already inactive")
	}

	// Revoke the token
	return s.tokenRepo.RevokeToken(tokenID, revokedBy)
}

// Helper function to join strings (simple implementation)
func joinStrings(strings []string, separator string) string {
	if len(strings) == 0 {
		return ""
	}

	result := strings[0]
	for i := 1; i < len(strings); i++ {
		result += separator + strings[i]
	}
	return result
}
