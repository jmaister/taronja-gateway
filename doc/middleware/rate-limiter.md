# Rate Limiter

**Middleware name:** `rate_limiter`
**Global chain position:** second — right after [`cors`](cors.md), before
analytics or logging.
**Depends on:** nothing.
**Enabled by default:** no.

## What it does

An in-memory rate limiter keyed by client IP, with three independent limits
that can be used separately or together:

1. **Requests per minute** — blocks an IP once it exceeds a request-count
   threshold in a rolling one-minute window.
2. **Error count** — blocks an IP once it accumulates too many `401`/`404`
   responses within the block window, a simple signal for credential
   stuffing or brute-force probing.
3. **Vulnerability scan detection** — blocks an IP once it triggers too many
   `404`s specifically on a configured list of known-scanner-probe paths
   (`/wp-admin/*`, `/.env`, `/.git/*`, and whatever else you list), separate
   from the general error count above so a handful of legitimate 404s
   elsewhere on the site don't count against it.

Once blocked, every request from that IP gets an immediate `429 Too Many
Requests` with a `Retry-After` header, without reaching the rest of the
chain, until the block window expires.

State is entirely in-memory (a `sync.Map` of per-IP entries, each holding
its own recent-request/error/scan timestamps) — it is **not** persisted or
shared across gateway instances, and a restart clears it. A background
goroutine periodically prunes expired timestamps and removes IPs with no
remaining activity, so long-running processes don't accumulate unbounded
memory from one-off visitors.

## Enabling it

Disabled by default — with `requestsPerMinute`, `maxErrors`, and
`vulnerabilityScan` all unset (or zero), `IsEnabled()` is false and the
middleware is a pure pass-through.

```yaml
management:
  rateLimiter:
    requestsPerMinute: 300
    maxErrors: 20
    blockMinutes: 15
```

Or, in an explicit [`middleware:` section](../../README.md#middleware-optional-advanced):

```yaml
middleware:
  global:
    - name: rate_limiter
      rateLimiter:
        requestsPerMinute: 300
        maxErrors: 20
        blockMinutes: 15
```

## Config options

All under `management.rateLimiter` (legacy flag path) or a `rateLimiter:`
block on the `middleware.global[]` entry (explicit path) — same fields
either way. Every numeric value is "0 disables that specific check"; the
three checks are independent, so you can enable any subset of them.

| Key | Type | Default | Description |
|---|---|---|---|
| `requestsPerMinute` | `int` | `0` (disabled) | Max requests per IP in a rolling 60-second window before it's blocked. |
| `maxErrors` | `int` | `0` (disabled) | Max `401`/`404` responses per IP within `blockMinutes` before it's blocked. |
| `blockMinutes` | `int` | `0` (no blocking) | How long, in minutes, a blocked IP stays blocked. Shared by the `requestsPerMinute` and `maxErrors` checks; the vulnerability scan check has its own `vulnerabilityScan.blockMinutes` (see below). |
| `vulnerabilityScan.urls` | `[]string` | `[]` (disabled) | Path patterns to watch for `404`s on — see the wildcard syntax below. |
| `vulnerabilityScan.max404` | `int` | `0` (disabled) | Max `404`s on a watched path per IP before it's blocked. |
| `vulnerabilityScan.blockMinutes` | `int` | `0` | How long, in minutes, an IP blocked for scanning stays blocked. |

### Vulnerability scan path patterns

Patterns support `*` and `**` glob wildcards
([doublestar](https://github.com/bmatcuk/doublestar) syntax). A bare `*`
segment (no `**` anywhere in the pattern) is automatically expanded to also
match at any nesting depth, not just where you wrote it — so you rarely need
`**` yourself:

| Pattern | Matches |
|---|---|
| `/wp-admin/*` | `/wp-admin/anything`, and (via the automatic expansion) `/some/nested/wp-admin/anything` too |
| `/*.php` | `/admin.php`, `/deep/nested/admin.php` |
| `/.env` | `/.env` exactly |
| `/backup/*.zip` | `/backup/x.zip`, `/backup/a/b/x.zip` |
| `/api/**/status` | `/api/status`, `/api/v1/status`, `/api/v1/nested/status` — an explicit `**` here means you're intentionally matching "status at the end, anything in between", not relying on the automatic expansion |

Patterns are checked once per configured entry per `404` response; matching
stops at the first hit. This list only needs common attack-surface paths
you actually want flagged — it does not need to (and should not) include
your application's real routes.

## Introspection

- **`GET <prefix>/api/statistics/rate-limiter`** (admin-only) — every
  tracked IP's current request/error/scan-404 counts and block-until time.
  Rendered as the **Rate Limiter** page in the admin dashboard
  (`webapp/src/pages/RateLimiterStatsPage.tsx`).
- **`GET <prefix>/api/config/rate-limiter`** (admin-only) — the effective
  configuration currently in force.
- **Health check** (`GET <prefix>/api/middleware`, `tg middleware list`):
  reports `healthy` whenever a rate limiter instance exists, with the
  message naming how many IPs are tracked and how many are currently
  blocked — the rate limiter has no failure mode of its own, so this is
  operational visibility, not an outage detector.

## See also

- [Middleware Architecture](../../README.md#middleware-architecture) — the
  factory/registry system, and how to inspect the running chain.
- [doc/middleware/cors.md](cors.md) — runs immediately before this one in
  the default chain.
- `doc/TODO.md`'s "Rate limiter" section for planned future work (persistent
  attacker tracking, geo info on blocked IPs, JA4-based filtering).
