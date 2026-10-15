package config

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jmaister/taronja-gateway/encryption"
	yaml "gopkg.in/yaml.v3"
)

// --- Configuration Structs ---

// ServerConfig defines the gateway server's network configuration.
type ServerConfig struct {
	Host string `yaml:"host"` // Bind address. Default: "127.0.0.1" (localhost only); "0.0.0.0" listens on all interfaces.
	Port int    `yaml:"port"` // Port number (e.g., 8080). Required.
	URL  string `yaml:"url"`  // Public URL, used for OAuth redirects (e.g., "https://example.com"). Optional; a warning is logged when empty.
}

// ManagementPrefix is the fixed URL prefix of the gateway's own endpoints:
// login, dashboard (/_/admin/), API (/_/api/) and static assets.
const ManagementPrefix = "/_"

// RouteOptions contains additional optional configuration for individual routes.
type RouteOptions struct {
	CacheControlSeconds *int `yaml:"cacheControlSeconds,omitempty"` // Cache control in seconds. Optional. nil = no cache header, 0 = "no-cache", >0 = "max-age=N"
}

// RouteTargets is one or more backend URLs for a proxy route. `to:` accepts a
// scalar or a list; lists are round-robined with failover (see
// gateway.newRoundRobinTransport). Targets must be interchangeable replicas; use
// separate routes to send different paths elsewhere.
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
	Name           string        `yaml:"name"`              // Human-readable route name for logging. Required.
	From           string        `yaml:"from"`              // Incoming request path pattern (e.g., "/api/*", "/"). Must start with "/". Required.
	To             RouteTargets  `yaml:"to,omitempty"`      // Target URL(s) for proxying (e.g., "https://api.example.com", or a list of them for load balancing). Required for proxy routes. See RouteTargets.
	ToFolder       string        `yaml:"toFolder"`          // Folder to serve static files from. A route sets exactly one of to, toFolder or toFile.
	ToFile         string        `yaml:"toFile"`            // Single file to serve. A route sets exactly one of to, toFolder or toFile.
	IsSPA          bool          `yaml:"isSPA"`             // SPA mode. Static routes serve index.html on 404; proxy routes re-request the upstream base URL on 404. Default: false
	RemoveFromPath string        `yaml:"removeFromPath"`    // Path prefix removed before proxying, on a path-segment boundary (e.g., "/api/v1"). Proxy routes only.
	Authenticated  bool          `yaml:"authenticated"`     // Require a logged-in user. Default: false
	Options        *RouteOptions `yaml:"options,omitempty"` // Additional route options (cache control, etc.). Optional.
}

// AuthProviderCredentials contains OAuth2 provider credentials.
// Required for OAuth2 authentication providers (Google, GitHub).
type AuthProviderCredentials struct {
	ClientId     string `yaml:"clientId"`     // OAuth2 client ID from provider. Can use environment variables (e.g., ${GOOGLE_CLIENT_ID})
	ClientSecret string `yaml:"clientSecret"` // OAuth2 client secret from provider. Can use environment variables (e.g., ${GOOGLE_CLIENT_SECRET})
}

// BasicAuthenticationConfig controls basic authentication provider.
type BasicAuthenticationConfig struct {
	Enabled bool `yaml:"enabled"` // Enable basic (username/password) authentication. Default: false.
}

// AuthenticationProviders defines the available authentication methods.
// At least one should be enabled if any route requires authentication.
type AuthenticationProviders struct {
	Basic  BasicAuthenticationConfig `yaml:"basic"`          // Basic username/password authentication
	Google AuthProviderCredentials   `yaml:"google"`         // Google OAuth2 authentication. Optional.
	Github AuthProviderCredentials   `yaml:"github"`         // GitHub OAuth2 authentication. Optional.
	OIDC   []OIDCProviderConfig      `yaml:"oidc,omitempty"` // Any OpenID Connect provider (Zitadel, Keycloak, Authentik, Authelia, Pocket ID, ...). Optional; one entry per provider.
}

// OIDCProviderConfig is one OpenID Connect identity provider. Its endpoints
// are read from <issuer>/.well-known/openid-configuration, so only the issuer
// and the client credentials are needed. PKCE is always used.
type OIDCProviderConfig struct {
	Name         string   `yaml:"name"`             // Short ID used in the URLs (/_/auth/<name>/login and /_/auth/<name>/callback). Lowercase letters, digits and '-'. Required.
	Label        string   `yaml:"label"`            // Shown on the login button ("Login with <label>"). Default: the name.
	Issuer       string   `yaml:"issuer"`           // Issuer URL, e.g. "https://auth.example.com". Required.
	ClientId     string   `yaml:"clientId"`         // Client ID registered at the provider. Can use environment variables. Required.
	ClientSecret string   `yaml:"clientSecret"`     // Client secret. Can use environment variables. Optional: leave it empty for a public (PKCE-only) client.
	Scopes       []string `yaml:"scopes,omitempty"` // Scopes to request. Default: openid, email, profile.
	// TrustEmail treats every email this provider returns as verified, for
	// providers that do not send the email_verified claim. Only enable it for
	// a provider you control: it lets the provider's users claim any address.
	// Default: false.
	TrustEmail bool `yaml:"trustEmail"`
}

// DisplayLabel returns the login button label.
func (o OIDCProviderConfig) DisplayLabel() string {
	if o.Label != "" {
		return o.Label
	}
	return o.Name
}

// oidcNamePattern keeps names safe as a URL segment.
var oidcNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,29}$`)

// reservedProviderNames are taken by the built-in login methods.
var reservedProviderNames = map[string]bool{"basic": true, "google": true, "github": true}

func validateOIDCProviders(providers []OIDCProviderConfig) error {
	seen := map[string]bool{}
	for i, p := range providers {
		where := fmt.Sprintf("authenticationProviders.oidc[%d]", i)
		if !oidcNamePattern.MatchString(p.Name) {
			return fmt.Errorf("%s.name %q must be 1-30 lowercase letters, digits or '-'", where, p.Name)
		}
		if reservedProviderNames[p.Name] {
			return fmt.Errorf("%s.name %q is reserved for a built-in login method", where, p.Name)
		}
		if seen[p.Name] {
			return fmt.Errorf("%s.name %q is used twice", where, p.Name)
		}
		seen[p.Name] = true
		issuer, err := url.Parse(p.Issuer)
		if err != nil || (issuer.Scheme != "https" && issuer.Scheme != "http") || issuer.Host == "" {
			return fmt.Errorf("%s.issuer must be an http(s) URL, got %q", where, p.Issuer)
		}
		if p.ClientId == "" {
			return fmt.Errorf("%s.clientId is required", where)
		}
	}
	return nil
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
	for _, p := range a.OIDC {
		fmt.Printf("[OAUTH] %s (OIDC) callback URL:\n", p.DisplayLabel())
		fmt.Println("   ", fmt.Sprintf("%s%s/auth/%s/callback", serverURL, managementPrefix, p.Name))
	}
}

// BrandingConfig contains visual customization options for the gateway UI.
type BrandingConfig struct {
	LogoUrl string `yaml:"logoUrl,omitempty"` // URL or path to custom logo image for login page. Optional.
}

// NotificationConfig defines notification settings. Notifications are always
// stored; delivery over each independent, optional channel block (email,
// Telegram) is added on top. See notification.Provider for adding a channel.
type NotificationConfig struct {
	Email           EmailNotificationConfig    `yaml:"email"`           // Email delivery via SMTP. Optional; disabled by default.
	Telegram        TelegramNotificationConfig `yaml:"telegram"`        // Telegram delivery via a bot. Optional; disabled by default.
	ResponseWebhook ResponseWebhookConfig      `yaml:"responseWebhook"` // Outbound callback fired when a user responds to a notification. Optional; disabled by default.
}

// EmailNotificationConfig configures the SMTP relay notifications with
// sendEmail: true are delivered through. This is a plain synchronous SMTP
// send per notification (net/smtp) — no queue, no retry — appropriate for
// the low volume a per-user notification system produces; a high-volume
// deployment should put a relay with its own queuing in front of this
// instead of expecting the gateway to grow one.
type EmailNotificationConfig struct {
	Enabled  bool   `yaml:"enabled"`  // Enable email delivery. Default: false. Requires host and from.
	Host     string `yaml:"host"`     // SMTP server hostname (e.g., "smtp.gmail.com"). Required when enabled.
	Port     int    `yaml:"port"`     // SMTP server port (e.g., 587 for STARTTLS, 465 for implicit TLS). Required when enabled.
	Username string `yaml:"username"` // SMTP authentication username. Can use environment variables. Optional — some relays allow unauthenticated local delivery.
	Password string `yaml:"password"` // SMTP authentication password. Can use environment variables.
	From     string `yaml:"from"`     // From email address. Can use environment variables. Required when enabled.
	FromName string `yaml:"fromName"` // From display name. Optional.
}

// IsConfigured reports whether email delivery has the minimum settings
// (host and from address) to actually attempt a send.
func (e EmailNotificationConfig) IsConfigured() bool {
	return e.Enabled && e.Host != "" && e.From != ""
}

// TelegramNotificationConfig configures delivery via a Telegram bot. Users link
// their account once through the deep link (see doc/notifications.md); only
// linked users receive messages. The bot long-polls, so no inbound exposure is needed.
type TelegramNotificationConfig struct {
	Enabled  bool   `yaml:"enabled"`  // Enable Telegram delivery. Default: false. Requires botToken.
	BotToken string `yaml:"botToken"` // Bot token from @BotFather (e.g., "123456:ABC-DEF..."). Can use environment variables. Required when enabled.
}

// IsConfigured reports whether Telegram delivery has a bot token to
// actually poll for updates and send messages with.
func (t TelegramNotificationConfig) IsConfigured() bool {
	return t.Enabled && t.BotToken != ""
}

// ResponseWebhookConfig configures the HTTP callback POSTed to a single URL when
// a user responds to a notification, so the creating app need not poll. It is
// not a delivery channel.
type ResponseWebhookConfig struct {
	Enabled bool   `yaml:"enabled"` // Enable the response webhook. Default: false. Requires url.
	URL     string `yaml:"url"`     // Absolute URL to POST the response event to. Can use environment variables. Required when enabled.
	// Secret, if set, HMAC-SHA256-signs each request body and sends the
	// hex digest in an X-Taronja-Signature: sha256=<hex> header, so the
	// receiver can verify the call actually came from this gateway. Can
	// use environment variables. Optional — omit it only on a network
	// where forging this request isn't a real concern (e.g. a private
	// network with no other untrusted senders).
	Secret string `yaml:"secret,omitempty"`
}

// IsConfigured reports whether the response webhook has a URL to actually
// POST to.
func (w ResponseWebhookConfig) IsConfigured() bool {
	return w.Enabled && w.URL != ""
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
	DurationSeconds int `yaml:"durationSeconds"` // Session lifetime in seconds. Default: 86400 (24 hours). After this time, users must log in again.
}

func (s *SessionConfig) GetDuration() time.Duration {
	return time.Duration(s.DurationSeconds) * time.Second
}

// ManagementConfig defines the management API and dashboard settings.
// The management API provides endpoints for metrics, user management, and admin dashboard.
type ManagementConfig struct {
	Admin   AdminConfig   `yaml:"admin"`   // Admin dashboard access configuration
	Session SessionConfig `yaml:"session"` // Session lifetime configuration for authenticated users
}

// GeolocationConfig defines IP geolocation service settings.
// Used to enrich analytics with geographic information about request origins.
type GeolocationConfig struct {
	IPLocateAPIKey string `yaml:"iplocateApiKey"` // iplocate.io API key; may use ${ENV_VAR}. Optional; without it the free freeipapi.com service is used.
}

// GatewayConfig is the root configuration structure for Taronja Gateway.
// It contains all settings needed to run the gateway including server, routing, authentication, and management.
// Configuration is loaded from a YAML file and supports environment variable expansion (${VAR_NAME}).
type GatewayConfig struct {
	Version                 *string                 `yaml:"version,omitempty"`       // Config schema version, MAJOR.MINOR, quoted ("1.0") or not (1.0). Required: a file without it is treated as pre-1.0 and must be migrated with `tg migrate`.
	Name                    string                  `yaml:"name"`                    // Gateway instance name for identification. Required.
	Server                  ServerConfig            `yaml:"server"`                  // Server network configuration. Required.
	Management              ManagementConfig        `yaml:"management"`              // Admin dashboard and session settings. Optional.
	Routes                  []RouteConfig           `yaml:"routes"`                  // List of routing rules. At least one route required.
	AuthenticationProviders AuthenticationProviders `yaml:"authenticationProviders"` // Login methods. Optional; without any, nobody can log in.
	Branding                BrandingConfig          `yaml:"branding,omitempty"`      // UI branding customization. Optional.
	Geolocation             GeolocationConfig       `yaml:"geolocation"`             // IP geolocation service settings. Optional.
	Notification            NotificationConfig      `yaml:"notification"`            // Notification system settings. Optional.
	Middleware              []MiddlewareConfig      `yaml:"middleware,omitempty"`    // Global middleware, run in list order. Optional; none when absent. See doc/middleware/.
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
	config.Server.Host = "127.0.0.1"
	config.Management.Session.DurationSeconds = 86400

	// The version is checked before the strict decode so an old config gets the
	// "run tg migrate" message instead of an unknown-field error.
	var probe struct {
		Version *string `yaml:"version"`
	}
	if err := yaml.Unmarshal([]byte(expandedData), &probe); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config data from '%s': %w", filename, err)
	}
	config.Version = probe.Version
	if err := checkConfigVersion(configAbsPath, config); err != nil {
		return nil, err
	}

	dec := yaml.NewDecoder(strings.NewReader(expandedData))
	dec.KnownFields(true)
	if err := dec.Decode(config); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("failed to unmarshal config data from '%s': %w", filename, err)
	}

	if config.Server.Port == 0 {
		return nil, fmt.Errorf("server.port must be specified")
	}
	if config.Server.URL == "" {
		log.Printf("Warning: server.url is not set in config. OAuth redirects might not work correctly.")
	}
	if !config.HasAnyAuthentication() {
		log.Printf("WARNING: No authentication providers are configured. Consider enabling at least one authentication method.")
	}

	if err := config.Validate(); err != nil {
		return nil, err
	}

	if config.Management.Admin.Enabled {
		if !encryption.IsPasswordHashed(config.Management.Admin.Password) {
			hashedPassword, err := encryption.GeneratePasswordHash(config.Management.Admin.Password)
			if err != nil {
				return nil, fmt.Errorf("failed to hash admin password: %w", err)
			}
			config.Management.Admin.Password = hashedPassword
			log.Printf("Admin password has been hashed for security")
		}
	} else {
		config.Management.Admin.Username = ""
		config.Management.Admin.Password = ""
		log.Printf("Admin access is disabled")
	}

	currentDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get current working directory: %w", err)
	}
	for i := range config.Routes {
		route := &config.Routes[i]
		route.ToFolder = resolvePath(currentDir, route.ToFolder)
		route.ToFile = resolvePath(currentDir, route.ToFile)
		if !strings.HasPrefix(route.From, "/") {
			log.Printf("Warning: Route '%s' From path '%s' does not start with '/'. Adding prefix.", route.Name, route.From)
			route.From = "/" + route.From
		}
	}

	return config, nil
}

// resolvePath makes a relative static path absolute against dir; "" stays "".
func resolvePath(dir, path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Clean(filepath.Join(dir, path))
}

// Validate checks the settings that do not depend on the filesystem or on
// middleware internals. LoadConfig runs it, and so do `tg validate` and gateway
// startup for configs built in code (where port 0 means an OS-assigned port, so
// the port is only required by LoadConfig).
func (c *GatewayConfig) Validate() error {
	if c.Management.Admin.Enabled && (c.Management.Admin.Username == "" || c.Management.Admin.Password == "") {
		return fmt.Errorf("management.admin is enabled but username and/or password is not set")
	}
	if c.Management.Session.DurationSeconds < 0 {
		return fmt.Errorf("management.session.durationSeconds cannot be negative")
	}
	if err := validateOIDCProviders(c.AuthenticationProviders.OIDC); err != nil {
		return err
	}
	if err := validateMiddleware(c.Middleware); err != nil {
		return err
	}
	for _, route := range c.Routes {
		if err := route.validate(); err != nil {
			return err
		}
	}
	return nil
}

// IsStatic reports whether the route serves files (toFolder or toFile) rather
// than proxying (to).
func (r RouteConfig) IsStatic() bool {
	return r.ToFolder != "" || r.ToFile != ""
}

func (r RouteConfig) validate() error {
	targets := 0
	for _, set := range []bool{len(r.To) > 0, r.ToFolder != "", r.ToFile != ""} {
		if set {
			targets++
		}
	}
	switch {
	case targets == 0:
		return fmt.Errorf("route '%s' needs one of to, toFolder or toFile", r.Name)
	case targets > 1:
		return fmt.Errorf("route '%s' sets more than one of to, toFolder and toFile", r.Name)
	case r.IsStatic() && r.RemoveFromPath != "":
		return fmt.Errorf("route '%s': removeFromPath only applies to proxy routes (to)", r.Name)
	case r.Options != nil && r.Options.CacheControlSeconds != nil && *r.Options.CacheControlSeconds < 0:
		return fmt.Errorf("route '%s': options.cacheControlSeconds cannot be negative", r.Name)
	}
	return nil
}

// --- Helper Functions ---

// HasAuthentication checks if any authentication is enabled in the config.
func (c *GatewayConfig) HasAnyAuthentication() bool {
	return c.AuthenticationProviders.Basic.Enabled ||
		c.AuthenticationProviders.Google.ClientId != "" ||
		c.AuthenticationProviders.Github.ClientId != "" ||
		len(c.AuthenticationProviders.OIDC) > 0 ||
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
		OIDC []OIDCLoginButton
	}
	Branding         BrandingConfig
	RedirectURL      string
	Message          string
	Error            string
	ManagementPrefix string
}

// OIDCLoginButton is one OIDC provider's button on the login page.
type OIDCLoginButton struct {
	Name  string
	Label string
}

// NewLoginPageData creates and populates a LoginPageData struct.
func NewLoginPageData(redirectURL string, gatewayConfig *GatewayConfig) loginPageData {
	data := loginPageData{
		RedirectURL:      redirectURL,
		ManagementPrefix: ManagementPrefix,
	}
	data.AuthenticationProviders.Basic.Enabled = gatewayConfig.AuthenticationProviders.Basic.Enabled || gatewayConfig.Management.Admin.Enabled
	data.AuthenticationProviders.Google.Enabled = gatewayConfig.AuthenticationProviders.Google.ClientId != ""
	data.AuthenticationProviders.Github.Enabled = gatewayConfig.AuthenticationProviders.Github.ClientId != ""
	for _, p := range gatewayConfig.AuthenticationProviders.OIDC {
		data.AuthenticationProviders.OIDC = append(data.AuthenticationProviders.OIDC, OIDCLoginButton{Name: p.Name, Label: p.DisplayLabel()})
	}
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
