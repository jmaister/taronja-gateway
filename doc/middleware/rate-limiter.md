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

## How it works

A sliding-window log per client IP (the connecting address, or the
right-most untrusted `X-Forwarded-For` entry behind a trusted proxy). Each IP
keeps the timestamps of its recent requests, `401`/`404` responses and
scanner `404`s; there are no fixed buckets, so a burst can't slip through at a
window edge.

On every request:

1. **Already blocked?** Answer `429` with `Retry-After` (seconds left). The
   request is not forwarded and not counted.
2. **Request rate.** Add the request's timestamp, drop timestamps older than
   60 s. More than `requestsPerMinute` left: block for `blockMinutes` and answer
   this request with `429` too.
3. **Forward the request.** The checks below run on the response, so they
   block the *next* request, not this one.
4. **Errors.** A `401` or `404` adds a timestamp; timestamps older than
   `blockMinutes` are dropped. More than `maxErrors` left: block for
   `blockMinutes`.
5. **Scanners.** A `404` on a path matching `vulnerabilityScan.urls` also adds
   a scan timestamp (so it counts in step 4 as well); timestamps older than
   `vulnerabilityScan.blockMinutes` are dropped. More than `max404` left: block
   for `vulnerabilityScan.blockMinutes`.

A limit trips at the first event *over* it: `requestsPerMinute: 100` allows
100 requests in any 60 s and blocks on the 101st. Requests refused while
blocked don't count, so the request log is empty when the block ends.

Every block is also written to the database for the history page. A
background task (every `blockMinutes`, at least once a minute) drops IPs with
no recent activity and no active block. State is per gateway process: it is
not shared between instances and starts empty after a restart.

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
| `GET /_/api/statistics/rate-limiter` | Tracked IPs with counts and block time (live). |
| `GET /_/api/rate-limiter/blocked?ip=&country=&limit=&offset=` | Every block ever recorded, newest first (`limit` max 200). |
| `GET /_/api/config/rate-limiter` | The options in force. |

The dashboard's **Rate Limiter** page shows the live table, an attacker map
and the block history; click a country to filter the history.
