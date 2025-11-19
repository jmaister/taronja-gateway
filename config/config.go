package config

import (
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
	Host string `yaml:"host"` // Server bind address (e.g., "127.0.0.1" for localhost only, "0.0.0.0" for all interfaces)
	Port int    `yaml:"port"` // Server port number (e.g., 8080). Required.
	URL  string `yaml:"url"`  // Full external URL for OAuth redirects (e.g., "https://example.com" or "http://localhost:8080")
}

// AuthenticationConfig controls whether authentication is required for a specific route.
type AuthenticationConfig struct {
	Enabled bool `yaml:"enabled"` // Enable authentication requirement for this route. Default: false
}

// RouteOptions contains additional optional configuration for individual routes.
type RouteOptions struct {
	CacheControlSeconds *int `yaml:"cacheControlSeconds,omitempty"` // Cache control in seconds. Optional. nil = no cache header, 0 = "no-cache", >0 = "max-age=N"
}

// RouteConfig defines a single routing rule for the gateway.
// Routes can proxy to remote servers or serve static files.
type RouteConfig struct {
	Name           string               `yaml:"name"`              // Human-readable route name for logging. Required.
	From           string               `yaml:"from"`              // Incoming request path pattern (e.g., "/api/*", "/"). Must start with "/". Required.
	To             string               `yaml:"to"`                // Target URL for proxying (e.g., "https://api.example.com"). Required for proxy routes.
	ToFolder       string               `yaml:"toFolder"`          // Local folder path for static content. Mutually exclusive with ToFile. Required if Static=true and ToFile not set.
	ToFile         string               `yaml:"toFile"`            // Specific file path for static content. Mutually exclusive with ToFolder. Optional.
	Static         bool                 `yaml:"static"`            // Enable static file serving. Default: false
	IsSPA          bool                 `yaml:"isSPA"`             // Enable SPA mode (fallback to index.html for 404s). Default: false
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
	Prefix    string        `yaml:"prefix"`    // URL prefix for management endpoints. Default: "/_". All management endpoints will be under this prefix.
	Logging   bool          `yaml:"logging"`   // Enable request/response logging. Default: false. Logs all HTTP requests.
	Analytics bool          `yaml:"analytics"` // Enable traffic analytics and metrics collection. Default: false. Stores request data for dashboard.
	Admin     AdminConfig   `yaml:"admin"`     // Admin dashboard access configuration
	Session   SessionConfig `yaml:"session"`   // Session lifetime configuration for authenticated users
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
	Name                    string                  `yaml:"name"`                    // Gateway instance name for identification. Required.
	Server                  ServerConfig            `yaml:"server"`                  // Server network configuration. Required.
	Management              ManagementConfig        `yaml:"management"`              // Management API and dashboard configuration. Required.
	Routes                  []RouteConfig           `yaml:"routes"`                  // List of routing rules. At least one route required.
	AuthenticationProviders AuthenticationProviders `yaml:"authenticationProviders"` // Available authentication methods. Required.
	Branding                BrandingConfig          `yaml:"branding,omitempty"`      // UI branding customization. Optional.
	Geolocation             GeolocationConfig       `yaml:"geolocation"`             // IP geolocation service settings. Optional.
	Notification            NotificationConfig      `yaml:"notification"`            // Notification system settings. Optional.
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
