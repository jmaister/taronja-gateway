package middleware

import (
	"fmt"

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

func NewTrafficMetricsFactory(trafficMetricRepo db.TrafficMetricRepository) *TrafficMetricsFactory {
	return &TrafficMetricsFactory{
		ConcreteFactory: ConcreteFactory{
			name:         config.MiddlewareNameTrafficMetrics,
			description:  "Records per-request traffic metrics (status, size, duration, user)",
			dependencies: []string{config.MiddlewareNameSessionExtraction},
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
