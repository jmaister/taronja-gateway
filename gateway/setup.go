package gateway

import (
	"fmt"
	"net/http"

	"github.com/jmaister/taronja-gateway/accounts"
	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/gateway/deps"
	"github.com/jmaister/taronja-gateway/middleware/builtin"
	"github.com/jmaister/taronja-gateway/session"
)

// gatewayRuntime bundles everything setup builds from cfg: the mux routes
// are registered on, the assembled global middleware chain handler, and the
// pieces the rest of the gateway (route registration, the admin API) needs a
// reference to.
type gatewayRuntime struct {
	mux             *http.ServeMux
	handler         http.Handler
	rateLimiter     *builtin.RateLimiter
	middlewareChain *builtin.GlobalChain
	authMiddleware  *builtin.AuthMiddleware
}

// buildRuntime assembles a gatewayRuntime for cfg: the global middleware
// chain, the rate limiter it shares with the management API, and the auth
// middleware used when registering individual routes.
func buildRuntime(cfg *config.GatewayConfig, d *deps.Dependencies) (*gatewayRuntime, error) {
	mux := http.NewServeMux()

	chain, err := builtin.ResolveGlobalChain(cfg)
	if err != nil {
		return nil, err
	}

	// The rate limiter outlives the chain: the management API reads its state.
	rlCfg, _ := builtin.GlobalOptions[builtin.RateLimiterConfig](chain, builtin.NameRateLimiter)
	rl := builtin.NewRateLimiter(rlCfg, d.BlockedClientRepo)

	globalMiddleware, err := chain.Build(builtin.GlobalDeps{
		SessionStore:      d.SessionStore,
		TokenService:      d.TokenService,
		TrafficMetricRepo: d.TrafficMetricRepo,
		RateLimiter:       rl,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build global middleware chain: %w", err)
	}

	return &gatewayRuntime{
		mux:             mux,
		handler:         globalMiddleware(mux),
		rateLimiter:     rl,
		middlewareChain: chain,
		authMiddleware:  builtin.NewAuthMiddleware(d.SessionStore, d.TokenService, config.ManagementPrefix),
	}, nil
}

// setup validates cfg, builds the gateway's runtime (middleware chain, mux,
// rate limiter), registers every route, and ensures the admin user exists —
// the one-time sequence NewGatewayWithDependencies runs to bring a gateway
// up.
func (g *Gateway) setup(cfg *config.GatewayConfig) error {
	// buildRuntime validates the middleware list.
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}
	builtin.LogMiddlewareStatus(cfg)

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
	g.MiddlewareChain = rt.middlewareChain
	g.AuthMiddleware = rt.authMiddleware
	g.Accounts = newAccountsService(g.Dependencies)

	// Registers every route/management handler onto the mux just built
	// above.
	if err := configureRoutes(g); err != nil {
		return fmt.Errorf("failed to configure routes: %w", err)
	}

	session.SetGeolocationConfig(&cfg.Geolocation)

	// Sliding sessions: renewed on use, cookie re-sent by RenewCookies.
	if store, ok := g.Dependencies.SessionStore.(*session.SessionStoreDB); ok {
		store.RenewFor = cfg.Management.Session.GetDuration()
	}
	g.handler = session.RenewCookies(rt.handler)

	return nil
}

// newAccountsService builds the accounts service.
func newAccountsService(d *deps.Dependencies) *accounts.Service {
	return accounts.NewService(accounts.Repositories{
		Users:      d.UserRepo,
		Sessions:   d.SessionRepo,
		Identities: d.UserIdentityRepo,
		Events:     d.AuthEventRepo,
	})
}
