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

// ResolveGlobalChainSpecs translates gateway configuration into an ordered
// list of MiddlewareSpec describing the global chain.
//
// If gatewayConfig.Middleware.Global is non-nil (a `middleware:` section with
// a `global:` key is present in the config file, even if listed as an empty
// array to explicitly mean "no global middleware at all"), it is used
// directly: each entry becomes a spec, in the order listed, skipping entries
// with Enabled=false. This is the Phase 2 declarative path (see
// doc/refactor01.md). Note this checks for nil, not length: config.LoadConfig
// (via YAML) and a literal Go struct both leave Global nil when no section is
// present, but `global: []` unmarshals to a non-nil empty slice — that
// distinction is what lets a config explicitly disable every global
// middleware, rather than an empty list being indistinguishable from an
// absent section and silently falling back to the legacy flags below.
//
// Otherwise (Global is nil), the legacy management.analytics /
// management.logging / management.rateLimiter / management.cors /
// management.compression flags are translated into the equivalent specs —
// the same chain the gateway always built — so existing config files keep
// working unchanged.
func ResolveGlobalChainSpecs(gatewayConfig *config.GatewayConfig) ([]middleware.MiddlewareSpec, error) {
	if gatewayConfig.Middleware.Global != nil {
		return specsFromMiddlewareSection(gatewayConfig)
	}
	return legacySpecsFromConfig(gatewayConfig), nil
}

// EffectiveRateLimiterConfig returns the config.RateLimiterConfig that
// resolving the global chain (ResolveGlobalChainSpecs) would actually use to
// build the rate_limiter middleware: a per-entry override from an explicit
// `middleware.global` rate_limiter entry if one is present, otherwise
// management.rateLimiter (which is also what an explicit entry with no
// override falls back to, and what's used when there's no explicit
// `middleware:` section at all).
//
// This exists because the gateway constructs its shared *RateLimiter
// instance (see gateway.go's createHTTPServer) before the registry is built,
// so it can't just rely on RateLimiterFactory.Create's cfg argument — that
// instance is reused directly by RateLimiterFactory whenever one is supplied
// (see factory.go), so whatever config *it* was built with is what actually
// takes effect, regardless of any per-entry override in the spec. Callers
// that construct the shared instance need to resolve the effective config
// with this function first, rather than reading gatewayConfig.Management.RateLimiter
// directly, or a per-entry override would be silently ignored.
//
// If gatewayConfig.Middleware.Global is invalid (e.g. an unknown middleware
// name), this returns gatewayConfig.Management.RateLimiter rather than an
// error; the real error will surface when NewGlobalChain is called
// against the same config.
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

// EffectiveCORSConfig returns the config.CORSConfig that resolving the
// global chain (ResolveGlobalChainSpecs) would actually use to build the
// cors middleware: a per-entry override from an explicit `middleware.global`
// cors entry if one is present, otherwise management.cors (which is also
// what an explicit entry with no override falls back to, and what's used
// when there's no explicit `middleware:` section at all).
//
// This exists for the same reason EffectiveRateLimiterConfig does: a
// validator that only ever reads gatewayConfig.Management.CORS directly
// silently misses a per-entry override in the `middleware:` section — which
// is exactly what let a config combining a wildcard origin with
// allowCredentials pass ValidateCORSMiddleware and then serve
// Access-Control-Allow-Credentials: true while reflecting any Origin back
// verbatim, completely defeating the one thing that check exists to
// prevent. Callers validating (or otherwise needing to know) the CORS
// config that will actually take effect at runtime must resolve it with
// this function first, not by reading gatewayConfig.Management.CORS
// directly.
//
// If gatewayConfig.Middleware.Global is invalid (e.g. an unknown middleware
// name), this returns gatewayConfig.Management.CORS rather than an error;
// the real error will surface when NewGlobalChain is called
// against the same config.
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

	// Compression runs next (outermost of what's left): it wraps the ResponseWriter that
	// every other middleware and the final route handler write through, so
	// it must sit at the very edge of the chain to compress the bytes that
	// actually reach the socket. Middlewares added after it (traffic_metrics,
	// logging) still observe the uncompressed status/byte count the handler
	// produced, since compression only affects what compressingResponseWriter
	// forwards to the real ResponseWriter beneath it, not what gets recorded
	// by wrappers above it in the chain.
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
