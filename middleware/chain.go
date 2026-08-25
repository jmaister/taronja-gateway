package middleware

import (
	"log"
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

// BuildGlobalChain builds the global middleware chain based on gateway
// configuration. It delegates to BuildGlobalChainV2 (the factory/registry
// system — see doc/refactor01.md Phase 2), so it now understands both the
// legacy management.analytics/logging/rateLimiter flags and an explicit
// `middleware:` config section, exactly like BuildGlobalChainV2.
//
// This function has no error return, for backward compatibility with any
// existing callers, so a build failure (e.g. an invalid explicit middleware
// section) is logged and results in an empty chain rather than a panic.
// Prefer calling BuildGlobalChainV2 directly where an error return is
// acceptable, since it surfaces misconfiguration instead of silently
// dropping the chain.
func BuildGlobalChain(
	gatewayConfig *config.GatewayConfig,
	sessionStore session.SessionStore,
	tokenService *auth.TokenService,
	trafficMetricRepo db.TrafficMetricRepository,
	rateLimiter *RateLimiter,
) *ChainBuilder {
	chain, err := BuildGlobalChainV2(gatewayConfig, sessionStore, tokenService, trafficMetricRepo, rateLimiter)
	if err != nil {
		log.Printf("BuildGlobalChain: failed to build middleware chain via registry: %v; falling back to an empty chain", err)
		return NewChainBuilder()
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

	// Authentication middleware (if enabled for this route)
	if routeConfig.Authentication.Enabled {
		// Redirect to login page for static routes and SPA proxy routes (browser-facing),
		// return 401 for plain proxy/API routes.
		shouldRedirect := routeConfig.Static || routeConfig.IsSPA
		chain.Add(r.authMiddleware.AuthMiddlewareFunc(shouldRedirect))
	}

	// Cache control middleware (always applied)
	chain.Add(r.cacheMiddleware.CacheControlMiddlewareFunc(routeConfig))

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

// NewGlobalMiddlewareRegistry builds a MiddlewareRegistryV2 with a factory
// registered for every built-in global middleware (rate_limiter,
// ja4_fingerprint, session_extraction, traffic_metrics, logging), wired to
// the given dependencies. It's exposed separately from BuildGlobalChainV2 so
// callers that need to introspect the registry after building the chain —
// e.g. a middleware status/health/metrics API endpoint (doc/refactor01.md
// Phase 3) — can keep a reference to it instead of it being discarded once
// the chain is built.
func NewGlobalMiddlewareRegistry(
	sessionStore session.SessionStore,
	tokenService *auth.TokenService,
	trafficMetricRepo db.TrafficMetricRepository,
	rateLimiter *RateLimiter,
) (*MiddlewareRegistryV2, error) {
	registry := NewMiddlewareRegistryV2()

	if err := registry.RegisterFactory(NewRateLimiterFactory(rateLimiter)); err != nil {
		return nil, err
	}
	if err := registry.RegisterFactory(NewJA4Factory()); err != nil {
		return nil, err
	}
	if err := registry.RegisterFactory(NewSessionExtractionFactory(sessionStore, tokenService)); err != nil {
		return nil, err
	}
	if err := registry.RegisterFactory(NewTrafficMetricsFactory(trafficMetricRepo)); err != nil {
		return nil, err
	}
	if err := registry.RegisterFactory(NewLoggingFactory()); err != nil {
		return nil, err
	}

	return registry, nil
}

// BuildGlobalChainV2 builds the global middleware chain using the factory +
// registry system (MiddlewareRegistryV2) instead of the hardcoded conditionals
// in BuildGlobalChain. It registers a factory for every middleware currently
// wired into BuildGlobalChain (via NewGlobalMiddlewareRegistry) and builds
// the chain from gatewayConfig via BuildGlobalChainFromConfigV2, so the
// resulting chain is behaviorally identical to BuildGlobalChain.
//
// This is an additive, backward-compatible entry point (see doc/refactor01.md
// Phase 1): BuildGlobalChain keeps working unchanged, and callers can opt into
// the registry-based path by calling this function instead. Callers that need
// the registry itself (not just the chain) should call
// NewGlobalMiddlewareRegistry and BuildGlobalChainFromConfigV2 directly.
func BuildGlobalChainV2(
	gatewayConfig *config.GatewayConfig,
	sessionStore session.SessionStore,
	tokenService *auth.TokenService,
	trafficMetricRepo db.TrafficMetricRepository,
	rateLimiter *RateLimiter,
) (*ChainBuilder, error) {
	registry, err := NewGlobalMiddlewareRegistry(sessionStore, tokenService, trafficMetricRepo, rateLimiter)
	if err != nil {
		return nil, err
	}
	return BuildGlobalChainFromConfigV2(registry, gatewayConfig)
}
