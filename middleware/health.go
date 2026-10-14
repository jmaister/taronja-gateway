package middleware

// MiddlewareHealth describes the outcome of a middleware's health check.
type MiddlewareHealth struct {
	Status  string `json:"status"`            // "healthy", "degraded", "unhealthy", or "unknown"
	Message string `json:"message,omitempty"` // Human-readable detail, e.g. current internal state.
}
