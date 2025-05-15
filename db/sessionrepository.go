package db

import (
	"net/http"
	"time"
)

// TODO: rename, find better place for this
const TokenLength = 32
const SessionCookieName = "tg_session_token"

// SessionRepository defines the interface for session persistence and operations.
type SessionRepository interface {
	GenerateToken() (string, error)
	CreateSession(token string, session *Session) error // Changed from CreateSession(session *Session) to include token
	GetSessionByToken(token string) (*Session, error)
	UpdateSession(session *Session) error
	DeleteSession(token string) error
	ValidateSession(r *http.Request) (*Session, bool) // This will internally use GetSessionByToken and UpdateSession
	GetSessionsByUserID(userID string) ([]Session, error)
	// CloseSession marks a session as closed without deleting it.
	CloseSession(token string) error
}

// NewSession is a constructor for the Session struct.
// It initializes a session with user details, provider, and validity duration.
// Other fields like Token, client information, and GORM-managed fields
// are expected to be set by other parts of the system.
func NewSession(user *User, provider string, validityDuration time.Duration) *Session {
	return &Session{
		UserID:          user.ID,
		Username:        user.Username,
		Email:           user.Email,
		IsAuthenticated: true,
		ValidUntil:      time.Now().Add(validityDuration),
		Provider:        provider,
	}
}
