package main

import "net/http"

// --- Configuration Structs ---

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
	URL  string `yaml:"url"`
}
type AuthenticationConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Method   string `yaml:"method"`   // "basic", "oauth2", "any", or ""
	Provider string `yaml:"provider"` // "google", "github", etc. (for oauth2)
}
type RouteConfig struct {
	Name           string               `yaml:"name"`
	From           string               `yaml:"from"`
	To             string               `yaml:"to"`
	ToFolder       string               `yaml:"toFolder"`
	Static         bool                 `yaml:"static"`
	RemoveFromPath string               `yaml:"removeFromPath"`
	Authentication AuthenticationConfig `yaml:"authentication"`
}
type AuthProviderCredentials struct {
	ClientId     string `yaml:"clientId"`
	ClientSecret string `yaml:"clientSecret"`
}
type AuthenticationProviders struct {
	Google AuthProviderCredentials `yaml:"google"`
	Github AuthProviderCredentials `yaml:"github"`
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

// New: Management API Configuration Structs
type ManagementConfig struct {
	Prefix string `yaml:"prefix"` // e.g., "/_"
}

// Main Config Struct including Management API config
type Config struct {
	Name                    string                  `yaml:"name"`
	Server                  ServerConfig            `yaml:"server"`
	Management              ManagementConfig        `yaml:"management"` // Add management config
	Routes                  []RouteConfig           `yaml:"routes"`
	AuthenticationProviders AuthenticationProviders `yaml:"authenticationProviders"`
	Notification            NotificationConfig      `yaml:"notification"`
}

// --- Gateway Struct ---

type Gateway struct {
	server      *http.Server
	config      *Config
	mux         *http.ServeMux
	authManager *AuthManager
}
