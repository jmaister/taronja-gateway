# Middleware (optional, advanced)

By default, the global middleware chain (compression, CORS, rate limiting,
JA4 fingerprinting, session extraction, traffic metrics, request logging —
see [doc/middleware/](README.md) for what each one does) is
controlled by the `compression` / `cors` / `logging` / `analytics` /
`rateLimiter` flags above. For explicit control over which middleware runs
and in what order, add a `middleware:` section — when present it fully
replaces those flags:

```yaml
middleware:
  global:
    - name: compression
    - name: cors
      cors:
        allowedOrigins: ["https://app.example.com"]
    - name: rate_limiter
      rateLimiter:
        requestsPerMinute: 1000
        maxErrors: 10
        blockMinutes: 5
    - name: ja4_fingerprint
    - name: session_extraction
    - name: traffic_metrics
      trafficMetrics:
        excludeStaticAssets: true
    - name: logging
      enabled: false   # listed but disabled
```

See [README.md#two-ways-to-enable-any-of-these](README.md#two-ways-to-enable-any-of-these)
for a full comparison of the two forms (including the "fully replaces those
flags" gotcha above spelled out in more detail — it's easy to trip on when
adding this section just to reorder or reconfigure one middleware).

See [Middleware Architecture](../../README.md#middleware-architecture) below for how to
inspect this at runtime, and `doc/middleware_development.md` for adding your
own middleware.
