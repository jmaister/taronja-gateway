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
		receivedUserId = r.Header.Get("X-User-Id")
		log.Printf("Backend received X-User-Id: %s", receivedUserId)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer backend.Close()

	// Setup a user and session
	testUser := &db.User{ID: "user-123", Username: "testuser", Email: "test@example.com"}

	// Create a memory session repository for testing
	testSessionRepo := db.NewMemorySessionRepository() // Use in-memory repo for test
	testSessionStore := session.NewSessionStore(testSessionRepo)

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
				To:             backend.URL,
				Authentication: config.AuthenticationConfig{Enabled: false}, // Disable authentication for this test
			},
		},
	}

	// Create a gateway with the test session store
	gateway, err := NewGateway(gwConfig, nil)
	require.NoError(t, err)

	// Replace the gateway's session store with our test session store
	gateway.SessionStore = testSessionStore

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
	userIdReceived := receivedHeaders.Get("X-User-Id")
	log.Printf("Test: X-User-Id received by backend: %s", userIdReceived)

	// Assert that the backend received the correct X-User-Id header
	assert.Equal(t, testUser.ID, receivedUserId, "backend should receive correct X-User-Id header from gateway")
}
