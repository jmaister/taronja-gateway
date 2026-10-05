# Filters

Global filters (compression, CORS, rate limiting, analytics, logging,
tracing) are listed under the root `filters:` key, in the order they run.
Each entry has a `name`, an optional `enabled: false`, and an optional
`config` block with that filter's options:

```yaml
filters:
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

See [doc/middleware/](README.md) for every filter, its options and its
dependencies.
