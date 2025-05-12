package gateway

import (
	"fmt"
	"html/template" // Added for template parsing
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath" // Added for path manipulation
	"runtime"       // Added to determine file paths
	"strings"
	"time"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/handlers"
	"github.com/jmaister/taronja-gateway/middleware"
	"github.com/jmaister/taronja-gateway/providers"
	"github.com/jmaister/taronja-gateway/session"
)

// --- Gateway Struct ---
type Gateway struct {
	Server         *http.Server
	GatewayConfig  *config.GatewayConfig
	Mux            *http.ServeMux                // Exported (changed from mux)
	SessionStore   session.SessionStore          // Exported (changed from sessionStore)
	UserRepository db.UserRepository             // Added UserRepository
	projectRoot    string                        // Added to store project root path
	templates      map[string]*template.Template // Changed from loginTemplate to a map
}

// --- NewGateway Function ---

func NewGateway(config *config.GatewayConfig) (*Gateway, error) { // Removed userRepository parameter
	mux := http.NewServeMux()

	// Create server handler based on logging configuration
	var handler http.Handler = mux
	if config.Management.Logging {
		log.Printf("Request logging enabled")
		handler = middleware.LoggingMiddleware(mux)
	}

	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", config.Server.Host, config.Server.Port),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  120 * time.Second,
		Handler:      handler,
	}

	sessionStore := session.NewMemorySessionStore()

	db.Init()                                                    // Initialize the database connection
	userRepository := db.NewDBUserRepository(db.GetConnection()) // Create UserRepository instance here

	// Determine base path for static files relative to this source file
	_, currentFilePath, _, ok := runtime.Caller(0)
	if !ok {
		return nil, fmt.Errorf("could not get current file path")
	}
	// gateway.go is in gateway/ directory, so project root is one level up.
	projectRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFilePath), ".."))

	// Initialize templates map
	templates := make(map[string]*template.Template)

	// Parse the login template
	loginTemplatePathKey := "./static/login.html" // Logical key for the template map
	actualLoginTemplatePath := filepath.Join(projectRoot, "static", "login.html")

	loginTmpl, err := template.ParseFiles(actualLoginTemplatePath)
	if err != nil {
		return nil, fmt.Errorf("error parsing login template '%s' (resolved to '%s'): %w", loginTemplatePathKey, actualLoginTemplatePath, err)
	}
	templates[loginTemplatePathKey] = loginTmpl // Use the logical key

	gateway := &Gateway{
		Server:         server,
		GatewayConfig:  config,
		Mux:            mux,
		SessionStore:   sessionStore,
		UserRepository: userRepository, // Assign created UserRepository
		projectRoot:    projectRoot,    // Store the calculated project root
		templates:      templates,      // Store the map of pre-parsed templates
	}

	// --- IMPORTANT: Register Management Routes FIRST ---
	// This ensures they take precedence over potentially broad user routes like "/"
	gateway.configureManagementRoutes()

	// Configure the standard proxy/static routes
	err = gateway.configureUserRoutes()
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
	prefix := g.GatewayConfig.Management.Prefix
	log.Printf("Registering management API routes under prefix: %s", prefix)

	// Health Endpoint
	healthPath := prefix + "/health"
	g.Mux.HandleFunc(healthPath, handlers.HandleHealth)
	log.Printf("Registered Management Route: %-25s | Path: %s | Auth: %t", "Health Check", healthPath, false)

	// Me Endpoint - only register if there are auth methods configured
	if g.GatewayConfig.HasAnyAuthentication() {
		mePath := prefix + "/me"
		// Create a handler that passes the session store to HandleMe
		meHandler := func(w http.ResponseWriter, r *http.Request) {
			handlers.HandleMe(w, r, g.SessionStore)
		}
		authWrappedMeHandler := g.wrapWithAuth(meHandler, false)
		g.Mux.HandleFunc(mePath, authWrappedMeHandler)
		log.Printf("Registered Management Route: %-25s | Path: %s | Auth: %t", "User Info", mePath, true)
	} else {
		log.Printf("Skipping Me endpoint registration as no authentication methods are configured")
	}

	// Login Routes for Basic and OAuth2 Authentication
	g.registerLoginRoutes(prefix)

	// Register the static content endpoint to load assets
	staticPath := prefix + "/static/"
	g.Mux.HandleFunc(staticPath, func(w http.ResponseWriter, r *http.Request) {
		// Serve static files from the static directory, using projectRoot
		staticDir := http.Dir(filepath.Join(g.projectRoot, "static"))
		fs := http.FileServer(staticDir)
		http.StripPrefix(staticPath, fs).ServeHTTP(w, r)
	})

}

// registerLoginRoutes adds login routes for basic and OAuth2 authentication.
func (g *Gateway) registerLoginRoutes(prefix string) {
	// Register all providers - basic, OAuth, etc.
	if g.GatewayConfig.HasAnyAuthentication() {
		// Register all authentication providers based on configuration
		providers.RegisterProviders(g.Mux, g.SessionStore, g.GatewayConfig, g.UserRepository)
	}

	// Login page handler
	loginPath := prefix + "/login"
	g.Mux.HandleFunc(loginPath, func(w http.ResponseWriter, r *http.Request) {
		// Populate data from config and request
		data := config.NewLoginPageData(r.URL.Query().Get("redirect"), g.GatewayConfig)

		// Retrieve the pre-parsed template from the map
		loginTemplatePath := "./static/login.html"
		tmpl, ok := g.templates[loginTemplatePath]
		if !ok || tmpl == nil {
			log.Printf("Error: Login template '%s' not found in cache", loginTemplatePath)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Execute the template
		err := tmpl.Execute(w, data)
		if err != nil {
			log.Printf("Error executing login template: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	})
	log.Printf("Registered Management Route: %-25s | Path: %s | Auth: %t", "Login Page", loginPath, false) // Added log for login page
}

// configureUserRoutes sets up the main proxy and static file routes defined by the user.
func (g *Gateway) configureUserRoutes() error {
	log.Printf("Registering user-defined routes...")
	for _, routeConfig := range g.GatewayConfig.Routes {
		var handler http.HandlerFunc

		// Create the base handler (proxy or static)
		if routeConfig.Static {
			handler = g.createStaticHandlerFunc(routeConfig)
			if handler == nil {
				// Skip to next route if handler creation failed
				continue
			}
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
			handler = g.wrapWithAuth(handler, routeConfig.Static)
		}

		// Register the final handler
		pattern := routeConfig.From
		// Handle wildcard prefix matching for ServeMux
		if strings.HasSuffix(pattern, "/*") {
			pattern = strings.TrimSuffix(pattern, "*") // Keep the trailing slash: /api/v1/
		}

		g.Mux.HandleFunc(pattern, handler)

		// Log registration details
		if routeConfig.Static {
			if routeConfig.ToFile != "" {
				log.Printf("Registered User Route  : %-25s | From: %-20s | To: %s/%s | Auth: %t",
					routeConfig.Name, routeConfig.From, routeConfig.ToFolder, routeConfig.ToFile, routeConfig.Authentication.Enabled)
			} else {
				log.Printf("Registered User Route  : %-25s | From: %-20s | To: %s | Auth: %t",
					routeConfig.Name, routeConfig.From, routeConfig.ToFolder, routeConfig.Authentication.Enabled)
			}
		} else {
			log.Printf("Registered User Route  : %-25s | From: %-20s | To: %s | Auth: %t",
				routeConfig.Name, routeConfig.From, routeConfig.To, routeConfig.Authentication.Enabled)
		}
	}
	return nil
}

// configureOAuthCallbackRoute remains the same
func (g *Gateway) configureOAuthCallbackRoute() {
	g.Mux.HandleFunc("/auth/callback/", func(w http.ResponseWriter, r *http.Request) {
		pathSegments := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(pathSegments) != 3 || pathSegments[0] != "auth" || pathSegments[1] != "callback" {
			log.Printf("OAuth Callback: Invalid callback path format: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		//provider := pathSegments[2]

		// TODO: Check if provider is valid

		//g.authManager.HandleOAuthCallback(provider, w, r)
	})
	log.Printf("Registered OAuth Callback Handler: /auth/callback/*")
}

// --- Authentication Middleware ---

// wrapWithAuth applies the authentication check based on config.
func (g *Gateway) wrapWithAuth(next http.HandlerFunc, isStatic bool) http.HandlerFunc {
	return middleware.SessionMiddleware(next, g.SessionStore, isStatic, g.GatewayConfig.Management.Prefix)
}

// --- Route Handler Creation ---
// createProxyHandlerFunc generates the core handler function for proxy routes (without auth).
func (g *Gateway) createProxyHandlerFunc(routeConfig config.RouteConfig, targetURL *url.URL) http.HandlerFunc {
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
		log.Printf("Proxying [%s]: %s -> %s%s", routeConfig.Name, req.URL.Path, targetURL.Scheme+"://"+targetURL.Host, req.URL.Path)
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
		http.Error(rw, "Bad Gateway", http.StatusBadGateway)
	}

	// Return the raw proxy handler function
	return func(w http.ResponseWriter, r *http.Request) {
		proxy.ServeHTTP(w, r)
	}
}

// createStaticHandlerFunc generates the core handler function for static routes (without auth).
func (g *Gateway) createStaticHandlerFunc(routeConfig config.RouteConfig) http.HandlerFunc {
	var fsPath string
	var isDir bool

	// Determine path from configuration
	if routeConfig.ToFile != "" {
		// Use ToFile directly as an independent path
		fsPath = routeConfig.ToFile
	} else if routeConfig.ToFolder != "" {
		// Use ToFolder directly
		fsPath = routeConfig.ToFolder
	} else {
		log.Printf("Warning: No path specified for static route '%s'. Skipping registration.", routeConfig.Name)
		return nil
	}

	// Validate the path exists
	fileInfo, statErr := os.Stat(fsPath)
	if statErr != nil {
		log.Printf("Warning: Invalid path '%s' for static route '%s': %v. Skipping registration.", fsPath, routeConfig.Name, statErr)
		return nil
	}

	isDir = fileInfo.IsDir()

	if isDir {
		// Directory serving
		fs := http.Dir(fsPath) // Serve from the resolved directory path
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
			// Prevent directory listing? Check for index.html.
			if strings.HasSuffix(r.URL.Path, "/") {
				relativePath := strings.TrimPrefix(r.URL.Path, stripPrefix)
				if !strings.HasPrefix(relativePath, "/") {
					relativePath = "/" + relativePath
				}
				cleanRelativePath := filepath.Clean(relativePath)
				indexPath := filepath.Join(fsPath, cleanRelativePath, "index.html")

				_, err := os.Stat(indexPath)
				if err != nil && os.IsNotExist(err) {
					// index.html does not exist. Disable listing explicitly.
					log.Printf("Static route [%s]: No index.html found at path: %s", routeConfig.Name, indexPath)
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
		filePath := fsPath // Already cleaned in loadConfig

		return func(w http.ResponseWriter, r *http.Request) {
			// Check existence/type at request time
			fileInfo, err := os.Stat(filePath)
			if err != nil {
				if os.IsNotExist(err) {
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

			http.ServeFile(w, r, filePath)
		}
	}
}

// --- Utility Functions ---

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
