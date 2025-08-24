package db

import (
	"fmt"
	"sync"
	"time"
)

// TokenRepositoryMemory is a memory implementation of TokenRepository
type TokenRepositoryMemory struct {
	tokens map[string]*Token
	mutex  sync.RWMutex
}

// NewTokenRepositoryMemory creates a new in-memory token repository
func NewTokenRepositoryMemory() *TokenRepositoryMemory {
	return &TokenRepositoryMemory{
		tokens: make(map[string]*Token),
	}
}

// CreateToken creates a new token in memory
func (r *TokenRepositoryMemory) CreateToken(token *Token) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if token.ID == "" {
		return fmt.Errorf("token ID cannot be empty")
	}

	if _, exists := r.tokens[token.ID]; exists {
		return fmt.Errorf("token with ID %s already exists", token.ID)
	}

	// Create a copy to avoid external modifications
	tokenCopy := *token
	r.tokens[token.ID] = &tokenCopy
	return nil
}

// GetTokenByID retrieves a token by its ID
func (r *TokenRepositoryMemory) GetTokenByID(tokenID string) (*Token, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	token, exists := r.tokens[tokenID]
	if !exists {
		return nil, fmt.Errorf("token with ID %s not found", tokenID)
	}

	// Return a copy to avoid external modifications
	tokenCopy := *token
	return &tokenCopy, nil
}

// FindTokenByHash finds a token by its hash
func (r *TokenRepositoryMemory) FindTokenByHash(tokenHash string) (*Token, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	for _, token := range r.tokens {
		if token.TokenHash == tokenHash {
			// Return a copy to avoid external modifications
			tokenCopy := *token
			return &tokenCopy, nil
		}
	}
	return nil, fmt.Errorf("token not found")
}

// FindTokensByUserID finds all tokens for a specific user
func (r *TokenRepositoryMemory) FindTokensByUserID(userID string) ([]*Token, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	var tokens []*Token
	for _, token := range r.tokens {
		if token.UserID == userID {
			// Create a copy to avoid external modifications
			tokenCopy := *token
			tokens = append(tokens, &tokenCopy)
		}
	}
	return tokens, nil
}

// ExpireToken marks a token as expired when accessed after expiration date
func (r *TokenRepositoryMemory) ExpireToken(tokenID string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	token, exists := r.tokens[tokenID]
	if !exists {
		return fmt.Errorf("token with ID %s not found", tokenID)
	}

	token.IsActive = false
	return nil
}

// RevokeToken marks a token as revoked
func (r *TokenRepositoryMemory) RevokeToken(tokenID string, revokedBy string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	token, exists := r.tokens[tokenID]
	if !exists {
		return fmt.Errorf("token with ID %s not found", tokenID)
	}

	now := time.Now()
	token.IsActive = false
	token.RevokedAt = &now
	token.RevokedBy = revokedBy
	return nil
}

// IncrementUsageCount increments the usage count and updates last used time
func (r *TokenRepositoryMemory) IncrementUsageCount(tokenID string, lastUsedAt time.Time) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	token, exists := r.tokens[tokenID]
	if !exists {
		return fmt.Errorf("token with ID %s not found", tokenID)
	}

	token.UsageCount++
	token.LastUsedAt = &lastUsedAt
	return nil
}

// GetActiveTokensByUserID finds all active tokens for a specific user
func (r *TokenRepositoryMemory) GetActiveTokensByUserID(userID string) ([]*Token, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	var tokens []*Token
	for _, token := range r.tokens {
		if token.UserID == userID && token.IsActive {
			// Create a copy to avoid external modifications
			tokenCopy := *token
			tokens = append(tokens, &tokenCopy)
		}
	}
	return tokens, nil
}
