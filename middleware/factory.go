package middleware

import (
	"fmt"
	"time"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/session"
)

// MiddlewareFactory creates middleware instances from configuration.
// Each existing middleware gets a factory so it can be registered, discovered,
// and composed at runtime by a MiddlewareRegistryV2 instead of being wired
// directly inside BuildGlobalChain.
type MiddlewareFactory interface {
	// Create builds the middleware instance. cfg is factory-specific; pass
	// GetDefaultConfig() (or a compatible value) when no override is needed.
	Create(cfg interface{}) (Middleware, error)
	// GetName returns the stable identifier used in MiddlewareSpec.Name.
	GetName() string
	// GetDescription returns a short human-readable summary of what the middleware does.
	GetDescription() string
	// GetDependencies returns the names of middleware that must be enabled
	// earlier in the same chain for this middleware to function correctly.
	GetDependencies() []string
	// GetDefaultConfig returns the zero-value/default configuration for this middleware.
	GetDefaultConfig() interface{}
}

// ConcreteFactory is an embeddable base implementation shared by all factories,
// providing the name/description/dependencies bookkeeping.
type ConcreteFactory struct {
	name         string
	description  string
	dependencies []string
}

func (f *ConcreteFactory) GetName() string           { return f.name }
func (f *ConcreteFactory) GetDescription() string    { return f.description }
func (f *ConcreteFactory) GetDependencies() []string { return f.dependencies }

// --- CORSFactory -----------------------------------------------------------

// CORSFactory creates the CORS (Cross-Origin Resource Sharing) response
// header middleware. Has no runtime dependencies of its own — unlike
// RateLimiterFactory, there's no shared instance to reuse, so Create simply
// builds a fresh middleware from whatever config.CORSConfig it's given each
// time.
type CORSFactory struct{ ConcreteFactory }

func NewCORSFactory() *CORSFactory {
	return &CORSFactory{
		ConcreteFactory: ConcreteFactory{
			name:        config.MiddlewareNameCORS,
			description: "Adds CORS response headers for cross-origin requests to the management API",
		},
	}
}

func (f *CORSFactory) Create(cfg interface{}) (Middleware, error) {
	corsCfg, ok := cfg.(config.CORSConfig)
	if !ok {
		return nil, fmt.Errorf("cors: invalid config type %T, expected config.CORSConfig", cfg)
	}
	return CORSMiddleware(corsCfg), nil
}

func (f *CORSFactory) GetDefaultConfig() interface{} {
	return config.CORSConfig{}
}

// --- RateLimiterFactory ---------------------------------------------------

// RateLimiterFactory creates the request rate limiting / vulnerability scan detection middleware.
//
// If constructed with an existing *RateLimiter instance (the common case: the
// gateway keeps one around so its stats/config can be exposed via the
// management API), Create reuses that instance's Handler so the chain and the
// API observe the same state. Otherwise it builds a stateless middleware
// directly from the supplied config.RateLimiterConfig.
type RateLimiterFactory struct {
	ConcreteFactory
	rateLimiter *RateLimiter
}

func NewRateLimiterFactory(rateLimiter *RateLimiter) *RateLimiterFactory {
	return &RateLimiterFactory{
		ConcreteFactory: ConcreteFactory{
			name:        config.MiddlewareNameRateLimiter,
			description: "Limits request rate per client IP and detects vulnerability scans",
		},
		rateLimiter: rateLimiter,
	}
}

func (f *RateLimiterFactory) Create(cfg interface{}) (Middleware, error) {
	if f.rateLimiter != nil {
		return f.rateLimiter.Handler, nil
	}

	rateLimiterCfg, ok := cfg.(config.RateLimiterConfig)
	if !ok {
		return nil, fmt.Errorf("rate_limiter: invalid config type %T, expected config.RateLimiterConfig", cfg)
	}
	return RateLimiterMiddleware(rateLimiterCfg), nil
}

func (f *RateLimiterFactory) GetDefaultConfig() interface{} {
	return config.RateLimiterConfig{}
}

// HealthCheck reports the rate limiter's current in-memory state: how many
// client IPs it's tracking and how many are presently blocked. It implements
// HealthChecker (see health.go). The rate limiter has no failure mode of its
// own — it's always considered "healthy" once an instance exists — so this
// exists to surface useful operational detail, not to detect outages.
func (f *RateLimiterFactory) HealthCheck() MiddlewareHealth {
	if f.rateLimiter == nil {
		return MiddlewareHealth{Status: "unknown", Message: "no rate limiter instance configured"}
	}

	stats := f.rateLimiter.Stats()
	now := time.Now()
	blocked := 0
	for _, s := range stats {
		if s.BlockedUntil.After(now) {
			blocked++
		}
	}

	return MiddlewareHealth{
		Status:  "healthy",
		Message: fmt.Sprintf("tracking %d client IP(s), %d currently blocked", len(stats), blocked),
	}
}

// --- JA4Factory ------------------------------------------------------------

// JA4Factory creates the JA4H TLS/HTTP fingerprinting middleware (cached variant).
type JA4Factory struct{ ConcreteFactory }

func NewJA4Factory() *JA4Factory {
	return &JA4Factory{
		ConcreteFactory: ConcreteFactory{
			name:        config.MiddlewareNameJA4Fingerprint,
			description: "Computes and caches a JA4H fingerprint for each request",
		},
	}
}

func (f *JA4Factory) Create(cfg interface{}) (Middleware, error) {
	return OptimizedJA4Middleware(true), nil
}

func (f *JA4Factory) GetDefaultConfig() interface{} {
	return struct{}{}
}

// --- SessionExtractionFactory ----------------------------------------------

// SessionExtractionFactory creates the middleware that resolves the current
// session/user (from cookie or bearer token) and attaches it to the request context.
type SessionExtractionFactory struct {
	ConcreteFactory
	sessionStore session.SessionStore
	tokenService session.TokenService
}

// NewSessionExtractionFactory declares a dependency on ja4_fingerprint,
// preserving the ordering of the original hardcoded BuildGlobalChain (whose
// comment read "JA4H fingerprinting middleware (first so fingerprint is
// available for other middlewares)"). Note this is a conservative choice,
// not a verified direct read: SessionExtractionMiddleware itself doesn't
// read the JA4H header. The actual consumer is session.NewClientInfo
// (session/clientinfo.go), invoked later during login/session creation —
// outside this middleware entirely. The dependency stays declared because
// removing it without being certain nothing else relies on JA4H already
// being on the request by this point in the chain isn't worth the risk for
// a purely cosmetic simplification.
func NewSessionExtractionFactory(sessionStore session.SessionStore, tokenService session.TokenService) *SessionExtractionFactory {
	return &SessionExtractionFactory{
		ConcreteFactory: ConcreteFactory{
			name:         config.MiddlewareNameSessionExtraction,
			description:  "Resolves the authenticated session/user and attaches it to the request context",
			dependencies: []string{config.MiddlewareNameJA4Fingerprint},
		},
		sessionStore: sessionStore,
		tokenService: tokenService,
	}
}

func (f *SessionExtractionFactory) Create(cfg interface{}) (Middleware, error) {
	return SessionExtractionMiddleware(f.sessionStore, f.tokenService), nil
}

func (f *SessionExtractionFactory) GetDefaultConfig() interface{} {
	return struct{}{}
}

// --- TrafficMetricsFactory ---------------------------------------------------

// TrafficMetricsFactory creates the middleware that records per-request traffic metrics.
type TrafficMetricsFactory struct {
	ConcreteFactory
	trafficMetricRepo db.TrafficMetricRepository
}

// NewTrafficMetricsFactory declares two dependencies, both verified reads
// (not just preserved ordering, unlike session_extraction's — see
// NewSessionExtractionFactory):
//   - session_extraction: TrafficMetricMiddleware reads the session from the
//     request context (middleware/trafficmetric.go), which
//     SessionExtractionMiddleware puts there.
//   - ja4_fingerprint: TrafficMetricMiddleware calls session.NewTrafficMetric,
//     which calls session.NewClientInfo, which reads the JA4H fingerprint
//     header (session/clientinfo.go) that JA4Middleware/OptimizedJA4Middleware
//     set. This is declared explicitly, even though building a chain with
//     session_extraction already transitively guarantees ja4_fingerprint ran
//     first (session_extraction itself declares that dependency, if only to
//     preserve the original chain's ordering) — spelling it out here means
//     traffic_metrics's real requirement doesn't silently break if
//     session_extraction's dependency list is ever "cleaned up" by someone
//     who (reasonably) can't find a direct read to justify it.
func NewTrafficMetricsFactory(trafficMetricRepo db.TrafficMetricRepository) *TrafficMetricsFactory {
	return &TrafficMetricsFactory{
		ConcreteFactory: ConcreteFactory{
			name:         config.MiddlewareNameTrafficMetrics,
			description:  "Records per-request traffic metrics (status, size, duration, user)",
			dependencies: []string{config.MiddlewareNameSessionExtraction, config.MiddlewareNameJA4Fingerprint},
		},
		trafficMetricRepo: trafficMetricRepo,
	}
}

func (f *TrafficMetricsFactory) Create(cfg interface{}) (Middleware, error) {
	return TrafficMetricMiddleware(f.trafficMetricRepo), nil
}

func (f *TrafficMetricsFactory) GetDefaultConfig() interface{} {
	return struct{}{}
}

// --- LoggingFactory ----------------------------------------------------------

// LoggingFactory creates the request/response access-logging middleware.
type LoggingFactory struct{ ConcreteFactory }

func NewLoggingFactory() *LoggingFactory {
	return &LoggingFactory{
		ConcreteFactory: ConcreteFactory{
			name:        config.MiddlewareNameLogging,
			description: "Logs each request/response (method, path, status, duration)",
		},
	}
}

func (f *LoggingFactory) Create(cfg interface{}) (Middleware, error) {
	return LoggingMiddleware, nil
}

func (f *LoggingFactory) GetDefaultConfig() interface{} {
	return struct{}{}
}

// --- Global factory registry --------------------------------------------

// MiddlewareFactoryMap is a package-level lookup of factories by name, primarily
// intended for discovery/introspection. MiddlewareRegistryV2 keeps its own
// per-instance registrations and does not read from this map.
var MiddlewareFactoryMap = map[string]MiddlewareFactory{}

// RegisterFactory adds a factory to the global MiddlewareFactoryMap, keyed by its name.
func RegisterFactory(factory MiddlewareFactory) {
	MiddlewareFactoryMap[factory.GetName()] = factory
}

// GetFactory looks up a factory in the global MiddlewareFactoryMap by name.
func GetFactory(name string) (MiddlewareFactory, bool) {
	factory, exists := MiddlewareFactoryMap[name]
	return factory, exists
}

// ListFactories returns the names of all factories in the global MiddlewareFactoryMap.
func ListFactories() []string {
	names := make([]string, 0, len(MiddlewareFactoryMap))
	for name := range MiddlewareFactoryMap {
		names = append(names, name)
	}
	return names
}
