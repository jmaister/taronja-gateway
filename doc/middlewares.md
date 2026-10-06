# Middleware architecture

How requests move through the gateway's global middleware and route middleware. For writing a middleware see [middleware_development.md](./middleware_development.md); for each middleware's options see [middleware/](./middleware/README.md).

## Packages

```
middleware/            Plumbing only
├── chain.go           ChainBuilder / Chain: compose func(http.Handler) http.Handler values
├── factory.go         MiddlewareFactory interface, ConcreteFactory, NoConfig, DecodeConfigInto, ConfigAs
├── registry.go        Registry: BuildChain (global, instrumented) and NewChain (per route, no side effects)
├── health.go          Optional HealthChecker interface for factories
└── metrics.go         Per-middleware request/error/time counters
middleware/builtin/    Implementations
├── global.go          NewGlobalRegistry, ResolveGlobalMiddleware, NewGlobalChain, ValidateGlobalMiddleware, MiddlewareSettings
├── route_chain.go     route_auth and route_cache factories, RouteChainBuilder
├── auth.go, cache.go  Route-level authentication and cache-control
├── session.go         Session middlewares, ValidateSessionFromRequest, CheckAdminAccess
├── tracing.go, compression.go, cors.go, ratelimiter.go, ja4.go, trafficmetric.go, logging.go
│                      One middleware each: name constant, options type with Validate, factory
├── validation.go      Startup checks for routes, admin settings and middleware
middleware/fingerprint/ JA4H helpers
```

## Middleware chain

The root `middleware:` list in the config is the whole chain: entries run in list order, disabled entries are skipped, and nothing runs when the list is absent. `config.MiddlewareConfig` keeps each entry's `config:` block as a raw YAML node; the config package knows no middleware names or options.

`builtin.NewGlobalChain` builds the chain once at startup:

1. `NewGlobalRegistry` registers every built-in factory with its runtime services (session store, token service, traffic metric repository, shared rate limiter).
2. `ResolveGlobalMiddleware` looks each entry up by name and calls the factory's `DecodeConfig`, which decodes the options over their defaults, rejects unknown keys and runs the options type's `Validate`.
3. `Registry.BuildChain` checks that every dependency appears earlier in the list (for example `traffic_metrics` needs `session_extraction` and `ja4_fingerprint`), creates each middleware, wraps it with metrics and records which ones are active.

`ValidateGlobalMiddleware` runs steps 1 to 2 and the dependency check without creating anything; `tg validate`, `tg middleware list` and gateway startup use it.

Code outside the chain that depends on a middleware's options reads them with `MiddlewareSettings[C](cfg, name)` (or `MiddlewareEnabled`), so it always agrees with the chain: the shared rate limiter, `gateway.InitTracing`, the proxy's trace propagation and the statistics API's `excludeStaticAssets`.

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
