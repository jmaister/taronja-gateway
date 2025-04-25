package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// --- Configuration Structs ---

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
	URL  string `yaml:"url"`
}
type AuthenticationConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Method   string `yaml:"method"`   // "basic", "oauth2", "any", or ""
	Provider string `yaml:"provider"` // "google", "github", etc. (for oauth2)
}
type RouteConfig struct {
	Name           string               `yaml:"name"`
	From           string               `yaml:"from"`
	To             string               `yaml:"to"`
	ToFolder       string               `yaml:"toFolder"`
	Static         bool                 `yaml:"static"`
	RemoveFromPath string               `yaml:"removeFromPath"`
	Authentication AuthenticationConfig `yaml:"authentication"`
}
type AuthProviderCredentials struct {
	ClientId     string `yaml:"clientId"`
	ClientSecret string `yaml:"clientSecret"`
}
type AuthenticationProviders struct {
	Google AuthProviderCredentials `yaml:"google"`
	Github AuthProviderCredentials `yaml:"github"`
}
type NotificationConfig struct {
	Email struct {
		Enabled bool `yaml:"enabled"`
		SMTP    struct {
			Host     string `yaml:"host"`
			Port     int    `yaml:"port"`
			Username string `yaml:"username"`
			Password string `yaml:"password"`
			From     string `yaml:"from"`
			FromName string `yaml:"fromName"`
		} `yaml:"smtp"`
	} `yaml:"email"`
}

// New: Management API Configuration Structs
type ManagementEndpointToggle struct {
	Enabled bool `yaml:"enabled"`
}
type ManagementEndpointAuth struct {
	Enabled        bool                 `yaml:"enabled"`
	Authentication AuthenticationConfig `yaml:"authentication"` // Auth config for this specific endpoint
}
type ManagementConfig struct {
	Prefix string                   `yaml:"prefix"` // e.g., "/_"
	Health ManagementEndpointToggle `yaml:"health"` // Simple enable/disable
	Me     ManagementEndpointAuth   `yaml:"me"`     // Endpoint with its own auth config
	// Add more management endpoints here
}

// Main Config Struct including Management API config
type Config struct {
	Name                    string                  `yaml:"name"`
	Server                  ServerConfig            `yaml:"server"`
	Management              ManagementConfig        `yaml:"management"` // Add management config
	Routes                  []RouteConfig           `yaml:"routes"`
	AuthenticationProviders AuthenticationProviders `yaml:"authenticationProviders"`
	Notification            NotificationConfig      `yaml:"notification"`
}

// --- Gateway Struct ---

type Gateway struct {
	server      *http.Server
	config      *Config
	mux         *http.ServeMux
	authManager *AuthManager
}

// --- NewGateway Function ---

func NewGateway(config *Config) (*Gateway, error) {
	mux := http.NewServeMux()
	authManager := NewAuthManager(config) // Initialize Auth Manager early

	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", config.Server.Host, config.Server.Port),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  120 * time.Second,
		Handler:      mux, // Mux will be populated later
	}

	gateway := &Gateway{
		server:      server,
		config:      config,
		mux:         mux,
		authManager: authManager,
	}

	// --- IMPORTANT: Register Management Routes FIRST ---
	// This ensures they take precedence over potentially broad user routes like "/"
	gateway.configureManagementRoutes()

	// Configure the standard proxy/static routes
	err := gateway.configureUserRoutes()
	if err != nil {
		return nil, fmt.Errorf("error configuring user routes: %w", err)
	}

	// Configure the OAuth callback handler (can be done after user routes)
	gateway.configureOAuthCallbackRoute()

	return gateway, nil
}

// --- Route Configuration ---

// configureManagementRoutes sets up internal API endpoints like /health, /me
func (g *Gateway) configureManagementRoutes() {
	prefix := g.config.Management.Prefix
	log.Printf("main.go: Registering management API routes under prefix: %s", prefix)

	// Health Endpoint
	if g.config.Management.Health.Enabled {
		healthPath := prefix + "/health"
		g.mux.HandleFunc(healthPath, handleHealth)
		log.Printf("main.go: Registered Management Route: %-25s | Path: %s | Auth: %t", "Health Check", healthPath, false)
	}

	// Me Endpoint
	if g.config.Management.Me.Enabled {
		mePath := prefix + "/me"
		meAuthConfig := g.config.Management.Me.Authentication
		if !meAuthConfig.Enabled {
			meAuthConfig.Enabled = true
		}
		authWrappedMeHandler := g.wrapWithAuth(handleMe, meAuthConfig)
		g.mux.HandleFunc(mePath, authWrappedMeHandler)
		log.Printf("main.go: Registered Management Route: %-25s | Path: %s | Auth: %t (Method: %s)", "User Info", mePath, true, meAuthConfig.Method)
	}

	// Login Routes for Basic and OAuth2 Authentication
	g.registerLoginRoutes(prefix)
}

// registerLoginRoutes adds login routes for basic and OAuth2 authentication.
func (g *Gateway) registerLoginRoutes(prefix string) {
	// Basic Auth Login Route
	basicLoginPath := prefix + "/auth/basic/login"
	g.mux.HandleFunc(basicLoginPath, func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Validate username and password (replace with your own logic)
		// TODO: handle users on a database
		if username == "admin" && password == "password" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Login successful"))
		} else {
			http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		}
	})
	log.Printf("main.go: Registered Login Route: %-25s | Path: %s", "Basic Auth Login", basicLoginPath)

	// OAuth2 Login and Callback Routes
	for provider, authenticator := range g.authManager.authenticators {
		if oauthAuthenticator, ok := authenticator.(*OAuth2Authenticator); ok {
			// Login Route
			loginPath := fmt.Sprintf("%s/auth/%s/login", prefix, provider)
			g.mux.HandleFunc(loginPath, func(w http.ResponseWriter, r *http.Request) {
				oauthAuthenticator.Authenticate(w, r)
			})
			log.Printf("main.go: Registered Login Route: %-25s | Path: %s", fmt.Sprintf("%s OAuth2 Login", provider), loginPath)

			// Callback Route
			callbackPath := fmt.Sprintf("%s/auth/%s/callback", prefix, provider)
			g.mux.HandleFunc(callbackPath, func(w http.ResponseWriter, r *http.Request) {
				g.authManager.HandleOAuthCallback(provider, w, r)
			})
			log.Printf("main.go: Registered Callback Route: %-25s | Path: %s", fmt.Sprintf("%s OAuth2 Callback", provider), callbackPath)
		}
	}
}

// configureUserRoutes sets up the main proxy and static file routes defined by the user.
// Renamed from configureRoutes to distinguish from management routes.
func (g *Gateway) configureUserRoutes() error {
	log.Printf("main.go: Registering user-defined routes...")
	for _, routeConfig := range g.config.Routes {
		var handler http.HandlerFunc

		// Create the base handler (proxy or static)
		if routeConfig.Static {
			if routeConfig.ToFolder == "" {
				log.Printf("Warning: Empty ToFolder for static route '%s'. Skipping registration.", routeConfig.Name)
				continue
			}
			fileInfo, statErr := os.Stat(routeConfig.ToFolder)
			if statErr != nil {
				log.Printf("Warning: Invalid ToFolder '%s' for static route '%s': %v. Skipping registration.", routeConfig.ToFolder, routeConfig.Name, statErr)
				continue
			}
			handler = g.createStaticHandlerFunc(routeConfig, fileInfo.IsDir())
		} else {
			if routeConfig.To == "" {
				log.Printf("Warning: Empty 'to' URL for proxy route '%s'. Skipping registration.", routeConfig.Name)
				continue
			}
			targetURL, parseErr := url.Parse(routeConfig.To)
			if parseErr != nil {
				log.Printf("Warning: Invalid target URL '%s' for proxy route '%s': %v. Skipping registration.", routeConfig.To, routeConfig.Name, parseErr)
				continue
			}
			handler = g.createProxyHandlerFunc(routeConfig, targetURL)
		}

		// Wrap the handler with authentication if enabled for this route
		if routeConfig.Authentication.Enabled {
			handler = g.wrapWithAuth(handler, routeConfig.Authentication)
		}

		// Register the final handler
		pattern := routeConfig.From
		// Handle wildcard prefix matching for ServeMux
		// Ensure the pattern ends with "/" if it's meant to be a prefix match
		if strings.HasSuffix(pattern, "/*") {
			pattern = strings.TrimSuffix(pattern, "*") // Keep the trailing slash: /api/v1/
		}

		g.mux.HandleFunc(pattern, handler)

		// Log registration details
		if routeConfig.Static {
			log.Printf("main.go: Registered User Route  : %-25s | From: %-20s | To: %s | Auth: %t (Method: %s)", routeConfig.Name, routeConfig.From, routeConfig.ToFolder, routeConfig.Authentication.Enabled, routeConfig.Authentication.Method)
		} else {
			log.Printf("main.go: Registered User Route  : %-25s | From: %-20s | To: %s | Auth: %t (Method: %s)", routeConfig.Name, routeConfig.From, routeConfig.To, routeConfig.Authentication.Enabled, routeConfig.Authentication.Method)
		}
	}
	// Add a final catch-all 404 handler? ServeMux does this by default.
	// If you want a custom 404 page, register "/" *last* with a specific handler
	// ONLY if no other "/" route is defined in the user config.
	// Example: Check if "/" is already registered before adding:
	// if _, exists := g.mux.Handler(&http.Request{URL: &url.URL{Path: "/"}}); !exists {
	//    g.mux.HandleFunc("/", handleCustomNotFound)
	// }
	return nil
}

// configureOAuthCallbackRoute remains the same
func (g *Gateway) configureOAuthCallbackRoute() {
	g.mux.HandleFunc("/auth/callback/", func(w http.ResponseWriter, r *http.Request) {
		pathSegments := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(pathSegments) != 3 || pathSegments[0] != "auth" || pathSegments[1] != "callback" {
			log.Printf("main.go: OAuth Callback: Invalid callback path format: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		provider := pathSegments[2]
		g.authManager.HandleOAuthCallback(provider, w, r)
	})
	log.Printf("main.go: Registered OAuth Callback Handler: /auth/callback/*")
}

// --- Authentication Middleware ---

// wrapWithAuth applies the authentication check based on config.
func (g *Gateway) wrapWithAuth(next http.HandlerFunc, authConfig AuthenticationConfig) http.HandlerFunc {
	authenticator, err := g.authManager.GetAuthenticator(authConfig)
	if err != nil {
		log.Printf("FATAL CONFIG ERROR for route auth (%s/%s): %v. Blocking route.", authConfig.Method, authConfig.Provider, err)
		return func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Internal Server Error - Misconfigured Authentication", http.StatusInternalServerError)
		}
	}

	if authenticator == nil {
		// This case should ideally not happen if authConfig.Enabled is true,
		// as GetAuthenticator should have returned an error or a specific authenticator.
		// If Enabled is false, this is expected.
		if authConfig.Enabled {
			log.Printf("Warning: Authentication enabled for route but no authenticator resolved (Method: '%s', Provider: '%s'). Allowing access.", authConfig.Method, authConfig.Provider)
		}
		return next // Pass through if no authenticator needed/found
	}

	// Return the middleware handler
	return func(w http.ResponseWriter, r *http.Request) {
		// Call the authenticator's Authenticate method
		// It now returns the potentially modified request (with context)
		isAuthenticated, reqWithCtx := authenticator.Authenticate(w, r)

		if isAuthenticated {
			// If authenticated, call the next handler with the request
			// that potentially has the user context added.
			next.ServeHTTP(w, reqWithCtx)
		}
		// If !isAuthenticated, the authenticator handled the response (401/redirect)
	}
}

// --- Route Handler Creation ---
// createProxyHandlerFunc and createStaticHandlerFunc remain unchanged from the previous version.
// ... (Keep the existing createProxyHandlerFunc and createStaticHandlerFunc code here) ...
// createProxyHandlerFunc generates the core handler function for proxy routes (without auth).
func (g *Gateway) createProxyHandlerFunc(routeConfig RouteConfig, targetURL *url.URL) http.HandlerFunc {
	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req) // Base setup (scheme, host)
		// Removed unused variable originalPath

		// Apply path stripping
		if routeConfig.RemoveFromPath != "" {
			// Use TrimPrefix for safer removal only from the beginning
			if strings.HasPrefix(req.URL.Path, routeConfig.RemoveFromPath) {
				req.URL.Path = strings.TrimPrefix(req.URL.Path, routeConfig.RemoveFromPath)
				// Ensure the remaining path starts with a '/' if not empty
				if len(req.URL.Path) > 0 && !strings.HasPrefix(req.URL.Path, "/") {
					req.URL.Path = "/" + req.URL.Path
				} else if len(req.URL.Path) == 0 {
					req.URL.Path = "/" // Default to root if removing resulted in empty path
				}
			}
		}

		// Combine target base path and remaining request path
		req.URL.Path = singleJoiningSlash(targetURL.Path, req.URL.Path)
		req.URL.RawPath = req.URL.EscapedPath() // Update RawPath too

		// Set forwarded headers
		req.Header.Set("X-Forwarded-Host", req.Host)
		// Determine scheme (consider X-Forwarded-Proto if gateway is behind another proxy)
		scheme := "http"
		if req.TLS != nil || req.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		req.Header.Set("X-Forwarded-Proto", scheme)
		// Set X-Forwarded-For
		if clientIP := req.RemoteAddr; clientIP != "" {
			// If header already exists, append
			if prior, ok := req.Header["X-Forwarded-For"]; ok {
				clientIP = strings.Join(prior, ", ") + ", " + clientIP
			}
			req.Header.Set("X-Forwarded-For", clientIP)
		}

		req.Host = targetURL.Host // Set Host header to target

		// Use a less verbose log level for routine proxying?
		// log.Printf("Proxying [%s]: %s -> %s%s", routeConfig.Name, originalPath, targetURL.Scheme+"://"+targetURL.Host, req.URL.Path)
	}

	proxy.ErrorHandler = func(rw http.ResponseWriter, r *http.Request, err error) {
		log.Printf("Proxy error for route '%s' (From: %s) to %s: %v", routeConfig.Name, routeConfig.From, routeConfig.To, err)
		// Avoid writing header if already written (e.g., by proxy director panic)
		if h, ok := rw.(http.Hijacker); ok {
			_, _, hijackErr := h.Hijack()
			if hijackErr == nil {
				log.Printf("Proxy error occurred after response may have started for route '%s'. Connection hijacked.", routeConfig.Name)
				// Cannot write status code anymore.
				return
			}
		}
		// Check if header has been written (best effort)
		if !headerWritten(rw) {
			http.Error(rw, "Bad Gateway", http.StatusBadGateway)
		} else {
			log.Printf("Proxy error occurred after header write for route '%s'. Cannot send error status.", routeConfig.Name)
		}
	}

	// Return the raw proxy handler function
	return func(w http.ResponseWriter, r *http.Request) {
		proxy.ServeHTTP(w, r)
	}
}

// headerWritten checks if the response header has been written.
func headerWritten(w http.ResponseWriter) bool {
	// This remains a best-effort check. A more reliable method involves
	// wrapping the ResponseWriter. For now, assume false in ErrorHandler.
	return false
}

// createStaticHandlerFunc generates the core handler function for static routes (without auth).
func (g *Gateway) createStaticHandlerFunc(routeConfig RouteConfig, isDir bool) http.HandlerFunc {
	// routeConfig.ToFolder is already resolved/absolute here
	targetPath := routeConfig.ToFolder

	if isDir {
		// Directory serving
		fs := http.Dir(targetPath) // Serve from the resolved directory path
		fileServer := http.FileServer(fs)
		stripPrefix := routeConfig.From
		if strings.HasSuffix(stripPrefix, "/*") {
			stripPrefix = strings.TrimSuffix(stripPrefix, "*") // Keep trailing slash: /static/
		}

		var finalHandler http.Handler
		if stripPrefix == "/" {
			finalHandler = fileServer
		} else {
			finalHandler = http.StripPrefix(stripPrefix, fileServer)
		}

		return func(w http.ResponseWriter, r *http.Request) {
			// Log access *before* serving
			// log.Printf("Serving Static Dir [%s]: Request '%s' from resolved path '%s' (stripping '%s')", routeConfig.Name, r.URL.Path, targetPath, stripPrefix)

			// Prevent directory listing? Check for index.html.
			if strings.HasSuffix(r.URL.Path, "/") {
				relativePath := strings.TrimPrefix(r.URL.Path, stripPrefix)
				if !strings.HasPrefix(relativePath, "/") {
					relativePath = "/" + relativePath
				}
				cleanRelativePath := filepath.Clean(relativePath)
				indexPath := filepath.Join(targetPath, cleanRelativePath, "index.html")

				_, err := os.Stat(indexPath)
				if err != nil && os.IsNotExist(err) {
					// index.html does not exist. Disable listing explicitly.
					// log.Printf("Serving Static Dir [%s]: index.html not found at '%s'. Denying directory listing.", routeConfig.Name, indexPath)
					http.NotFound(w, r) // Return 404 instead of listing
					return
				} else if err != nil {
					log.Printf("Serving Static Dir [%s]: Error checking index.html at '%s': %v", routeConfig.Name, indexPath, err)
					// Fall through, maybe FileServer can handle it, maybe not.
				}
			}
			finalHandler.ServeHTTP(w, r)
		}

	} else {
		// Single file serving
		filePath := targetPath // Already cleaned in loadConfig

		return func(w http.ResponseWriter, r *http.Request) {
			// Check existence/type at request time
			fileInfo, err := os.Stat(filePath)
			if err != nil {
				if os.IsNotExist(err) {
					// log.Printf("Static file not found [%s]: %s for request %s", routeConfig.Name, filePath, r.URL.Path)
					http.NotFound(w, r)
				} else {
					log.Printf("Error accessing static file [%s] (%s): %v", routeConfig.Name, filePath, err)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
				return
			}
			if fileInfo.IsDir() {
				log.Printf("Configuration Error: Static route [%s] points to a directory %s but is not a directory route (/*)", routeConfig.Name, filePath)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			// log.Printf("Serving Static File [%s]: %s for request %s", routeConfig.Name, filePath, r.URL.Path)
			http.ServeFile(w, r, filePath)
		}
	}
}

// --- Utility Functions ---

// loadConfig reads, parses, and validates the YAML configuration file.
func loadConfig(filename string) (*Config, error) {
	configAbsPath, err := filepath.Abs(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for config file '%s': %w", filename, err)
	}
	log.Printf("main.go: Loading configuration from: %s", configAbsPath)

	file, err := os.Open(configAbsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file '%s': %w", filename, err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file '%s': %w", filename, err)
	}

	expandedData := os.ExpandEnv(string(data))
	config := &Config{}

	// Set defaults *before* unmarshalling
	config.Management.Prefix = "/_"                    // Default prefix
	config.Management.Health.Enabled = true            // Default enabled
	config.Management.Me.Enabled = true                // Default enabled
	config.Management.Me.Authentication.Enabled = true // Default auth enabled for /me
	config.Management.Me.Authentication.Method = "any" // Default auth method for /me

	err = yaml.Unmarshal([]byte(expandedData), config)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal config data from '%s': %w", filename, err)
	}

	// --- Post-Unmarshal Validation and Path Resolution ---
	// Validate server config
	if config.Server.Port == 0 {
		return nil, fmt.Errorf("server.port must be specified")
	}
	if config.Server.URL == "" {
		log.Printf("Warning: server.url is not set in config. OAuth redirects might not work correctly.")
	}

	// Validate management config
	if config.Management.Prefix == "" {
		log.Printf("Warning: management.prefix is empty, defaulting to '/_'.")
		config.Management.Prefix = "/_"
	}
	config.Management.Prefix = "/" + strings.Trim(config.Management.Prefix, "/") // Ensure leading/no trailing slash

	if config.Management.Me.Enabled && !config.Management.Me.Authentication.Enabled {
		log.Printf("Warning: management.me.enabled is true, but management.me.authentication.enabled is false. Forcing authentication enabled for /me.")
		config.Management.Me.Authentication.Enabled = true
	}
	if config.Management.Me.Enabled && config.Management.Me.Authentication.Method == "" {
		log.Printf("Warning: management.me.authentication.method is empty. Defaulting to 'any'.")
		config.Management.Me.Authentication.Method = "any"
	}

	// Resolve static route paths
	exePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("failed to get executable path: %w", err)
	}
	exeDir := filepath.Dir(exePath)
	log.Printf("main.go: Executable directory: %s", exeDir)

	for i := range config.Routes {
		route := &config.Routes[i]
		if route.Static {
			if route.ToFolder == "" {
				log.Printf("Warning: Static route '%s' has an empty 'toFolder'.", route.Name)
				continue
			}
			originalPath := route.ToFolder
			resolvedPath := originalPath
			if !filepath.IsAbs(originalPath) {
				resolvedPath = filepath.Join(exeDir, originalPath)
			}
			route.ToFolder = filepath.Clean(resolvedPath)
			if originalPath != route.ToFolder && !filepath.IsAbs(originalPath) {
				log.Printf("main.go: Resolved relative ToFolder for route '%s' from '%s' to '%s'", route.Name, originalPath, route.ToFolder)
			}
		}
		// Validate route 'From' path? Ensure it starts with '/'?
		if !strings.HasPrefix(route.From, "/") {
			log.Printf("Warning: Route '%s' From path '%s' does not start with '/'. Adding prefix.", route.Name, route.From)
			route.From = "/" + route.From
		}
	}

	return config, nil
}

// singleJoiningSlash remains unchanged
func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		if b == "" {
			return a
		}
		if len(b) > 0 && !strings.HasPrefix(b, "/") {
			b = "/" + b
		}
		return a + b
	case aslash && !bslash:
		return a + b
	case !aslash && bslash:
		return a + b
	}
	return a + b
}

// --- Main Function ---

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile) // Include file/line number
	log.Println("main.go: Starting API Gateway...")

	// 1. Load Configuration
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run . <path/to/config.yaml>")
		os.Exit(1)
	}
	configFilePath := os.Args[1]
	config, err := loadConfig(configFilePath)
	if err != nil {
		log.Fatalf("FATAL: Failed to load configuration: %v", err)
	}
	log.Printf("main.go: Configuration loaded successfully: %s", config.Name)

	// 2. Create Gateway Instance
	gateway, err := NewGateway(config)
	if err != nil {
		log.Fatalf("FATAL: Failed to create gateway instance: %v", err)
	}

	// 3. Start the HTTP Server
	log.Printf("main.go: API Gateway '%s' listening on %s", config.Name, gateway.server.Addr)
	log.Printf("main.go: Gateway public URL set to: %s", config.Server.URL)
	log.Printf("main.go: Management API prefix: %s", config.Management.Prefix)

	err = gateway.server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		log.Fatalf("FATAL: Failed to start server: %v", err)
	}

	log.Println("main.go: API Gateway shut down gracefully.")
}
