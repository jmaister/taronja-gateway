package db

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// SessionRepository defines the interface for session persistence and operations.
type SessionRepository interface {
	CreateSession(token string, session *Session) error // Changed from CreateSession(session *Session) to include token
	FindSessionByToken(token string) (*Session, error)
	UpdateSession(session *Session) error
	GetSessionsByUserID(userID string) ([]Session, error)
	CloseSession(token string) error
}

// SessionStoreDB implements the SessionRepository interface using a database.
type SessionStoreDB struct {
	dbConn *gorm.DB
}

// NewSessionRepositoryDB creates a new SessionStoreDB with a specific database connection.
// This is useful for testing with isolated database instances.
func NewSessionRepositoryDB(db *gorm.DB) SessionRepository {
	return &SessionStoreDB{
		dbConn: db,
	}
}

// CreateSession creates a new session in the database.
func (s *SessionStoreDB) CreateSession(token string, sessionData *Session) error {
	if sessionData == nil {
		return errors.New("session data cannot be nil")
	}
	// Ensure the token is set on the sessionData
	sessionData.Token = token
	// Initialize GORM Model fields if they are zero, GORM might do this automatically
	// but being explicit can avoid issues if not using AutoCreate/AutoUpdate time.
	// .UTC() to match this schema's UTC convention (db.utcNowFunc would set
	// these to the same thing via GORM's own clock, but only if they're
	// still zero by the time Create's callbacks run — this pre-set already
	// wins, so it has to get UTC right itself).
	if sessionData.CreatedAt.IsZero() {
		sessionData.CreatedAt = time.Now().UTC()
	}
	if sessionData.UpdatedAt.IsZero() {
		sessionData.UpdatedAt = time.Now().UTC()
	}

	result := s.dbConn.Create(sessionData)
	return result.Error
}

// FindSessionByToken retrieves a session by its token.
// Returns nil, nil if not found. Returns error for closed sessions.
func (s *SessionStoreDB) FindSessionByToken(token string) (*Session, error) {
	var sessionData Session // Changed type to db.Session
	result := s.dbConn.Where("token_hash = ?", hashSessionToken(token)).First(&sessionData)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil // Not found
		}
		return nil, result.Error // Other DB error
	}

	// Token itself is never persisted (see Session.Token's doc comment),
	// so GORM has nothing to populate it from — set it from the caller's
	// own already-known raw value instead. Every downstream consumer of a
	// *Session this returns (ValidateSession's own CloseSession-on-expiry
	// call, middleware/session.go's admin-denied auto-logout,
	// middleware/trafficmetric.go's recorded SessionID, X-User-Data's
	// "token" field) reads it expecting the real session token, not an
	// empty string.
	sessionData.Token = token

	// Check if the session has been closed
	if sessionData.ClosedOn != nil && !sessionData.ClosedOn.IsZero() {
		return nil, ErrSessionClosed
	}

	return &sessionData, nil
}

// UpdateSession updates an existing session in the database.
func (s *SessionStoreDB) UpdateSession(sessionData *Session) error {
	if sessionData == nil {
		return errors.New("session data cannot be nil")
	}
	result := s.dbConn.Save(sessionData)
	return result.Error
}

// DeleteSession marks a session as closed (soft delete).
func (s *SessionStoreDB) DeleteSession(token string) error {
	return s.CloseSession(token) // Delegate to CloseSession
}

// GetSessionsByUserID retrieves all sessions for a given user ID.
func (s *SessionStoreDB) GetSessionsByUserID(userID string) ([]Session, error) {
	var sessions []Session // Changed type to db.Session
	now := time.Now()
	result := s.dbConn.Where("user_id = ?", userID, now).Find(&sessions)
	if result.Error != nil {
		return nil, result.Error
	}
	return sessions, nil
}

// CloseSession marks a session as closed by setting its ClosedOn timestamp.
func (s *SessionStoreDB) CloseSession(token string) error {
	// .UTC(): this is a raw single-column Update against an empty
	// &Session{} model, so Session.BeforeSave never sees this value —
	// normalize it here instead, to match this schema's UTC convention.
	now := time.Now().UTC()
	tokenHash := hashSessionToken(token)
	result := s.dbConn.Model(&Session{}).Where("token_hash = ? AND closed_on IS NULL", tokenHash).Update("closed_on", now)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		// Could be because token not found, or already closed.
		// Check if it exists at all to differentiate.
		var tempSession Session // Changed type to db.Session
		err := s.dbConn.Where("token_hash = ?", tokenHash).First(&tempSession).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("session not found")
		}
		return errors.New("session already closed or not found")
	}
	return nil
}
