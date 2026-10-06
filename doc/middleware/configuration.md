# Middleware

Global middleware (compression, CORS, rate limiting, analytics, logging,
tracing) are listed under the root `middleware:` key, in the order they run.
Each entry has a `name`, an optional `enabled: false`, and an optional
`config` block with that middleware's options:

```yaml
middleware:
  - name: compression
  - name: cors
    config:
      allowedOrigins: ["https://app.example.com"]
  - name: rate_limiter
    config:
      requestsPerMinute: 1000
      maxErrors: 10
      blockMinutes: 5
  - name: ja4_fingerprint
  - name: session_extraction
  - name: traffic_metrics
    config:
      excludeStaticAssets: true
  - name: logging
    enabled: false   # listed but disabled
```

See [doc/middleware/](README.md) for every middleware, its options and its
dependencies.
