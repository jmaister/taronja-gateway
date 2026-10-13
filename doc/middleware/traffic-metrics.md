# traffic_metrics

Records every request (method, path, status, size, duration, user, IP,
geolocation, device, fingerprint) for the dashboard's statistics pages.

```yaml
middleware:
  - name: ja4_fingerprint
  - name: session_extraction
  - name: traffic_metrics
    config:
      excludeStaticAssets: true
```

| Option | Default | Description |
|---|---|---|
| `excludeStaticAssets` | `false` | Don't record CSS, JS, images, fonts and other static assets. Choose it before collecting data: the dashboard then hides the request-type filter. |

**Depends on:** `ja4_fingerprint` and `session_extraction`, listed before it.

- Writes are asynchronous and batched (every 100 rows or 500 ms); a hard
  crash can lose the last 500 ms.
- Geolocation uses iplocate.io when `geolocation.iplocateApiKey` is set,
  otherwise freeipapi.com. Results are cached for 7 days, failures for 1
  minute; local addresses are never looked up.

## Admin API

| Endpoint | Returns |
|---|---|
| `GET <prefix>/api/statistics/requests` | Counts by status, country, device, platform, browser, user, fingerprint. |
| `GET <prefix>/api/statistics/requests/details` | Individual requests. |
| `GET <prefix>/api/statistics/timeseries` | Requests, visitors, errors and response time per time bucket. |

All three take the same filters: `start_date`, `end_date`, `is_static`,
`status_code`, `country`, `device_type`, `platform`, `browser`, `username`,
`fingerprint`, `fingerprint_type`.

A visitor (fingerprint) counts as **new** in the bucket holding its first
request ever, and **returning** afterwards. First requests are kept in the
`fingerprint_first_seen` table, which a database trigger updates on every
insert.
