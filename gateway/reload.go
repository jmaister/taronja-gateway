package gateway

import (
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/gateway/deps"
	"github.com/jmaister/taronja-gateway/middleware"
	"github.com/jmaister/taronja-gateway/session"
)

// reloadableHandler is the http.Server's Handler: a small indirection that
// lets ReloadConfig swap in a freshly-built handler (new middleware chain +
// mux) without rebinding the listening socket. A plain mutex, not
// atomic.Value, because atomic.Value requires every Store() to use the exact
// same concrete type, which the middleware chain's returned http.Handler
// does not promise to be across rebuilds.
type reloadableHandler struct {
	mu sync.RWMutex
	h  http.Handler
}

func (r *reloadableHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mu.RLock()
	h := r.h
	r.mu.RUnlock()
	h.ServeHTTP(w, req)
}

func (r *reloadableHandler) Store(h http.Handler) {
	r.mu.Lock()
	r.h = h
	r.mu.Unlock()
}

// gatewayRuntime bundles everything that a config change requires rebuilding
// from scratch: the mux routes are registered on, the assembled global
// middleware chain handler, and the middleware instances the rest of the
// gateway (route registration, the admin API) needs a reference to. Built
// once per generation, by buildRuntime, and swapped into the Gateway as a
// unit so a config reload can never leave it with e.g. a new rate limiter
// but the old middleware registry.
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
// used when registering individual routes. It has no side effects on any
// existing Gateway — safe to call speculatively (e.g. to validate a config
// reload) and discard the result on error.
func buildRuntime(cfg *config.GatewayConfig, d *deps.Dependencies) (*gatewayRuntime, error) {
	mux := http.NewServeMux()

	// Built from the *effective* config (a per-entry middleware.global
	// rate_limiter override if present, otherwise management.rateLimiter) —
	// see the equivalent comment this replaces in the old createHTTPServer.
	rl := middleware.NewRateLimiter(middleware.EffectiveRateLimiterConfig(cfg))

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

// applyConfig validates cfg, builds a fresh gatewayRuntime from it, and
// swaps it in as the gateway's running configuration. Used both for the
// gateway's initial construction and for every later ReloadConfig call —
// the same validate-build-swap sequence, so a reload can never leave the
// gateway half-updated: either every step below succeeds and the new
// generation goes live, or an error is returned and the previous generation
// (or, on the very first call, nothing) keeps serving unchanged.
//
// Steps that can fail (validation, buildRuntime, ensureAdminUser) all run
// before anything on g is touched. Only the final swap — updating g's
// fields, wiring the new mux via configureRoutes, and finally publishing the
// new handler — happens after, and that sequence has no fallible step of
// its own today. g.reloadMu serializes concurrent calls (e.g. a file-watch
// event and a SIGHUP arriving together) so two reloads can never interleave
// their swaps.
//
// Two things a reload can never change, because they'd require rebinding
// the listening socket or restarting an already-open connection:
// server.host/port, and the database/session-store connection itself.
func (g *Gateway) applyConfig(cfg *config.GatewayConfig) error {
	g.reloadMu.Lock()
	defer g.reloadMu.Unlock()

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

	g.configMu.Lock()
	g.GatewayConfig = cfg
	g.Mux = rt.mux
	g.RateLimiter = rt.rateLimiter
	g.MiddlewareRegistry = rt.registry
	g.AuthMiddleware = rt.authMiddleware
	g.HttpCacheMiddleware = rt.cacheMiddleware
	g.RouteChainBuilder = rt.routeChainBuilder
	g.configMu.Unlock()

	// Registers every route/management handler onto the new (not yet live)
	// mux. Reads g's just-swapped fields; safe without configMu here since
	// reloadMu already excludes any other writer for the duration of this
	// call, and nothing else reads these particular fields outside a
	// reload — see currentConfig's doc comment for the one field
	// (GatewayConfig) that's also read live, per-request, and therefore
	// always goes through configMu even from inside this package.
	if err := configureRoutes(g); err != nil {
		return fmt.Errorf("failed to configure routes: %w", err)
	}

	session.SetGeolocationConfig(&cfg.Geolocation)

	// The one step that actually takes effect for traffic: everything above
	// prepared the new generation without it being reachable yet.
	g.handler.Store(rt.handler)

	return nil
}

// ReloadConfig re-reads configFilePath and, if it parses and validates
// successfully, atomically swaps it in as the gateway's running
// configuration: the middleware chain, routes, rate limiter, and CORS/auth
// settings all take effect for requests received after this call returns.
// Requests already in flight keep running against whatever generation was
// already serving them.
//
// Fails safe: any error (parse failure, unsupported schema version, a
// route/middleware validation failure) is returned and the gateway keeps
// running its previous configuration unchanged — a bad edit to the config
// file never interrupts a running gateway, the same way "tg run" simply
// refuses to start against an invalid file rather than starting broken.
func (g *Gateway) ReloadConfig(configFilePath string) error {
	cfg, err := config.LoadConfig(configFilePath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	if err := g.applyConfig(cfg); err != nil {
		return err
	}

	log.Printf("Configuration reloaded from %s: %q (%d routes)", configFilePath, cfg.Name, len(cfg.Routes))
	return nil
}

// currentConfig returns the gateway's currently-active config. Reads go
// through configMu because, unlike the other fields applyConfig swaps
// (Mux, RateLimiter, etc. — see applyConfig's doc comment for why those
// don't need it), GatewayConfig is also read live, per incoming request, by
// the login-page handler (registerLoginRoutes) — a real concurrent reader
// that a reload's writer must synchronize against. The returned pointer
// itself is never mutated after config.LoadConfig produces it, so callers
// may read its fields freely without holding any lock themselves.
func (g *Gateway) currentConfig() *config.GatewayConfig {
	g.configMu.RLock()
	defer g.configMu.RUnlock()
	return g.GatewayConfig
}

// CurrentConfig is currentConfig's exported counterpart, for callers outside
// this package (e.g. main.go, or a future health/status endpoint) that want
// to read the gateway's live config while ReloadConfig may be running
// concurrently on another goroutine. The plain GatewayConfig field is not
// safe for that: it's fine to read once, right after construction, before
// any reload could plausibly race with it (as main.go's own startup log
// lines do), but polling it — or reading it from a long-lived goroutine
// that outlives startup — needs this instead.
func (g *Gateway) CurrentConfig() *config.GatewayConfig {
	return g.currentConfig()
}
