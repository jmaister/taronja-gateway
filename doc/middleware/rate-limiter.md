# rate_limiter

Per-IP limits in memory (not shared between instances, cleared on restart).
A blocked IP gets `429` with `Retry-After` until the block expires.

```yaml
middleware:
  - name: rate_limiter
    config:
      requestsPerMinute: 300
      maxErrors: 20            # 401/404 responses
      blockMinutes: 15
      vulnerabilityScan:
        urls: ["/wp-admin/*", "/*.php", "/.env"]
        max404: 3
        blockMinutes: 60
```

| Option | Default | Description |
|---|---|---|
| `requestsPerMinute` | `0` (off) | Requests per IP in a rolling minute. |
| `maxErrors` | `0` (off) | `401`/`404` responses per IP within `blockMinutes`. |
| `blockMinutes` | — | Block time for the two checks above. Required when any check is on. |
| `blockedClientRetentionDays` | `90` | How long block events are kept in the database. |
| `vulnerabilityScan.urls` | `[]` | Paths scanners probe (glob patterns, see below). |
| `vulnerabilityScan.max404` | `0` (off) | `404`s on those paths before blocking. |
| `vulnerabilityScan.blockMinutes` | — | Block time for scanners. Required when `max404` is set. |

**Depends on:** nothing.

Scan patterns use [doublestar](https://github.com/bmatcuk/doublestar) globs; a
pattern with `*` but no `**` also matches at any depth:

| Pattern | Matches |
|---|---|
| `/wp-admin/*` | `/wp-admin/x`, `/a/b/wp-admin/x` |
| `/*.php` | `/admin.php`, `/a/b/admin.php` |
| `/.env` | `/.env` only |
| `/api/**/status` | `/api/status`, `/api/v1/x/status` |

## Admin API and dashboard

| Endpoint | Returns |
|---|---|
| `GET <prefix>/api/statistics/rate-limiter` | Tracked IPs with counts and block time (live). |
| `GET <prefix>/api/rate-limiter/blocked?ip=&country=&limit=&offset=` | Every block ever recorded, newest first (`limit` max 200). |
| `GET <prefix>/api/config/rate-limiter` | The options in force. |

The dashboard's **Rate Limiter** page shows the live table, an attacker map
and the block history; click a country to filter the history.
