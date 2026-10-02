package builtin

import (
	"fmt"
	"time"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/middleware"
	"github.com/jmaister/taronja-gateway/session"
)

// CORSFactory creates the CORS (Cross-Origin Resource Sharing) response
// header middleware. Has no runtime dependencies of its own — unlike
// RateLimiterFactory, there's no shared instance to reuse, so Create simply
// builds a fresh middleware from whatever config.CORSConfig it's given each
// time.
type CORSFactory struct{ middleware.ConcreteFactory }

func NewCORSFactory() *CORSFactory {
	return &CORSFactory{
		ConcreteFactory: middleware.NewConcreteFactory(config.MiddlewareNameCORS, "Adds CORS response headers for cross-origin requests to the management API"),
	}
}

func (f *CORSFactory) Create(cfg interface{}) (middleware.Middleware, error) {
	corsCfg, ok := cfg.(config.CORSConfig)
	if !ok {
		return nil, fmt.Errorf("cors: invalid config type %T, expected config.CORSConfig", cfg)
	}
	return CORSMiddleware(corsCfg), nil
}

func (f *CORSFactory) GetDefaultConfig() interface{} {
	return config.CORSConfig{}
}

// --- CompressionFactory -----------------------------------------------------

// CompressionFactory creates the brotli/zstd/gzip/deflate response
// compression middleware. Has no runtime dependencies and no configuration
// of its own — see CompressionMiddleware's doc comment for why.
type CompressionFactory struct{ middleware.ConcreteFactory }

func NewCompressionFactory() *CompressionFactory {
	return &CompressionFactory{
		ConcreteFactory: middleware.NewConcreteFactory(config.MiddlewareNameCompression, "Compresses response bodies with brotli, zstd, gzip, or deflate when the client accepts it"),
	}
}

func (f *CompressionFactory) Create(cfg interface{}) (middleware.Middleware, error) {
	return CompressionMiddleware, nil
}

func (f *CompressionFactory) GetDefaultConfig() interface{} {
	return struct{}{}
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
	middleware.ConcreteFactory
	rateLimiter *RateLimiter
}

func NewRateLimiterFactory(rateLimiter *RateLimiter) *RateLimiterFactory {
	return &RateLimiterFactory{
		ConcreteFactory: middleware.NewConcreteFactory(config.MiddlewareNameRateLimiter, "Limits request rate per client IP and detects vulnerability scans"),
		rateLimiter:     rateLimiter,
	}
}

func (f *RateLimiterFactory) Create(cfg interface{}) (middleware.Middleware, error) {
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
func (f *RateLimiterFactory) HealthCheck() middleware.MiddlewareHealth {
	if f.rateLimiter == nil {
		return middleware.MiddlewareHealth{Status: "unknown", Message: "no rate limiter instance configured"}
	}

	stats := f.rateLimiter.Stats()
	now := time.Now()
	blocked := 0
	for _, s := range stats {
		if s.BlockedUntil.After(now) {
			blocked++
		}
	}

	return middleware.MiddlewareHealth{
		Status:  "healthy",
		Message: fmt.Sprintf("tracking %d client IP(s), %d currently blocked", len(stats), blocked),
	}
}

// --- JA4Factory ------------------------------------------------------------

// JA4Factory creates the JA4H TLS/HTTP fingerprinting middleware (cached variant).
type JA4Factory struct{ middleware.ConcreteFactory }

func NewJA4Factory() *JA4Factory {
	return &JA4Factory{
		ConcreteFactory: middleware.NewConcreteFactory(config.MiddlewareNameJA4Fingerprint, "Computes and caches a JA4H fingerprint for each request"),
	}
}

func (f *JA4Factory) Create(cfg interface{}) (middleware.Middleware, error) {
	return OptimizedJA4Middleware(true), nil
}

func (f *JA4Factory) GetDefaultConfig() interface{} {
	return struct{}{}
}

// --- SessionExtractionFactory ----------------------------------------------

// SessionExtractionFactory creates the middleware that resolves the current
// session/user (from cookie or bearer token) and attaches it to the request context.
type SessionExtractionFactory struct {
	middleware.ConcreteFactory
	sessionStore session.SessionStore
	tokenService session.TokenService
}

// NewSessionExtractionFactory declares a dependency on ja4_fingerprint,
// preserving the gateway's original chain order (JA4H fingerprinting first so
// the fingerprint is available to other middlewares). Note this is a conservative choice,
// not a verified direct read: SessionExtractionMiddleware itself doesn't
// read the JA4H header. The actual consumer is session.NewClientInfo
// (session/clientinfo.go), invoked later during login/session creation —
// outside this middleware entirely. The dependency stays declared because
// removing it without being certain nothing else relies on JA4H already
// being on the request by this point in the chain isn't worth the risk for
// a purely cosmetic simplification.
func NewSessionExtractionFactory(sessionStore session.SessionStore, tokenService session.TokenService) *SessionExtractionFactory {
	return &SessionExtractionFactory{
		ConcreteFactory: middleware.NewConcreteFactory(config.MiddlewareNameSessionExtraction, "Resolves the authenticated session/user and attaches it to the request context", config.MiddlewareNameJA4Fingerprint),
		sessionStore:    sessionStore,
		tokenService:    tokenService,
	}
}

func (f *SessionExtractionFactory) Create(cfg interface{}) (middleware.Middleware, error) {
	return SessionExtractionMiddleware(f.sessionStore, f.tokenService), nil
}

func (f *SessionExtractionFactory) GetDefaultConfig() interface{} {
	return struct{}{}
}

// --- TrafficMetricsFactory ---------------------------------------------------

// TrafficMetricsFactory creates the middleware that records per-request traffic metrics.
type TrafficMetricsFactory struct {
	middleware.ConcreteFactory
	trafficMetricRepo db.TrafficMetricRepository
}

// NewTrafficMetricsFactory declares two dependencies, both verified reads
// (not just preserved ordering, unlike session_extraction's — see
// NewSessionExtractionFactory):
//   - session_extraction: TrafficMetricMiddleware reads the session from the
//     request context (middleware/builtin/trafficmetric.go), which
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
		ConcreteFactory:   middleware.NewConcreteFactory(config.MiddlewareNameTrafficMetrics, "Records per-request traffic metrics (status, size, duration, user)", config.MiddlewareNameSessionExtraction, config.MiddlewareNameJA4Fingerprint),
		trafficMetricRepo: trafficMetricRepo,
	}
}

func (f *TrafficMetricsFactory) Create(cfg interface{}) (middleware.Middleware, error) {
	tmCfg, _ := cfg.(config.TrafficMetricsConfig) // zero value (ExcludeStaticAssets: false) if cfg is unset/wrong type
	return TrafficMetricMiddleware(f.trafficMetricRepo, tmCfg.ExcludeStaticAssets), nil
}

func (f *TrafficMetricsFactory) GetDefaultConfig() interface{} {
	return config.TrafficMetricsConfig{}
}

// --- LoggingFactory ----------------------------------------------------------

// LoggingFactory creates the request/response access-logging middleware.
type LoggingFactory struct{ middleware.ConcreteFactory }

func NewLoggingFactory() *LoggingFactory {
	return &LoggingFactory{
		ConcreteFactory: middleware.NewConcreteFactory(config.MiddlewareNameLogging, "Logs each request/response (method, path, status, duration)"),
	}
}

func (f *LoggingFactory) Create(cfg interface{}) (middleware.Middleware, error) {
	return LoggingMiddleware, nil
}

func (f *LoggingFactory) GetDefaultConfig() interface{} {
	return struct{}{}
}
