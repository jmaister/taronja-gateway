# Global Middleware Reference

One page per built-in global middleware — what it does, how to enable and
configure it, and what it depends on. These are the eight middlewares
`middleware.NewGlobalMiddlewareRegistry` registers; see [Middleware
Architecture](../../README.md#middleware-architecture) in the main README
for the factory/registry system they're built on, how to inspect a running
chain (`tg middleware list`, `GET <prefix>/api/middleware`), and how to add
your own.

Listed in the order they run by default:

| Middleware | Enabled by default | Depends on | Summary |
|---|---|---|---|
| [`tracing`](tracing.md) | No | — | Creates an OpenTelemetry span per request and propagates trace context to proxied backends. |
| [`compression`](compression.md) | No | — | Compresses response bodies with brotli, zstd, gzip, or deflate when the client accepts it. |
| [`cors`](cors.md) | No | — | Adds CORS response headers for cross-origin requests to the management API. |
| [`rate_limiter`](rate-limiter.md) | No | — | Limits request rate per client IP and detects vulnerability scans. |
| [`ja4_fingerprint`](ja4-fingerprint.md) | No¹ | — | Computes and caches a JA4H fingerprint for each request. |
| [`session_extraction`](session-extraction.md) | No¹ | `ja4_fingerprint` (ordering only) | Resolves the authenticated session/user and attaches it to the request context, without enforcing authentication. |
| [`traffic_metrics`](traffic-metrics.md) | No¹ | `session_extraction`, `ja4_fingerprint` (both verified reads) | Records per-request traffic metrics (status, size, duration, user, geolocation). |
| [`logging`](logging.md) | No | — | Logs each request/response (method, path, status, duration). |

¹ These three are grouped under the single `management.analytics: true`
flag in the common case (no explicit `middleware:` section) — see each
page's "Enabling it" section for both the legacy-flag and explicit-config
forms.

Every middleware here has no per-entry config beyond enable/disable
**except** `rate_limiter` (requests-per-minute, error, and
vulnerability-scan limits), `cors` (allowed origins/methods/headers), and
`traffic_metrics` (`excludeStaticAssets`) — see their individual pages for
the full option tables. `compression` in particular is deliberately
option-free by design, not just undocumented — see its page for why.
`tracing` sits in between: it does have configuration (`endpoint`,
`insecure`), but it's top-level (`tracing:`, not
`middleware.global[].tracing`) and applies once, globally, at startup —
see its page for why a per-entry override wouldn't make sense for it.

Route-level middleware (authentication, cache-control headers) isn't part
of this global chain and isn't covered here — see the main README's
[Routes](../../README.md#routes) section and
[`doc/CACHE_CONTROL.md`](../CACHE_CONTROL.md).

## Two ways to enable any of these

Every page above shows two YAML snippets under "Enabling it" — a legacy
`management.*` flag and an explicit `middleware:` section entry — without
saying which to reach for. Here's the comparison:

| | Legacy `management.*` flags | Explicit `middleware:` section |
|---|---|---|
| Where it lives | Scattered: `management.logging`, `.compression`, `.analytics`, `.rateLimiter`, `.cors`, `.excludeStaticAssets` | One ordered list: `middleware.global[]` |
| Execution order | Fixed — the order in the table above, not adjustable | Whatever order you list entries in |
| Enable/disable granularity | Per flag, except the `analytics` group, which turns [`ja4_fingerprint`](ja4-fingerprint.md), [`session_extraction`](session-extraction.md), and [`traffic_metrics`](traffic-metrics.md) on/off **together** — no way to have just one or two of them | Fully individual — list exactly the middlewares you want, in any combination, including just one of the three "analytics" middlewares alone |
| Per-entry config overrides | Not possible — one `management.rateLimiter`/`.cors`/`.excludeStaticAssets` value applies everywhere | Possible for `rate_limiter`, `cors`, and `traffic_metrics`: a config block on that entry overrides the shared `management.*` value for just that one middleware |
| Explicitly running zero global middleware | Not expressible — the absence of every flag just means "nothing enabled," indistinguishable from "not thought about yet" | `middleware: {global: []}` — an explicit, self-documenting empty chain |
| Best for | An existing config file, or a setup that's happy with the default order and the common on/off flags | Needing a non-default order, a per-middleware config override, or selective control over the `analytics` group |

**The one thing to know before adding a `middleware:` section: it's
all-or-nothing.** The moment a config file has a `middleware:` section with
a `global:` key — even one entry — it **fully replaces every legacy flag**,
not just the middleware(s) you listed. A `middleware: {global: [{name:
rate_limiter, ...}]}` added purely to reorder or reconfigure the rate
limiter silently turns off logging, CORS, compression, and analytics too,
if their `management.*` flags were still set in the same file — those flags
are no longer read at all once the section exists. Bring every middleware
you still want across explicitly when you switch.
