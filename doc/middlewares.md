# Middleware architecture

How requests move through the gateway's global middleware and route middleware. For writing a middleware see [middleware_development.md](./middleware_development.md); for each middleware's options see [middleware/](./middleware/README.md).

## Packages

```
middleware/            Plumbing only
├── chain.go           Middleware type and Chain: compose func(http.Handler) http.Handler values
├── health.go          MiddlewareHealth
└── metrics.go         Counter: per-middleware request/error/time counters
middleware/builtin/    Implementations
├── definition.go      Definition, the definitions list, GlobalDeps, decodeOptions/noOptions
├── global_chain.go    GlobalChain: ResolveGlobalChain, Build, GlobalOptions, Status, Metrics
├── route_chain.go     WrapRoute: route authentication and cache-control
├── auth.go, cache.go  Route-level authentication and cache-control
├── session.go         Session middlewares, ValidateSessionFromRequest, CheckAdminAccess
├── tracing.go, compression.go, cors.go, ratelimiter.go, ja4.go, trafficmetric.go, logging.go
│                      One middleware each: name constant, options type with Validate, Definition
├── validation.go      ValidateConfigOnly (`tg validate`), startup status log
middleware/fingerprint/ JA4H helpers
```

## Middleware chain

The root `middleware:` list in the config is the whole chain: entries run in list order, disabled entries are skipped, and nothing runs when the list is absent. `config.MiddlewareConfig` keeps each entry's `config:` block as a raw YAML node; the config package knows no middleware names or options.

Each built-in middleware is a `Definition` value: name, description, dependencies, a `Decode` func for its options and a `New` func that builds it from those options and the runtime services (`GlobalDeps`).

1. `ResolveGlobalChain(cfg)` looks each enabled entry up by name, checks that its dependencies appear earlier in the list (for example `traffic_metrics` needs `session_extraction` and `ja4_fingerprint`), and decodes its options: over their defaults, rejecting unknown keys, running the options type's `Validate`. It needs no runtime services, so `tg validate` and `tg middleware list` use it too.
2. `GlobalChain.Build(deps)` runs once at startup: it creates each middleware and wraps it with metrics.

The resolved `*GlobalChain` never changes afterwards; the admin API reads its status and metrics without locking. Code outside the chain that depends on a middleware's options reads them with `GlobalOptions[C](chain, name)` (or `chain.Enabled(name)`), so it always agrees with the chain: the shared rate limiter, `gateway.InitTracing`, the proxy's trace propagation and the statistics API's `excludeStaticAssets`.

## Route chain

`WrapRoute` wraps each configured route's handler:

1. Authentication, only when the route sets `authenticated: true`. Browser routes (static, SPA) redirect to the login page; proxy routes answer 401.
2. Cache control, always. Sets `Cache-Control` from the route options.

Route middleware is configured on the route, not in `middleware:`, and is not instrumented.

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
  -> route authentication (if enabled)
  -> route cache control
  -> handler / proxy
```
