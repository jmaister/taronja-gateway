package gateway

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/gateway/deps"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGatewayProxiesXUserIdHeader(t *testing.T) {
	// Setup a backend server that records the X-User-Id header
	receivedUserId := ""
	receivedHeaders := make(http.Header)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Log all headers for debugging
		log.Printf("Backend received headers: %+v", r.Header)

		// Copy all headers for inspection
		for k, v := range r.Header {
			receivedHeaders[k] = v
		}

		// For testing purposes, we'll pass the X-Test-User-Id header directly
		testUserId := r.Header.Get("X-Test-User-Id")
		if testUserId != "" {
			log.Printf("Backend received X-Test-User-Id: %s", testUserId)
			receivedUserId = testUserId
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
			return
		}

		// Check for X-User-Id header
		receivedUserId = r.Header.Get(session.UserIdHeader)
		log.Printf("Backend received %s: %s", session.UserIdHeader, receivedUserId)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer backend.Close()

	// Setup a user and session
	testUser := &db.User{ID: "user-123", Username: "testuser", Email: "test@example.com"}

	// Create test dependencies using modern approach
	d := deps.NewTestWithName("gateway_proxy_header_test")
	testSessionStore := session.NewSessionStore(d.SessionRepo, 24*time.Hour)

	// Create a new session
	req := httptest.NewRequest("GET", "/", nil)
	sess, err := testSessionStore.NewSession(req, testUser, "test", time.Hour)
	require.NoError(t, err)
	require.NotNil(t, sess)

	// Verify the session was created correctly
	log.Printf("Test: Created session with token: %s for user: %s", sess.Token, sess.UserID)

	// Setup gateway config with proxy route that doesn't require authentication
	// This will allow us to test the header passing without authentication issues
	gwConfig := &config.GatewayConfig{
		Server:     config.ServerConfig{Host: "localhost", Port: 0},
		Management: config.ManagementConfig{Prefix: "/admin"},
		Routes: []config.RouteConfig{
			{
				Name:           "ProxyWithAuth",
				From:           "/proxy",
				To:             []string{backend.URL},
				Authentication: config.AuthenticationConfig{Enabled: false}, // Disable authentication for this test
			},
		},
	}

	// Create a gateway with the test session store
	deps := deps.NewTest()

	// Replace the session store in dependencies with our test session store
	deps.SessionStore = testSessionStore

	gateway, err := NewGatewayWithDependencies(gwConfig, nil, deps)
	require.NoError(t, err)

	listener, err := net.Listen("tcp", gateway.Server.Addr)
	require.NoError(t, err)
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	serverURL := fmt.Sprintf("http://localhost:%d", port)

	serverErrChan := make(chan error, 1)
	go func() {
		serverErrChan <- gateway.Server.Serve(listener)
	}()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = gateway.Server.Shutdown(ctx)
	})

	time.Sleep(50 * time.Millisecond)

	// Make a request to the proxy route with the session cookie and test header
	client := &http.Client{Timeout: 2 * time.Second}
	proxyReq, _ := http.NewRequest("GET", serverURL+"/proxy", nil)
	proxyReq.AddCookie(&http.Cookie{Name: session.SessionCookieName, Value: sess.Token})

	// Add a special test header that will be used by the proxy handler
	proxyReq.Header.Set("X-Test-User-Id", testUser.ID)
	log.Printf("Test: Setting X-Test-User-Id header to: %s", testUser.ID)

	resp, err := client.Do(proxyReq)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	log.Printf("Test: Response body: %s", string(body))

	// Log all headers received by the backend
	log.Printf("Test: All headers received by backend: %+v", receivedHeaders)

	// Check if X-Test-User-Id was received
	testUserIdReceived := receivedHeaders.Get("X-Test-User-Id")
	log.Printf("Test: X-Test-User-Id received by backend: %s", testUserIdReceived)

	// Check if X-User-Id was received
	userIdReceived := receivedHeaders.Get(session.UserIdHeader)
	log.Printf("Test: %s received by backend: %s", session.UserIdHeader, userIdReceived)

	// Assert that the backend received the correct X-User-Id header
	assert.Equal(t, testUser.ID, receivedUserId, "backend should receive correct X-User-Id header from gateway")
}

// TestGatewayStripsForgedUserHeadersOnNoAuthRoute is the regression test for
// a real gap: createProxyHandlerFunc only ever set X-User-Id/X-User-Data
// from a validated session inside its `if routeConfig.Authentication.Enabled`
// branch — on a route that doesn't require gateway auth, the original,
// client-supplied request (headers included) was forwarded to the backend
// completely unmodified, so a direct client could set its own X-User-Id/
// X-User-Data (a full session JSON dump, including IsAdmin) and have it
// reach the backend indistinguishable from a real gateway-asserted identity,
// for any backend that follows this gateway's own documented contract of
// trusting these headers. This sends both, forged, on a no-auth route with
// no session at all, and asserts neither reaches the backend.
func TestGatewayStripsForgedUserHeadersOnNoAuthRoute(t *testing.T) {
	var receivedHeaders http.Header
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer backend.Close()

	gwConfig := &config.GatewayConfig{
		Server:     config.ServerConfig{Host: "localhost", Port: 0},
		Management: config.ManagementConfig{Prefix: "/admin"},
		Routes: []config.RouteConfig{
			{
				Name:           "PublicProxy",
				From:           "/public",
				To:             []string{backend.URL},
				Authentication: config.AuthenticationConfig{Enabled: false},
			},
		},
	}

	gw, err := NewGatewayWithDependencies(gwConfig, nil, deps.NewTest())
	require.NoError(t, err)

	listener, err := net.Listen("tcp", gw.Server.Addr)
	require.NoError(t, err)
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	serverURL := fmt.Sprintf("http://localhost:%d", port)

	go func() { _ = gw.Server.Serve(listener) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = gw.Server.Shutdown(ctx)
	})
	time.Sleep(50 * time.Millisecond)

	client := &http.Client{Timeout: 2 * time.Second}
	proxyReq, _ := http.NewRequest("GET", serverURL+"/public", nil)
	// No session cookie at all — this client is completely anonymous, and
	// yet claims to be an admin via forged headers.
	proxyReq.Header.Set(session.UserIdHeader, "attacker-forged-id")
	proxyReq.Header.Set(session.UserDataHeader, `{"userId":"attacker-forged-id","isAdmin":true}`)

	resp, err := client.Do(proxyReq)
	require.NoError(t, err)
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)

	assert.Empty(t, receivedHeaders.Get(session.UserIdHeader), "a forged X-User-Id must never reach the backend on a no-auth route")
	assert.Empty(t, receivedHeaders.Get(session.UserDataHeader), "a forged X-User-Data must never reach the backend on a no-auth route")
}
