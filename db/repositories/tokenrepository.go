package repositories

import (
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"gorm.io/gorm"
)

// TokenRepository defines the interface for token persistence and operations.
type TokenRepository interface {
	CreateToken(token *db.Token) error
	GetTokenByID(tokenID string) (*db.Token, error)
	FindTokenByHash(tokenHash string) (*db.Token, error)
	FindTokensByUserID(userID string) ([]*db.Token, error)
	IncrementUsageCount(tokenID string, lastUsedAt time.Time) error
	ExpireToken(tokenID string) error                   // Mark token as expired when accessed after expiration
	RevokeToken(tokenID string, revokedBy string) error // Revoke a token
}

// TokenRepositoryDB is a database implementation of TokenRepository
type TokenRepositoryDB struct {
	db *gorm.DB
}

// NewTokenRepositoryDB creates a new database token repository
func NewTokenRepositoryDB(db *gorm.DB) *TokenRepositoryDB {
	return &TokenRepositoryDB{db: db}
}

// CreateToken creates a new token in the database
func (r *TokenRepositoryDB) CreateToken(token *db.Token) error {
	return r.db.Create(token).Error
}

// GetTokenByID retrieves a token by its ID
func (r *TokenRepositoryDB) GetTokenByID(tokenID string) (*db.Token, error) {
	var token db.Token
	err := r.db.Where("id = ?", tokenID).First(&token).Error
	if err != nil {
		return nil, err
	}
	return &token, nil
}

// FindTokenByHash finds a token by its hash
func (r *TokenRepositoryDB) FindTokenByHash(tokenHash string) (*db.Token, error) {
	var token db.Token
	err := r.db.Where("token_hash = ?", tokenHash).First(&token).Error
	if err != nil {
		return nil, err
	}
	return &token, nil
}

// FindTokensByUserID finds all tokens for a specific user
func (r *TokenRepositoryDB) FindTokensByUserID(userID string) ([]*db.Token, error) {
	var tokens []*db.Token
	err := r.db.Where("user_id = ?", userID).Find(&tokens).Error
	return tokens, err
}

// ExpireToken marks a token as expired when accessed after expiration date
func (r *TokenRepositoryDB) ExpireToken(tokenID string) error {
	return r.db.Model(&db.Token{}).Where("id = ?", tokenID).Update("is_active", false).Error
}

// IncrementUsageCount increments the usage count and updates last used time.
// .UTC(): this is a raw map-based Updates against an empty &Token{} model,
// so Token.BeforeSave never sees this value — normalize the caller-supplied
// lastUsedAt here instead, to match this schema's UTC convention.
func (r *TokenRepositoryDB) IncrementUsageCount(tokenID string, lastUsedAt time.Time) error {
	return r.db.Model(&db.Token{}).Where("id = ?", tokenID).Updates(map[string]interface{}{
		"usage_count":  gorm.Expr("usage_count + 1"),
		"last_used_at": lastUsedAt.UTC(),
	}).Error
}

// RevokeToken marks a token as revoked
func (r *TokenRepositoryDB) RevokeToken(tokenID string, revokedBy string) error {
	// .UTC(): same reasoning as IncrementUsageCount above — a raw map-based
	// Updates that Token.BeforeSave never gets a chance to normalize.
	now := time.Now().UTC()
	return r.db.Model(&db.Token{}).Where("id = ?", tokenID).Updates(map[string]interface{}{
		"is_active":  false,
		"revoked_at": now,
		"revoked_by": revokedBy,
	}).Error
}
