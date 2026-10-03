package config

// Known global middleware names. These are the identifiers accepted in
// `middleware.global[].name` (see MiddlewareEntryConfig) and are also used by
// the middleware package's factories (middleware/builtin/factories.go) so both sides
// agree on naming without middleware needing to import config, or config
// needing to import middleware (which already imports config).
const (
	MiddlewareNameCORS              = "cors"
	MiddlewareNameCompression       = "compression"
	MiddlewareNameRateLimiter       = "rate_limiter"
	MiddlewareNameJA4Fingerprint    = "ja4_fingerprint"
	MiddlewareNameSessionExtraction = "session_extraction"
	MiddlewareNameTrafficMetrics    = "traffic_metrics"
	MiddlewareNameLogging           = "logging"
	MiddlewareNameTracing           = "tracing"
)

// KnownMiddlewareNames lists every global middleware name the gateway
// understands today. Used to validate `middleware.global[].name` entries at
// config load time.
var KnownMiddlewareNames = []string{
	MiddlewareNameCORS,
	MiddlewareNameCompression,
	MiddlewareNameRateLimiter,
	MiddlewareNameJA4Fingerprint,
	MiddlewareNameSessionExtraction,
	MiddlewareNameTrafficMetrics,
	MiddlewareNameLogging,
	MiddlewareNameTracing,
}

// IsMiddlewareNameKnown reports whether name is a recognized global middleware.
func IsMiddlewareNameKnown(name string) bool {
	for _, n := range KnownMiddlewareNames {
		if n == name {
			return true
		}
	}
	return false
}

// TrafficMetricsConfig holds the options for the "traffic_metrics" global
// middleware.
type TrafficMetricsConfig struct {
	ExcludeStaticAssets bool `yaml:"excludeStaticAssets"` // See ManagementConfig.ExcludeStaticAssets; same meaning, per-entry override.
}

// MiddlewareEntryConfig declares one middleware in the `middleware.global` list,
// in run order. rate_limiter, cors and traffic_metrics have typed per-entry config;
// the others take no options, so listing them only enables and positions them.
type MiddlewareEntryConfig struct {
	Name           string                `yaml:"name"`                     // Middleware identifier. Must be one of KnownMiddlewareNames.
	Enabled        *bool                 `yaml:"enabled,omitempty"`        // Enable/disable this middleware. Default: true (listing it implies enabled).
	RateLimiter    *RateLimiterConfig    `yaml:"rateLimiter,omitempty"`    // Per-entry override for "rate_limiter". Falls back to management.rateLimiter when nil.
	CORS           *CORSConfig           `yaml:"cors,omitempty"`           // Per-entry override for "cors". Falls back to management.cors when nil.
	TrafficMetrics *TrafficMetricsConfig `yaml:"trafficMetrics,omitempty"` // Per-entry override for "traffic_metrics". Falls back to management.excludeStaticAssets when nil.
}

// IsEnabled reports whether this entry is enabled. An absent Enabled field
// defaults to true: appearing in the list is enough to opt in.
func (e MiddlewareEntryConfig) IsEnabled() bool {
	return e.Enabled == nil || *e.Enabled
}

// MiddlewareSection declares the global chain explicitly, in execution order.
// When Global is nil (no `middleware:` section) the legacy management flags are
// used; see middleware.ResolveGlobalChainSpecs. When present it takes over entirely,
// and `global: []` means no middleware (a non-nil empty slice, which YAML preserves).
type MiddlewareSection struct {
	Global []MiddlewareEntryConfig `yaml:"global,omitempty"`
}
