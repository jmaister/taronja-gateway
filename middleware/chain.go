package middleware

import "net/http"

// Middleware represents a middleware function
type Middleware func(http.Handler) http.Handler

// Chain wraps handler so the middlewares run in the order given.
// Usage: Chain(handler, middleware1, middleware2, ...)
func Chain(handler http.Handler, middlewares ...Middleware) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}
