package builtin

import (
	"fmt"
	"net/http"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/middleware"
	yaml "gopkg.in/yaml.v3"
)

const (
	routeMiddlewareAuth  = "route_auth"
	routeMiddlewareCache = "route_cache"
)

// RouteAuthConfig configures the per-route authentication middleware.
// Redirect sends unauthenticated browsers to the login page instead of
// answering 401.
type RouteAuthConfig struct {
	Redirect bool
}

// RouteCacheConfig configures the per-route Cache-Control middleware.
type RouteCacheConfig struct {
	Route config.RouteConfig
}

type routeAuthFactory struct {
	middleware.ConcreteFactory
	auth *AuthMiddleware
}

func (f *routeAuthFactory) Create(cfg interface{}) (middleware.Middleware, error) {
	c, ok := cfg.(RouteAuthConfig)
	if !ok {
		return nil, fmt.Errorf("%s: invalid config type %T, expected RouteAuthConfig", routeMiddlewareAuth, cfg)
	}
	return f.auth.AuthMiddlewareFunc(c.Redirect), nil
}

// DecodeConfig is not used: route filters get their config from the route.
func (f *routeAuthFactory) DecodeConfig(*yaml.Node) (interface{}, error) {
	return RouteAuthConfig{}, nil
}

type routeCacheFactory struct {
	middleware.ConcreteFactory
	cache *HttpCacheMiddleware
}

func (f *routeCacheFactory) Create(cfg interface{}) (middleware.Middleware, error) {
	c, ok := cfg.(RouteCacheConfig)
	if !ok {
		return nil, fmt.Errorf("%s: invalid config type %T, expected RouteCacheConfig", routeMiddlewareCache, cfg)
	}
	return f.cache.CacheControlMiddlewareFunc(c.Route), nil
}

// DecodeConfig is not used: route filters get their config from the route.
func (f *routeCacheFactory) DecodeConfig(*yaml.Node) (interface{}, error) {
	return RouteCacheConfig{}, nil
}

// RouteChainBuilder builds the middleware chain of individual routes from the
// same factory/spec mechanism the global chain uses, in a registry of its own
// so route chains never touch the global middleware status and metrics.
type RouteChainBuilder struct {
	registry *middleware.Registry
}

// NewRouteChainBuilder registers the per-route auth and cache factories.
func NewRouteChainBuilder(authMiddleware *AuthMiddleware, cacheMiddleware *HttpCacheMiddleware) *RouteChainBuilder {
	registry := middleware.NewRegistry()
	factories := []middleware.MiddlewareFactory{
		&routeAuthFactory{
			ConcreteFactory: middleware.NewConcreteFactory(routeMiddlewareAuth, "Requires a valid session on a route"),
			auth:            authMiddleware,
		},
		&routeCacheFactory{
			ConcreteFactory: middleware.NewConcreteFactory(routeMiddlewareCache, "Sets Cache-Control on a route"),
			cache:           cacheMiddleware,
		},
	}
	for _, f := range factories {
		if err := registry.RegisterFactory(f); err != nil {
			panic(err) // names above are constants; a duplicate is a programming error
		}
	}
	return &RouteChainBuilder{registry: registry}
}

// routeSpecs lists a route's middleware in execution order: auth (when
// enabled) then cache control, which always applies.
func routeSpecs(routeConfig config.RouteConfig) []middleware.MiddlewareSpec {
	var specs []middleware.MiddlewareSpec
	if routeConfig.Authentication.Enabled {
		// Browser-facing routes (static, SPA) redirect to login; plain
		// proxy/API routes answer 401.
		specs = append(specs, middleware.MiddlewareSpec{
			Name:   routeMiddlewareAuth,
			Config: RouteAuthConfig{Redirect: routeConfig.Static || routeConfig.IsSPA},
		})
	}
	return append(specs, middleware.MiddlewareSpec{
		Name:   routeMiddlewareCache,
		Config: RouteCacheConfig{Route: routeConfig},
	})
}

// BuildRouteChain wraps handler in the middleware chain for routeConfig.
func (r *RouteChainBuilder) BuildRouteChain(handler http.HandlerFunc, routeConfig config.RouteConfig) http.HandlerFunc {
	chain, err := r.registry.NewChain(routeSpecs(routeConfig))
	if err != nil {
		// Both factories are registered above and take the config types
		// routeSpecs produces, so this cannot happen.
		panic(fmt.Sprintf("route chain: %v", err))
	}
	return chain.Build(handler).(http.HandlerFunc)
}
