package db

import (
	"net/http"
	"sync"
	"time"
)

type memorySessionRepo struct {
	store map[string]*Session
	lock  sync.RWMutex
}

// NewMemorySessionRepository returns a new in-memory SessionRepository (for tests)
func NewMemorySessionRepository() SessionRepository {
	return &memorySessionRepo{
		store: make(map[string]*Session),
	}
}

func (m *memorySessionRepo) GenerateToken() (string, error) {
	return generateRandomToken(), nil
}

func (m *memorySessionRepo) CreateSession(token string, session *Session) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	session.Token = token
	m.store[token] = session
	return nil
}

func (m *memorySessionRepo) GetSessionByToken(token string) (*Session, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()
	s, ok := m.store[token]
	if !ok {
		return nil, nil
	}
	if s.ClosedOn != nil {
		return nil, ErrSessionClosed
	}
	return s, nil
}

func (m *memorySessionRepo) UpdateSession(session *Session) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.store[session.Token] = session
	return nil
}

func (m *memorySessionRepo) DeleteSession(token string) error {
	return m.CloseSession(token)
}

func (m *memorySessionRepo) ValidateSession(r *http.Request) (*Session, bool) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return nil, false
	}
	s, err := m.GetSessionByToken(cookie.Value)
	if err != nil || s == nil {
		return nil, false
	}
	if s.ValidUntil.Before(time.Now()) {
		_ = m.CloseSession(s.Token)
		return nil, false
	}
	s.LastActivity = time.Now()
	_ = m.UpdateSession(s)
	return s, true
}

func (m *memorySessionRepo) GetSessionsByUserID(userID string) ([]Session, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()
	var sessions []Session
	now := time.Now()
	for _, s := range m.store {
		if s.UserID == userID && s.ClosedOn == nil && s.ValidUntil.After(now) {
			sessions = append(sessions, *s)
		}
	}
	return sessions, nil
}

func (m *memorySessionRepo) CloseSession(token string) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	s, ok := m.store[token]
	if !ok {
		return ErrSessionNotFound
	}
	if s.ClosedOn != nil {
		return ErrSessionClosed
	}
	now := time.Now()
	s.ClosedOn = &now
	return nil
}

// Helpers and errors
var ErrSessionClosed = &SessionError{"session has been closed"}
var ErrSessionNotFound = &SessionError{"session not found"}

type SessionError struct{ msg string }

func (e *SessionError) Error() string { return e.msg }

func generateRandomToken() string {
	b := make([]byte, 32)
	for i := range b {
		b[i] = byte(65 + i%26)
	}
	return string(b)
}
