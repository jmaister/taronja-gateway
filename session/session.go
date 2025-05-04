package session

import (
	"context"
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

type SessionStore interface {
	GenerateKey() (string, error)
	Set(key string, value SessionObject) error
	Get(key string) (SessionObject, error)
	Validate(r *http.Request) (SessionObject, bool)
}

type SessionObject struct {
	UserId          string
	Username        string
	Email           string
	IsAuthenticated bool
	ValidUntil      time.Time
	Provider        string
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
	return value, nil

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
	if sessionObject.ValidUntil.Before(time.Now()) {
		delete(s.store, cookie.Value)
		return SessionObject{}, false
	}
	return sessionObject, true
}

// SessionKey is the key used to store session in context
const SessionKey contextKey = "session"

// SessionMiddleware validates that the session cookie is present and valid
func SessionMiddleware(next http.HandlerFunc, store SessionStore, isStatic bool, managementPrefix string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionObject, exists := store.Validate(r)
		if !exists {
			if isStatic {
				// Redirect to login page for static files
				http.Redirect(w, r, managementPrefix+"/login", http.StatusFound)
			} else {
				// Return 401 Unauthorized for API requests
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
			}
			return
		}
		ctx := r.Context()
		ctx = context.WithValue(ctx, SessionKey, sessionObject)
		next.ServeHTTP(w, r.WithContext(ctx))
	}

}
