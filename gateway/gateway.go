package gateway

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template" // Added for template parsing
	"io"
	"log"
	"mime"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath" // Still needed for user-defined static routes from OS filesystem
	"strings"
	"sync"
	"time"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/gateway/deps"
	"github.com/jmaister/taronja-gateway/handlers"
	"github.com/jmaister/taronja-gateway/middleware"
	"github.com/jmaister/taronja-gateway/providers"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/jmaister/taronja-gateway/static"
)

// --- Gateway Struct ---
type Gateway struct {
	Server        *http.Server
	GatewayConfig *config.GatewayConfig
	Mux           *http.ServeMux
	Dependencies  *deps.Dependencies
	// RedirectServer is the plain-HTTP listener that redirects every request
	// to HTTPS on Server's port, when TLS is enabled — see gateway/tls.go.
	// nil when TLS is disabled or the redirect listener is turned off
	// (server.tls.redirectPort: 0).
	RedirectServer *http.Server
	// tlsCertReloader holds the live TLS certificate when TLS is enabled —
	// see Gateway.ReloadTLSCertificate and gateway/tls.go's certReloader.
	// nil when TLS is disabled.
	tlsCertReloader *certReloader
	// Middleware components (created during gateway initialization)
	AuthMiddleware      *middleware.AuthMiddleware
	HttpCacheMiddleware *middleware.HttpCacheMiddleware
	RouteChainBuilder   *middleware.RouteChainBuilder
	// Rate limiter instance (for stats/config APIs)
	RateLimiter *middleware.RateLimiter
	// Registry of global middleware factories, built by applyConfig. Kept
	// on the Gateway so the middleware status/health/metrics API (see
	// doc/refactor01.md Phase 3) can introspect it after startup.
	MiddlewareRegistry *middleware.MiddlewareRegistryV2
	templates          map[string]*template.Template
	WebappEmbedFS      *embed.FS
	StartTime          time.Time

	// handler is the http.Server's actual Handler — see reload.go. Every
	// field above that applyConfig swaps on a config reload (GatewayConfig,
	// Mux, RateLimiter, MiddlewareRegistry, AuthMiddleware,
	// HttpCacheMiddleware, RouteChainBuilder) is a snapshot belonging to
	// whichever generation is currently live in handler.
	handler *reloadableHandler
	// configMu guards GatewayConfig specifically — see currentConfig's doc
	// comment for why only that one field needs it.
	configMu sync.RWMutex
	// reloadMu serializes applyConfig calls (e.g. a file-watch event and a
	// SIGHUP arriving together), so two reloads can never interleave.
	reloadMu sync.Mutex
}

// --- NewGatewayWithDependencies Function ---

// NewGatewayWithDependencies creates a new gateway instance with pre-initialized dependencies
func NewGatewayWithDependencies(cfg *config.GatewayConfig, webappEmbedFS *embed.FS, deps *deps.Dependencies) (*Gateway, error) {
	// Initialize templates
	templates, err := parseTemplates(static.StaticAssetsFS, "login.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse templates: %w", err)
	}

	gateway := &Gateway{
		Dependencies:  deps,
		templates:     templates,
		WebappEmbedFS: webappEmbedFS,
		StartTime:     time.Now(),
		handler:       &reloadableHandler{},
	}

	// Validates, builds the middleware chain/mux/rate limiter, registers all
	// routes, and ensures the admin user — the same sequence a later
	// ReloadConfig runs. See applyConfig's doc comment.
	if err := gateway.applyConfig(cfg); err != nil {
		return nil, err
	}

	gateway.Server = &http.Server{
		// Host/port are fixed at construction: changing them on a reload
		// would mean rebinding the listening socket, which applyConfig
		// deliberately doesn't attempt — see ReloadConfig's doc comment.
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  120 * time.Second,
		Handler:      gateway.handler,
	}

	// TLS, like host/port, is fixed at construction — enabling/disabling it
	// or changing the cert/key paths on a reload would mean rebinding the
	// listener with a different protocol entirely, which applyConfig
	// deliberately doesn't attempt (see warnIfImmutableFieldsChanged). Only
	// the certificate's *content* hot-reloads, via ReloadTLSCertificate,
	// independent of config reload entirely — see gateway/tls.go.
	if cfg.Server.TLS.Enabled {
		reloader, err := newCertReloader(cfg.Server.TLS.CertFile, cfg.Server.TLS.KeyFile)
		if err != nil {
			return nil, err
		}
		gateway.tlsCertReloader = reloader
		gateway.Server.TLSConfig = newTLSConfig(reloader)
		gateway.RedirectServer = buildRedirectServer(cfg)
	}

	return gateway, nil
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
	prefix := g.currentConfig().Management.Prefix
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

		// A path with a dot might be a real static asset; one without is
		// certainly a client-side SPA route (e.g. /admin/users) and never
		// worth an embed.FS lookup. This used to be a fixed 12-extension
		// whitelist (.js/.css/.json/.png/...), which meant any asset type
		// Vite happened to emit that wasn't on the list — a .wasm chunk, a
		// .map source map, a .webmanifest, anything — would never even be
		// attempted below and would silently always fall back to
		// index.html instead of being served. The ReadFile attempt right
		// after this is the real, authoritative check; this is only a
		// cheap pre-filter to skip that attempt for the common case of an
		// extensionless route.
		looksLikeAsset := strings.Contains(path, ".")

		var data []byte
		var err error
		var finalPath string

		if path == "" || path == "/" || !looksLikeAsset {
			// Serve index.html for root requests or SPA routes (no file extension)
			finalPath = "webapp/dist/index.html"
		} else {
			// Try to serve the actual static asset
			finalPath = "webapp/dist/" + path
			data, err = g.WebappEmbedFS.ReadFile(finalPath)
			if err != nil {
				// Static asset not found, serve index.html for SPA routing
				finalPath = "webapp/dist/index.html"
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

		// Determine content type from the final served file's extension.
		// mime.TypeByExtension is a map lookup (O(1) and complete — every
		// extension Go/the OS knows about) replacing what used to be a
		// hand-maintained chain of up to 12 sequential HasSuffix checks
		// that only covered exactly the same 12 extensions as
		// looksLikeAsset above; any other real asset type fell through to
		// "text/html", which is actively wrong for a binary file (a
		// browser could try to render it as a page) rather than merely
		// incomplete. index.html resolves correctly through the same
		// lookup (mime.TypeByExtension(".html")), so it needs no special
		// case here.
		contentType := mime.TypeByExtension(filepath.Ext(finalPath))
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		w.Header().Set("Content-Type", contentType)
		w.Write(data)
	}

	// Wrap dashboard handler with admin session authentication
	authenticatedDashboardHandler := middleware.SessionMiddleware(dashboardHandler, g.Dependencies.SessionStore, g.Dependencies.TokenService, true, g.currentConfig().Management.Prefix, true)

	g.Mux.HandleFunc(dashboardPath, authenticatedDashboardHandler)
	log.Printf("Registered Dashboard Route: %-25s | Path: %s | Auth admin required: %t", "Dashboard", dashboardPath, true)
}

func (g *Gateway) registerOpenAPIRoutes(prefix string) {
	// --- Register OpenAPI Routes ---
	// Use the new StrictApiServer
	strictApiServer := handlers.NewStrictApiServer(
		g.Dependencies.SessionStore,
		g.Dependencies.UserRepo,
		g.Dependencies.TrafficMetricRepo,
		g.Dependencies.TokenRepo,
		g.Dependencies.CountersRepo,
		g.Dependencies.TokenService,
		g.StartTime,
		g.RateLimiter,
		g.MiddlewareRegistry,
	)
	// Convert the StrictServerInterface to the standard ServerInterface

	strictSessionMiddleware := middleware.StrictSessionMiddleware(g.Dependencies.SessionStore, g.Dependencies.TokenService, g.currentConfig().Management.Prefix, false)

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
	cfg := g.currentConfig()

	// Register all providers - basic, OAuth, etc.
	if cfg.HasAnyAuthentication() {
		// Register all authentication providers based on configuration
		providers.RegisterProviders(g.Mux, g.Dependencies.SessionStore, cfg, g.Dependencies.UserRepo)
	}

	// Login page handler
	loginPath := cfg.Management.Prefix + "/login"
	g.Mux.HandleFunc(loginPath, func(w http.ResponseWriter, r *http.Request) {
		// Populate data from config and request. Reads the *live* config
		// (not the cfg captured above at route-registration time) since this
		// closure keeps running against whichever generation is current —
		// see currentConfig's doc comment.
		data := config.NewLoginPageData(r.URL.Query().Get("redirect"), g.currentConfig())

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
	for _, routeConfig := range g.currentConfig().Routes {
		var handler http.HandlerFunc

		// Create the base handler (proxy or static)
		if routeConfig.Static {
			handler = g.createStaticHandlerFunc(routeConfig)
			if handler == nil {
				// Skip to next route if handler creation failed
				continue
			}
		} else {
			if len(routeConfig.To) == 0 {
				log.Printf("Warning: Empty 'to' URL for proxy route '%s'. Skipping registration.", routeConfig.Name)
				continue
			}
			targetURLs := make([]*url.URL, 0, len(routeConfig.To))
			var parseErr error
			for _, to := range routeConfig.To {
				targetURL, err := url.Parse(to)
				if err != nil {
					parseErr = fmt.Errorf("target %q: %w", to, err)
					break
				}
				targetURLs = append(targetURLs, targetURL)
			}
			if parseErr != nil {
				log.Printf("Warning: Invalid target URL(s) %v for proxy route '%s': %v. Skipping registration.", []string(routeConfig.To), routeConfig.Name, parseErr)
				continue
			}
			handler = g.createProxyHandlerFunc(routeConfig, targetURLs)
			if len(targetURLs) > 1 {
				log.Printf("Proxy Route [%s]: load balancing across %d backends: %s", routeConfig.Name, len(targetURLs), formatTargets(routeConfig.To))
			}
			if routeConfig.IsSPA {
				log.Printf("Proxy Route [%s]: SPA mode enabled - upstream 404s will fall back to base URL: %s", routeConfig.Name, formatTargets(routeConfig.To))
			}
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
				routeConfig.Name, routeConfig.From, formatTargets(routeConfig.To), routeConfig.Authentication.Enabled, basePattern, pattern)
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
					routeConfig.Name, routeConfig.From, formatTargets(routeConfig.To), routeConfig.Authentication.Enabled,
					routeConfig.From, patternWithSlash)
			} else {
				// For other routes, ensure the pattern ends with a slash for consistency
				if !strings.HasSuffix(pattern, "/") {
					pattern += "/"
				}
				g.Mux.HandleFunc(pattern, handler)
				log.Printf("Registered User Route  : %-25s | From: %-20s | To: %s | Auth: %t (pattern: %s)",
					routeConfig.Name, routeConfig.From, formatTargets(routeConfig.To), routeConfig.Authentication.Enabled, pattern)
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
// createProxyHandlerFunc generates the core handler function for proxy
// routes (without auth). targetURLs holds one entry for a plain
// single-backend route, or more than one for a load-balanced route (see
// config.RouteTargets) — either way, targetURLs[0] is used to build the
// base httputil.ReverseProxy and for path composition below, since
// multiple targets are assumed to be interchangeable replicas sharing the
// same path structure. Backend *selection* per request (round-robin, with
// failover to the next target if one's connection attempt fails) happens
// in proxy.Transport (see newRoundRobinTransport), not here — everything
// in this function runs once, at route-registration time, the same as
// before targetURLs could hold more than one entry.
func (g *Gateway) createProxyHandlerFunc(routeConfig config.RouteConfig, targetURLs []*url.URL) http.HandlerFunc {
	targetURL := targetURLs[0]

	// Create the proxy once when the handler is created
	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.Transport = newRoundRobinTransport(targetURLs, routeConfig.Name)

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
	}

	// Set up SPA fallback via ModifyResponse when isSPA is enabled
	if routeConfig.IsSPA {
		proxy.ModifyResponse = func(resp *http.Response) error {
			if resp.StatusCode != http.StatusNotFound {
				return nil
			}

			// resp.Request is the request roundRobinTransport actually sent —
			// for a load-balanced route that's whichever backend answered
			// this specific request, not necessarily targetURLs[0]. Reusing
			// its Scheme/Host (and targetURL.Path, the shared base path —
			// see createProxyHandlerFunc's doc comment) means the SPA
			// fallback re-fetches from the same backend that returned the
			// 404, rather than always the first configured target.
			fallbackBaseURL := &url.URL{
				Scheme: resp.Request.URL.Scheme,
				Host:   resp.Request.URL.Host,
				Path:   targetURL.Path,
			}

			log.Printf("Proxy Route [%s]: SPA fallback - upstream returned 404 for %s, fetching base URL: %s",
				routeConfig.Name, resp.Request.URL.Path, fallbackBaseURL.String())

			fallbackReq, err := http.NewRequestWithContext(resp.Request.Context(), http.MethodGet, fallbackBaseURL.String(), nil)
			if err != nil {
				log.Printf("Proxy Route [%s]: SPA fallback failed - could not create request: %v", routeConfig.Name, err)
				return nil
			}

			client := &http.Client{Timeout: 10 * time.Second}
			fallbackResp, err := client.Do(fallbackReq)
			if err != nil {
				log.Printf("Proxy Route [%s]: SPA fallback failed - could not fetch base URL: %v", routeConfig.Name, err)
				return nil
			}

			// Drain and close the original 404 response body
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()

			// Replace the 404 response with the fallback response
			resp.StatusCode = fallbackResp.StatusCode
			resp.Status = fallbackResp.Status
			for key := range resp.Header {
				delete(resp.Header, key)
			}
			for key, values := range fallbackResp.Header {
				resp.Header[key] = values
			}
			resp.Body = fallbackResp.Body
			resp.ContentLength = fallbackResp.ContentLength

			return nil
		}
	}

	// Set up error handler. This fires whenever the round trip to the
	// upstream fails (connection refused, DNS failure, timeout, etc.).
	//
	// Previously this checked rw.(http.Hijacker) and, if the assertion
	// succeeded, called Hijack() "to check whether the header was already
	// written" before deciding whether it was safe to call http.Error.
	// That check has a side effect: Hijack() unconditionally takes the raw
	// connection away from net/http, and nothing here ever wrote to or
	// closed it afterwards. Since the plain http.ResponseWriter net/http
	// hands out implements Hijacker, and neither analytics nor request
	// logging are wrapping it unless Management.Logging or .Analytics is
	// turned on (both default false), this fired on essentially every
	// upstream failure in an out-of-the-box config: the connection was
	// hijacked and then abandoned, so the client received nothing at all
	// and simply hung until its own timeout instead of a fast 502.
	//
	// http.Error's WriteHeader is safe to call even if a response was
	// already partially written (net/http logs "superfluous
	// WriteHeader call" and no-ops) — matching httputil.ReverseProxy's own
	// default ErrorHandler, which just calls rw.WriteHeader unconditionally.
	proxy.ErrorHandler = func(rw http.ResponseWriter, r *http.Request, err error) {
		log.Printf("Proxy error for route '%s' (From: %s) to %s: %v", routeConfig.Name, routeConfig.From, formatTargets(routeConfig.To), err)
		http.Error(rw, "Bad Gateway", http.StatusBadGateway)
	}

	// Return the handler function
	return func(w http.ResponseWriter, r *http.Request) {
		// For authenticated routes, extract user ID and set header
		if routeConfig.Authentication.Enabled {

			// Try to get session from request
			var sessionObject *db.Session

			// First check if session is in context
			if r.Context() != nil {
				if ctxSession, ok := r.Context().Value(session.SessionKey).(*db.Session); ok && ctxSession != nil {
					sessionObject = ctxSession
				}
			}

			// If not in context, try to get from cookie
			if sessionObject == nil {
				cookie, err := r.Cookie(session.SessionCookieName)
				if err == nil && cookie != nil {

					// Try direct lookup in memory repository for test scenarios
					if memStore, ok := g.Dependencies.SessionStore.(*session.SessionStoreDB); ok {
						session, err := memStore.Repo.FindSessionByToken(cookie.Value)
						if err == nil && session != nil {
							sessionObject = session
						} else {
							log.Printf("[auth] Session not found in repository: %v", err)
						}
					} else {
						log.Printf("[auth] SessionStore is not a SessionStoreDB, type: %T", g.Dependencies.SessionStore)
					}

					// If still not found, try normal validation
					if sessionObject == nil {
						validSession, exists := g.Dependencies.SessionStore.ValidateSession(r)
						if exists && validSession != nil {
							sessionObject = validSession
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
				}

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
			finalHandler.ServeHTTP(w, r)
		}

	} else {
		// Single file serving
		filePath := fsPath // Already cleaned in loadConfig
		log.Printf("Static Route [%s]: Setting up single file serving - filePath: %s", routeConfig.Name, filePath)

		// For single file routes, we need to handle both with and without trailing slash
		// Register the handler for both patterns to avoid redirects
		return func(w http.ResponseWriter, r *http.Request) {

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

// convertToServeMuxPattern converts a route "from" glob pattern that contains
// wildcard "*" segments into a Go 1.22+ net/http.ServeMux named-wildcard pattern.
//
// Rules:
//   - A "*" in a non-final segment becomes a named wildcard {wN} that matches
//     exactly one path segment.
//   - A "*" as the final segment is removed and a trailing slash is used instead,
//     which makes ServeMux match everything below that prefix (subtree match).
//
// Examples:
//
//	/api/boxes/*/certs        → /api/boxes/{w0}/certs
//	/a/*/b/*/c                → /a/{w0}/b/{w1}/c
//	/api/boxes/*/certs/*      → /api/boxes/{w0}/certs/
func convertToServeMuxPattern(from string) string {
	segments := strings.Split(from, "/")
	wildcardIdx := 0
	for i, seg := range segments {
		if seg == "*" {
			if i == len(segments)-1 {
				// Last segment: strip it; the trailing slash left by the join provides subtree match.
				segments[i] = ""
			} else {
				segments[i] = fmt.Sprintf("{w%d}", wildcardIdx)
				wildcardIdx++
			}
		}
	}
	return strings.Join(segments, "/")
}

// hasMiddleWildcard reports whether from contains a "*" wildcard that is not
// solely a trailing "/*" (i.e. there is at least one wildcard in a non-final
// position, or there are multiple wildcards).
func hasMiddleWildcard(from string) bool {
	if !strings.Contains(from, "*") {
		return false
	}
	// A single trailing /* is handled by the existing subtree-match code path.
	if strings.HasSuffix(from, "/*") && strings.Count(from, "*") == 1 {
		return false
	}
	return true
}

// formatTargets renders a route's config.RouteTargets for a log line — a
// single target prints as itself, unadorned, so existing single-backend
// routes' log output is unchanged; more than one prints comma-joined
// rather than Go's default "[a b]" slice format.
func formatTargets(targets config.RouteTargets) string {
	return strings.Join(targets, ", ")
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
