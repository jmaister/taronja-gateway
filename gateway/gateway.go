package gateway

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template" // Added for template parsing
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath" // Still needed for user-defined static routes from OS filesystem
	"strings"
	"time"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/handlers"
	"github.com/jmaister/taronja-gateway/middleware"
	"github.com/jmaister/taronja-gateway/providers"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/jmaister/taronja-gateway/static"
)

// --- Gateway Struct ---
type Gateway struct {
	Server         *http.Server
	GatewayConfig  *config.GatewayConfig
	Mux            *http.ServeMux
	SessionStore   session.SessionStore
	UserRepository db.UserRepository
	templates      map[string]*template.Template
	WebappEmbedFS  *embed.FS
}

// --- NewGateway Function ---

func NewGateway(config *config.GatewayConfig, webappEmbedFS *embed.FS) (*Gateway, error) {
	// Initialize the database connection
	db.Init()

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

	sessionStore := session.NewSessionStore(db.NewSessionRepositoryDB())

	userRepository := db.NewDBUserRepository(db.GetConnection())

	// Initialize and parse templates
	templates, err := parseTemplates(static.StaticAssetsFS, "login.html")
	if err != nil {
		return nil, err // Propagate error from template parsing
	}

	gateway := &Gateway{
		Server:         server,
		GatewayConfig:  config,
		Mux:            mux,
		SessionStore:   sessionStore,
		UserRepository: userRepository,
		templates:      templates,
		WebappEmbedFS:  webappEmbedFS,
	}

	// --- IMPORTANT: Register Management Routes FIRST ---
	gateway.configureManagementRoutes(static.StaticAssetsFS) // Modified to pass it

	// Configure the standard proxy/static routes (user-defined routes still use OS filesystem)
	err = gateway.configureUserRoutes()
	if err != nil {
		return nil, fmt.Errorf("error configuring user routes: %w", err)
	}

	// Configure the OAuth callback handler (can be done after user routes)
	gateway.configureOAuthCallbackRoute()

	return gateway, nil
}

// parseTemplates loads and parses HTML templates from an embedded FS.
func parseTemplates(fs embed.FS, templateNames ...string) (map[string]*template.Template, error) {
	templates := make(map[string]*template.Template)
	for _, tmplName := range templateNames {
		t := template.New(tmplName).Funcs(template.FuncMap{
			"FormatDate": handlers.FormatDate,
		})
		t, err := t.ParseFS(fs, tmplName)
		if err != nil {
			return nil, fmt.Errorf("error parsing template '%s' from embedded FS: %w", tmplName, err)
		}
		templates[tmplName] = t
		log.Printf("Successfully parsed template: %s", tmplName)
	}
	return templates, nil
}

// --- Route Configuration ---

// configureManagementRoutes sets up internal gateway endpoints
func (g *Gateway) configureManagementRoutes(staticAssetsFS embed.FS) {
	prefix := g.GatewayConfig.Management.Prefix
	log.Printf("Registering management API routes under prefix: %s", prefix)

	// Login Routes for Basic and OAuth2 Authentication
	g.registerLoginRoutes()

	// Register the static content endpoint to load assets from the provided embedded FS
	staticPath := prefix + "/static/"
	g.Mux.HandleFunc(staticPath, func(w http.ResponseWriter, r *http.Request) {
		fileServer := http.FileServer(http.FS(staticAssetsFS))
		http.StripPrefix(staticPath, fileServer).ServeHTTP(w, r)
	})

	// Register the OpenAPI routes (e.g., /_/api/)
	g.registerOpenAPIRoutes(prefix)

	// Register dashboard
	g.registerDashboard(prefix)

}

func (g *Gateway) registerDashboard(prefix string) {
	dashboardPath := prefix + "/admin/"
	// TODO: must be authenticated to access the dashboard
	// TODO: only specific users can access the dashboard, based on config/attributes/other...
	g.Mux.HandleFunc(dashboardPath, func(w http.ResponseWriter, r *http.Request) {
		// Get the path after stripping the dashboard prefix
		path := strings.TrimPrefix(r.URL.Path, dashboardPath)

		// Check if this looks like a static asset (has file extension)
		isStaticAsset := strings.Contains(path, ".") && (strings.HasSuffix(path, ".js") ||
			strings.HasSuffix(path, ".css") ||
			strings.HasSuffix(path, ".json") ||
			strings.HasSuffix(path, ".png") ||
			strings.HasSuffix(path, ".jpg") ||
			strings.HasSuffix(path, ".jpeg") ||
			strings.HasSuffix(path, ".gif") ||
			strings.HasSuffix(path, ".svg") ||
			strings.HasSuffix(path, ".ico") ||
			strings.HasSuffix(path, ".woff") ||
			strings.HasSuffix(path, ".woff2") ||
			strings.HasSuffix(path, ".ttf") ||
			strings.HasSuffix(path, ".eot"))

		var data []byte
		var err error
		var finalPath string

		if path == "" || path == "/" || !isStaticAsset {
			// Serve index.html for root requests or SPA routes (no file extension)
			finalPath = "webapp/dist/index.html"
			log.Printf("Dashboard: Serving SPA route '%s' with index.html", r.URL.Path)
		} else {
			// Try to serve the actual static asset
			finalPath = "webapp/dist/" + path
			data, err = g.WebappEmbedFS.ReadFile(finalPath)
			if err != nil {
				// Static asset not found, serve index.html for SPA routing
				log.Printf("Dashboard: Static asset not found '%s', serving index.html for SPA routing", finalPath)
				finalPath = "webapp/dist/index.html"
			} else {
				log.Printf("Dashboard: Serving static asset: %s", finalPath)
			}
		}

		// Read the final file (index.html or static asset)
		if data == nil {
			data, err = g.WebappEmbedFS.ReadFile(finalPath)
			if err != nil {
				log.Printf("Dashboard: Could not read file '%s': %v", finalPath, err)
				http.NotFound(w, r)
				return
			}
		}

		// Determine content type based on the final served file extension
		contentType := "text/html"
		if strings.HasSuffix(finalPath, ".js") {
			contentType = "application/javascript"
		} else if strings.HasSuffix(finalPath, ".css") {
			contentType = "text/css"
		} else if strings.HasSuffix(finalPath, ".json") {
			contentType = "application/json"
		} else if strings.HasSuffix(finalPath, ".png") {
			contentType = "image/png"
		} else if strings.HasSuffix(finalPath, ".jpg") || strings.HasSuffix(finalPath, ".jpeg") {
			contentType = "image/jpeg"
		} else if strings.HasSuffix(finalPath, ".gif") {
			contentType = "image/gif"
		} else if strings.HasSuffix(finalPath, ".svg") {
			contentType = "image/svg+xml"
		} else if strings.HasSuffix(finalPath, ".ico") {
			contentType = "image/x-icon"
		} else if strings.HasSuffix(finalPath, ".woff") {
			contentType = "font/woff"
		} else if strings.HasSuffix(finalPath, ".woff2") {
			contentType = "font/woff2"
		} else if strings.HasSuffix(finalPath, ".ttf") {
			contentType = "font/ttf"
		} else if strings.HasSuffix(finalPath, ".eot") {
			contentType = "application/vnd.ms-fontobject"
		}

		w.Header().Set("Content-Type", contentType)
		w.Write(data)
		log.Printf("Dashboard request served: %s -> %s", r.URL.Path, finalPath)
	})
	log.Printf("Registered Dashboard Route: %-25s | Path: %s | Auth: %t", "Dashboard", dashboardPath, true)
}

func (g *Gateway) registerOpenAPIRoutes(prefix string) {
	// --- Register OpenAPI Routes ---
	// Use the new StrictApiServer
	strictApiServer := handlers.NewStrictApiServer(
		g.SessionStore,
		g.UserRepository,
	)
	// Convert the StrictServerInterface to the standard ServerInterface

	strictSessionMiddleware := middleware.StrictSessionMiddleware(g.SessionStore, g.GatewayConfig.Management.Prefix)

	// Define custom ResponseErrorHandlerFunc
	responseErrorHandler := func(w http.ResponseWriter, r *http.Request, err error) {
		var errorWithResponse *middleware.ErrorWithResponse
		if errors.As(err, &errorWithResponse) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			responseText := "Unauthorized" // Default response text
			if errorWithResponse.Message != "" {
				responseText = errorWithResponse.Message
			}
			encodeErr := json.NewEncoder(w).Encode(api.Error{
				Code:    http.StatusUnauthorized,
				Message: responseText,
			})
			if encodeErr != nil {
				log.Printf("Error encoding %d response: %v", errorWithResponse.Code, encodeErr)
				// Fallback to plain text error if JSON encoding fails
				http.Error(w, responseText, errorWithResponse.Code)
			}
			return
		}
		// Default behavior for other errors
		log.Printf("Internal server error: %v", err) // Log the error
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}

	strictHandlerOptions := api.StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, err.Error(), http.StatusBadRequest)
		},
		ResponseErrorHandlerFunc: responseErrorHandler,
	}

	standardApiServer := api.NewStrictHandlerWithOptions(strictApiServer, []api.StrictMiddlewareFunc{
		strictSessionMiddleware,
	}, strictHandlerOptions)

	openApiHandler := api.HandlerWithOptions(standardApiServer, api.StdHTTPServerOptions{
		BaseURL: "", // Ensure BaseURL is appropriate for your setup, likely "" or "/"
		// Middlewares for the StdHTTPServerOptions are applied *after* the strict handler's processing
		// Middlewares: []api.MiddlewareFunc{},
		// ErrorHandlerFunc can be customized if needed
		Middlewares: []api.MiddlewareFunc{
			// middleware.SessionMiddlewareFunc(g.SessionStore, false, g.GatewayConfig.Management.Prefix),
		},
	})
	// Ensure the pattern ends with a trailing slash for ServeMux to correctly match subpaths
	apiPattern := prefix
	if !strings.HasSuffix(apiPattern, "/") {
		apiPattern += "/"
	}
	g.Mux.Handle(apiPattern, http.StripPrefix(strings.TrimSuffix(prefix, "/"), openApiHandler))
	log.Printf("Registered OpenAPI Routes under prefix: %s. Individual routes are not dynamically logged.", prefix)
	// --- End Register OpenAPI Routes ---
}

// registerLoginRoutes adds login routes for basic and OAuth2 authentication.
func (g *Gateway) registerLoginRoutes() {
	// Register all providers - basic, OAuth, etc.
	if g.GatewayConfig.HasAnyAuthentication() {
		// Register all authentication providers based on configuration
		providers.RegisterProviders(g.Mux, g.SessionStore, g.GatewayConfig, g.UserRepository)
	}

	// Login page handler
	loginPath := g.GatewayConfig.Management.Prefix + "/login"
	g.Mux.HandleFunc(loginPath, func(w http.ResponseWriter, r *http.Request) {
		// Populate data from config and request
		data := config.NewLoginPageData(r.URL.Query().Get("redirect"), g.GatewayConfig)

		// Retrieve the pre-parsed template from the map (parsed from embedded FS)
		loginTemplatePath := "login.html" // Key for the template map, path relative to embedded FS root
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
// This function continues to serve user-defined static routes from the OS filesystem.
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
// This function continues to serve user-defined static routes from the OS filesystem.
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

				_, statErr := os.Stat(indexPath) // Renamed err to statErr to avoid conflict
				// Corrected: os.IsNotExist returns a bool, not an error.
				// Store the boolean result in a new variable.
				isNotExist := false
				if statErr != nil {
					isNotExist = os.IsNotExist(statErr)
				}

				if isNotExist { // Use the boolean variable here
					// index.html does not exist. Disable listing explicitly.
					log.Printf("Static route [%s]: No index.html found at path: %s", routeConfig.Name, indexPath)
					http.NotFound(w, r) // Return 404 instead of listing
					return
				} else if statErr != nil { // Check original statErr for other errors
					log.Printf("Serving Static Dir [%s]: Error checking index.html at '%s': %v", routeConfig.Name, indexPath, statErr)
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
