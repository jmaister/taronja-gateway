package gateway

import (
	"fmt"
	"net/http"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/gateway/deps"
	"github.com/jmaister/taronja-gateway/middleware"
	"github.com/jmaister/taronja-gateway/session"
)

// gatewayRuntime bundles everything setup builds from cfg: the mux routes
// are registered on, the assembled global middleware chain handler, and the
// middleware instances the rest of the gateway (route registration, the
// admin API) needs a reference to. Built as a unit by buildRuntime so setup
// has one single, self-contained value to wire onto the Gateway, rather
// than several separately-fallible pieces.
type gatewayRuntime struct {
	mux               *http.ServeMux
	handler           http.Handler
	rateLimiter       *middleware.RateLimiter
	registry          *middleware.MiddlewareRegistryV2
	authMiddleware    *middleware.AuthMiddleware
	cacheMiddleware   *middleware.HttpCacheMiddleware
	routeChainBuilder *middleware.RouteChainBuilder
}

// buildRuntime assembles a gatewayRuntime for cfg: the rate limiter, the
// global middleware registry/chain built through it (see
// doc/refactor01.md Phases 1-3), and the auth/cache/route-chain middleware
// used when registering individual routes.
func buildRuntime(cfg *config.GatewayConfig, d *deps.Dependencies) (*gatewayRuntime, error) {
	mux := http.NewServeMux()

	// Built from the *effective* config (a per-entry middleware.global
	// rate_limiter override if present, otherwise management.rateLimiter) —
	// see the equivalent comment this replaces in the old createHTTPServer.
	rl := middleware.NewRateLimiter(middleware.EffectiveRateLimiterConfig(cfg), d.BlockedClientRepo)

	registry, err := middleware.NewGlobalMiddlewareRegistry(d.SessionStore, d.TokenService, d.TrafficMetricRepo, rl)
	if err != nil {
		return nil, fmt.Errorf("failed to build middleware registry: %w", err)
	}
	globalChain, err := middleware.BuildGlobalChainFromConfigV2(registry, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to build middleware chain: %w", err)
	}
	handler := globalChain.Build(mux)

	authMiddleware := middleware.NewAuthMiddleware(d.SessionStore, d.TokenService, cfg.Management.Prefix)
	cacheMiddleware := middleware.NewHttpCacheMiddleware()
	routeChainBuilder := middleware.NewRouteChainBuilder(authMiddleware, cacheMiddleware)

	return &gatewayRuntime{
		mux:               mux,
		handler:           handler,
		rateLimiter:       rl,
		registry:          registry,
		authMiddleware:    authMiddleware,
		cacheMiddleware:   cacheMiddleware,
		routeChainBuilder: routeChainBuilder,
	}, nil
}

// setup validates cfg, builds the gateway's runtime (middleware chain, mux,
// rate limiter), registers every route, and ensures the admin user exists —
// the one-time sequence NewGatewayWithDependencies runs to bring a gateway
// up.
func (g *Gateway) setup(cfg *config.GatewayConfig) error {
	if err := middleware.ValidateAllMiddleware(g.Dependencies, cfg); err != nil {
		return fmt.Errorf("middleware validation failed: %w", err)
	}
	middleware.LogMiddlewareStatus(cfg)

	rt, err := buildRuntime(cfg, g.Dependencies)
	if err != nil {
		return err
	}

	if err := ensureAdminUser(cfg, g.Dependencies.UserRepo); err != nil {
		return fmt.Errorf("failed to ensure admin user: %w", err)
	}

	g.GatewayConfig = cfg
	g.Mux = rt.mux
	g.RateLimiter = rt.rateLimiter
	g.MiddlewareRegistry = rt.registry
	g.AuthMiddleware = rt.authMiddleware
	g.HttpCacheMiddleware = rt.cacheMiddleware
	g.RouteChainBuilder = rt.routeChainBuilder

	// Registers every route/management handler onto the mux just built
	// above.
	if err := configureRoutes(g); err != nil {
		return fmt.Errorf("failed to configure routes: %w", err)
	}

	session.SetGeolocationConfig(&cfg.Geolocation)

	// TLS JA4 capture (see gateway/ja4tls.go) wraps outside rt.handler
	// entirely, rather than going through the MiddlewareRegistryV2 like the
	// seven global middlewares: it's a TLS-connection-level concern, not an
	// HTTP one. g.tlsJA4 is nil when TLS is disabled.
	g.handler = rt.handler
	if g.tlsJA4 != nil {
		g.handler = g.tlsJA4.middleware(g.handler)
	}

	return nil
}
