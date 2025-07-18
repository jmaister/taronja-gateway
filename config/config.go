package config

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmaister/taronja-gateway/encryption"
	yaml "gopkg.in/yaml.v3"
)

// --- Configuration Structs ---

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
	URL  string `yaml:"url"`
}

type AuthenticationConfig struct {
	Enabled bool `yaml:"enabled"`
}
type RouteConfig struct {
	Name           string               `yaml:"name"`
	From           string               `yaml:"from"`
	To             string               `yaml:"to"`
	ToFolder       string               `yaml:"toFolder"` // Folder path for static content
	ToFile         string               `yaml:"toFile"`   // Optional specific file within folder
	Static         bool                 `yaml:"static"`
	IsSPA          bool                 `yaml:"isSPA"` // Enable SPA routing (fallback to index.html)
	RemoveFromPath string               `yaml:"removeFromPath"`
	Authentication AuthenticationConfig `yaml:"authentication"`
}
type AuthProviderCredentials struct {
	ClientId     string `yaml:"clientId"`
	ClientSecret string `yaml:"clientSecret"`
}
type BasicAuthenticationConfig struct {
	Enabled bool `yaml:"enabled"`
}
type AuthenticationProviders struct {
	Basic  BasicAuthenticationConfig `yaml:"basic"`
	Google AuthProviderCredentials   `yaml:"google"`
	Github AuthProviderCredentials   `yaml:"github"`
}
type NotificationConfig struct {
	Email struct {
		Enabled bool `yaml:"enabled"`
		SMTP    struct {
			Host     string `yaml:"host"`
			Port     int    `yaml:"port"`
			Username string `yaml:"username"`
			Password string `yaml:"password"`
			From     string `yaml:"from"`
			FromName string `yaml:"fromName"`
		} `yaml:"smtp"`
	} `yaml:"email"`
}

// Admin configuration for dashboard and other management features.
type AdminConfig struct {
	Enabled  bool   `yaml:"enabled"`  // Enable admin access
	Username string `yaml:"username"` // Admin username
	Password string `yaml:"password"` // Admin password (will be hashed)
	Email    string `yaml:"email"`    // Admin email for notifications
}

// New: Management API Configuration Structs
type ManagementConfig struct {
	Prefix    string      `yaml:"prefix"`    // e.g., "/_"
	Logging   bool        `yaml:"logging"`   // Enable logging
	Analytics bool        `yaml:"analytics"` // Enable request/response analytics collection
	Admin     AdminConfig `yaml:"admin"`     // Admin access configuration
}

// Geolocation configuration for IP geolocation services
type GeolocationConfig struct {
	IPLocateAPIKey string `yaml:"iplocateApiKey"` // API key for iplocate.io service
}

// Main GatewayConfig Struct including Management API config
type GatewayConfig struct {
	Name                    string                  `yaml:"name"`
	Server                  ServerConfig            `yaml:"server"`
	Management              ManagementConfig        `yaml:"management"` // Add management config
	Routes                  []RouteConfig           `yaml:"routes"`
	AuthenticationProviders AuthenticationProviders `yaml:"authenticationProviders"`
	Geolocation             GeolocationConfig       `yaml:"geolocation"`
	Notification            NotificationConfig      `yaml:"notification"`
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
	config.Management.Prefix = "/_" // Default prefix

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

// LoginPageData is the data structure for the login.html template
type LoginPageData struct {
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
	RedirectURL      string
	ManagementPrefix string
}

// NewLoginPageData creates and populates a LoginPageData struct.
func NewLoginPageData(redirectURL string, gatewayConfig *GatewayConfig) LoginPageData {
	data := LoginPageData{
		RedirectURL:      redirectURL,
		ManagementPrefix: gatewayConfig.Management.Prefix,
	}
	data.AuthenticationProviders.Basic.Enabled = gatewayConfig.AuthenticationProviders.Basic.Enabled || gatewayConfig.Management.Admin.Enabled
	data.AuthenticationProviders.Google.Enabled = gatewayConfig.AuthenticationProviders.Google.ClientId != ""
	data.AuthenticationProviders.Github.Enabled = gatewayConfig.AuthenticationProviders.Github.ClientId != ""
	return data
}
