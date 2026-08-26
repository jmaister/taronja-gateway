package config

// Known global middleware names. These are the identifiers accepted in
// `middleware.global[].name` (see MiddlewareEntryConfig) and are also used by
// the middleware package's factories (middleware/factory.go) so both sides
// agree on naming without middleware needing to import config, or config
// needing to import middleware (which already imports config).
const (
	MiddlewareNameCORS              = "cors"
	MiddlewareNameRateLimiter       = "rate_limiter"
	MiddlewareNameJA4Fingerprint    = "ja4_fingerprint"
	MiddlewareNameSessionExtraction = "session_extraction"
	MiddlewareNameTrafficMetrics    = "traffic_metrics"
	MiddlewareNameLogging           = "logging"
)

// KnownMiddlewareNames lists every global middleware name the gateway
// understands today. Used to validate `middleware.global[].name` entries at
// config load time.
var KnownMiddlewareNames = []string{
	MiddlewareNameCORS,
	MiddlewareNameRateLimiter,
	MiddlewareNameJA4Fingerprint,
	MiddlewareNameSessionExtraction,
	MiddlewareNameTrafficMetrics,
	MiddlewareNameLogging,
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

// MiddlewareEntryConfig declares one middleware in the `middleware.global`
// list, in the order it should run.
//
// Only rate_limiter and cors currently have their own typed per-entry
// configuration; the other built-in middlewares (ja4_fingerprint,
// session_extraction, traffic_metrics, logging) take no options today, so
// listing them just enables/positions them. Per-middleware config for the
// rest is future work (see doc/refactor01.md Improvement 4) — adding it here
// without matching runtime support would be misleading.
type MiddlewareEntryConfig struct {
	Name        string             `yaml:"name"`                  // Middleware identifier. Must be one of KnownMiddlewareNames.
	Enabled     *bool              `yaml:"enabled,omitempty"`     // Enable/disable this middleware. Default: true (listing it implies enabled).
	RateLimiter *RateLimiterConfig `yaml:"rateLimiter,omitempty"` // Per-entry override for "rate_limiter". Falls back to management.rateLimiter when nil.
	CORS        *CORSConfig        `yaml:"cors,omitempty"`        // Per-entry override for "cors". Falls back to management.cors when nil.
}

// IsEnabled reports whether this entry is enabled. An absent Enabled field
// defaults to true: appearing in the list is enough to opt in.
func (e MiddlewareEntryConfig) IsEnabled() bool {
	return e.Enabled == nil || *e.Enabled
}

// MiddlewareSection declares the global middleware chain explicitly, in
// execution order. When Global is nil — i.e. there's no `middleware:`
// section at all, the common case for existing config files — the gateway
// falls back to the legacy management.analytics / management.logging /
// management.rateLimiter flags. See middleware.ResolveGlobalChainSpecs.
//
// Once a `middleware:` section with a `global:` key is present, it takes
// over entirely and the legacy flags are ignored — including when it's
// explicitly listed empty (`global: []`), which means "no global middleware
// at all" rather than falling back. This relies on Global being nil (unset)
// vs. a non-nil empty slice (explicitly `[]`), a distinction YAML
// unmarshaling already preserves correctly (verified in middleware_test.go).
type MiddlewareSection struct {
	Global []MiddlewareEntryConfig `yaml:"global,omitempty"`
}
