package providers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmaister/taronja-gateway/session"
)

func TestLogout(t *testing.T) {
	// Create a new session store
	sessionStore := session.NewMemorySessionStore()

	// Create a test session
	sessionKey, _ := sessionStore.GenerateKey()
	sessionObject := session.SessionObject{
		Username:        "testuser",
		Email:           "test@example.com",
		IsAuthenticated: true,
		Provider:        "test",
	}
	sessionStore.Set(sessionKey, sessionObject)

	// Create a test server with the logout handler
	mux := http.NewServeMux()
	registerLogout(mux, sessionStore, "/_")

	// Create a request with a session cookie
	req := httptest.NewRequest("GET", "/_/logout", nil)
	req.AddCookie(&http.Cookie{
		Name:  session.SessionCookieName,
		Value: sessionKey,
	})

	// Record the response
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)

	// Check status code (should be a redirect)
	if recorder.Code != http.StatusFound {
		t.Errorf("Expected status code %d, got %d", http.StatusFound, recorder.Code)
	}

	// Check that the session was deleted
	_, exists := sessionStore.Get(sessionKey)
	if exists == nil {
		t.Error("Session was not deleted")
	}

	// Check that the session cookie was cleared
	cookies := recorder.Result().Cookies()
	var foundCookie bool
	for _, cookie := range cookies {
		if cookie.Name == session.SessionCookieName {
			foundCookie = true
			if cookie.MaxAge != -1 {
				t.Errorf("Expected MaxAge to be -1, got %d", cookie.MaxAge)
			}
			if cookie.Value != "" {
				t.Errorf("Expected cookie value to be empty, got %s", cookie.Value)
			}
		}
	}
	if !foundCookie {
		t.Error("Session cookie was not found in response")
	}
}

func TestLogoutWithNoSession(t *testing.T) {
	// Create a new session store
	sessionStore := session.NewMemorySessionStore()

	// Create a test server with the logout handler
	mux := http.NewServeMux()
	registerLogout(mux, sessionStore, "/_")

	// Create a request without a session cookie
	req := httptest.NewRequest("GET", "/_/logout", nil)

	// Record the response
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)

	// Check status code (should be a redirect)
	if recorder.Code != http.StatusFound {
		t.Errorf("Expected status code %d, got %d", http.StatusFound, recorder.Code)
	}

	// Check redirect location
	location := recorder.Header().Get("Location")
	if location != "/" {
		t.Errorf("Expected redirect to '/', got '%s'", location)
	}
}

func TestLogoutWithRedirect(t *testing.T) {
	// Create a new session store
	sessionStore := session.NewMemorySessionStore()

	// Create a test server with the logout handler
	mux := http.NewServeMux()
	registerLogout(mux, sessionStore, "/_")

	// Create a request with a session cookie and redirect parameter
	req := httptest.NewRequest("GET", "/_/logout?redirect=/login", nil)
	req.AddCookie(&http.Cookie{
		Name:  session.SessionCookieName,
		Value: "test-session",
	})

	// Record the response
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)

	// Check status code (should be a redirect)
	if recorder.Code != http.StatusFound {
		t.Errorf("Expected status code %d, got %d", http.StatusFound, recorder.Code)
	}

	// Check redirect location
	location := recorder.Header().Get("Location")
	if location != "/login" {
		t.Errorf("Expected redirect to '/login', got '%s'", location)
	}
}
