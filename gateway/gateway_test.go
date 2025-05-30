package gateway

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			got, err := NewGateway(tt.config, nil)

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

func TestGatewayStaticFileRouting(t *testing.T) {
	// Create temporary directory and file for testing
	tempDir := t.TempDir()

	// Create a test file
	testFileName := "test.txt"
	testFileContent := "Hello from test file"
	testFilePath := filepath.Join(tempDir, testFileName)
	err := os.WriteFile(testFilePath, []byte(testFileContent), 0644)
	require.NoError(t, err)

	// Create a subfolder with an index.html
	subDir := filepath.Join(tempDir, "subdir")
	err = os.MkdirAll(subDir, 0755)
	require.NoError(t, err)

	indexContent := "<html><body>Index Page</body></html>"
	indexPath := filepath.Join(subDir, "index.html")
	err = os.WriteFile(indexPath, []byte(indexContent), 0644)
	require.NoError(t, err)

	tests := []struct {
		name           string
		routes         []config.RouteConfig
		requestPath    string
		expectedStatus int
		expectedBody   string
		expectAuth     bool
	}{
		{
			name: "Single file serving - no auth",
			routes: []config.RouteConfig{
				{
					Name:   "Single File Route",
					From:   "/file",
					ToFile: testFilePath,
					Static: true,
					Authentication: config.AuthenticationConfig{
						Enabled: false,
					},
				},
			},
			requestPath:    "/file",
			expectedStatus: http.StatusOK,
			expectedBody:   testFileContent,
			expectAuth:     false,
		},
		{
			name: "Single file serving - with auth",
			routes: []config.RouteConfig{
				{
					Name:   "Protected File Route",
					From:   "/protected-file",
					ToFile: testFilePath,
					Static: true,
					Authentication: config.AuthenticationConfig{
						Enabled: true,
					},
				},
			},
			requestPath:    "/protected-file",
			expectedStatus: http.StatusFound, // Redirect to login
			expectedBody:   "",
			expectAuth:     true,
		},
		{
			name: "Folder serving - no auth",
			routes: []config.RouteConfig{
				{
					Name:     "Folder Route",
					From:     "/folder/",
					ToFolder: tempDir,
					Static:   true,
					Authentication: config.AuthenticationConfig{
						Enabled: false,
					},
				},
			},
			requestPath:    "/folder/" + testFileName,
			expectedStatus: http.StatusOK,
			expectedBody:   testFileContent,
			expectAuth:     false,
		},
		{
			name: "Folder serving with index.html - no auth",
			routes: []config.RouteConfig{
				{
					Name:     "Folder with Index Route",
					From:     "/indexed/",
					ToFolder: subDir,
					Static:   true,
					Authentication: config.AuthenticationConfig{
						Enabled: false,
					},
				},
			},
			requestPath:    "/indexed/",
			expectedStatus: http.StatusOK,
			expectedBody:   indexContent,
			expectAuth:     false,
		},
		{
			name: "Folder serving - with auth",
			routes: []config.RouteConfig{
				{
					Name:     "Protected Folder Route",
					From:     "/protected-folder/",
					ToFolder: tempDir,
					Static:   true,
					Authentication: config.AuthenticationConfig{
						Enabled: true,
					},
				},
			},
			requestPath:    "/protected-folder/" + testFileName,
			expectedStatus: http.StatusFound, // Redirect to login
			expectedBody:   "",
			expectAuth:     true,
		},
		{
			name: "Mixed routes - static and proxy",
			routes: []config.RouteConfig{
				{
					Name:   "File Route",
					From:   "/api-file",
					ToFile: testFilePath,
					Static: true,
					Authentication: config.AuthenticationConfig{
						Enabled: false,
					},
				},
				{
					Name:     "Folder Route",
					From:     "/assets/",
					ToFolder: tempDir,
					Static:   true,
					Authentication: config.AuthenticationConfig{
						Enabled: false,
					},
				},
			},
			requestPath:    "/assets/" + testFileName,
			expectedStatus: http.StatusOK,
			expectedBody:   testFileContent,
			expectAuth:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create gateway config
			gatewayConfig := &config.GatewayConfig{
				Server: config.ServerConfig{
					Host: "localhost",
					Port: 0, // Use port 0 to let the system assign an available port
				},
				Management: config.ManagementConfig{
					Prefix:  "/admin",
					Logging: false,
				},
				Routes: tt.routes,
			}

			// Create a new gateway
			gateway, err := NewGateway(gatewayConfig, nil)
			require.NoError(t, err, "Failed to create gateway")

			// Create a listener manually since we're using port 0
			listener, err := net.Listen("tcp", gateway.Server.Addr)
			require.NoError(t, err, "Failed to create listener")
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
			time.Sleep(50 * time.Millisecond)

			// Create an HTTP client and make a request
			client := &http.Client{
				Timeout: 5 * time.Second,
				CheckRedirect: func(req *http.Request, via []*http.Request) error {
					// Don't follow redirects, we want to check them
					return http.ErrUseLastResponse
				},
			}

			resp, err := client.Get(serverURL + tt.requestPath)
			require.NoError(t, err, "Failed to make request")
			defer resp.Body.Close()

			// Check the response status code
			assert.Equal(t, tt.expectedStatus, resp.StatusCode, "Unexpected status code")

			if tt.expectedBody != "" {
				// Read and verify the response body
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err, "Failed to read response body")
				assert.Equal(t, tt.expectedBody, string(body), "Unexpected response body")
			}

			if tt.expectAuth && resp.StatusCode == http.StatusFound {
				// Verify redirect to login page for protected routes
				location := resp.Header.Get("Location")
				assert.Contains(t, location, "/admin/login", "Expected redirect to login page")
				assert.Contains(t, location, "redirect=", "Expected redirect parameter in login URL")
			}

			// Shut down the server
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err = gateway.Server.Shutdown(ctx)
			require.NoError(t, err, "Server shutdown failed")

			// Check if the server returned an error
			select {
			case err := <-serverErrChan:
				if err != nil && err != http.ErrServerClosed {
					t.Fatalf("Server error: %v", err)
				}
			default:
				// No error, that's fine
			}
		})
	}
}

func TestGatewayConfigurationErrors(t *testing.T) {
	tempDir := t.TempDir()

	// Create a test file
	testFilePath := filepath.Join(tempDir, "test.txt")
	err := os.WriteFile(testFilePath, []byte("test"), 0644)
	require.NoError(t, err)

	tests := []struct {
		name        string
		routes      []config.RouteConfig
		expectError bool
		description string
	}{
		{
			name: "Valid single file configuration",
			routes: []config.RouteConfig{
				{
					Name:   "Valid File",
					From:   "/file",
					ToFile: testFilePath,
					Static: true,
				},
			},
			expectError: false,
			description: "Valid file path should work",
		},
		{
			name: "Valid folder configuration",
			routes: []config.RouteConfig{
				{
					Name:     "Valid Folder",
					From:     "/folder/",
					ToFolder: tempDir,
					Static:   true,
				},
			},
			expectError: false,
			description: "Valid folder path should work",
		},
		{
			name: "Empty proxy route 'to' field",
			routes: []config.RouteConfig{
				{
					Name: "Empty To",
					From: "/empty",
					To:   "",
				},
			},
			expectError: false, // Gateway should handle this gracefully and skip the route
			description: "Empty 'to' field should be handled gracefully",
		},
		{
			name: "Mixed valid routes",
			routes: []config.RouteConfig{
				{
					Name:   "File Route",
					From:   "/file",
					ToFile: testFilePath,
					Static: true,
				},
				{
					Name:     "Folder Route",
					From:     "/folder/",
					ToFolder: tempDir,
					Static:   true,
				},
				{
					Name: "Proxy Route",
					From: "/api",
					To:   "http://localhost:8080",
				},
			},
			expectError: false,
			description: "Mix of valid file, folder, and proxy routes should work",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create gateway config
			gatewayConfig := &config.GatewayConfig{
				Server: config.ServerConfig{
					Host: "localhost",
					Port: 8080,
				},
				Management: config.ManagementConfig{
					Prefix: "/admin",
				},
				Routes: tt.routes,
			}

			// Create a new gateway
			gateway, err := NewGateway(gatewayConfig, nil)

			if tt.expectError {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
				assert.NotNil(t, gateway, "Gateway should not be nil")
			}
		})
	}
}

func TestGatewayAuthenticationIntegration(t *testing.T) {
	tempDir := t.TempDir()

	// Create a test file
	testFileContent := "Protected content"
	testFilePath := filepath.Join(tempDir, "protected.txt")
	err := os.WriteFile(testFilePath, []byte(testFileContent), 0644)
	require.NoError(t, err)

	// Create gateway config with authentication
	gatewayConfig := &config.GatewayConfig{
		Server: config.ServerConfig{
			Host: "localhost",
			Port: 0,
		},
		Management: config.ManagementConfig{
			Prefix:  "/admin",
			Logging: false,
		},
		Routes: []config.RouteConfig{
			{
				Name:   "Public File",
				From:   "/public",
				ToFile: testFilePath,
				Static: true,
				Authentication: config.AuthenticationConfig{
					Enabled: false,
				},
			},
			{
				Name:   "Protected File",
				From:   "/protected",
				ToFile: testFilePath,
				Static: true,
				Authentication: config.AuthenticationConfig{
					Enabled: true,
				},
			},
			{
				Name:     "Protected Folder",
				From:     "/protected-folder/",
				ToFolder: tempDir,
				Static:   true,
				Authentication: config.AuthenticationConfig{
					Enabled: true,
				},
			},
		},
	}

	// Create a new gateway
	gateway, err := NewGateway(gatewayConfig, nil)
	require.NoError(t, err, "Failed to create gateway")

	// Create a listener manually since we're using port 0
	listener, err := net.Listen("tcp", gateway.Server.Addr)
	require.NoError(t, err, "Failed to create listener")
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
	time.Sleep(50 * time.Millisecond)

	// Create an HTTP client that doesn't follow redirects
	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	t.Run("Public file should be accessible", func(t *testing.T) {
		resp, err := client.Get(serverURL + "/public")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, testFileContent, string(body))
	})

	t.Run("Protected file should redirect to login", func(t *testing.T) {
		resp, err := client.Get(serverURL + "/protected")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusFound, resp.StatusCode)

		location := resp.Header.Get("Location")
		assert.Contains(t, location, "/admin/login")
		assert.Contains(t, location, "redirect=")
	})

	t.Run("Protected folder should redirect to login", func(t *testing.T) {
		resp, err := client.Get(serverURL + "/protected-folder/protected.txt")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusFound, resp.StatusCode)

		location := resp.Header.Get("Location")
		assert.Contains(t, location, "/admin/login")
		assert.Contains(t, location, "redirect=")
	})

	// Shut down the server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = gateway.Server.Shutdown(ctx)
	require.NoError(t, err, "Server shutdown failed")

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
	gateway, err := NewGateway(config, nil)
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

// TestGatewaySPARouting tests SPA (Single Page Application) routing functionality
func TestGatewaySPARouting(t *testing.T) {
	// Create temporary directory structure for testing SPA
	tempDir := t.TempDir()

	// Create index.html (main SPA file)
	indexContent := `<!DOCTYPE html>
<html>
<head><title>SPA App</title></head>
<body>
<div id="app">SPA Application</div>
<script>
// Mock SPA routing logic
window.onload = function() {
	if (window.location.pathname === '/about') {
		document.getElementById('app').innerHTML = 'About Page';
	} else if (window.location.pathname === '/contact') {
		document.getElementById('app').innerHTML = 'Contact Page';
	}
};
</script>
</body>
</html>`
	indexPath := filepath.Join(tempDir, "index.html")
	err := os.WriteFile(indexPath, []byte(indexContent), 0644)
	require.NoError(t, err)

	// Create a static asset file (CSS)
	cssContent := "body { background-color: blue; }"
	cssPath := filepath.Join(tempDir, "style.css")
	err = os.WriteFile(cssPath, []byte(cssContent), 0644)
	require.NoError(t, err)

	// Create a JavaScript file
	jsContent := "console.log('SPA app loaded');"
	jsPath := filepath.Join(tempDir, "app.js")
	err = os.WriteFile(jsPath, []byte(jsContent), 0644)
	require.NoError(t, err)

	tests := []struct {
		name           string
		routes         []config.RouteConfig
		requestPath    string
		expectedStatus int
		expectedBody   string
		description    string
	}{
		{
			name: "SPA route - serve index.html for root",
			routes: []config.RouteConfig{
				{
					Name:     "SPA Route",
					From:     "/*",
					ToFolder: tempDir,
					Static:   true,
					IsSPA:    true,
				},
			},
			requestPath:    "/",
			expectedStatus: http.StatusOK,
			expectedBody:   "SPA Application",
			description:    "Root path should serve index.html",
		},
		{
			name: "SPA route - serve index.html for client-side route",
			routes: []config.RouteConfig{
				{
					Name:     "SPA Route",
					From:     "/*",
					ToFolder: tempDir,
					Static:   true,
					IsSPA:    true,
				},
			},
			requestPath:    "/about",
			expectedStatus: http.StatusOK,
			expectedBody:   "SPA Application", // Should serve index.html, not 404
			description:    "Client-side route should fallback to index.html",
		},
		{
			name: "SPA route - serve static asset directly",
			routes: []config.RouteConfig{
				{
					Name:     "SPA Route",
					From:     "/*",
					ToFolder: tempDir,
					Static:   true,
					IsSPA:    true,
				},
			},
			requestPath:    "/style.css",
			expectedStatus: http.StatusOK,
			expectedBody:   cssContent,
			description:    "Static assets should be served directly",
		},
		{
			name: "SPA route - serve JS file directly",
			routes: []config.RouteConfig{
				{
					Name:     "SPA Route",
					From:     "/*",
					ToFolder: tempDir,
					Static:   true,
					IsSPA:    true,
				},
			},
			requestPath:    "/app.js",
			expectedStatus: http.StatusOK,
			expectedBody:   jsContent,
			description:    "JavaScript files should be served directly",
		},
		{
			name: "Non-SPA route - should return 404 for missing files",
			routes: []config.RouteConfig{
				{
					Name:     "Non-SPA Route",
					From:     "/*",
					ToFolder: tempDir,
					Static:   true,
					IsSPA:    false, // SPA disabled
				},
			},
			requestPath:    "/nonexistent",
			expectedStatus: http.StatusNotFound,
			expectedBody:   "",
			description:    "Non-SPA routes should return 404 for missing files",
		},
		{
			name: "SPA route with prefix - fallback to index.html",
			routes: []config.RouteConfig{
				{
					Name:     "SPA Route with Prefix",
					From:     "/app/*",
					ToFolder: tempDir,
					Static:   true,
					IsSPA:    true,
				},
			},
			requestPath:    "/app/dashboard",
			expectedStatus: http.StatusOK,
			expectedBody:   "SPA Application",
			description:    "SPA with prefix should fallback to index.html for client routes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create gateway config
			gatewayConfig := &config.GatewayConfig{
				Server: config.ServerConfig{
					Host: "127.0.0.1",
					Port: 0, // Let OS choose port
				},
				Management: config.ManagementConfig{
					Prefix: "/_",
				},
				Routes: tt.routes,
			}

			// Create gateway
			gateway, err := NewGateway(gatewayConfig, nil)
			require.NoError(t, err, "Failed to create gateway")

			// Create test server
			server := httptest.NewServer(gateway.Mux)
			defer server.Close()

			// Make request
			resp, err := http.Get(server.URL + tt.requestPath)
			require.NoError(t, err, "Failed to make request")
			defer resp.Body.Close()

			// Check status
			assert.Equal(t, tt.expectedStatus, resp.StatusCode,
				"Status code mismatch for %s", tt.description)

			// Check body if expected
			if tt.expectedBody != "" {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err, "Failed to read response body")
				assert.Contains(t, string(body), tt.expectedBody,
					"Response body should contain expected content for %s", tt.description)
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return substr != "" && s != substr && s != "" && strings.Contains(s, substr)
}
