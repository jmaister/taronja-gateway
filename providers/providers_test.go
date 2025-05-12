package providers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmaister/taronja-gateway/session"
)

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
