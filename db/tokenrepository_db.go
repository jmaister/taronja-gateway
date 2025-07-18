package db

import (
	"time"

	"gorm.io/gorm"
)

// TokenRepositoryDB is a database implementation of TokenRepository
type TokenRepositoryDB struct {
	db *gorm.DB
}

// NewTokenRepositoryDB creates a new database token repository
func NewTokenRepositoryDB(db *gorm.DB) *TokenRepositoryDB {
	return &TokenRepositoryDB{db: db}
}

// CreateToken creates a new token in the database
func (r *TokenRepositoryDB) CreateToken(token *Token) error {
	return r.db.Create(token).Error
}

// GetTokenByID retrieves a token by its ID
func (r *TokenRepositoryDB) GetTokenByID(tokenID string) (*Token, error) {
	var token Token
	err := r.db.Where("id = ?", tokenID).First(&token).Error
	if err != nil {
		return nil, err
	}
	return &token, nil
}

// FindTokenByHash finds a token by its hash
func (r *TokenRepositoryDB) FindTokenByHash(tokenHash string) (*Token, error) {
	var token Token
	err := r.db.Where("token_hash = ?", tokenHash).First(&token).Error
	if err != nil {
		return nil, err
	}
	return &token, nil
}

// FindTokensByUserID finds all tokens for a specific user
func (r *TokenRepositoryDB) FindTokensByUserID(userID string) ([]*Token, error) {
	var tokens []*Token
	err := r.db.Where("user_id = ?", userID).Find(&tokens).Error
	return tokens, err
}

// ExpireToken marks a token as expired when accessed after expiration date
func (r *TokenRepositoryDB) ExpireToken(tokenID string) error {
	return r.db.Model(&Token{}).Where("id = ?", tokenID).Update("is_active", false).Error
}

// IncrementUsageCount increments the usage count and updates last used time
func (r *TokenRepositoryDB) IncrementUsageCount(tokenID string, lastUsedAt time.Time) error {
	return r.db.Model(&Token{}).Where("id = ?", tokenID).Updates(map[string]interface{}{
		"usage_count":  gorm.Expr("usage_count + 1"),
		"last_used_at": lastUsedAt,
	}).Error
}

// GetActiveTokensByUserID finds all active tokens for a specific user
func (r *TokenRepositoryDB) GetActiveTokensByUserID(userID string) ([]*Token, error) {
	var tokens []*Token
	err := r.db.Where("user_id = ? AND is_active = ?", userID, true).Find(&tokens).Error
	return tokens, err
}
