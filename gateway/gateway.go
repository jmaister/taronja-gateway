package gateway

import (
	"embed"
	"fmt"
	"html/template" // Added for template parsing
	"log"
	"net/http" // Still needed for user-defined static routes from OS filesystem
	"time"

	"github.com/jmaister/taronja-gateway/accounts"
	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db/repositories"
	"github.com/jmaister/taronja-gateway/gateway/deps"
	"github.com/jmaister/taronja-gateway/handlers"
	"github.com/jmaister/taronja-gateway/middleware/builtin"
	"github.com/jmaister/taronja-gateway/static"
)

// --- Gateway Struct ---
type Gateway struct {
	Server        *http.Server
	GatewayConfig *config.GatewayConfig
	Mux           *http.ServeMux
	Dependencies  *deps.Dependencies
	// Route authentication (created during gateway initialization)
	AuthMiddleware *builtin.AuthMiddleware
	// Rate limiter instance (for stats/config APIs)
	RateLimiter *builtin.RateLimiter
	// The resolved `middleware:` list, built by setup. Read by the
	// middleware status/health/metrics API and by code that depends on a
	// middleware's options.
	MiddlewareChain *builtin.GlobalChain
	// Accounts maps external logins (Google, GitHub, OpenID Connect) to users.
	Accounts      *accounts.Service
	templates     map[string]*template.Template
	authTemplates map[string]*template.Template
	WebappEmbedFS *embed.FS
	StartTime     time.Time

	handler http.Handler
}

// --- NewGatewayWithDependencies Function ---

// NewGatewayWithDependencies creates a new gateway instance with pre-initialized dependencies
func NewGatewayWithDependencies(cfg *config.GatewayConfig, webappEmbedFS *embed.FS, deps *deps.Dependencies) (*Gateway, error) {
	// Initialize templates
	templates, err := parseTemplates(static.StaticAssetsFS, "login.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse templates: %w", err)
	}

	authTemplates, err := parseAuthTemplates(static.StaticAssetsFS)
	if err != nil {
		return nil, err
	}

	gateway := &Gateway{
		Dependencies:  deps,
		templates:     templates,
		authTemplates: authTemplates,
		WebappEmbedFS: webappEmbedFS,
		StartTime:     time.Now(),
	}

	// Validates, builds the middleware chain/mux/rate limiter, registers all
	// routes, and ensures the admin user.
	if err := gateway.setup(cfg); err != nil {
		return nil, err
	}

	gateway.Server = &http.Server{
		Addr: fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		// No Read/WriteTimeout: they cap whole request bodies and responses,
		// which would cut uploads, downloads and streams proxied to backends.
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       120 * time.Second,
		Handler:           gateway.handler,
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

	return nil
}

// ensureAdminUser creates the admin user if configured
func ensureAdminUser(config *config.GatewayConfig, userRepository repositories.UserRepository) error {
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
