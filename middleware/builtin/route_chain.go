package builtin

import (
	"net/http"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/middleware"
)

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
	chain := middleware.NewChainBuilder()

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
