package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/config"
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

	// Create a gateway instance. NewGateway calls configureManagementRoutes,
	// which we will modify to call registerLogout.
	gw, err := NewGateway(cfg)
	if err != nil {
		t.Fatalf("Failed to create gateway: %v", err)
	}

	// Create a test session
	sessionKey, err := gw.SessionStore.GenerateKey()
	if err != nil {
		t.Fatalf("Failed to generate session key: %v", err)
	}
	sessionObject := session.SessionObject{
		Username:        "testuser",
		Email:           "test@example.com",
		IsAuthenticated: true,
		Provider:        "test",
		ValidUntil:      time.Now().Add(1 * time.Hour), // Ensure session is valid
	}
	err = gw.SessionStore.Set(sessionKey, sessionObject)
	if err != nil {
		t.Fatalf("Failed to set session: %v", err)
	}

	// Create a request to the logout endpoint
	logoutPath := cfg.Management.Prefix + "/logout"
	req := httptest.NewRequest("GET", logoutPath, nil)
	req.AddCookie(&http.Cookie{
		Name:  session.SessionCookieName,
		Value: sessionKey,
		Path:  "/", // Ensure cookie path matches what the server expects/sets
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

	// Check that the session was deleted from the store
	_, errGet := gw.SessionStore.Get(sessionKey)
	if errGet == nil { // If errGet is nil, session was found (not deleted)
		t.Error("Session was not deleted from store")
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
		if cookie.Name == session.SessionCookieName {
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
	sessionKey, err := gw.SessionStore.GenerateKey()
	if err != nil {
		t.Fatalf("Failed to generate session key: %v", err)
	}
	sessionObject := session.SessionObject{
		Username:        "testuser",
		Email:           "test@example.com",
		IsAuthenticated: true,
		Provider:        "test",
		ValidUntil:      time.Now().Add(1 * time.Hour),
	}
	err = gw.SessionStore.Set(sessionKey, sessionObject)
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

	// Check that the session was deleted from the store
	_, errGet := gw.SessionStore.Get(sessionKey)
	if errGet == nil {
		t.Error("Session was not deleted from store")
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
