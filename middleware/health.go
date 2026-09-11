package middleware

// MiddlewareHealth describes the outcome of a middleware's health check.
type MiddlewareHealth struct {
	Status  string `json:"status"`            // "healthy", "degraded", "unhealthy", or "unknown"
	Message string `json:"message,omitempty"` // Human-readable detail, e.g. current internal state.
}

// HealthChecker is optionally implemented by a MiddlewareFactory to report
// the runtime health of the middleware it creates. Most of the built-in
// global middlewares are effectively stateless per request and have nothing
// meaningful to check; only factories with real runtime state (currently
// just RateLimiterFactory) implement this. GetStatus and GetHealth report
// "unknown" for factories that don't implement it, rather than fabricating a
// "healthy" result for something that was never actually checked.
type HealthChecker interface {
	HealthCheck() MiddlewareHealth
}

// GetHealth returns the health of the named middleware. The second return
// value is false if name is not a registered factory; a registered factory
// that doesn't implement HealthChecker still returns true, with
// MiddlewareHealth{Status: "unknown"}.
func (r *MiddlewareRegistryV2) GetHealth(name string) (MiddlewareHealth, bool) {
	factory, exists := r.factories[name]
	if !exists {
		return MiddlewareHealth{}, false
	}
	if hc, ok := factory.(HealthChecker); ok {
		return hc.HealthCheck(), true
	}
	return MiddlewareHealth{Status: "unknown", Message: "no health check implemented"}, true
}
