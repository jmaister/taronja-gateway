package gateway

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"log"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/handlers"
	"github.com/jmaister/taronja-gateway/middleware/builtin"
	"github.com/jmaister/taronja-gateway/providers"
	"github.com/jmaister/taronja-gateway/session"
)

// configureManagementRoutes sets up internal gateway endpoints
func (g *Gateway) configureManagementRoutes(staticAssetsFS embed.FS) {
	prefix := config.ManagementPrefix
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

	// The page SMS/WhatsApp notification links open
	g.registerNotificationPage()

}

func (g *Gateway) registerDashboard(prefix string) {
	dashboardPath := prefix + "/admin/"

	// Create the dashboard handler
	dashboardHandler := func(w http.ResponseWriter, r *http.Request) {
		// Get the path after stripping the dashboard prefix
		path := strings.TrimPrefix(r.URL.Path, dashboardPath)

		// A path without a dot is a client-side SPA route; skip the embed.FS lookup.
		// The ReadFile attempt below is the authoritative check.
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

		// mime.TypeByExtension covers every asset type, including .html.
		contentType := mime.TypeByExtension(filepath.Ext(finalPath))
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		w.Header().Set("Content-Type", contentType)
		w.Write(data)
	}

	// Wrap dashboard handler with admin session authentication
	authenticatedDashboardHandler := builtin.SessionMiddleware(dashboardHandler, g.Dependencies.SessionStore, g.Dependencies.TokenService, true, config.ManagementPrefix, true)

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
		g.Dependencies.BlockedClientRepo,
		g.Dependencies.TokenService,
		g.StartTime,
		g.RateLimiter,
		g.MiddlewareChain,
		g.Dependencies.NotificationService,
	)
	metricsCfg, _ := builtin.GlobalOptions[builtin.TrafficMetricsConfig](g.MiddlewareChain, builtin.NameTrafficMetrics)
	strictApiServer.SetExcludeStaticAssets(metricsCfg.ExcludeStaticAssets)
	strictApiServer.SetFeatureFlags(g.Dependencies.FeatureFlags)
	strictApiServer.SetUserPhones(g.Dependencies.UserPhoneRepo)
	strictApiServer.SetOrganizations(g.Dependencies.OrganizationRepo)
	if g.Dependencies.Billing != nil {
		strictApiServer.SetBilling(g.Dependencies.Billing)
	}
	// Convert the StrictServerInterface to the standard ServerInterface

	strictSessionMiddleware := builtin.StrictSessionMiddleware(g.Dependencies.SessionStore, g.Dependencies.TokenService, config.ManagementPrefix, false)

	// Define custom ResponseErrorHandlerFunc
	responseErrorHandler := func(w http.ResponseWriter, r *http.Request, err error) {
		var errorWithResponse *builtin.ErrorWithResponse
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
			// middleware.SessionMiddlewareFunc(g.SessionStore, false, config.ManagementPrefix),
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
	cfg := g.GatewayConfig

	// Register all providers - basic, OAuth, etc.
	if cfg.HasAnyAuthentication() {
		// Background context: registration runs once at startup and provider goroutines
		// live until the process exits.
		providers.RegisterProviders(context.Background(), g.Mux, g.Dependencies.SessionStore, cfg, g.Dependencies.UserRepo, g.Accounts)
	}

	// Login page handler
	loginPath := config.ManagementPrefix + "/login"
	g.Mux.HandleFunc(loginPath, g.handleLoginPage)
	log.Printf("Registered Management Route: %-25s | Path: %s | Auth: %t", "Login Page", loginPath, false) // Added log for login page

	g.registerAccountRoutes()
	g.registerBillingRoutes()
}

// handleLoginPage serves /_/login. A user who is already logged in goes
// straight to ?redirect=; otherwise the login page is shown.
func (g *Gateway) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	redirect := session.SanitizeRedirectPath(query.Get("redirect"))
	// Never back to the login page itself, or it would redirect forever.
	if strings.HasPrefix(redirect, config.ManagementPrefix+"/login") {
		redirect = "/"
	}

	result := builtin.ValidateSessionFromRequest(r, g.Dependencies.SessionStore, g.Dependencies.TokenService)
	if result.IsAuthenticated && result.Session != nil {
		http.Redirect(w, r, redirect, http.StatusFound)
		return
	}

	g.renderLoginPage(w, r, http.StatusOK, loginErrors[query.Get("error")])
}

// renderLoginPage renders login.html with an optional error.
func (g *Gateway) renderLoginPage(w http.ResponseWriter, r *http.Request, status int, errorText string) {
	// Reads the live config. The redirect is sanitized so .RedirectURL is always a
	// safe same-origin path wherever the template embeds it.
	data := config.NewLoginPageData(session.SanitizeRedirectPath(r.URL.Query().Get("redirect")), g.GatewayConfig)
	data.Error = errorText
	data.Version = DisplayVersion(g.Version)

	tmpl, ok := g.templates["login.html"]
	if !ok || tmpl == nil {
		log.Printf("Error: Login template 'login.html' not found in cache")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	err := tmpl.Execute(w, data)
	if err != nil {
		log.Printf("Error executing login template: %v", err)
	}
}
