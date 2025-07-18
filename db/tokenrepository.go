package db

import "time"

// TokenRepository defines the interface for token persistence and operations.
type TokenRepository interface {
	CreateToken(token *Token) error
	GetTokenByID(tokenID string) (*Token, error)
	FindTokenByHash(tokenHash string) (*Token, error)
	FindTokensByUserID(userID string) ([]*Token, error)
	IncrementUsageCount(tokenID string, lastUsedAt time.Time) error
	GetActiveTokensByUserID(userID string) ([]*Token, error)
	ExpireToken(tokenID string) error // Mark token as expired when accessed after expiration
}
