# Release Notes: Middleware Architecture Refactor

**Companion to:** [`doc/refactor01.md`](./refactor01.md) (the full design doc and phase-by-phase plan)
**Scope:** All five phases — Foundation, Config Integration, Monitoring & Observability, Documentation & Tooling, and the Follow-ups from Self-Review phase added afterward.

This refactor replaces the gateway's hardcoded, conditional middleware setup
(`if config.Management.Analytics { chain.Add(...) }` inside
`BuildGlobalChain`) with a declarative Factory + Registry system, while
keeping every existing config file and every existing behavior unchanged
unless you opt into the new pieces. Nothing described here requires any
change to an existing `config.yaml`.

## What's new

**A factory + registry for the global middleware chain.** Every built-in
global middleware (`rate_limiter`, `ja4_fingerprint`, `session_extraction`,
`traffic_metrics`, `logging`) now has a `MiddlewareFactory`
(`middleware/factory.go`) registered into a `MiddlewareRegistryV2`
(`middleware/registry_v2.go`), which builds the chain from an ordered,
inspectable list of specs and validates declared dependencies between them at
build time — e.g. `traffic_metrics` (which reads the JA4 fingerprint header
via `session.NewClientInfo` to enrich the metrics it records) now fails fast
at startup if `ja4_fingerprint` isn't also enabled, instead of silently
recording metrics without it.

**An optional declarative `middleware:` config section.** Alongside the
existing `management.analytics` / `management.logging` /
`management.rateLimiter` flags — which still work exactly as before — a
config file can now declare the global chain explicitly, in order, with
per-middleware `enabled` flags:

```yaml
middleware:
  global:
    - name: rate_limiter
      rateLimiter:
        requestsPerMinute: 1000
        maxErrors: 10
        blockMinutes: 5
    - name: ja4_fingerprint
    - name: session_extraction
    - name: traffic_metrics
    - name: logging
      enabled: false
```

When this section is present it fully replaces the legacy flags; when it's
absent (the common case today), behavior is identical to before. `rate_limiter`
is currently the only middleware with real per-entry configuration, since
it's the only one with tunable options that already existed.

**Runtime status, health, and metrics.** Two new admin-only API endpoints:

- `GET <prefix>/api/middleware` — every global middleware's status
  (active/available), enabled flag, dependencies, and health where a check is
  implemented (only `rate_limiter` reports real health today — tracked/blocked
  IP counts; the others honestly report `"unknown"` rather than a fabricated
  `"healthy"`).
- `GET <prefix>/api/middleware/{name}/metrics` — request count, error count,
  and average duration for one middleware, tracked in-memory since the chain
  was built.
- `GET <prefix>/api/middleware/metrics` (Phase 5) — every middleware's
  metrics in one call, plus a `/middleware` page in the admin dashboard that
  renders all of the above (status, health, dependencies, live metrics).

**A CLI command for offline introspection.**

```bash
tg middleware list --config ./sample/config.yaml
```

prints the same status information without starting the server, opening a
database connection, or making any network call — safe to run against a
production config to sanity-check a change before deploying it.

**A documented, tested extension path.** [`doc/middleware_development.md`](./middleware_development.md)
walks through implementing a new middleware, and
[`examples/middleware-plugin/`](../examples/middleware-plugin/) is a complete,
compiled, tested example (`request_id` — an `X-Request-Id` tracing header
middleware) showing a middleware built and registered using only the
gateway's public `middleware` package, with no changes to the gateway's own
source.

## Compatibility

- **No breaking changes.** Every existing config file behaves identically.
  `BuildGlobalChain` (the original function) still exists and now delegates
  to the new registry internally, but produces the same chain for the same
  config either way.
- **New, optional Go API surface**: `middleware.MiddlewareFactory`,
  `middleware.MiddlewareRegistryV2`, `middleware.HealthChecker`,
  `middleware.NewGlobalMiddlewareRegistry`, `middleware.ResolveGlobalChainSpecs`,
  `middleware.BuildGlobalChainFromConfigV2`, `middleware.ValidateGlobalChainSpecs`.
  None of it needs to be used to keep existing behavior.
- **One internal signature change**: `handlers.NewStrictApiServer` gained a
  trailing `*middleware.MiddlewareRegistryV2` parameter (pass `nil` if you
  don't need the new endpoints to return real data). This only affects direct
  callers of that constructor — the packaged gateway (`gateway.NewGatewayWithDependencies`)
  wires it automatically.
- **Small, expected performance change**: every middleware built via the
  registry is now wrapped with request-metrics instrumentation (a timer and a
  few atomic increments per middleware per request) so the new metrics
  endpoint has real data instead of none. This is the only per-request
  behavior change in the whole refactor; it's negligible relative to the
  actual middleware work already being timed.

## Where to look

| I want to... | Look at |
|---|---|
| Understand the design and rationale | `doc/refactor01.md` |
| Add a new middleware | `doc/middleware_development.md` + `examples/middleware-plugin/` |
| See what's active for my config | `tg middleware list --config <path>` or `GET <prefix>/api/middleware` |
| Declare middleware in YAML | `middleware:` section, see example above and `config/middleware.go` |
| Monitor a middleware in production | `GET <prefix>/api/middleware/{name}/metrics` |
