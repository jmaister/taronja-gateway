# logging

Logs one line per request after it completes.

```yaml
middleware:
  - name: logging
```

```
2025-01-15T10:30:00.123Z - 192.0.2.1:54321 "GET /api/users" 200 12.34ms
```

**Options:** none. **Depends on:** nothing. Place it last so it logs the
final status. For queryable request history use
[`traffic_metrics`](traffic-metrics.md).
