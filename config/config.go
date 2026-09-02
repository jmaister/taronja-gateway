package config

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jmaister/taronja-gateway/encryption"
	yaml "gopkg.in/yaml.v3"
)

// --- Configuration Structs ---

// ServerConfig defines the gateway server's network configuration.
// All fields are required.
type ServerConfig struct {
	Host string    `yaml:"host"`          // Server bind address (e.g., "127.0.0.1" for localhost only, "0.0.0.0" for all interfaces)
	Port int       `yaml:"port"`          // Server port number (e.g., 8080). Required. The HTTPS port when tls.enabled is true.
	URL  string    `yaml:"url"`           // Full external URL for OAuth redirects (e.g., "https://example.com" or "http://localhost:8080")
	TLS  TLSConfig `yaml:"tls,omitempty"` // HTTPS termination. Optional; disabled by default (plain HTTP).
}

// AuthenticationConfig controls whether authentication is required for a specific route.
type AuthenticationConfig struct {
	Enabled bool `yaml:"enabled"` // Enable authentication requirement for this route. Default: false
}

// RouteOptions contains additional optional configuration for individual routes.
type RouteOptions struct {
	CacheControlSeconds *int `yaml:"cacheControlSeconds,omitempty"` // Cache control in seconds. Optional. nil = no cache header, 0 = "no-cache", >0 = "max-age=N"
}

// RouteTargets is one or more backend URLs a proxy route sends requests to.
// A `to:` field accepts either a single scalar string (the original,
// still-most-common form: one backend, no load balancing) or a YAML list
// (multiple backends: the gateway round-robins across them per request and
// fails over to the next one if a backend's connection attempt fails — see
// gateway.newRoundRobinTransport). Both forms unmarshal into this same
// []string-backed type, so every existing single-backend config keeps
// working unchanged.
//
// Multiple targets are assumed to be interchangeable replicas of the same
// backend (same path structure, differing only in scheme/host) — this is
// what "load balancing" means here, not a way to route different paths to
// different places. Use separate route entries (with different `from:`
// patterns) for that instead.
type RouteTargets []string

// UnmarshalYAML implements custom decoding so `to:` accepts a bare string
// or a list interchangeably. yaml.v3 calls this with the value node for the
// `to:` key itself (not the whole route mapping), so Kind is always either
// ScalarNode (a string) or SequenceNode (a list) for valid input.
func (t *RouteTargets) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		var single string
		if err := value.Decode(&single); err != nil {
			return err
		}
		if single == "" {
			*t = nil
			return nil
		}
		*t = RouteTargets{single}
		return nil
	case yaml.SequenceNode:
		var list []string
		if err := value.Decode(&list); err != nil {
			return err
		}
		*t = RouteTargets(list)
		return nil
	default:
		return fmt.Errorf("line %d: 'to' must be a string or a list of strings", value.Line)
	}
}

// RouteConfig defines a single routing rule for the gateway.
// Routes can proxy to remote servers or serve static files.
type RouteConfig struct {
	Name           string               `yaml:"name"`              // Human-readable route name for logging. Required.
	From           string               `yaml:"from"`              // Incoming request path pattern (e.g., "/api/*", "/"). Must start with "/". Required.
	To             RouteTargets         `yaml:"to,omitempty"`      // Target URL(s) for proxying (e.g., "https://api.example.com", or a list of them for load balancing). Required for proxy routes. See RouteTargets.
	ToFolder       string               `yaml:"toFolder"`          // Local folder path for static content. Mutually exclusive with ToFile. Required if Static=true and ToFile not set.
	ToFile         string               `yaml:"toFile"`            // Specific file path for static content. Mutually exclusive with ToFolder. Optional.
	Static         bool                 `yaml:"static"`            // Enable static file serving. Default: false
	IsSPA          bool                 `yaml:"isSPA"`             // Enable SPA mode. For static routes: serves index.html on 404. For proxy routes: re-requests the upstream base URL on 404. Default: false
	RemoveFromPath string               `yaml:"removeFromPath"`    // Path prefix to remove before proxying (e.g., "/api/v1/"). Optional.
	Authentication AuthenticationConfig `yaml:"authentication"`    // Authentication requirements for this route
	Options        *RouteOptions        `yaml:"options,omitempty"` // Additional route options (cache control, etc.). Optional.
}

// AuthProviderCredentials contains OAuth2 provider credentials.
// Required for OAuth2 authentication providers (Google, GitHub).
type AuthProviderCredentials struct {
	ClientId     string `yaml:"clientId"`     // OAuth2 client ID from provider. Can use environment variables (e.g., ${GOOGLE_CLIENT_ID})
	ClientSecret string `yaml:"clientSecret"` // OAuth2 client secret from provider. Can use environment variables (e.g., ${GOOGLE_CLIENT_SECRET})
}

// BasicAuthenticationConfig controls basic authentication provider.
type BasicAuthenticationConfig struct {
	Enabled bool `yaml:"enabled"` // Enable basic (username/password) authentication. Default: false
}

// AuthenticationProviders defines all available authentication methods.
// At least one provider should be enabled if authentication is required on any route.
type AuthenticationProviders struct {
	Basic  BasicAuthenticationConfig `yaml:"basic"`  // Basic username/password authentication
	Google AuthProviderCredentials   `yaml:"google"` // Google OAuth2 authentication. Optional.
	Github AuthProviderCredentials   `yaml:"github"` // GitHub OAuth2 authentication. Optional.
}

// PrintOAuthCallbackURLs prints the OAuth callback URLs for configured providers.
func (a *AuthenticationProviders) PrintOAuthCallbackURLs(serverURL, managementPrefix string) {
	if a.Google.ClientId != "" && a.Google.ClientSecret != "" {
		googleCallback := fmt.Sprintf("%s%s/auth/google/callback", serverURL, managementPrefix)
		fmt.Println("[OAUTH] Google callback URL:")
		fmt.Println("   ", googleCallback)
	}
	if a.Github.ClientId != "" && a.Github.ClientSecret != "" {
		githubCallback := fmt.Sprintf("%s%s/auth/github/callback", serverURL, managementPrefix)
		fmt.Println("[OAUTH] GitHub callback URL:")
		fmt.Println("   ", githubCallback)
	}
}

// BrandingConfig contains visual customization options for the gateway UI.
type BrandingConfig struct {
	LogoUrl string `yaml:"logoUrl,omitempty"` // URL or path to custom logo image for login page. Optional.
}

// NotificationConfig defines notification system settings.
type NotificationConfig struct {
	Email struct {
		Enabled bool `yaml:"enabled"` // Enable email notifications. Default: false
		SMTP    struct {
			Host     string `yaml:"host"`     // SMTP server hostname (e.g., "smtp.gmail.com")
			Port     int    `yaml:"port"`     // SMTP server port (e.g., 587 for TLS, 465 for SSL)
			Username string `yaml:"username"` // SMTP authentication username. Can use environment variables.
			Password string `yaml:"password"` // SMTP authentication password. Can use environment variables.
			From     string `yaml:"from"`     // From email address. Can use environment variables.
			FromName string `yaml:"fromName"` // From display name. Can use environment variables.
		} `yaml:"smtp"`
	} `yaml:"email"`
}

// AdminConfig configures administrative access to the management dashboard.
// When enabled, allows a single admin user to access the dashboard at <management.prefix>/admin/
type AdminConfig struct {
	Enabled  bool   `yaml:"enabled"`  // Enable admin dashboard access. Default: false. Required if accessing dashboard.
	Username string `yaml:"username"` // Admin username for login. Required if Enabled=true.
	Password string `yaml:"password"` // Admin password (will be automatically hashed). Required if Enabled=true.
	Email    string `yaml:"email"`    // Admin email address for notifications. Optional.
}

// SessionConfig defines session lifetime for authenticated users.
type SessionConfig struct {
	SecondsDuration int `yaml:"secondsDuration"` // Session duration in seconds. Default: 86400 (24 hours). After this time, users must re-authenticate.
}

func (s *SessionConfig) GetDuration() time.Duration {
	return time.Duration(s.SecondsDuration) * time.Second
}

// ManagementConfig defines the management API and dashboard settings.
// The management API provides endpoints for metrics, user management, and admin dashboard.
type ManagementConfig struct {
	Prefix              string            `yaml:"prefix"`              // URL prefix for management endpoints. Default: "/_". All management endpoints will be under this prefix.
	Logging             bool              `yaml:"logging"`             // Enable request/response logging. Default: false. Logs all HTTP requests.
	Compression         bool              `yaml:"compression"`         // Enable gzip/deflate response compression, negotiated per-request via the client's Accept-Encoding header. Default: false. Has no other options — see middleware.CompressionMiddleware.
	Analytics           bool              `yaml:"analytics"`           // Enable traffic analytics and metrics collection. Default: false. Stores request data for dashboard.
	ExcludeStaticAssets bool              `yaml:"excludeStaticAssets"` // Skip traffic-metrics collection for requests to static assets (by extension/path, see middleware.IsStaticAssetPath). Default: false, so existing configs keep recording everything Analytics already did. Has no effect when Analytics is false. Reduces per-request overhead and stats-table volume on asset-heavy sites; rows already recorded before this is enabled are unaffected and stay filterable by "is it a static asset" in the request-details report.
	Admin               AdminConfig       `yaml:"admin"`               // Admin dashboard access configuration
	Session             SessionConfig     `yaml:"session"`             // Session lifetime configuration for authenticated users
	RateLimiter         RateLimiterConfig `yaml:"rateLimiter"`         // Rate limiter settings. Optional; zero values disable.
	CORS                CORSConfig        `yaml:"cors"`                // Cross-origin request settings. Optional; empty allowedOrigins disables CORS entirely (no headers added — the pre-CORS-support behavior).
}

// RateLimiterConfig contains simple in-memory rate limiting settings.
// All values are positive integers; zero means the feature is disabled.
// A single configuration block keeps the gateway easy to configure.
// The middleware applies limits per client IP address.
// VulnerabilityScanConfig contains a simple list of URL paths that are
// likely to be probed by automated scanners. When a client triggers too many
// 404 responses for those paths within the configured window, the IP is
// temporarily blocked. This is a lightweight signature‑free scanner detector.
type VulnerabilityScanConfig struct {
	URLs         []string `yaml:"urls"`         // paths to watch (supports wildcard patterns)
	Max404       int      `yaml:"max404"`       // max 404s on watched paths before blocking
	BlockMinutes int      `yaml:"blockMinutes"` // how many minutes to block offending IPs
}

// RateLimiterConfig contains simple in-memory rate limiting settings.
// All values are positive integers; zero means the feature is disabled.
// A single configuration block keeps the gateway easy to configure.
// The middleware applies limits per client IP address.
type RateLimiterConfig struct {
	RequestsPerMinute int `yaml:"requestsPerMinute"` // Max requests per IP per 60s window. 0 = disabled.
	MaxErrors         int `yaml:"maxErrors"`         // Max number of 401 or 404 responses before blocking. 0 = disabled.
	BlockMinutes      int `yaml:"blockMinutes"`      // Duration (in minutes) to block offending IPs. 0 = no blocking.

	VulnerabilityScan VulnerabilityScanConfig `yaml:"vulnerabilityScan"` // Optional scanner detector
}

// IsEnabled reports whether any rate-limiting or vulnerability-scan feature is active.
func (r RateLimiterConfig) IsEnabled() bool {
	return r.RequestsPerMinute > 0 || r.MaxErrors > 0 ||
		r.VulnerabilityScan.Max404 > 0 || len(r.VulnerabilityScan.URLs) > 0
}

// GeolocationConfig defines IP geolocation service settings.
// Used to enrich analytics with geographic information about request origins.
type GeolocationConfig struct {
	IPLocateAPIKey string `yaml:"iplocateApiKey"` // API key for iplocate.io service. Optional. Can use environment variables (e.g., ${IPLOCATE_IO_API_KEY}). Without this, geolocation features are disabled.
}

// GatewayConfig is the root configuration structure for Taronja Gateway.
// It contains all settings needed to run the gateway including server, routing, authentication, and management.
// Configuration is loaded from a YAML file and supports environment variable expansion (${VAR_NAME}).
type GatewayConfig struct {
	Version                 *int                    `yaml:"version,omitempty"`       // Config schema version. Optional and nil when absent — every config file written before this field existed had no way to declare one, and that's a genuinely different state from declaring "version: 1" explicitly, not the same thing spelled two ways. See CurrentConfigVersion and LoadConfig's version-check behavior in version.go.
	Name                    string                  `yaml:"name"`                    // Gateway instance name for identification. Required.
	Server                  ServerConfig            `yaml:"server"`                  // Server network configuration. Required.
	Management              ManagementConfig        `yaml:"management"`              // Management API and dashboard configuration. Required.
	Routes                  []RouteConfig           `yaml:"routes"`                  // List of routing rules. At least one route required.
	AuthenticationProviders AuthenticationProviders `yaml:"authenticationProviders"` // Available authentication methods. Required.
	Branding                BrandingConfig          `yaml:"branding,omitempty"`      // UI branding customization. Optional.
	Geolocation             GeolocationConfig       `yaml:"geolocation"`             // IP geolocation service settings. Optional.
	Notification            NotificationConfig      `yaml:"notification"`            // Notification system settings. Optional.
	Middleware              MiddlewareSection       `yaml:"middleware,omitempty"`    // Explicit, ordered global middleware chain. Optional; when absent, derived from management.analytics/logging/rateLimiter.
}

// LoadConfig reads, parses, and validates the YAML configuration file.
func LoadConfig(filename string) (*GatewayConfig, error) {
	configAbsPath, err := filepath.Abs(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for config file '%s': %w", filename, err)
	}
	log.Printf("Loading configuration from: %s", configAbsPath)

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
	config := &GatewayConfig{}

	// Set defaults *before* unmarshalling
	config.Management.Prefix = "/_"                   // Default prefix
	config.Management.Session.SecondsDuration = 86400 // Default 24 hours

	err = yaml.Unmarshal([]byte(expandedData), config)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal config data from '%s': %w", filename, err)
	}

	// Refuse to run against a config file older than CurrentConfigVersion —
	// see version.go. Run `tg migrate --config <path>` to upgrade it first.
	if err := checkConfigVersion(configAbsPath, config); err != nil {
		return nil, err
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

	// Process admin credentials
	// If admin access is enabled, ensure both username and password are set
	if config.Management.Admin.Enabled {
		if config.Management.Admin.Username == "" || config.Management.Admin.Password == "" {
			return nil, fmt.Errorf("admin access is enabled but username and/or password is not set")
		}
		// Hash the password if it's not already hashed
		if !encryption.IsPasswordHashed(config.Management.Admin.Password) {
			hashedPassword, err := encryption.GeneratePasswordHash(config.Management.Admin.Password)
			if err != nil {
				return nil, fmt.Errorf("failed to hash admin password: %w", err)
			}
			config.Management.Admin.Password = hashedPassword
			log.Printf("Admin password has been hashed for security")
		}
	} else {
		// If admin access is not enabled, clear username and password
		config.Management.Admin.Username = ""
		config.Management.Admin.Password = ""
		log.Printf("Admin access is disabled")
	}

	// Validate explicit middleware section, if present
	seenMiddleware := make(map[string]bool, len(config.Middleware.Global))
	for _, entry := range config.Middleware.Global {
		if entry.Name == "" {
			return nil, fmt.Errorf("middleware.global: entry is missing 'name'")
		}
		if !IsMiddlewareNameKnown(entry.Name) {
			return nil, fmt.Errorf("middleware.global: unknown middleware '%s' (known: %v)", entry.Name, KnownMiddlewareNames)
		}
		if seenMiddleware[entry.Name] {
			return nil, fmt.Errorf("middleware.global: middleware '%s' is listed more than once", entry.Name)
		}
		seenMiddleware[entry.Name] = true
	}

	// Validate authentication providers
	if !config.HasAnyAuthentication() {
		log.Printf("WARNING: No authentication providers are configured. Consider enabling at least one authentication method:")
	}

	// Resolve static route paths
	currentDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get current working directory: %w", err)
	}
	log.Printf("Current working directory: %s", currentDir)

	// Validate TLS config. Resolving the paths and confirming the cert/key
	// actually parse here (not just deferring to the gateway's own startup)
	// means "tg validate" catches a bad cert/key pair before deploy, the
	// same way it already catches a bad admin/CORS/route config — this is a
	// pure local file read, no network call, so it's safe for validate's
	// no-side-effects contract. The gateway loads the pair again for real at
	// startup (config doesn't hold a *tls.Certificate itself); the point
	// here is catching the error early with a clear message, not caching it.
	if config.Server.TLS.Enabled {
		if config.Server.TLS.CertFile == "" || config.Server.TLS.KeyFile == "" {
			return nil, fmt.Errorf("server.tls.enabled is true but certFile and/or keyFile is not set")
		}
		if !filepath.IsAbs(config.Server.TLS.CertFile) {
			config.Server.TLS.CertFile = filepath.Clean(filepath.Join(currentDir, config.Server.TLS.CertFile))
		}
		if !filepath.IsAbs(config.Server.TLS.KeyFile) {
			config.Server.TLS.KeyFile = filepath.Clean(filepath.Join(currentDir, config.Server.TLS.KeyFile))
		}
		if _, err := tls.LoadX509KeyPair(config.Server.TLS.CertFile, config.Server.TLS.KeyFile); err != nil {
			return nil, fmt.Errorf("server.tls: failed to load certificate/key pair (certFile=%q, keyFile=%q): %w", config.Server.TLS.CertFile, config.Server.TLS.KeyFile, err)
		}
	}

	for i := range config.Routes {
		route := &config.Routes[i]

		if route.Static {
			// Validate that ToFolder and ToFile are mutually exclusive
			if route.ToFolder != "" && route.ToFile != "" {
				return nil, fmt.Errorf("route '%s' cannot have both 'toFolder' and 'toFile' specified, they are mutually exclusive", route.Name)
			}

			// Validate that at least one of ToFolder or ToFile is specified
			if route.ToFolder == "" && route.ToFile == "" {
				return nil, fmt.Errorf("route '%s' is marked as static but neither 'toFolder' nor 'toFile' is specified", route.Name)
			}

			// Resolve folder path
			if route.ToFolder != "" {
				originalPath := route.ToFolder
				resolvedPath := originalPath
				if !filepath.IsAbs(originalPath) {
					resolvedPath = filepath.Join(currentDir, originalPath)
				}
				route.ToFolder = filepath.Clean(resolvedPath)

				if originalPath != route.ToFolder && !filepath.IsAbs(originalPath) {
					log.Printf("Route '%s' folder path resolved. Original: '%s', Resolved: '%s'",
						route.Name, originalPath, route.ToFolder)
				}
			}

			// Resolve file path
			if route.ToFile != "" {
				originalPath := route.ToFile
				resolvedPath := originalPath
				if !filepath.IsAbs(originalPath) {
					resolvedPath = filepath.Join(currentDir, originalPath)
				}
				route.ToFile = filepath.Clean(resolvedPath)

				if originalPath != route.ToFile && !filepath.IsAbs(originalPath) {
					log.Printf("Route '%s' file path resolved. Original: '%s', Resolved: '%s'",
						route.Name, originalPath, route.ToFile)
				}
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

// --- Helper Functions ---

// HasAuthentication checks if any authentication is enabled in the config.
func (c *GatewayConfig) HasAnyAuthentication() bool {
	return c.AuthenticationProviders.Basic.Enabled ||
		c.AuthenticationProviders.Google.ClientId != "" ||
		c.AuthenticationProviders.Github.ClientId != "" ||
		c.Management.Admin.Enabled
}

// loginPageData is the data structure for the login.html template
type loginPageData struct {
	AuthenticationProviders struct {
		Basic struct {
			Enabled bool
		}
		Google struct {
			Enabled bool
		}
		Github struct {
			Enabled bool
		}
	}
	Branding         BrandingConfig
	RedirectURL      string
	ManagementPrefix string
}

// NewLoginPageData creates and populates a LoginPageData struct.
func NewLoginPageData(redirectURL string, gatewayConfig *GatewayConfig) loginPageData {
	data := loginPageData{
		RedirectURL:      redirectURL,
		ManagementPrefix: gatewayConfig.Management.Prefix,
	}
	data.AuthenticationProviders.Basic.Enabled = gatewayConfig.AuthenticationProviders.Basic.Enabled || gatewayConfig.Management.Admin.Enabled
	data.AuthenticationProviders.Google.Enabled = gatewayConfig.AuthenticationProviders.Google.ClientId != ""
	data.AuthenticationProviders.Github.Enabled = gatewayConfig.AuthenticationProviders.Github.ClientId != ""
	data.Branding.LogoUrl = gatewayConfig.Branding.LogoUrl
	return data
}

// --- RouteOptions Helper Methods ---

// getCacheControlHeader returns the appropriate Cache-Control header value based on the configuration.
// Returns empty string if no cache header should be set.
func (opts *RouteOptions) getCacheControlHeader() string {
	if opts == nil || opts.CacheControlSeconds == nil {
		return "" // No cache header
	}

	if *opts.CacheControlSeconds == 0 {
		return "no-cache" // Explicit no-cache
	}

	if *opts.CacheControlSeconds > 0 {
		return fmt.Sprintf("max-age=%d", *opts.CacheControlSeconds)
	}

	return "" // Negative values mean no cache header
}

// GetCacheControlHeader returns the appropriate Cache-Control header value for this route.
func (route *RouteConfig) GetCacheControlHeader() string {
	if route.Options == nil {
		return ""
	}
	return route.Options.getCacheControlHeader()
}

// ShouldSetCacheHeader returns true if this route should set a Cache-Control header.
func (route *RouteConfig) ShouldSetCacheHeader() bool {
	return route.Options != nil && route.Options.CacheControlSeconds != nil && *route.Options.CacheControlSeconds >= 0
}
