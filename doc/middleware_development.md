# Middleware Development Guide

This guide is for anyone adding a new middleware to Taronja Gateway's global
chain — whether it's a built-in one that ships with the gateway, or a
third-party one that lives in its own module and is wired in at startup. It
documents the factory/registry architecture introduced in `doc/refactor01.md`
(phases 1–3) and the patterns it expects you to follow.

A complete, runnable, tested example accompanies this guide at
[`examples/middleware-plugin/`](../examples/middleware-plugin/) — read this
document alongside it.

---

## The moving parts

Four pieces make up the system, all in the `middleware` package:

- **`Middleware`** (`middleware/chain.go`) — the actual middleware function,
  `func(http.Handler) http.Handler`. Nothing new here; this is the same
  `net/http`-style middleware signature the gateway has always used.
- **`MiddlewareFactory`** (`middleware/factory.go`) — the interface that lets
  the registry discover, describe, and build a `Middleware`. This is what you
  implement.
- **`MiddlewareRegistryV2`** (`middleware/registry_v2.go`) — holds registered
  factories and builds a `ChainBuilder` from an ordered list of
  `MiddlewareSpec`, validating that every spec's declared dependencies are
  satisfied by an earlier spec in the same list.
- **`HealthChecker`** (`middleware/health.go`) — an *optional* interface a
  factory can additionally implement to report runtime health.

The gateway's own five built-in middlewares (`rate_limiter`,
`ja4_fingerprint`, `session_extraction`, `traffic_metrics`, `logging`) are
implemented exactly this way in `middleware/factory.go` — read that file
alongside this guide; it's the reference implementation, not a special case.

## 1. Implement `MiddlewareFactory`

```go
type MiddlewareFactory interface {
	Create(cfg interface{}) (Middleware, error)
	GetName() string
	GetDescription() string
	GetDependencies() []string
	GetDefaultConfig() interface{}
}
```

- **`GetName()`** returns a stable identifier (snake_case, matching the
  gateway's own convention: `rate_limiter`, `session_extraction`, ...). This
  is what appears in `MiddlewareSpec.Name`, in the `middleware.global[].name`
  config field (if wired into the gateway's own config — see §4), and in the
  `GET /api/middleware` status endpoint.
- **`GetDescription()`** is a one-line human-readable summary, shown in the
  status endpoint and the `tg middleware list` CLI output.
- **`GetDependencies()`** lists the names of other middleware that must
  already be earlier in the same chain. Returning `nil`/`[]string{}` means no
  ordering requirement. `MiddlewareRegistryV2.BuildChain` (and
  `ValidateSpecs`) reject a chain where a dependency isn't satisfied by an
  earlier spec — e.g. the gateway's own `session_extraction` depends on
  `ja4_fingerprint`, so building a chain with `session_extraction` but not
  `ja4_fingerprint` fails fast with a clear error instead of the middleware
  silently missing data it expected to already be on the request.
- **`GetDefaultConfig()`** returns the zero-value/default config for this
  middleware. `BuildChain` calls this when a `MiddlewareSpec` has a `nil`
  `Config`, so `Create` always receives a usable value.
- **`Create(cfg interface{})`** builds and returns the `Middleware`. Runs once
  at chain-build time (typically at gateway startup), not per request — it's
  fine to do real setup here (open a connection, warm a cache, ...), the same
  way `RateLimiterFactory.Create` in the gateway's own code either reuses an
  existing `*RateLimiter` or constructs one.

A factory usually needs constructor dependencies (a DB repo, a client, a
config struct) — accept them in a `NewXFactory(...)` constructor and store
them on the struct, the same way `NewSessionExtractionFactory(sessionStore,
tokenService)` does in `middleware/factory.go`.

## 2. (Optional) Implement `HealthChecker`

```go
type HealthChecker interface {
	HealthCheck() MiddlewareHealth
}

type MiddlewareHealth struct {
	Status  string // "healthy", "degraded", "unhealthy", or "unknown"
	Message string
}
```

Only implement this if your middleware has genuine runtime state worth
reporting — a connection pool, a circuit breaker, a rate limiter's
tracked/blocked IPs (see `RateLimiterFactory.HealthCheck` in
`middleware/factory.go`). If there's nothing meaningful to check, **don't**
implement it: `MiddlewareRegistryV2.GetHealth`/`GetStatus` report
`Status: "unknown"` for a factory that doesn't implement `HealthChecker`,
which is more honest than a hardcoded `"healthy"` that was never actually
verified. Three of the gateway's five built-ins (`ja4_fingerprint`,
`session_extraction`, `traffic_metrics`, `logging`) deliberately skip it for
this reason.

## 3. Register it and build a chain

```go
registry := middleware.NewMiddlewareRegistryV2()
if err := registry.RegisterFactory(NewMyFactory(...)); err != nil {
	// a factory with this name is already registered
}

chain, err := registry.BuildChain([]middleware.MiddlewareSpec{
	{Name: "my_middleware", Config: myConfig}, // Config nil is fine too — GetDefaultConfig() fills in
})
handler := chain.Build(mux) // wrap your actual http.Handler / mux
```

This is exactly what `examples/middleware-plugin/requestid_test.go` does to
prove the example works, and exactly what `middleware.NewGlobalMiddlewareRegistry`
+ `BuildGlobalChainFromConfigV2` do inside the gateway itself
(`middleware/chain.go`, `gateway/gateway.go`).

## 4. Wiring into the actual gateway

The gateway's own global chain is closed over its five built-in factories —
`NewGlobalMiddlewareRegistry` in `middleware/chain.go` is the single place
that registers them. There are two ways to add a middleware to a running
gateway:

- **Built into the gateway itself**: add a factory to `middleware/factory.go`
  and a registration call to `NewGlobalMiddlewareRegistry`, plus a name
  constant in `config/middleware.go` (`config.MiddlewareNameXxx`) so
  `config.LoadConfig` accepts it in a `middleware:` YAML section
  (`doc/refactor01.md` Phase 2) and it appears as an option there. This is
  the right choice for something that ships with and is maintained alongside
  the gateway.
- **A separate module, wired in by whoever embeds the gateway**: build your
  own `main.go` (or fork the gateway's) that constructs
  `middleware.NewGlobalMiddlewareRegistry(...)`, additionally calls
  `registry.RegisterFactory(yourpackage.NewFactory(...))`, and passes the
  extended registry to `middleware.BuildGlobalChainFromConfigV2` instead of
  using the one-call `middleware.BuildGlobalChainV2` helper. This is the
  `examples/middleware-plugin/` scenario: nothing in `middleware/` or
  `gateway/` needs to change.

Either way, once a middleware is in the registry, it participates fully in
the existing tooling: `GET <prefix>/api/middleware` reports its status and
health, `GET <prefix>/api/middleware/{name}/metrics` reports its request
count/error count/average duration (every middleware built via
`MiddlewareRegistryV2.BuildChain` is automatically wrapped with this
instrumentation — see `middleware/metrics.go`, no opt-in needed), and
`tg middleware list --config <path>` prints it from the command line without
starting the server.

## Conventions checklist

When adding a middleware, match what the built-ins already do:

- Name: lowercase `snake_case`, short and specific (`rate_limiter`, not
  `RateLimiterMiddleware` or `rl`).
- One factory per middleware, in its own well-documented type — don't share a
  `ConcreteFactory` instance across unrelated middlewares.
- `Create` should be cheap and side-effect-light beyond genuine setup; it may
  run more than once (e.g. once per `BuildChain` call) so avoid anything that
  isn't idempotent.
- If your middleware wraps the `http.ResponseWriter` to observe the status
  code or response size, follow the existing pattern in
  `middleware/trafficmetric.go` / `middleware/logging.go` /
  `middleware/metrics.go` (a thin wrapper implementing `WriteHeader`), rather
  than reimplementing response buffering.
- Write tests the same way `middleware/registry_v2_test.go` and
  `middleware/health_metrics_test.go` do: build a chain through the real
  `MiddlewareRegistryV2`/`ChainBuilder` and drive it with
  `net/http/httptest`, rather than only unit-testing the bare middleware
  function in isolation.
