package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/session"
)

func TestGatewayLogout(t *testing.T) {
	cfg := &config.GatewayConfig{
		Management: config.ManagementConfig{Prefix: "/_"},                                            // Using a common test prefix
		Server:     config.ServerConfig{Host: "localhost", Port: 8080, URL: "http://localhost:8080"}, // Ensure URL is set for NewGateway logic if needed
		Routes:     []config.RouteConfig{},                                                           // Provide empty routes to prevent nil panic during setup
		AuthenticationProviders: config.AuthenticationProviders{ // Ensure this is not nil if NewGateway accesses it
			Basic:  config.BasicAuthenticationConfig{},
			Github: config.AuthProviderCredentials{},
			Google: config.AuthProviderCredentials{},
		},
	}

	// Create a gateway instance.
	gw, err := NewGateway(cfg)
	if err != nil {
		t.Fatalf("Failed to create gateway: %v", err)
	}

	// Create a dummy user for the session
	testUser := &db.User{ID: "testlogoutuser", Username: "testlogoutusername"}

	// Create a test session using SessionStore's NewSession
	// NewSession handles token generation and creation in the repository.
	sessionData, err := gw.SessionStore.NewSession(nil, testUser, "test-provider", time.Hour)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	sessionKey := sessionData.Token // Get the token from the created session

	// Create a request to the logout endpoint
	logoutPath := cfg.Management.Prefix + "/logout"
	req := httptest.NewRequest("GET", logoutPath, nil)
	req.AddCookie(&http.Cookie{
		Name:  session.SessionCookieName,
		Value: sessionKey, // Use the session token from the created session
		Path:  "/",        // Ensure cookie path matches what the server expects/sets
	})

	recorder := httptest.NewRecorder()
	gw.Mux.ServeHTTP(recorder, req) // Serve using the gateway's mux

	// Check status code (should be a redirect)
	if recorder.Code != http.StatusFound {
		t.Errorf("Expected status code %d, got %d. Body: %s", http.StatusFound, recorder.Code, recorder.Body.String())
	}

	// Check redirect location (default is "/" as per gateway.go's registerLogout)
	expectedRedirect := "/"
	if location := recorder.Header().Get("Location"); location != expectedRedirect {
		t.Errorf("Expected redirect to '%s', got '%s'", expectedRedirect, location)
	}

	// Check that the session was deleted from the store (actually closed)
	// Access Repo via type assertion on SessionStore if it's SessionStoreDB
	var retrievedSession *db.Session
	var errGet error
	if storeDB, ok := gw.SessionStore.(*session.SessionStoreDB); ok {
		retrievedSession, errGet = storeDB.Repo.FindSessionByToken(sessionKey)
	} else {
		t.Fatalf("SessionStore is not of type *session.SessionStoreDB, cannot access Repo")
	}

	if errGet == nil && retrievedSession != nil { // If errGet is nil and session is found, it was not properly closed/deleted
		t.Error("Session was not deleted/closed from store")
	} else if errGet != nil && errGet.Error() != db.ErrSessionClosed.Error() {
		// If there's an error, it should be because the session is marked as closed
		t.Errorf("Expected '%s' error, got: %v", db.ErrSessionClosed.Error(), errGet)
	}

	// Check that the session cookie was cleared in the response
	cookies := recorder.Result().Cookies()
	var foundClearedCookie bool
	for _, cookie := range cookies {
		if cookie.Name == session.SessionCookieName {
			if cookie.Value == "" && cookie.MaxAge == -1 {
				foundClearedCookie = true
				break
			} else {
				t.Errorf("Session cookie was found, but not properly cleared. Value: '%s', MaxAge: %d", cookie.Value, cookie.MaxAge)
			}
		}
	}
	if !foundClearedCookie {
		t.Error("Cleared session cookie was not found in response")
	}
}

func TestGatewayLogoutWithNoSession(t *testing.T) {
	cfg := &config.GatewayConfig{
		Management: config.ManagementConfig{Prefix: "/_"},
		Server:     config.ServerConfig{Host: "localhost", Port: 8080, URL: "http://localhost:8080"},
		Routes:     []config.RouteConfig{},
		AuthenticationProviders: config.AuthenticationProviders{
			Basic:  config.BasicAuthenticationConfig{},
			Github: config.AuthProviderCredentials{},
			Google: config.AuthProviderCredentials{},
		},
	}

	gw, err := NewGateway(cfg)
	if err != nil {
		t.Fatalf("Failed to create gateway: %v", err)
	}

	// Create a request to the logout endpoint without a session cookie
	logoutPath := cfg.Management.Prefix + "/logout"
	req := httptest.NewRequest("GET", logoutPath, nil)

	recorder := httptest.NewRecorder()
	gw.Mux.ServeHTTP(recorder, req)

	// Check status code (should be a redirect)
	if recorder.Code != http.StatusFound {
		t.Errorf("Expected status code %d, got %d. Body: %s", http.StatusFound, recorder.Code, recorder.Body.String())
	}

	// Check redirect location (default is "/" as per gateway.go's registerLogout when no session)
	expectedRedirect := "/"
	if location := recorder.Header().Get("Location"); location != expectedRedirect {
		t.Errorf("Expected redirect to '%s', got '%s'", expectedRedirect, location)
	}

	// Check that no session cookie was set or cleared, as none was present
	cookies := recorder.Result().Cookies()
	for _, cookie := range cookies {
		if cookie.Name == session.SessionCookieName && cookie.Value != "" {
			t.Errorf("Session cookie was unexpectedly found in response: Name=%s, Value=%s", cookie.Name, cookie.Value)
		}
	}
}

func TestGatewayLogoutWithRedirect(t *testing.T) {
	cfg := &config.GatewayConfig{
		Management: config.ManagementConfig{Prefix: "/_"},
		Server:     config.ServerConfig{Host: "localhost", Port: 8080, URL: "http://localhost:8080"},
		Routes:     []config.RouteConfig{},
		AuthenticationProviders: config.AuthenticationProviders{
			Basic:  config.BasicAuthenticationConfig{},
			Github: config.AuthProviderCredentials{},
			Google: config.AuthProviderCredentials{},
		},
	}

	gw, err := NewGateway(cfg)
	if err != nil {
		t.Fatalf("Failed to create gateway: %v", err)
	}

	// Create a test session (even if redirecting, logout should clear it)
	sessionKey, err := session.GenerateToken() // Use package level GenerateToken
	if err != nil {
		t.Fatalf("Failed to generate session key: %v", err)
	}
	sessionObject := db.Session{ // This is db.Session
		Token:           sessionKey, // Set the token
		UserID:          "testuser",
		Username:        "testuser",
		Email:           "test@example.com",
		IsAuthenticated: true,
		Provider:        "test",
		ValidUntil:      time.Now().Add(1 * time.Hour),
		// Removed CreatedAt and UpdatedAt as they are not in db.Session based on previous errors
	}
	// Access Repo via type assertion on SessionStore if it's SessionStoreDB
	if storeDB, ok := gw.SessionStore.(*session.SessionStoreDB); ok {
		err = storeDB.Repo.CreateSession(sessionKey, &sessionObject)
	} else {
		t.Fatalf("SessionStore is not of type *session.SessionStoreDB, cannot access Repo")
	}
	if err != nil {
		t.Fatalf("Failed to set session: %v", err)
	}

	// Create a request to the logout endpoint with a redirect parameter
	logoutPath := cfg.Management.Prefix + "/logout?redirect=/customlogin"
	req := httptest.NewRequest("GET", logoutPath, nil)
	req.AddCookie(&http.Cookie{
		Name:  session.SessionCookieName,
		Value: sessionKey,
		Path:  "/",
	})

	recorder := httptest.NewRecorder()
	gw.Mux.ServeHTTP(recorder, req)

	// Check status code (should be a redirect)
	if recorder.Code != http.StatusFound {
		t.Errorf("Expected status code %d, got %d. Body: %s", http.StatusFound, recorder.Code, recorder.Body.String())
	}

	// Check redirect location
	expectedRedirect := "/customlogin"
	if location := recorder.Header().Get("Location"); location != expectedRedirect {
		t.Errorf("Expected redirect to '%s', got '%s'", expectedRedirect, location)
	}

	// Check that the session was deleted from the store (actually closed)
	// Access Repo via type assertion on SessionStore if it's SessionStoreDB
	var retrievedSession *db.Session
	var errGet error
	if storeDB, ok := gw.SessionStore.(*session.SessionStoreDB); ok {
		retrievedSession, errGet = storeDB.Repo.FindSessionByToken(sessionKey)
	} else {
		t.Fatalf("SessionStore is not of type *session.SessionStoreDB, cannot access Repo")
	}

	if errGet == nil && retrievedSession != nil { // If errGet is nil and session is found, it was not properly closed/deleted
		t.Error("Session was not deleted/closed from store")
	} else if errGet != nil && errGet.Error() != db.ErrSessionClosed.Error() {
		// If there's an error, it should be because the session is marked as closed
		t.Errorf("Expected '%s' error, got: %v", db.ErrSessionClosed.Error(), errGet)
	}

	// Check that the session cookie was cleared in the response
	cookies := recorder.Result().Cookies()
	var foundClearedCookie bool
	for _, cookie := range cookies {
		if cookie.Name == session.SessionCookieName {
			if cookie.Value == "" && cookie.MaxAge == -1 {
				foundClearedCookie = true
				break
			} else {
				t.Errorf("Session cookie was found, but not properly cleared. Value: '%s', MaxAge: %d", cookie.Value, cookie.MaxAge)
			}
		}
	}
	if !foundClearedCookie {
		t.Error("Cleared session cookie was not found in response")
	}
}
