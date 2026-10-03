package builtin

import (
	"fmt"

	"github.com/jmaister/taronja-gateway/auth"
	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/middleware"
	"github.com/jmaister/taronja-gateway/session"
)

// newGlobalRegistry registers a factory for every built-in global middleware
// (compression, cors, rate_limiter, ja4_fingerprint, session_extraction,
// traffic_metrics, logging, tracing), wired to the given dependencies.
// Factories tolerate nil dependencies as long as Create is not called, which
// lets ValidateGlobalChainSpecs reuse it without live dependencies.
func newGlobalRegistry(
	sessionStore session.SessionStore,
	tokenService *auth.TokenService,
	trafficMetricRepo db.TrafficMetricRepository,
	rateLimiter *RateLimiter,
) (*middleware.Registry, error) {
	registry := middleware.NewRegistry()
	factories := []middleware.MiddlewareFactory{
		NewCompressionFactory(),
		NewCORSFactory(),
		NewRateLimiterFactory(rateLimiter),
		NewJA4Factory(),
		NewSessionExtractionFactory(sessionStore, tokenService),
		NewTrafficMetricsFactory(trafficMetricRepo),
		NewLoggingFactory(),
		NewTracingFactory(),
	}
	for _, f := range factories {
		if err := registry.RegisterFactory(f); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

// NewGlobalChain is the single entry point for building the global
// middleware chain. It resolves gatewayConfig to an ordered spec list and
// builds the chain through a registry of the built-in factories. The registry
// is returned so callers can expose status, health and metrics.
func NewGlobalChain(
	gatewayConfig *config.GatewayConfig,
	sessionStore session.SessionStore,
	tokenService *auth.TokenService,
	trafficMetricRepo db.TrafficMetricRepository,
	rateLimiter *RateLimiter,
) (*middleware.Registry, *middleware.ChainBuilder, error) {
	registry, err := newGlobalRegistry(sessionStore, tokenService, trafficMetricRepo, rateLimiter)
	if err != nil {
		return nil, nil, err
	}
	specs, err := ResolveGlobalChainSpecs(gatewayConfig)
	if err != nil {
		return nil, nil, err
	}
	chain, err := registry.BuildChain(specs)
	if err != nil {
		return nil, nil, err
	}
	return registry, chain, nil
}

// ValidateGlobalChainSpecs validates specs (typically produced by
// ResolveGlobalChainSpecs) against the built-in global middleware factories:
// every name must be recognized and every dependency must be satisfied by an
// earlier spec. It does not require real middleware dependencies.
func ValidateGlobalChainSpecs(specs []middleware.MiddlewareSpec) error {
	registry, err := newGlobalRegistry(nil, nil, nil, nil)
	if err != nil {
		return err
	}
	return registry.ValidateSpecs(specs)
}

// ResolveGlobalChainSpecs translates configuration into the ordered global chain.
//
// If Middleware.Global is non-nil (even an empty list, meaning no middleware)
// it is used as listed, skipping disabled entries. Otherwise the legacy
// management.analytics/logging/rateLimiter/cors/compression flags are
// translated into the equivalent specs.
func ResolveGlobalChainSpecs(gatewayConfig *config.GatewayConfig) ([]middleware.MiddlewareSpec, error) {
	if gatewayConfig.Middleware.Global != nil {
		return specsFromMiddlewareSection(gatewayConfig)
	}
	return legacySpecsFromConfig(gatewayConfig), nil
}

// EffectiveRateLimiterConfig returns the rate limiter config the resolved chain
// will use: a per-entry override from middleware.global, else
// management.rateLimiter. The gateway builds its shared RateLimiter before the
// registry, so it must resolve the config with this rather than reading
// Management.RateLimiter or an override would be ignored. If Middleware.Global is
// invalid it falls back to Management.RateLimiter; NewGlobalChain reports the error.
func EffectiveRateLimiterConfig(gatewayConfig *config.GatewayConfig) config.RateLimiterConfig {
	specs, err := ResolveGlobalChainSpecs(gatewayConfig)
	if err != nil {
		return gatewayConfig.Management.RateLimiter
	}
	for _, spec := range specs {
		if spec.Name != config.MiddlewareNameRateLimiter {
			continue
		}
		if cfg, ok := spec.Config.(config.RateLimiterConfig); ok {
			return cfg
		}
	}
	return gatewayConfig.Management.RateLimiter
}

// EffectiveCORSConfig returns the CORS config the resolved chain will use: a
// per-entry override from middleware.global, else management.cors. Validators
// must use it, or a per-entry wildcard origin with allowCredentials would pass
// validation. If Middleware.Global is invalid it falls back to Management.CORS;
// NewGlobalChain reports the error.
func EffectiveCORSConfig(gatewayConfig *config.GatewayConfig) config.CORSConfig {
	specs, err := ResolveGlobalChainSpecs(gatewayConfig)
	if err != nil {
		return gatewayConfig.Management.CORS
	}
	for _, spec := range specs {
		if spec.Name != config.MiddlewareNameCORS {
			continue
		}
		if cfg, ok := spec.Config.(config.CORSConfig); ok {
			return cfg
		}
	}
	return gatewayConfig.Management.CORS
}

// specsFromMiddlewareSection builds specs from an explicit
// gatewayConfig.Middleware.Global list.
func specsFromMiddlewareSection(gatewayConfig *config.GatewayConfig) ([]middleware.MiddlewareSpec, error) {
	specs := make([]middleware.MiddlewareSpec, 0, len(gatewayConfig.Middleware.Global))

	for _, entry := range gatewayConfig.Middleware.Global {
		if !config.IsMiddlewareNameKnown(entry.Name) {
			// config.LoadConfig already rejects unknown names at load time; this
			// guards GatewayConfig values built programmatically (e.g. in tests)
			// that bypass LoadConfig.
			return nil, fmt.Errorf("middleware.global: unknown middleware '%s'", entry.Name)
		}
		if !entry.IsEnabled() {
			continue
		}

		spec := middleware.MiddlewareSpec{Name: entry.Name}
		switch entry.Name {
		case config.MiddlewareNameRateLimiter:
			if entry.RateLimiter != nil {
				spec.Config = *entry.RateLimiter
			} else {
				spec.Config = gatewayConfig.Management.RateLimiter
			}
		case config.MiddlewareNameCORS:
			if entry.CORS != nil {
				spec.Config = *entry.CORS
			} else {
				spec.Config = gatewayConfig.Management.CORS
			}
		case config.MiddlewareNameTrafficMetrics:
			if entry.TrafficMetrics != nil {
				spec.Config = *entry.TrafficMetrics
			} else {
				spec.Config = config.TrafficMetricsConfig{ExcludeStaticAssets: gatewayConfig.Management.ExcludeStaticAssets}
			}
		}
		specs = append(specs, spec)
	}

	return specs, nil
}

// legacySpecsFromConfig derives specs from the pre-Phase-2 configuration
// flags: management.compression, management.cors, management.rateLimiter,
// management.analytics, management.logging, and the top-level tracing
// section.
func legacySpecsFromConfig(gatewayConfig *config.GatewayConfig) []middleware.MiddlewareSpec {
	specs := []middleware.MiddlewareSpec{}

	// Tracing runs before even compression: it needs to see the request
	// before anything else touches it, to extract an incoming W3C
	// "traceparent" header (continuing a caller's trace) as early as
	// possible, and its span should cover the full request lifecycle —
	// compression time included — to report an accurate total duration.
	if gatewayConfig.Tracing.Enabled {
		specs = append(specs, middleware.MiddlewareSpec{Name: config.MiddlewareNameTracing})
	}

	// Compression wraps the ResponseWriter, so it sits at the edge of the chain.
	// Wrappers after it still record the uncompressed status and size.
	if gatewayConfig.Management.Compression {
		specs = append(specs, middleware.MiddlewareSpec{Name: config.MiddlewareNameCompression})
	}

	// CORS runs next, before anything else gets a chance to reject a
	// preflight OPTIONS request (rate limiting, auth, ...) — a preflight is
	// never meant to reach application logic at all, so it needs to be
	// answered before any of that runs.
	if gatewayConfig.Management.CORS.IsEnabled() {
		specs = append(specs, middleware.MiddlewareSpec{
			Name:   config.MiddlewareNameCORS,
			Config: gatewayConfig.Management.CORS,
		})
	}

	// Rate limiter runs next, before analytics.
	if gatewayConfig.Management.RateLimiter.IsEnabled() {
		specs = append(specs, middleware.MiddlewareSpec{
			Name:   config.MiddlewareNameRateLimiter,
			Config: gatewayConfig.Management.RateLimiter,
		})
	}

	// Analytics group: JA4H fingerprint -> session extraction -> traffic metrics.
	if gatewayConfig.Management.Analytics {
		specs = append(specs, middleware.MiddlewareSpec{Name: config.MiddlewareNameJA4Fingerprint})
		specs = append(specs, middleware.MiddlewareSpec{Name: config.MiddlewareNameSessionExtraction})
		specs = append(specs, middleware.MiddlewareSpec{
			Name:   config.MiddlewareNameTrafficMetrics,
			Config: config.TrafficMetricsConfig{ExcludeStaticAssets: gatewayConfig.Management.ExcludeStaticAssets},
		})
	}

	// Logging.
	if gatewayConfig.Management.Logging {
		specs = append(specs, middleware.MiddlewareSpec{Name: config.MiddlewareNameLogging})
	}

	return specs
}
