package session

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"time"

	"github.com/jmaister/taronja-gateway/db"
)

// contextKey is a custom type for context keys to avoid collisions.
type contextKey string

// SessionKey is the key used to store session in context.
const SessionKey contextKey = "session"

const TokenLength = 32
const SessionCookieName = "tg_session_token"

// SessionStore defines the interface for session validation and retrieval, decoupled from persistence.
type SessionStore interface {
	ValidateSession(r *http.Request) (*db.Session, bool)
	NewSession(r *http.Request, user *db.User, provider string, validityDuration time.Duration) (*db.Session, error)
	EndSession(token string) error
	FindSessionsByUserID(userID string) ([]db.Session, error)
}

// SessionStoreDB implements SessionStore and uses a SessionRepository to access session data.
type SessionStoreDB struct {
	Repo db.SessionRepository
}

// NewSessionStoreDB creates a new SessionStoreDB instance with the provided session repository.
func NewSessionStoreDB(repo db.SessionRepository) *SessionStoreDB {
	return &SessionStoreDB{
		Repo: repo,
	}
}

// ValidateSession checks if a session is valid based on the request's cookie.
// It delegates to the repository for session retrieval and update.
func (s *SessionStoreDB) ValidateSession(r *http.Request) (*db.Session, bool) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return nil, false // No cookie
	}
	sessionData, err := s.Repo.FindSessionByToken(cookie.Value)
	if err != nil || sessionData == nil {
		return nil, false
	}
	now := time.Now()
	if sessionData.ValidUntil.Before(now) {
		_ = s.Repo.CloseSession(sessionData.Token)
		return nil, false
	}
	sessionData.LastActivity = now
	_ = s.Repo.UpdateSession(sessionData)
	return sessionData, true
}

// GenerateToken generates a new random token.
func GenerateToken() (string, error) {
	tokenBytes := make([]byte, TokenLength) // Changed to session.TokenLength
	_, err := rand.Read(tokenBytes)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(tokenBytes), nil
}

// NewSession creates a new session with the given user and provider.
// It initializes the session with the current time and a validity duration.
func (s *SessionStoreDB) NewSession(r *http.Request, user *db.User, provider string, validityDuration time.Duration) (*db.Session, error) {
	token, err := GenerateToken()
	if err != nil {
		return nil, err
	}
	newSession := &db.Session{
		Token:           token,
		UserID:          user.ID,
		Username:        user.Username,
		Email:           user.Email,
		IsAuthenticated: true,
		ValidUntil:      time.Now().Add(validityDuration),
		Provider:        provider,
	}

	// Extract client information from the request
	if r != nil {
		PopulateClientInfo(r, newSession)
	}
	err = s.Repo.CreateSession(token, newSession)
	if err != nil {
		return nil, err
	}
	newSession.Token = token
	return newSession, nil
}

func (s *SessionStoreDB) EndSession(token string) error {
	return s.Repo.CloseSession(token)
}

func (s *SessionStoreDB) FindSessionsByUserID(userID string) ([]db.Session, error) {
	sessions, err := s.Repo.GetSessionsByUserID(userID)
	if err != nil {
		return nil, err
	}
	return sessions, nil
}
