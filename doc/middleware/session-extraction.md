# session_extraction

Attaches the caller's session (cookie or bearer token), when there is one, to
the request. It never blocks a request; route authentication is set per route
with `authentication.enabled`.

```yaml
middleware:
  - name: ja4_fingerprint
  - name: session_extraction
```

**Options:** none. **Depends on:** `ja4_fingerprint` (listed before it).
Used by [`traffic_metrics`](traffic-metrics.md) to record who made each
request.
