# 9. Rate Limiter

Date: 2026-03-05

## Status

Proposed

## Context

The gateway is exposed to the public internet. Without rate limiting, a single IP can flood the server with thousands of requests per second, crawl for vulnerabilities by probing well-known sensitive paths (e.g., `/admin.php`, `/.env`, `/wp-login.php`), or run brute-force attacks against the login endpoint. All three scenarios need mitigation with minimal operational overhead, and without introducing external infrastructure such as Redis or a sidecar database.

## Decision

### Approach

Implement a pure Go, in-memory rate limiter as a standard `net/http` middleware. No external dependencies and no persistent storage.

State is kept in a `sync.Map` keyed by client IP address. A background goroutine periodically evicts entries that have been inactive beyond the block window, preventing unbounded memory growth.

### Three Limiting Modes

| Mode | Trigger | Action |
|---|---|---|
| **Request rate** | More than N requests in one minute from one IP | Return `429 Too Many Requests` |
| **Scan detection** | More than N responses with status 404 from one IP in the block window | Block the IP and return `429` |
| **Auth failure** | More than N responses with status 401 from one IP in the block window | Block the IP and return `429` |

All three modes share the same block duration and eviction logic.

### Algorithm

- **Sliding window counter** for all three limiters. Each IP entry holds a circular list of timestamps for the current window. This requires no floating-point arithmetic and is straightforward to implement with the standard library only.
- Request rate window is always **one minute**.
- Error detection (404 and 401) window equals `blockMinutes`.

### Configuration

A single `rateLimiter` block is added under `management` in `config.yaml`. Three parameters are enough:

```yaml
management:
  rateLimiter:
    requestsPerMinute: 100   # Max requests per IP per minute. 0 = disabled.
    maxErrors: 20            # Max 404 or 401 responses per IP before blocking. 0 = disabled.
    blockMinutes: 15         # How long a blocked IP stays blocked.
```

The corresponding Go struct added to `ManagementConfig`:

```go
type RateLimiterConfig struct {
    RequestsPerMinute int `yaml:"requestsPerMinute"`
    MaxErrors         int `yaml:"maxErrors"`
    BlockMinutes      int `yaml:"blockMinutes"`
}
```

Default values when the block is omitted: all fields zero (all modes disabled).

### Middleware Placement

`RateLimiterMiddleware` is injected early in the middleware chain, before authentication and proxying, so blocked requests never reach upstream services. It wraps the response writer to inspect the status code written by downstream handlers and update the 404/401 counters accordingly.

### File Layout

| File | Purpose |
|---|---|
| `middleware/ratelimiter.go` | Middleware implementation and per-IP state |
| `middleware/ratelimiter_test.go` | Unit tests using an in-memory HTTP handler |
| `config/config.go` | `RateLimiterConfig` struct added to `ManagementConfig` |

### Blocked Response

When an IP is blocked the gateway returns:

```
HTTP/1.1 429 Too Many Requests
Retry-After: <seconds until unblock>
Content-Type: text/plain

Rate limit exceeded
```

### Blocking vulnerability scanners

Some attackers try to make a large number of requests to the gateway, trying to find vulnerabilities. For example, they may probe common paths that are not present (e.g. `/admin.php`, `/.env`, `/wp-login.php`) and look for 404s. If an IP generates too many such responses, it is almost certainly an automated scanner and should be blocked.

The configuration accepts a list of URL patterns (glob-style). Wildcards are supported so you can specify `/*.php` or `/**.sh` to catch entire classes of probes without enumerating every file name.

The limiter tracks 404 responses for any path matching these patterns. A separate, lower threshold can be used compared to the general rate limit. For example, three matches within the block window may trigger a 15‑minute ban. This provides lightweight scanner detection without a signature database.

Example parameters:

```yaml
RateLimiterConfig
    ...
    vulnerabilityScan:
        urls: ["/admin.php", "/*.php", "/**.sh"]  # glob patterns, doublestar syntax
        max404: 3                                      # Max 404 matches before blocking
        blockMinutes: 15                               # How long to block offending IPs
```

### URL patterns

You can spcify URL patterns with wildcards like "*" and "**" (doublestar syntax).

- `*` (star) Matches everithing up to the next slash. For example, `/*.php` matches `/admin.php` but not `/admin.php/config`.
- `**` (doublestar) Matches any number of directories or segments recursively, can match zero or more segments. For example, `/**/setup.sh` matches `/setup.sh` and `/scripts/setup.sh`.

Examples table:

| Pattern | Matches | Does Not Match | Note |
| :--- | :--- | :--- | :--- |
| `/admin.php` | `/admin.php` | `/index.php` | **Exact Match**: Only matches this specific file. |
| `/*.php` | `/index.php`, `/login.php` | `/api/v1/login.php` | **Shallow Wildcard**: Only matches files in the root. |
| `/**.php` | ❌ | ❌ | **Invalid Pattern**: `**` cannot be used in the middle of a segment. |
| `/**/*.sh` | `/setup.sh`, `/bin/run.sh` | `/scripts/run.py` | **Recursive**: Matches files in any directory depth. |
| `/admin/*` | `/admin/config`, `/admin/logs` | `/admin/logs/today.txt`, `/admin.php` | **Folder Wildcard**: Matches one level deep inside `/admin/`. |
| `/admin/**` | `/admin/logs`, `/admin/a/b/c` | `/index.php` | **Full Path**: Matches anything starting with `/admin/`. |
| `/api/*/v1` | `/api/users/v1` | `/api/v1`, `/api/a/b/v1` | **Middle Wildcard**: Exactly one segment between slashes. |
| `/api/**/v1` | `/api/v1`, `/api/a/b/v1` | `/api/v1/extra` | **Middle Recursive**: Any number of segments between slashes. |

| Pattern | Path | Match? | Reason |
| `/api/**/status` | `/api/status` | ✅ **True** | `**` matches **zero** folders in the middle. |
| `/logs/**` | `/logs/` | ✅ **True** | `**` at the end matches the base directory (zero subfolders). |
| `/**/*.js` | `/app.js` | ✅ **True** | `**` at the start matches zero folders (file is in root). |
| `/api/*` | `/api/` | ✅ **True** | `*` matches the "empty" string after the trailing slash. |
| `/api/*` | `/api` | ❌ **False** | There is no slash, so the pattern expects a segment that doesn't exist. |
| `/api/**` | `/api` | ❌ **False** | Standard globbing requires the prefix `/api/` to match recursively. |

### Memory Safety

- Each IP entry is a small struct: two slices of timestamps plus a block-expiry time. Memory per entry is bounded to `O(requestsPerMinute + maxErrors)` timestamps.
- The cleanup goroutine runs every `blockMinutes` and removes entries whose block has expired and whose timestamp slices are empty.

## Consequences

- No Redis, no Memcached, no sidecar: rate-limit state lives in the gateway process only. State is lost on restart — acceptable for short-lived block windows.
- Multiple gateway instances do not share state. For clustered deployments a distributed store would be needed, but that is out of scope for this decision.
- CPU overhead is negligible: one `sync.Map` lookup and a slice scan per request, all O(n) with n bounded by `requestsPerMinute`.
- Legitimate users behind a shared NAT (corporate proxy, IPv4 CGNAT) share the same IP and therefore the same rate-limit bucket. This is an accepted trade-off given the minimal configuration goal.
- The 404 scan detector effectively catches automated vulnerability scanners probing for common paths without requiring any signature database.
- The 401 detector mitigates credential-stuffing attacks against the login endpoint without complex session tracking.

## Related Decisions

- ADR-0002: Login Workflow
- ADR-0006: Token Authentication

## References

- [net/http — standard library](https://pkg.go.dev/net/http)
- [sync.Map — standard library](https://pkg.go.dev/sync#Map)
- [OWASP — Rate Limiting](https://owasp.org/www-community/controls/Blocking_Brute_Force_Attacks)
- [RFC 6585 — 429 Too Many Requests](https://www.rfc-editor.org/rfc/rfc6585)

## Testing Script

The following bash snippet can be used to exercise the rate limiter against a
running gateway (`localhost:8080` by default). It sends a rapid burst of requests
and prints the HTTP status codes:

```bash
#!/usr/bin/env bash

# make sure the gateway has rateLimiter enabled in its YAML
# (e.g. requestsPerMinute: 50).
URL="http://localhost:8080/"
for i in {1..100}; do
  status=$(curl -s -o /dev/null -w "%{http_code}" "$URL")
  echo "request $i -> $status"
  # shorten or remove sleep to push past the rate limit quickly
  sleep 0.1
done
```

If the rate limiter is disabled (zero values in the config) no 429s will
appear; that's why a run of only 300 requests at 0.1s intervals with the
default **disabled** configuration saw no failures.  To hit the limit you must
both enable it and send more than `requestsPerMinute` within a minute.  You can
also target a 404/401 path (e.g. `/nonexistent` or `/_/auth/basic/login` with
bad credentials) to exercise the error‑count limiter.
