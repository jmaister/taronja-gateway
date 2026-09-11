// Package requestid is a worked example of a third-party middleware for
// Taronja Gateway, written exactly as an external module would be: it only
// imports the gateway's public packages (middleware), and plugs into the
// global middleware chain purely by implementing middleware.MiddlewareFactory
// — no changes to the gateway's own source are needed.
//
// See doc/middleware_development.md for the accompanying walkthrough, and
// requestid_test.go for it being registered and built through the real
// middleware.MiddlewareRegistryV2, the same way the gateway's own built-in
// middleware are.
package requestid

import (
	"context"
	"crypto/rand"
	"net/http"

	"github.com/jmaister/taronja-gateway/middleware"
	"github.com/lucsky/cuid"
)

// HeaderName is the HTTP header used to carry the request ID, both inbound
// (a caller-supplied ID is honored, useful for tracing across services) and
// outbound (always echoed back on the response).
const HeaderName = "X-Request-Id"

type contextKey string

// ContextKey is where Middleware stores the resolved request ID in the
// request context.
const ContextKey contextKey = "request_id"

// FromContext returns the request ID Middleware attached to ctx, or "" if
// none is present (e.g. the middleware wasn't installed).
func FromContext(ctx context.Context) string {
	id, _ := ctx.Value(ContextKey).(string)
	return id
}

// Middleware assigns a request ID to every request: it reuses an inbound
// X-Request-Id header if the client already set one, or generates a new one
// otherwise. The ID is attached to the request context (read it back with
// FromContext) and set on the response header either way.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(HeaderName)
		if id == "" {
			generated, err := cuid.NewCrypto(rand.Reader)
			if err != nil {
				// Extremely unlikely (crypto/rand failure). Fall back rather than
				// fail the request over a tracing header.
				generated = "unavailable"
			}
			id = generated
		}

		w.Header().Set(HeaderName, id)
		ctx := context.WithValue(r.Context(), ContextKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Factory implements middleware.MiddlewareFactory — the interface the
// gateway's registry (middleware.MiddlewareRegistryV2) uses to discover and
// build middleware. This is the only integration point a third-party
// middleware needs; nothing here is exported by or coupled to the gateway's
// internal packages.
type Factory struct{}

// NewFactory creates a Factory. In a real third-party middleware, this is
// typically where you'd accept and store constructor dependencies (a logger,
// a metrics client, ...); Middleware here needs none.
func NewFactory() *Factory { return &Factory{} }

// Create returns the middleware function. cfg is unused since this
// middleware takes no configuration — see GetDefaultConfig.
func (f *Factory) Create(cfg interface{}) (middleware.Middleware, error) {
	return Middleware, nil
}

// GetName returns the stable identifier used in middleware.MiddlewareSpec.Name
// and in config's `middleware.global[].name` (if this factory were registered
// into the gateway's own registry — see doc/middleware_development.md).
func (f *Factory) GetName() string { return "request_id" }

func (f *Factory) GetDescription() string {
	return "Assigns (or propagates) a per-request X-Request-Id for tracing"
}

// GetDependencies returns nil: request ID assignment has no ordering
// requirement relative to any other middleware. A non-empty slice here would
// mean "the named middleware(s) must already be in the chain before this
// one" — see MiddlewareRegistryV2.BuildChain.
func (f *Factory) GetDependencies() []string { return nil }

// GetDefaultConfig returns struct{}{}: this middleware takes no configuration.
func (f *Factory) GetDefaultConfig() interface{} { return struct{}{} }

// HealthCheck implements the optional middleware.HealthChecker interface,
// purely to demonstrate the pattern — this middleware has no real failure
// mode, so it always reports healthy. A factory with genuine runtime state (a
// connection pool, a circuit breaker, ...) would report degraded/unhealthy
// here instead; a factory with nothing meaningful to check can simply not
// implement HealthChecker at all (see the gateway's own JA4Factory,
// SessionExtractionFactory, TrafficMetricsFactory, LoggingFactory), and
// GetStatus/GetHealth will report "unknown" rather than a fabricated
// "healthy".
func (f *Factory) HealthCheck() middleware.MiddlewareHealth {
	return middleware.MiddlewareHealth{Status: "healthy", Message: "stateless; no failure modes"}
}

// Compile-time check that Factory satisfies both interfaces.
var (
	_ middleware.MiddlewareFactory = (*Factory)(nil)
	_ middleware.HealthChecker     = (*Factory)(nil)
)
