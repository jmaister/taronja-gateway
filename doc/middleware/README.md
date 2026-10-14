# Middleware

Global middleware runs on every request, top to bottom, before the route.
List it under the root `middleware:` key:

```yaml
middleware:
  - name: compression
  - name: cors
    config:
      allowedOrigins: ["https://app.example.com"]
  - name: rate_limiter
    config:
      requestsPerMinute: 300
      blockMinutes: 15
  - name: ja4_fingerprint
  - name: session_extraction
  - name: traffic_metrics
  - name: logging
    enabled: false   # kept, not running
```

| Key | Description |
|---|---|
| `name` | Middleware name, once per list. |
| `enabled` | `false` to skip an entry. Default `true`. |
| `config` | Options; unknown keys are rejected. |

No `middleware:` key means none runs. Check a config without starting the
gateway with `tg validate --config <file>` or `tg middleware list --config <file>`.

| Middleware | Depends on | Does |
|---|---|---|
| [`tracing`](tracing.md) | — | OpenTelemetry span per request. |
| [`compression`](compression.md) | — | brotli/zstd/gzip/deflate responses. |
| [`cors`](cors.md) | — | CORS headers and preflights. |
| [`rate_limiter`](rate-limiter.md) | — | Per-IP limits, scanner blocking. |
| [`ja4_fingerprint`](ja4-fingerprint.md) | — | Request fingerprint header. |
| [`session_extraction`](session-extraction.md) | `ja4_fingerprint` | Attaches the caller's session. |
| [`traffic_metrics`](traffic-metrics.md) | `session_extraction`, `ja4_fingerprint` | Records requests for the dashboard. |
| [`logging`](logging.md) | — | One log line per request. |

Dependencies must come earlier in the list. The order above is the suggested
one: `tracing` first, `cors` before `rate_limiter`, `logging` last.

Route authentication and cache headers are set per route, see
[Routes](../../README.md#routes).
