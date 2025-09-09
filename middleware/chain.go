package middleware

import (
	"net/http"

	"github.com/jmaister/taronja-gateway/auth"
	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/session"
)

// ChainBuilder provides a fluent interface for building middleware chains
type ChainBuilder struct {
	middlewares []Middleware
}

// Middleware represents a middleware function
type Middleware func(http.Handler) http.Handler

// NewChainBuilder creates a new middleware chain builder
func NewChainBuilder() *ChainBuilder {
	return &ChainBuilder{
		middlewares: make([]Middleware, 0),
	}
}

// Add adds a middleware to the chain
func (c *ChainBuilder) Add(middleware Middleware) *ChainBuilder {
	c.middlewares = append(c.middlewares, middleware)
	return c
}

// Build creates the final middleware chain by wrapping all middlewares around the given handler
func (c *ChainBuilder) Build(handler http.Handler) http.Handler {
	// Apply middlewares in reverse order so they execute in the order they were added
	for i := len(c.middlewares) - 1; i >= 0; i-- {
		handler = c.middlewares[i](handler)
	}
	return handler
}

// BuildGlobalChain builds the global middleware chain based on gateway configuration
func BuildGlobalChain(
	gatewayConfig *config.GatewayConfig,
	sessionStore session.SessionStore,
	tokenService *auth.TokenService,
	trafficMetricRepo db.TrafficMetricRepository,
) *ChainBuilder {
	chain := NewChainBuilder()

	// Add middlewares conditionally based on configuration
	if gatewayConfig.Management.Analytics {
		// JA4H fingerprinting middleware (first so fingerprint is available for other middlewares)
		// chain.Add(JA4Middleware)
		chain.Add(OptimizedJA4Middleware(true))

		// Session extraction middleware (before traffic metrics to capture user info)
		chain.Add(SessionExtractionMiddleware(sessionStore, tokenService))

		// Traffic metrics middleware
		chain.Add(TrafficMetricMiddleware(trafficMetricRepo))
	}

	// Logging middleware (if enabled)
	if gatewayConfig.Management.Logging {
		chain.Add(LoggingMiddleware)
	}

	return chain
}

// RouteChainBuilder builds middleware chains for individual routes
type RouteChainBuilder struct {
	authMiddleware  *AuthMiddleware
	cacheMiddleware *HttpCacheMiddleware
}

// NewRouteChainBuilder creates a new route chain builder
func NewRouteChainBuilder(authMiddleware *AuthMiddleware, cacheMiddleware *HttpCacheMiddleware) *RouteChainBuilder {
	return &RouteChainBuilder{
		authMiddleware:  authMiddleware,
		cacheMiddleware: cacheMiddleware,
	}
}

// BuildRouteChain builds a middleware chain for a specific route using the same pattern as global chain
func (r *RouteChainBuilder) BuildRouteChain(handler http.HandlerFunc, routeConfig config.RouteConfig) http.HandlerFunc {
	chain := NewChainBuilder()

	// Add middlewares conditionally based on route configuration
	// Cache control middleware (always applied)
	chain.Add(r.cacheMiddleware.CacheControlMiddlewareFunc(routeConfig))

	// Authentication middleware (if enabled for this route)
	if routeConfig.Authentication.Enabled {
		chain.Add(r.authMiddleware.AuthMiddlewareFunc(routeConfig.Static))
	}

	return chain.Build(handler).(http.HandlerFunc)
}

// Chain is a simple utility function to chain middlewares without using a builder
// Usage: Chain(handler, middleware1, middleware2, ...)
func Chain(handler http.Handler, middlewares ...Middleware) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}
