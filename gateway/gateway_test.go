package gateway

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/config"
)

func TestNewGateway(t *testing.T) {
	tests := []struct {
		name      string
		config    *config.GatewayConfig
		wantErr   bool
		errSubstr string
	}{
		{
			name: "Valid config",
			config: &config.GatewayConfig{
				Server: config.ServerConfig{
					Host: "localhost",
					Port: 8080,
				},
				Management: config.ManagementConfig{
					Prefix: "/admin",
				},
				Routes: []config.RouteConfig{
					{
						Name: "Test Route",
						From: "/",
						To:   "http://localhost:8081",
					},
					{
						Name:     "Test Route 2",
						From:     "/static",
						ToFolder: "./static",
						Static:   true,
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewGateway(tt.config)

			if tt.wantErr {
				if err == nil {
					t.Error("NewGateway() error = nil, wantErr true")
					return
				}
				if tt.errSubstr != "" && !contains(err.Error(), tt.errSubstr) {
					t.Errorf("NewGateway() error = %v, want error containing %v", err, tt.errSubstr)
				}
				return
			}

			if err != nil {
				t.Errorf("NewGateway() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if got == nil {
				t.Error("NewGateway() returned nil gateway")
				return
			}

			// Check that the gateway was properly initialized
			if got.GatewayConfig != tt.config {
				t.Error("NewGateway() gateway.config not properly set")
			}
			if got.Mux == nil {
				t.Error("NewGateway() gateway.mux is nil")
			}
			if got.Server == nil {
				t.Error("NewGateway() gateway.server is nil")
			}
			if got.SessionStore == nil {
				t.Error("NewGateway() gateway.sessionStore is nil")
			}
			if got.UserRepository == nil {
				t.Error("NewGateway() gateway.userRepository is nil")
			}

			// Check server configuration
			if got.Server.Addr != "localhost:8080" {
				t.Errorf("NewGateway() server.Addr = %v, want %v", got.Server.Addr, "localhost:8080")
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return substr != "" && s != substr && s != "" && strings.Contains(s, substr)
}

func TestHelloEndpoint(t *testing.T) {
	// Create a simple test HTTP server to respond to our proxy requests
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello, World!"))
	}))
	defer testServer.Close()

	// Create a gateway config with a hello endpoint that has no authentication
	config := &config.GatewayConfig{
		Server: config.ServerConfig{
			Host: "localhost",
			Port: 0, // Use port 0 to let the system assign an available port
		},
		Management: config.ManagementConfig{
			Prefix:  "/admin",
			Logging: true,
		},
		Routes: []config.RouteConfig{
			{
				Name: "Hello Endpoint",
				From: "/hello",
				To:   testServer.URL, // Forward to our test server
				Authentication: config.AuthenticationConfig{
					Enabled: false, // No authentication
				},
			},
		},
	}

	// Create a new gateway
	gateway, err := NewGateway(config)
	if err != nil {
		t.Fatalf("Failed to create gateway: %v", err)
	}

	// Create a listener manually since we're using port 0
	listener, err := net.Listen("tcp", gateway.Server.Addr)
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	// Get the actual port that was assigned
	port := listener.Addr().(*net.TCPAddr).Port
	serverURL := fmt.Sprintf("http://localhost:%d", port)

	// Start the server in a goroutine
	serverErrChan := make(chan error, 1)
	go func() {
		serverErrChan <- gateway.Server.Serve(listener)
	}()

	// Give the server a moment to start
	time.Sleep(100 * time.Millisecond)

	// Create an HTTP client and make a request to the /hello endpoint
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(serverURL + "/hello")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Check the response status code
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", resp.StatusCode)
	}

	// Read and verify the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	expectedResponse := "Hello, World!"
	if string(body) != expectedResponse {
		t.Errorf("Expected response %q, got %q", expectedResponse, string(body))
	}

	// Shut down the server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := gateway.Server.Shutdown(ctx); err != nil {
		t.Fatalf("Server shutdown failed: %v", err)
	}

	// Check if the server returned an error
	select {
	case err := <-serverErrChan:
		if err != nil && err != http.ErrServerClosed {
			t.Fatalf("Server error: %v", err)
		}
	default:
		// No error, that's fine
	}
}
