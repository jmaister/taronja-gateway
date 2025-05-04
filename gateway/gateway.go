package gateway

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/handlers"
	"github.com/jmaister/taronja-gateway/middleware"
	"github.com/jmaister/taronja-gateway/session"
)

// --- Gateway Struct ---
type Gateway struct {
	Server        *http.Server
	GatewayConfig *config.GatewayConfig
	Mux           *http.ServeMux       // Exported (changed from mux)
	SessionStore  session.SessionStore // Exported (changed from sessionStore)
}

// --- NewGateway Function ---

func NewGateway(config *config.GatewayConfig) (*Gateway, error) {
	mux := http.NewServeMux()

	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", config.Server.Host, config.Server.Port),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  120 * time.Second,
		Handler:      mux, // Mux will be populated later
	}

	sessionStore := session.NewMemorySessionStore()

	gateway := &Gateway{
		Server:        server,
		GatewayConfig: config,
		Mux:           mux,
		SessionStore:  sessionStore,
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
	prefix := g.GatewayConfig.Management.Prefix
	log.Printf("main.go: Registering management API routes under prefix: %s", prefix)

	// Health Endpoint
	healthPath := prefix + "/health"
	g.Mux.HandleFunc(healthPath, handlers.HandleHealth)
	log.Printf("main.go: Registered Management Route: %-25s | Path: %s | Auth: %t", "Health Check", healthPath, false)

	// Me Endpoint - only register if there are auth methods configured
	if g.GatewayConfig.HasAnyAuthentication() {
		mePath := prefix + "/me"
		authWrappedMeHandler := g.wrapWithAuth(handlers.HandleMe, false)
		g.Mux.HandleFunc(mePath, authWrappedMeHandler)
		log.Printf("main.go: Registered Management Route: %-25s | Path: %s | Auth: %t", "User Info", mePath, true)
	} else {
		log.Printf("main.go: Skipping Me endpoint registration as no authentication methods are configured")
	}

	// Login Routes for Basic and OAuth2 Authentication
	g.registerLoginRoutes(prefix)
}

// registerLoginRoutes adds login routes for basic and OAuth2 authentication.
func (g *Gateway) registerLoginRoutes(prefix string) {
	// Basic Auth Login Route
	basicLoginPath := prefix + "/auth/basic/login"
	g.Mux.HandleFunc(basicLoginPath, func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Validate username and password (replace with your own logic)
		if username == "admin" && password == "password" {
			// Generate session token
			token, err := g.SessionStore.GenerateKey()
			if err != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			// Store session
			so := session.SessionObject{
				Username: username,
				// TODO: add all fields
			}
			g.SessionStore.Set(token, so)

			// Set session token in a cookie
			http.SetCookie(w, &http.Cookie{
				Name:     session.SessionCookieName,
				Value:    token,
				Path:     "/",
				HttpOnly: true,
				Secure:   r.TLS != nil,
			})

			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Login successful"))
		} else {
			http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		}
	})
	log.Printf("main.go: Registered Login Route: %-25s | Path: %s", "Basic Auth Login", basicLoginPath)

	// OAuth2 Login and Callback Routes
	// TODO: Refactor to use a loop for all providers
	/*
		for provider, authenticator := range g.authManager.authenticators {
			if oauthAuthenticator, ok := authenticator.(*OAuth2Authenticator); ok {
				// Login Route
				loginPath := fmt.Sprintf("%s/auth/%s/login", prefix, provider)
				g.Mux.HandleFunc(loginPath, func(w http.ResponseWriter, r *http.Request) {
					oauthAuthenticator.Authenticate(w, r)
				})
				log.Printf("main.go: Registered Login Route: %-25s | Path: %s", fmt.Sprintf("%s OAuth2 Login", provider), loginPath)

				// Callback Route
				callbackPath := fmt.Sprintf("%s/auth/%s/callback", prefix, provider)
				g.Mux.HandleFunc(callbackPath, func(w http.ResponseWriter, r *http.Request) {
					g.authManager.HandleOAuthCallback(provider, w, r)
				})
				log.Printf("main.go: Registered Callback Route: %-25s | Path: %s", fmt.Sprintf("%s OAuth2 Callback", provider), callbackPath)
			}
		}
	*/

	g.Mux.HandleFunc(prefix+"/login", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./static/login.html")
	})
}

// configureUserRoutes sets up the main proxy and static file routes defined by the user.
func (g *Gateway) configureUserRoutes() error {
	log.Printf("main.go: Registering user-defined routes...")
	for _, routeConfig := range g.GatewayConfig.Routes {
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
			log.Printf("main.go: Registered User Route  : %-25s | From: %-20s | To: %s | Auth: %t", routeConfig.Name, routeConfig.From, routeConfig.ToFolder, routeConfig.Authentication.Enabled)
		} else {
			log.Printf("main.go: Registered User Route  : %-25s | From: %-20s | To: %s | Auth: %t", routeConfig.Name, routeConfig.From, routeConfig.To, routeConfig.Authentication.Enabled)
		}
	}
	return nil
}

// configureOAuthCallbackRoute remains the same
func (g *Gateway) configureOAuthCallbackRoute() {
	g.Mux.HandleFunc("/auth/callback/", func(w http.ResponseWriter, r *http.Request) {
		pathSegments := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(pathSegments) != 3 || pathSegments[0] != "auth" || pathSegments[1] != "callback" {
			log.Printf("main.go: OAuth Callback: Invalid callback path format: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		//provider := pathSegments[2]

		// TODO: Check if provider is valid

		//g.authManager.HandleOAuthCallback(provider, w, r)
	})
	log.Printf("main.go: Registered OAuth Callback Handler: /auth/callback/*")
}

// --- Authentication Middleware ---

// wrapWithAuth applies the authentication check based on config.
func (g *Gateway) wrapWithAuth(next http.HandlerFunc, isStatic bool) http.HandlerFunc {
	return middleware.SessionMiddleware(next, g.SessionStore, isStatic, g.GatewayConfig.Management.Prefix)
}

// --- Route Handler Creation ---
// createProxyHandlerFunc and createStaticHandlerFunc remain unchanged from the previous version.
// ... (Keep the existing createProxyHandlerFunc and createStaticHandlerFunc code here) ...
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
func (g *Gateway) createStaticHandlerFunc(routeConfig config.RouteConfig, isDir bool) http.HandlerFunc {
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
