package session

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"time"
)

const TokenLength = 32
const SessionCookieName = "tg_session_token"

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

// SessionKey is the key used to store session in context
const SessionKey contextKey = "session"

type SessionStore interface {
	GenerateKey() (string, error)
	Set(key string, value SessionObject) error
	Get(key string) (SessionObject, error)
	Delete(key string) error
	Validate(r *http.Request) (SessionObject, bool)
	GetSessionsByUserID(userID string) ([]SessionObject, error)
}

type SessionObject struct {
	UserID          string
	Username        string
	Email           string
	IsAuthenticated bool
	ValidUntil      time.Time
	Provider        string
	ClosedOn        *time.Time
}

type MemorySessionStore struct {
	store map[string]SessionObject
}

func NewMemorySessionStore() SessionStore {
	store := make(map[string]SessionObject, 10)
	return &MemorySessionStore{
		store: store,
	}
}

func (s MemorySessionStore) GenerateKey() (string, error) {
	tokenBytes := make([]byte, TokenLength)
	_, err := rand.Read(tokenBytes)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(tokenBytes), nil
}

func (s MemorySessionStore) Set(key string, value SessionObject) error {
	s.store[key] = value
	return nil
}

func (s MemorySessionStore) Get(key string) (SessionObject, error) {
	value, found := s.store[key]
	if !found {
		return SessionObject{}, errors.New("not found")
	}

	// Check if session is closed
	if value.ClosedOn != nil && !value.ClosedOn.IsZero() {
		return SessionObject{}, errors.New("session has been closed")
	}

	return value, nil
}

func (s MemorySessionStore) Delete(key string) error {
	session, exists := s.store[key]
	if exists {
		now := time.Now()
		session.ClosedOn = &now
		s.store[key] = session
	}
	return nil
}

func (s MemorySessionStore) Validate(r *http.Request) (SessionObject, bool) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return SessionObject{}, false
	}

	sessionObject, exists := s.store[cookie.Value]
	if !exists {
		return SessionObject{}, false
	}

	now := time.Now()
	// Check if session is expired or closed
	if sessionObject.ValidUntil.Before(now) || (sessionObject.ClosedOn != nil && !sessionObject.ClosedOn.IsZero()) {
		if sessionObject.ClosedOn == nil {
			// Mark as closed if not already
			sessionObject.ClosedOn = &now
			s.store[cookie.Value] = sessionObject
		}
		return SessionObject{}, false
	}

	return sessionObject, true
}

// GetSessionsByUserID retrieves all active sessions for a specific user from memory store
func (s MemorySessionStore) GetSessionsByUserID(userID string) ([]SessionObject, error) {
	var sessions []SessionObject

	for _, session := range s.store {
		// Only include sessions that belong to this user and haven't been closed
		if session.UserID == userID && (session.ClosedOn == nil || session.ClosedOn.IsZero()) {
			sessions = append(sessions, session)
		}
	}

	return sessions, nil
}
