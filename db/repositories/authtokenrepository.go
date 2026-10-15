package repositories

import (
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"gorm.io/gorm"
)

// AuthTokenRepository stores one-time email tokens. Lookups of a missing
// token return gorm.ErrRecordNotFound.
type AuthTokenRepository interface {
	Create(token *db.AuthToken) error
	FindByTokenHash(tokenHash string) (*db.AuthToken, error)
	// FindActiveByBindingHash returns the newest unused, unexpired email login
	// token issued to the browser holding the binding cookie.
	FindActiveByBindingHash(bindingHash string, now time.Time) (*db.AuthToken, error)
	// LatestCreatedAt returns when the newest token for purpose and email was
	// issued, or the zero time if there is none.
	LatestCreatedAt(purpose, email string) (time.Time, error)
	// MarkUsed marks a token used. It returns false if it already was, so two
	// concurrent uses cannot both succeed.
	MarkUsed(id string, now time.Time) (bool, error)
	IncrementAttempts(id string) error
	SetCodeHash(id, codeHash string) error
	// InvalidateForUser marks every unused token of purpose for userID used, so
	// an older reset link stops working once a newer one is used.
	InvalidateForUser(userID, purpose string, now time.Time) error
}

// AuthTokenRepositoryDB is the GORM-backed implementation.
type AuthTokenRepositoryDB struct {
	db *gorm.DB
}

// NewAuthTokenRepositoryDB creates a new database-backed AuthTokenRepository.
func NewAuthTokenRepositoryDB(db *gorm.DB) *AuthTokenRepositoryDB {
	return &AuthTokenRepositoryDB{db: db}
}

func (r *AuthTokenRepositoryDB) Create(token *db.AuthToken) error {
	return r.db.Create(token).Error
}

func (r *AuthTokenRepositoryDB) FindByTokenHash(tokenHash string) (*db.AuthToken, error) {
	var token db.AuthToken
	err := r.db.Where("token_hash = ?", tokenHash).First(&token).Error
	if err != nil {
		return nil, err
	}
	return &token, nil
}

func (r *AuthTokenRepositoryDB) FindActiveByBindingHash(bindingHash string, now time.Time) (*db.AuthToken, error) {
	var token db.AuthToken
	err := r.db.Where("binding_hash = ? AND used_at IS NULL AND expires_at > ?", bindingHash, now.UTC()).
		Order("created_at DESC").First(&token).Error
	if err != nil {
		return nil, err
	}
	return &token, nil
}

func (r *AuthTokenRepositoryDB) LatestCreatedAt(purpose, email string) (time.Time, error) {
	var token db.AuthToken
	err := r.db.Where("purpose = ? AND email = ?", purpose, email).Order("created_at DESC").First(&token).Error
	if err == gorm.ErrRecordNotFound {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	return token.CreatedAt, nil
}

func (r *AuthTokenRepositoryDB) MarkUsed(id string, now time.Time) (bool, error) {
	result := r.db.Model(&db.AuthToken{}).Where("id = ? AND used_at IS NULL", id).Update("used_at", now.UTC())
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

func (r *AuthTokenRepositoryDB) IncrementAttempts(id string) error {
	return r.db.Model(&db.AuthToken{}).Where("id = ?", id).Update("attempts", gorm.Expr("attempts + 1")).Error
}

func (r *AuthTokenRepositoryDB) InvalidateForUser(userID, purpose string, now time.Time) error {
	return r.db.Model(&db.AuthToken{}).
		Where("user_id = ? AND purpose = ? AND used_at IS NULL", userID, purpose).
		Update("used_at", now.UTC()).Error
}

func (r *AuthTokenRepositoryDB) SetCodeHash(id, codeHash string) error {
	return r.db.Model(&db.AuthToken{}).Where("id = ?", id).Update("code_hash", codeHash).Error
}
