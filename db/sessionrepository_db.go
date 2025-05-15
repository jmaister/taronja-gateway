package db

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"time"

	"gorm.io/gorm"
)

// SessionStoreDB implements the SessionRepository interface using a database.
type SessionStoreDB struct {
	dbConn *gorm.DB
}

// NewSessionRepositoryDB creates a new SessionStoreDB.
// It now returns SessionRepository directly.
func NewSessionRepositoryDB() SessionRepository {
	return &SessionStoreDB{
		dbConn: GetConnection(),
	}
}

// GenerateToken generates a new random token.
func (s *SessionStoreDB) GenerateToken() (string, error) {
	tokenBytes := make([]byte, TokenLength) // Changed to session.TokenLength
	_, err := rand.Read(tokenBytes)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(tokenBytes), nil
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
	if sessionData.CreatedAt.IsZero() {
		sessionData.CreatedAt = time.Now()
	}
	if sessionData.UpdatedAt.IsZero() {
		sessionData.UpdatedAt = time.Now()
	}

	result := s.dbConn.Create(sessionData)
	return result.Error
}

// GetSessionByToken retrieves a session by its token.
// Returns nil, nil if not found. Returns error for closed sessions.
func (s *SessionStoreDB) GetSessionByToken(token string) (*Session, error) {
	var sessionData Session // Changed type to db.Session
	result := s.dbConn.Where("token = ?", token).First(&sessionData)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil // Not found
		}
		return nil, result.Error // Other DB error
	}

	// Check if the session has been closed
	if sessionData.ClosedOn != nil && !sessionData.ClosedOn.IsZero() {
		return nil, errors.New("session has been closed")
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

// ValidateSession checks if a session is valid based on the request's cookie.
// Updates LastActivity and handles expiration.
func (s *SessionStoreDB) ValidateSession(r *http.Request) (*Session, bool) {
	cookie, err := r.Cookie(SessionCookieName) // Changed to session.SessionCookieName
	if err != nil {
		return nil, false // No cookie
	}

	sessionData, err := s.GetSessionByToken(cookie.Value) // Changed type to db.Session
	if err != nil || sessionData == nil {
		// Error retrieving session (e.g. DB error, closed session) or session not found
		return nil, false
	}

	now := time.Now()
	if sessionData.ValidUntil.Before(now) {
		// Session expired, mark it as closed
		_ = s.CloseSession(sessionData.Token) // Attempt to close, ignore error for validation purposes
		return nil, false
	}

	// Update last activity time
	sessionData.LastActivity = now
	// Note: ExtractClientInfo could be called here if more dynamic client info updates are needed.
	// For now, only LastActivity is updated by ValidateSession itself.
	// Other fields like IPAddress, UserAgent are typically set once on creation via ExtractClientInfo.

	if err := s.UpdateSession(sessionData); err != nil {
		// Log error, but for this flow, consider session valid if main checks passed.
		// log.Printf("Error updating session during validation: %v", err)
	}

	return sessionData, true
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
	now := time.Now()
	result := s.dbConn.Model(&Session{}).Where("token = ? AND closed_on IS NULL", token).Update("closed_on", now) // Changed type to db.Session
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		// Could be because token not found, or already closed.
		// Check if it exists at all to differentiate.
		var tempSession Session // Changed type to db.Session
		err := s.dbConn.Where("token = ?", token).First(&tempSession).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("session not found")
		}
		return errors.New("session already closed or not found")
	}
	return nil
}
