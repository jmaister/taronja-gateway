# Middleware architecture

How requests move through the gateway's middleware. For writing a new middleware see [middleware_development.md](./middleware_development.md); for per-middleware options see [middleware/](./middleware/README.md).

## Packages

```
middleware/            Plumbing only
├── chain.go           ChainBuilder / Chain: compose func(http.Handler) http.Handler values
├── factory.go         MiddlewareFactory interface and ConcreteFactory helper
├── registry.go        Registry: BuildChain (global, instrumented) and NewChain (per route, no side effects)
├── health.go          Optional HealthChecker interface for factories
└── metrics.go         Per-middleware request/error/time counters
middleware/builtin/    Implementations
├── factories.go       One factory per global middleware
├── global.go          ResolveGlobalChainSpecs, NewGlobalChain, Effective*Config helpers
├── route_chain.go     route_auth and route_cache factories, RouteChainBuilder
├── auth.go, cache.go  Route-level authentication and cache-control
├── session.go         Session middlewares, ValidateSessionFromRequest, CheckAdminAccess
├── ratelimiter.go, compression.go, cors.go, tracing.go, logging.go, trafficmetric.go, validation.go
middleware/fingerprint/ JA4H helpers
```

## Global chain

Built once at startup by `builtin.NewGlobalChain`, from `middleware.global` when that section exists, otherwise from the `management.*` flags (`ResolveGlobalChainSpecs`). With the flags, the order is:

1. `tracing` (when `tracing.enabled`)
2. `compression`
3. `cors`
4. `rate_limiter`
5. `ja4_fingerprint`, `session_extraction`, `traffic_metrics` (when `management.analytics`)
6. `logging`

`Registry.BuildChain` checks each factory's declared dependencies (for example `session_extraction` needs `ja4_fingerprint` earlier in the chain), wraps every middleware with metrics, and records which ones are active. `ValidateAllMiddleware` runs the same resolution at startup so a bad chain fails before the server starts.

When a setting can come from either `management.*` or a per-entry override, ask the resolved chain, not the raw config: `EffectiveRateLimiterConfig`, `EffectiveCORSConfig` and `EffectiveExcludeStaticAssets` return what the chain will really use.

## Route chain

Each configured route is wrapped by `RouteChainBuilder.BuildRouteChain`, which resolves two factories from its own registry through `Registry.NewChain`:

1. `route_auth`, only when the route has authentication enabled. Browser routes (static, SPA) redirect to the login page; proxy routes answer 401.
2. `route_cache`, always. Sets `Cache-Control` from the route config.

`NewChain` does not touch the global registry's status or metrics, which is why the route chain has a registry of its own.

## Sessions

`session.go` holds all session handling:

- `ValidateSessionFromRequest` reads the cookie or bearer token and returns the session.
- `SessionExtractionMiddleware` attaches a session to the request context without enforcing anything (used by analytics).
- `SessionMiddleware` and `StrictSessionMiddleware` enforce a session on the dashboard and on the OpenAPI routes.
- `CheckAdminAccess` is the single admin check. The OpenAPI handlers in `handlers/session_guard.go` call it too.

## Request flow

```
Request
  -> global chain (order above)
  -> route match
  -> route_auth (if enabled)
  -> route_cache
  -> handler / proxy
```
