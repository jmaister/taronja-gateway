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
	"github.com/jmaister/taronja-gateway/auth"
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
	Server            *http.Server
	GatewayConfig     *config.GatewayConfig
	Mux               *http.ServeMux
	SessionStore      session.SessionStore
	UserRepository    db.UserRepository
	TrafficMetricRepo db.TrafficMetricRepository
	TokenRepository   db.TokenRepository
	TokenService      *auth.TokenService
	templates         map[string]*template.Template
	WebappEmbedFS     *embed.FS
	StartTime         time.Time
	// Middleware instances
	AuthMiddleware      *middleware.AuthMiddleware
	HttpCacheMiddleware *middleware.HttpCacheMiddleware
	RouteChainBuilder   *middleware.RouteChainBuilder
}

// --- NewGateway Function ---

func NewGateway(config *config.GatewayConfig, webappEmbedFS *embed.FS) (*Gateway, error) {
	// Initialize the database connection
	db.Init()

	// Initialize repositories and services
	deps, err := initializeDependencies(config)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize dependencies: %w", err)
	}

	// Validate middleware dependencies
	if err := middleware.ValidateAllMiddleware(deps.MiddlewareDeps); err != nil {
		return nil, fmt.Errorf("middleware validation failed: %w", err)
	}

	// Log middleware status
	middleware.LogMiddlewareStatus(config)

	// Create HTTP server with middleware chain
	server, mux, err := createHTTPServer(config, deps)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP server: %w", err)
	}

	// Initialize templates
	templates, err := parseTemplates(static.StaticAssetsFS, "login.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse templates: %w", err)
	}

	// Create gateway instance
	gateway := &Gateway{
		Server:              server,
		GatewayConfig:       config,
		Mux:                 mux,
		SessionStore:        deps.SessionStore,
		UserRepository:      deps.UserRepository,
		TrafficMetricRepo:   deps.TrafficMetricRepo,
		TokenRepository:     deps.TokenRepository,
		TokenService:        deps.TokenService,
		templates:           templates,
		WebappEmbedFS:       webappEmbedFS,
		StartTime:           time.Now(),
		AuthMiddleware:      deps.AuthMiddleware,
		HttpCacheMiddleware: deps.HttpCacheMiddleware,
		RouteChainBuilder:   deps.RouteChainBuilder,
	}

	// Configure routes
	if err := configureRoutes(gateway); err != nil {
		return nil, fmt.Errorf("failed to configure routes: %w", err)
	}

	// Ensure admin user exists if configured
	if err := ensureAdminUser(config, deps.UserRepository); err != nil {
		return nil, fmt.Errorf("failed to ensure admin user: %w", err)
	}

	return gateway, nil
}

// GatewayDependencies holds all the dependencies needed for gateway initialization
type GatewayDependencies struct {
	SessionStore        session.SessionStore
	UserRepository      db.UserRepository
	TrafficMetricRepo   db.TrafficMetricRepository
	TokenRepository     db.TokenRepository
	TokenService        *auth.TokenService
	AuthMiddleware      *middleware.AuthMiddleware
	HttpCacheMiddleware *middleware.HttpCacheMiddleware
	RouteChainBuilder   *middleware.RouteChainBuilder
	MiddlewareDeps      *middleware.Dependencies
}

// initializeDependencies creates all repositories, services, and middleware instances
func initializeDependencies(config *config.GatewayConfig) (*GatewayDependencies, error) {
	// Initialize repositories
	sessionStore := session.NewSessionStore(db.NewSessionRepositoryDB())
	userRepository := db.NewDBUserRepository(db.GetConnection())
	trafficMetricRepo := db.NewTrafficMetricRepository(db.GetConnection())
	tokenRepository := db.NewTokenRepositoryDB(db.GetConnection())

	// Initialize services
	tokenService := auth.NewTokenService(tokenRepository, userRepository)

	// Initialize middleware instances
	authMiddleware := middleware.NewAuthMiddleware(sessionStore, tokenService, config.Management.Prefix)
	cacheMiddleware := middleware.NewHttpCacheMiddleware()
	routeChainBuilder := middleware.NewRouteChainBuilder(authMiddleware, cacheMiddleware)

	// Create middleware dependencies for validation
	middlewareDeps := &middleware.Dependencies{
		SessionStore:      sessionStore,
		TokenService:      tokenService,
		UserRepository:    userRepository,
		TrafficMetricRepo: trafficMetricRepo,
		TokenRepository:   tokenRepository,
		GatewayConfig:     config,
	}

	return &GatewayDependencies{
		SessionStore:        sessionStore,
		UserRepository:      userRepository,
		TrafficMetricRepo:   trafficMetricRepo,
		TokenRepository:     tokenRepository,
		TokenService:        tokenService,
		AuthMiddleware:      authMiddleware,
		HttpCacheMiddleware: cacheMiddleware,
		RouteChainBuilder:   routeChainBuilder,
		MiddlewareDeps:      middlewareDeps,
	}, nil
}

// createHTTPServer creates the HTTP server with middleware chain
func createHTTPServer(config *config.GatewayConfig, deps *GatewayDependencies) (*http.Server, *http.ServeMux, error) {
	mux := http.NewServeMux()

	// Build the global middleware chain
	globalChain := middleware.BuildGlobalChain(config, deps.SessionStore, deps.TokenService, deps.TrafficMetricRepo)
	handler := globalChain.Build(mux)

	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", config.Server.Host, config.Server.Port),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  120 * time.Second,
		Handler:      handler,
	}

	return server, mux, nil
}

// configureRoutes sets up all the gateway routes
func configureRoutes(gateway *Gateway) error {
	// Register Management Routes FIRST
	gateway.configureManagementRoutes(static.StaticAssetsFS)

	// Configure the standard proxy/static routes
	if err := gateway.configureUserRoutes(); err != nil {
		return fmt.Errorf("error configuring user routes: %w", err)
	}

	// Configure the OAuth callback handler
	gateway.configureOAuthCallbackRoute()

	return nil
}

// ensureAdminUser creates the admin user if configured
func ensureAdminUser(config *config.GatewayConfig, userRepository db.UserRepository) error {
	if !config.Management.Admin.Enabled {
		return nil
	}

	err := userRepository.EnsureAdminUser(
		config.Management.Admin.Username,
		config.Management.Admin.Email,
		config.Management.Admin.Password,
	)
	if err != nil {
		return fmt.Errorf("error ensuring admin user exists: %w", err)
	}

	log.Printf("Admin user ensured in database: %s", config.Management.Admin.Username)
	return nil
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

	// Create the dashboard handler
	dashboardHandler := func(w http.ResponseWriter, r *http.Request) {
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
	}

	// Wrap dashboard handler with admin session authentication
	authenticatedDashboardHandler := middleware.SessionMiddleware(dashboardHandler, g.SessionStore, g.TokenService, true, g.GatewayConfig.Management.Prefix, true)

	g.Mux.HandleFunc(dashboardPath, authenticatedDashboardHandler)
	log.Printf("Registered Dashboard Route: %-25s | Path: %s | Auth admin required: %t", "Dashboard", dashboardPath, true)
}

func (g *Gateway) registerOpenAPIRoutes(prefix string) {
	// --- Register OpenAPI Routes ---
	// Use the new StrictApiServer
	strictApiServer := handlers.NewStrictApiServer(
		g.SessionStore,
		g.UserRepository,
		g.TrafficMetricRepo,
		g.TokenRepository,
		g.TokenService,
		g.StartTime,
	)
	// Convert the StrictServerInterface to the standard ServerInterface

	strictSessionMiddleware := middleware.StrictSessionMiddleware(g.SessionStore, g.TokenService, g.GatewayConfig.Management.Prefix, false)

	// Define custom ResponseErrorHandlerFunc
	responseErrorHandler := func(w http.ResponseWriter, r *http.Request, err error) {
		var errorWithResponse *middleware.ErrorWithResponse
		if errors.As(err, &errorWithResponse) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(errorWithResponse.Code)
			responseText := errorWithResponse.Message
			if responseText == "" {
				responseText = "Error" // Default response text
			}
			encodeErr := json.NewEncoder(w).Encode(api.Error{
				Code:    errorWithResponse.Code,
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

		// Wrap with cache control for all routes
		handler = g.RouteChainBuilder.BuildRouteChain(handler, routeConfig)

		// Register the final handler for routeConfig.From
		// To match /api with /api/hello and /api/foo, the pattern must be "/api/"
		pattern := routeConfig.From
		if strings.HasSuffix(pattern, "/*") {
			// Register both the base and wildcard pattern for ServeMux
			basePattern := strings.TrimSuffix(pattern, "*")
			g.Mux.HandleFunc(basePattern, handler)
			log.Printf("Registered User Route  : %-25s | From: %-20s | To: %s | Auth: %t (patterns: %s, %s)",
				routeConfig.Name, routeConfig.From, routeConfig.To, routeConfig.Authentication.Enabled, basePattern, pattern)
		} else {
			// For static file routes, register both with and without trailing slash to avoid redirects
			if routeConfig.Static && routeConfig.ToFile != "" {
				// Register without trailing slash
				g.Mux.HandleFunc(routeConfig.From, handler)

				// Also register with trailing slash
				patternWithSlash := routeConfig.From
				if !strings.HasSuffix(patternWithSlash, "/") {
					patternWithSlash += "/"
				}
				g.Mux.HandleFunc(patternWithSlash, handler)

				log.Printf("Registered User Route  : %-25s | From: %-20s | To: %s | Auth: %t (patterns: %s, %s)",
					routeConfig.Name, routeConfig.From, routeConfig.To, routeConfig.Authentication.Enabled,
					routeConfig.From, patternWithSlash)
			} else {
				// For other routes, ensure the pattern ends with a slash for consistency
				if !strings.HasSuffix(pattern, "/") {
					pattern += "/"
				}
				g.Mux.HandleFunc(pattern, handler)
				log.Printf("Registered User Route  : %-25s | From: %-20s | To: %s | Auth: %t (pattern: %s)",
					routeConfig.Name, routeConfig.From, routeConfig.To, routeConfig.Authentication.Enabled, pattern)
			}
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

// --- Route Handler Creation ---
// createProxyHandlerFunc generates the core handler function for proxy routes (without auth).
func (g *Gateway) createProxyHandlerFunc(routeConfig config.RouteConfig, targetURL *url.URL) http.HandlerFunc {
	// Create the proxy once when the handler is created
	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	// Store the original director
	originalDirector := proxy.Director

	// Create a custom director that modifies the request
	proxy.Director = func(req *http.Request) {
		// Call the original director first
		originalDirector(req)

		// Apply path stripping and join logic
		if routeConfig.RemoveFromPath != "" {
			if strings.HasPrefix(req.URL.Path, routeConfig.RemoveFromPath) {
				req.URL.Path = strings.TrimPrefix(req.URL.Path, routeConfig.RemoveFromPath)

				if len(req.URL.Path) > 0 && !strings.HasPrefix(req.URL.Path, "/") {
					req.URL.Path = "/" + req.URL.Path
				} else if len(req.URL.Path) == 0 {
					req.URL.Path = "/"
				}

				// When using RemoveFromPath, we should NOT join with targetURL.Path
				// as that would reintroduce the prefix we just removed
				req.URL.RawPath = req.URL.EscapedPath()
			} else {
				req.URL.Path = singleJoiningSlash(targetURL.Path, req.URL.Path)
				req.URL.RawPath = req.URL.EscapedPath()
			}
		} else {
			// No RemoveFromPath, just join as before
			req.URL.Path = singleJoiningSlash(targetURL.Path, req.URL.Path)
			req.URL.RawPath = req.URL.EscapedPath()
		}

		// Set forwarded headers
		req.Header.Set("X-Forwarded-Host", req.Host)
		scheme := "http"
		if req.TLS != nil || req.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		req.Header.Set("X-Forwarded-Proto", scheme)
		if clientIP := req.RemoteAddr; clientIP != "" {
			if prior, ok := req.Header["X-Forwarded-For"]; ok {
				clientIP = strings.Join(prior, ", ") + ", " + clientIP
			}
			req.Header.Set("X-Forwarded-For", clientIP)
		}

		req.Host = targetURL.Host

		log.Printf("Proxying [%s]: %s -> %s%s", routeConfig.Name, req.URL.Path, targetURL.Scheme+"://"+targetURL.Host, req.URL.Path)
	}

	// Set up error handler
	proxy.ErrorHandler = func(rw http.ResponseWriter, r *http.Request, err error) {
		log.Printf("Proxy error for route '%s' (From: %s) to %s: %v", routeConfig.Name, routeConfig.From, routeConfig.To, err)
		// Avoid writing header if already written
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

	// Return the handler function
	return func(w http.ResponseWriter, r *http.Request) {
		// For authenticated routes, extract user ID and set header
		if routeConfig.Authentication.Enabled {
			log.Printf("[auth] Handling authenticated route %s", routeConfig.Name)

			// Try to get session from request
			var sessionObject *db.Session

			// First check if session is in context
			if r.Context() != nil {
				if ctxSession, ok := r.Context().Value(session.SessionKey).(*db.Session); ok && ctxSession != nil {
					log.Printf("[auth] Found session in context: %+v", ctxSession)
					sessionObject = ctxSession
				}
			}

			// If not in context, try to get from cookie
			if sessionObject == nil {
				cookie, err := r.Cookie(session.SessionCookieName)
				if err == nil && cookie != nil {
					log.Printf("[auth] Found session cookie: %s", cookie.Value)

					// Try direct lookup in memory repository for test scenarios
					if memStore, ok := g.SessionStore.(*session.SessionStoreDB); ok {
						log.Printf("[auth] Using SessionStoreDB repository to find session")
						session, err := memStore.Repo.FindSessionByToken(cookie.Value)
						if err == nil && session != nil {
							log.Printf("[auth] Found session in repository: %+v", session)
							sessionObject = session
						} else {
							log.Printf("[auth] Session not found in repository: %v", err)
						}
					} else {
						log.Printf("[auth] SessionStore is not a SessionStoreDB, type: %T", g.SessionStore)
					}

					// If still not found, try normal validation
					if sessionObject == nil {
						validSession, exists := g.SessionStore.ValidateSession(r)
						if exists && validSession != nil {
							log.Printf("[auth] Validated session from cookie: %+v", validSession)
							sessionObject = validSession
						} else {
							log.Printf("[auth] Failed to validate session from cookie")
						}
					}
				} else {
					log.Printf("[auth] No session cookie found: %v", err)
				}
			}

			if sessionObject != nil && sessionObject.UserID != "" {
				// Modify the original request directly using header constants
				r.Header.Set(session.UserIdHeader, sessionObject.UserID)
				// Set X-User-Data header with serialized session object (JSON)
				sessionJson, err := json.Marshal(sessionObject)
				if err == nil {
					r.Header.Set(session.UserDataHeader, string(sessionJson))
					log.Printf("[auth] Setting %s header for user %s: %s", session.UserDataHeader, sessionObject.UserID, string(sessionJson))
				} else {
					log.Printf("[auth] Failed to serialize session object for user %s: %v", sessionObject.UserID, err)
				}
				log.Printf("[auth] Setting %s header to: %s for route %s", session.UserIdHeader, sessionObject.UserID, routeConfig.Name)

				// Serve the request with the modified headers
				proxy.ServeHTTP(w, r)
				return
			}
			log.Printf("[auth] No valid session found for authenticated route %s", routeConfig.Name)
		}

		// For non-authenticated routes or if no session found
		proxy.ServeHTTP(w, r)
	}
}

// createStaticHandlerFunc generates the core handler function for static routes (without auth).
// This function continues to serve user-defined static routes from the OS filesystem.
func (g *Gateway) createStaticHandlerFunc(routeConfig config.RouteConfig) http.HandlerFunc {
	var fsPath string
	var isDir bool

	log.Printf("Static Route [%s]: Creating handler for route '%s'", routeConfig.Name, routeConfig.From)

	// Determine path from configuration
	if routeConfig.ToFile != "" {
		// Use ToFile directly as an independent path
		fsPath = routeConfig.ToFile
		log.Printf("Static Route [%s]: Using ToFile path: %s", routeConfig.Name, fsPath)
	} else if routeConfig.ToFolder != "" {
		// Use ToFolder directly
		fsPath = routeConfig.ToFolder
		log.Printf("Static Route [%s]: Using ToFolder path: %s", routeConfig.Name, fsPath)
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
	log.Printf("Static Route [%s]: Path '%s' is directory: %t", routeConfig.Name, fsPath, isDir)

	// Check if removeFromPath is used with static routes (not applicable)
	if routeConfig.RemoveFromPath != "" {
		log.Printf("Warning: Static Route [%s]: 'removeFromPath' field (%s) is not applicable to static routes and will be ignored. This field is only used for proxy routes.",
			routeConfig.Name, routeConfig.RemoveFromPath)
	}

	if isDir {
		// Directory serving
		fs := http.Dir(fsPath) // Serve from the resolved directory path
		fileServer := http.FileServer(fs)

		// For static routes, determine if we should strip the route prefix
		routePrefix := routeConfig.From
		if strings.HasSuffix(routePrefix, "/*") {
			routePrefix = strings.TrimSuffix(routePrefix, "*") // Keep trailing slash: /dashboard/
		}

		// Check if the target directory contains subdirectories that match the route prefix
		// This helps decide whether to preserve the full URL path or strip the prefix
		shouldPreserveFullPath := false
		if routePrefix != "/" && len(routePrefix) > 1 {
			// Extract the first path component from the route prefix
			trimmedPrefix := strings.Trim(routePrefix, "/")
			firstComponent := strings.Split(trimmedPrefix, "/")[0]

			// Check if a subdirectory with this name exists in the target folder
			potentialSubdir := filepath.Join(fsPath, firstComponent)
			if stat, err := os.Stat(potentialSubdir); err == nil && stat.IsDir() {
				shouldPreserveFullPath = true
				log.Printf("Static Route [%s]: Found matching subdirectory '%s', preserving full URL path", routeConfig.Name, firstComponent)
			}
		}

		log.Printf("Static Route [%s]: Setting up directory serving - fsPath: %s, From: %s, routePrefix: %s, preserveFullPath: %t",
			routeConfig.Name, fsPath, routeConfig.From, routePrefix, shouldPreserveFullPath)

		// Choose handler based on whether to preserve full path
		var finalHandler http.Handler
		if routePrefix == "/" || shouldPreserveFullPath {
			finalHandler = fileServer
			log.Printf("Static Route [%s]: Using direct file server handler (preserving full URL path)", routeConfig.Name)
		} else {
			finalHandler = http.StripPrefix(routePrefix, fileServer)
			log.Printf("Static Route [%s]: Using StripPrefix handler with prefix: %s", routeConfig.Name, routePrefix)
		}

		// Wrap with SPA handler if needed
		if routeConfig.IsSPA {
			finalHandler = g.createSPAHandler(finalHandler, fsPath, routeConfig)
			log.Printf("Static Route [%s]: Wrapped with SPA handler", routeConfig.Name)
		}

		return func(w http.ResponseWriter, r *http.Request) {
			log.Printf("Static Route [%s]: Request received - URL: %s, RemoteAddr: %s", routeConfig.Name, r.URL.Path, r.RemoteAddr)
			log.Printf("Static Route [%s]: Route config - From: %s, ToFolder: %s, RemoveFromPath: %s, routePrefix: %s, preserveFullPath: %t, isSPA: %t",
				routeConfig.Name, routeConfig.From, routeConfig.ToFolder, routeConfig.RemoveFromPath, routePrefix, shouldPreserveFullPath, routeConfig.IsSPA)

			finalHandler.ServeHTTP(w, r)
		}

	} else {
		// Single file serving
		filePath := fsPath // Already cleaned in loadConfig
		log.Printf("Static Route [%s]: Setting up single file serving - filePath: %s", routeConfig.Name, filePath)

		// For single file routes, we need to handle both with and without trailing slash
		// Register the handler for both patterns to avoid redirects
		return func(w http.ResponseWriter, r *http.Request) {
			log.Printf("Static Route [%s]: Single file request received - URL: %s, serving file: %s, RemoteAddr: %s",
				routeConfig.Name, r.URL.Path, filePath, r.RemoteAddr)

			// Check existence/type at request time
			fileInfo, err := os.Stat(filePath)
			if err != nil {
				if os.IsNotExist(err) {
					log.Printf("Static Route [%s]: File not found: %s - returning 404", routeConfig.Name, filePath)
					http.NotFound(w, r)
				} else {
					log.Printf("Static Route [%s]: Error accessing static file (%s): %v - returning 500", routeConfig.Name, filePath, err)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
				return
			}
			if fileInfo.IsDir() {
				log.Printf("Static Route [%s]: Configuration Error - path points to directory %s but route is not configured for directory serving (/*)", routeConfig.Name, filePath)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			log.Printf("Static Route [%s]: Successfully serving file: %s (size: %d bytes)", routeConfig.Name, filePath, fileInfo.Size())
			http.ServeFile(w, r, filePath)
		}
	}
}

// createSPAHandler wraps a file server handler with SPA (Single Page Application) routing logic.
// When a file is not found (404), it serves the index.html from the root of the static folder.
func (g *Gateway) createSPAHandler(handler http.Handler, fsPath string, routeConfig config.RouteConfig) http.Handler {
	return &spaHandler{
		handler:     handler,
		fsPath:      fsPath,
		routeConfig: routeConfig,
	}
}

// spaHandler implements http.Handler and provides SPA routing functionality
type spaHandler struct {
	handler     http.Handler
	fsPath      string
	routeConfig config.RouteConfig
}

func (s *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Create a custom ResponseWriter to capture 404 errors
	recorder := &spaResponseRecorder{
		ResponseWriter: w,
		status:         200, // Default to 200
	}

	// Let the original handler process the request
	s.handler.ServeHTTP(recorder, r)

	// If we got a 404 and this is a SPA route, serve index.html instead
	if recorder.status == 404 && !recorder.responseWritten {
		indexPath := filepath.Join(s.fsPath, "index.html")

		// Check if index.html exists
		if _, err := os.Stat(indexPath); err == nil {
			log.Printf("Static Route [%s]: SPA fallback - File not found, serving index.html: %s", s.routeConfig.Name, indexPath)

			// Clear any headers and set appropriate content type
			for key := range w.Header() {
				w.Header().Del(key)
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")

			// Serve the index.html file directly (this will handle status code)
			http.ServeFile(w, r, indexPath)
			return
		} else {
			log.Printf("Static Route [%s]: SPA fallback failed - index.html not found at: %s", s.routeConfig.Name, indexPath)
			// Write the 404 response if index.html doesn't exist
			w.WriteHeader(404)
			return
		}
	}

	// If not a 404 or response was already written, write the captured response
	if recorder.responseWritten {
		return // Response already sent
	}

	// Write the status and data if response wasn't written yet
	w.WriteHeader(recorder.status)
	if len(recorder.data) > 0 {
		w.Write(recorder.data)
	}
}

// spaResponseRecorder is a custom ResponseWriter that captures the status code
type spaResponseRecorder struct {
	http.ResponseWriter
	status          int
	wroteHeader     bool
	responseWritten bool
	data            []byte
}

func (r *spaResponseRecorder) WriteHeader(status int) {
	if !r.wroteHeader {
		r.status = status
		r.wroteHeader = true
		// Don't write to the underlying response yet
	}
}

func (r *spaResponseRecorder) Write(data []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(200)
	}
	// Capture the data instead of writing it immediately
	r.data = append(r.data, data...)
	return len(data), nil
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
