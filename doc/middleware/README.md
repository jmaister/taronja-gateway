# Middleware

Global middleware runs on every request, top to bottom, before it reaches a
route. It is configured in one place, the root `middleware:` list of the
config file, and each middleware has its own page here with its options and
dependencies.

```yaml
middleware:
  - name: compression
  - name: rate_limiter
    config:
      requestsPerMinute: 100
      blockMinutes: 15
  - name: ja4_fingerprint
  - name: session_extraction
  - name: traffic_metrics
  - name: logging
    enabled: false   # kept in the list, not running
```

Every entry takes the same three keys:

| Key | Required | Meaning |
|---|---|---|
| `name` | yes | Which middleware (see the table below). Each name may appear once. |
| `enabled` | no | `false` keeps the entry without running it. Default `true`. |
| `config` | no | The middleware's own options. Unknown keys are rejected. |

With no `middleware:` key, no middleware runs. `tg middleware list --config <file>`
prints the chain a config resolves to; `tg validate` checks names, options
and dependencies without starting the gateway.

## Available middleware

In the order the sample config lists them:

| Middleware | Options | Depends on | Summary |
|---|---|---|---|
| [`tracing`](tracing.md) | `endpoint` (required), `insecure` | — | OpenTelemetry span per request, trace context forwarded to backends. |
| [`compression`](compression.md) | none | — | brotli, zstd, gzip or deflate, negotiated via Accept-Encoding. |
| [`cors`](cors.md) | origins, methods, headers, credentials, max age | — | CORS headers and preflight answers. |
| [`rate_limiter`](rate-limiter.md) | per-IP limits, block time, scanner detection | — | Limits request rate per client IP and blocks scanners. |
| [`ja4_fingerprint`](ja4-fingerprint.md) | none | — | Computes a JA4H fingerprint per request. |
| [`session_extraction`](session-extraction.md) | none | `ja4_fingerprint` | Attaches the caller's session to the request, without enforcing login. |
| [`traffic_metrics`](traffic-metrics.md) | `excludeStaticAssets` | `session_extraction`, `ja4_fingerprint` | Records each request for the statistics dashboard. |
| [`logging`](logging.md) | none | — | Logs each request (method, path, status, duration). |

A dependency must appear earlier in the list; the gateway refuses to start
otherwise. The dashboard's statistics pages need the three analytics middleware
(`ja4_fingerprint`, `session_extraction`, `traffic_metrics`).

### Ordering

Order is yours to choose, but a few placements matter:

- `tracing` first, so its span covers the whole request.
- `compression` near the top: it wraps the response writer, and middleware
  after it still see the uncompressed status and size.
- `cors` before `rate_limiter`, so preflights are answered without being
  counted or blocked.
- `logging` last, so it logs the final status.

Route-level behavior (authentication, cache-control headers) is configured on
each route, not here; see [Routes](../../README.md#routes) and
[`doc/CACHE_CONTROL.md`](../CACHE_CONTROL.md).

## Writing a middleware

A middleware is one file in `middleware/builtin/` holding its name, its options
type, its validation and a factory, registered in
`middleware/builtin/global.go`. See
[`doc/middleware_development.md`](../middleware_development.md).
