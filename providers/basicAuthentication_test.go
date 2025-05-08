package providers

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/session"
	"github.com/stretchr/testify/assert"
)

// MockSessionStore implements the session.SessionStore interface for testing
type MockSessionStore struct {
	sessions map[string]session.SessionObject
	keyCount int    // Add a counter to generate unique keys
	lastKey  string // Track the last generated key
	err      error
}

func NewMockSessionStore() *MockSessionStore {
	return &MockSessionStore{
		sessions: make(map[string]session.SessionObject),
		keyCount: 0,
	}
}

func (m *MockSessionStore) GenerateKey() (string, error) {
	if m.err != nil {
		return "", m.err
	}
	m.keyCount++
	m.lastKey = fmt.Sprintf("test-session-token-%d", m.keyCount)
	return m.lastKey, m.err
}

func (m *MockSessionStore) Set(key string, value session.SessionObject) error {
	m.sessions[key] = value
	return nil
}

func (m *MockSessionStore) Get(key string) (session.SessionObject, error) {
	value, found := m.sessions[key]
	if !found {
		return session.SessionObject{}, errors.New("not found")
	}
	return value, nil
}

func (m *MockSessionStore) Delete(key string) error {
	delete(m.sessions, key)
	return nil
}

func (m *MockSessionStore) Validate(r *http.Request) (session.SessionObject, bool) {
	cookie, err := r.Cookie(session.SessionCookieName)
	if err != nil {
		return session.SessionObject{}, false
	}
	session, err := m.Get(cookie.Value)
	if err != nil {
		return session, false
	}
	if session.ValidUntil.Before(time.Now()) {
		return session, false
	}
	return session, true
}

func TestRegisterBasicAuth(t *testing.T) {
	// Setup
	mux := http.NewServeMux()
	mockSessionStore := NewMockSessionStore()
	managementPrefix := "/_"

	// Register the basic auth handler
	RegisterBasicAuth(mux, mockSessionStore, managementPrefix)

	// Test scenarios
	t.Run("successful form authentication", func(t *testing.T) {
		// Create a request with valid form credentials
		formData := url.Values{
			"username": {"admin"},
			"password": {"password"},
		}
		req := httptest.NewRequest("POST", "/_/auth/basic/login", strings.NewReader(formData.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		// Dispatch the request to the mux
		mux.ServeHTTP(w, req)

		// Assert response is a redirect
		assert.Equal(t, http.StatusFound, w.Code)
		assert.Equal(t, "/", w.Header().Get("Location"))

		// Verify session was created
		assert.Len(t, mockSessionStore.sessions, 1)

		// Verify cookie was set
		cookies := w.Result().Cookies()
		assert.Len(t, cookies, 1)
		assert.Equal(t, session.SessionCookieName, cookies[0].Name)
		assert.Equal(t, mockSessionStore.lastKey, cookies[0].Value)
		assert.Equal(t, "/", cookies[0].Path)
		assert.True(t, cookies[0].HttpOnly)
		assert.Equal(t, 86400, cookies[0].MaxAge) // 24 hours in seconds

		// Verify session data
		sessionObj, found := mockSessionStore.sessions[mockSessionStore.lastKey]
		assert.True(t, found)
		assert.Equal(t, "admin", sessionObj.Username)
		assert.Equal(t, "admin@example.com", sessionObj.Email)
		assert.True(t, sessionObj.IsAuthenticated)
		assert.Equal(t, "basic", sessionObj.Provider)
		// Check valid until is roughly 24 hours in the future
		expectedTime := time.Now().Add(24 * time.Hour)
		assert.WithinDuration(t, expectedTime, sessionObj.ValidUntil, 10*time.Second)
	})

	t.Run("failed authentication - invalid credentials", func(t *testing.T) {
		// Create a request with invalid credentials
		formData := url.Values{
			"username": {"admin"},
			"password": {"wrongpassword"},
		}
		req := httptest.NewRequest("POST", "/_/auth/basic/login", strings.NewReader(formData.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		// Dispatch the request to the mux
		mux.ServeHTTP(w, req)

		// Assert response
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Equal(t, "Invalid credentials\n", w.Body.String())

		// No new session should have been created
		assert.Len(t, mockSessionStore.sessions, 1) // Still only the one from previous test
	})

	t.Run("GET request returns login form", func(t *testing.T) {
		// This test just ensures the function attempts to serve a file
		// We can't actually test the file serving here without mocking os.Open,
		// so we'll just check that it doesn't panic or return an unexpected error

		req := httptest.NewRequest("GET", "/_/auth/basic/login", nil)
		w := httptest.NewRecorder()

		// This will fail with a file not found error, but that's expected in tests
		// We just want to make sure the handler tries to serve a file
		mux.ServeHTTP(w, req)

		// Check that the handler responded with an error (file not found)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("successful authentication with redirect", func(t *testing.T) {
		// Create a request with valid credentials and redirect parameter
		formData := url.Values{
			"username": {"admin"},
			"password": {"password"},
		}
		req := httptest.NewRequest("POST", "/_/auth/basic/login?redirect=/dashboard", strings.NewReader(formData.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		// Dispatch the request to the mux
		mux.ServeHTTP(w, req)

		// Assert response is a redirect
		assert.Equal(t, http.StatusFound, w.Code)
		assert.Equal(t, "/dashboard", w.Header().Get("Location"))

		// Verify session was created (now should have 2 sessions)
		assert.Len(t, mockSessionStore.sessions, 2)
	})

	t.Run("user with valid session is redirected without authentication", func(t *testing.T) {
		// Store a valid session
		validSession := session.SessionObject{
			Username:        "existinguser",
			Email:           "existing@example.com",
			IsAuthenticated: true,
			ValidUntil:      time.Now().Add(1 * time.Hour),
			Provider:        "basic",
		}
		sessionKey := "existing-session-token"
		mockSessionStore.Set(sessionKey, validSession)

		// Create a request with an existing session cookie
		req := httptest.NewRequest("GET", "/_/auth/basic/login?redirect=/protected", nil)
		req.AddCookie(&http.Cookie{
			Name:  session.SessionCookieName,
			Value: sessionKey,
		})
		w := httptest.NewRecorder()

		// Dispatch the request to the mux
		mux.ServeHTTP(w, req)

		// Assert response is a redirect without requiring authentication
		assert.Equal(t, http.StatusFound, w.Code)
		assert.Equal(t, "/protected", w.Header().Get("Location"))

		// Verify no new session was created (still have the original valid session plus the 2 from previous tests)
		assert.Len(t, mockSessionStore.sessions, 3)
	})

	t.Run("user with valid session is redirected to root when no redirect param", func(t *testing.T) {
		// Store a valid session
		validSession := session.SessionObject{
			Username:        "rootuser",
			Email:           "root@example.com",
			IsAuthenticated: true,
			ValidUntil:      time.Now().Add(1 * time.Hour),
			Provider:        "basic",
		}
		sessionKey := "root-session-token"
		mockSessionStore.Set(sessionKey, validSession)

		// Create a request with an existing session cookie but no redirect parameter
		req := httptest.NewRequest("GET", "/_/auth/basic/login", nil)
		req.AddCookie(&http.Cookie{
			Name:  session.SessionCookieName,
			Value: sessionKey,
		})
		w := httptest.NewRecorder()

		// Dispatch the request to the mux
		mux.ServeHTTP(w, req)

		// Assert response is a redirect to root
		assert.Equal(t, http.StatusFound, w.Code)
		assert.Equal(t, "/", w.Header().Get("Location"))

		// Verify no new session was created
		assert.Len(t, mockSessionStore.sessions, 4)
	})
}
