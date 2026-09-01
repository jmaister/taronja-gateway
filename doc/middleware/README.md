# Global Middleware Reference

One page per built-in global middleware — what it does, how to enable and
configure it, and what it depends on. These are the six middlewares
`middleware.NewGlobalMiddlewareRegistry` registers; see [Middleware
Architecture](../../README.md#middleware-architecture) in the main README
for the factory/registry system they're built on, how to inspect a running
chain (`tg middleware list`, `GET <prefix>/api/middleware`), and how to add
your own.

Listed in the order they run by default:

| Middleware | Enabled by default | Depends on | Summary |
|---|---|---|---|
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

Every middleware here has no config beyond enable/disable **except**
`rate_limiter` (requests-per-minute, error, and vulnerability-scan limits),
`cors` (allowed origins/methods/headers), and `traffic_metrics`
(`excludeStaticAssets`) — see their individual pages for the full option
tables.

Route-level middleware (authentication, cache-control headers) isn't part
of this global chain and isn't covered here — see the main README's
[Routes](../../README.md#routes) section and
[`doc/CACHE_CONTROL.md`](../CACHE_CONTROL.md).
