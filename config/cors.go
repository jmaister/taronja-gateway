package config

// CORSConfig controls whether and how the gateway adds CORS
// (Cross-Origin Resource Sharing) response headers to management API
// requests. Disabled by default (IsEnabled reports false when
// AllowedOrigins is empty) — the gateway's dashboard is always served
// same-origin, so CORS is only needed when a separately-hosted frontend
// needs to call the management API directly.
type CORSConfig struct {
	// AllowedOrigins lists the exact origins (scheme + host + optional port,
	// e.g. "https://app.example.com") allowed to make cross-origin requests.
	// A single literal "*" allows any origin, but only when AllowCredentials
	// is false — the CORS spec forbids combining a wildcard origin with
	// credentialed requests, and browsers reject it outright, so this
	// combination is rejected at config load time instead of silently not
	// working. Empty (the default) disables CORS entirely: no
	// Access-Control-* headers are added, identical to before CORS support
	// existed.
	AllowedOrigins []string `yaml:"allowedOrigins"`
	// AllowedMethods lists the HTTP methods allowed in a preflight response.
	// Defaults to "GET, POST, PUT, PATCH, DELETE, OPTIONS" when unset.
	AllowedMethods []string `yaml:"allowedMethods,omitempty"`
	// AllowedHeaders lists the request headers allowed in a preflight
	// response. Defaults to "Content-Type, Authorization" when unset.
	AllowedHeaders []string `yaml:"allowedHeaders,omitempty"`
	// AllowCredentials sets Access-Control-Allow-Credentials: true, letting
	// browsers send cookies/credentials on cross-origin requests. Requires
	// AllowedOrigins to be an explicit list — see the "*" restriction above.
	AllowCredentials bool `yaml:"allowCredentials,omitempty"`
	// MaxAgeSeconds sets Access-Control-Max-Age: how long browsers may cache
	// a preflight response before sending another one. Defaults to 600 (10
	// minutes) when unset.
	MaxAgeSeconds int `yaml:"maxAgeSeconds,omitempty"`
}

// IsEnabled reports whether CORS handling should run at all: only when at
// least one allowed origin is configured.
func (c CORSConfig) IsEnabled() bool {
	return len(c.AllowedOrigins) > 0
}

// AllowsAnyOrigin reports whether AllowedOrigins contains the literal "*"
// wildcard.
func (c CORSConfig) AllowsAnyOrigin() bool {
	for _, o := range c.AllowedOrigins {
		if o == "*" {
			return true
		}
	}
	return false
}
