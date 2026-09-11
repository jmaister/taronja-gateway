# Traffic Metrics

**Middleware name:** `traffic_metrics`
**Global chain position:** last of the "analytics" group — after
[`ja4_fingerprint`](ja4-fingerprint.md) and
[`session_extraction`](session-extraction.md), before
[`logging`](logging.md).
**Depends on:** `session_extraction`, `ja4_fingerprint` (both verified
reads — see below).
**Enabled by default:** no — part of the `analytics` flag/group (see below).

## What it does

Records one row per request — method, path, status code, response size,
response time, the authenticated user/session if
[`session_extraction`](session-extraction.md) found one, client info (IP,
geolocation, browser/OS/device, JA4H fingerprint from
[`ja4_fingerprint`](ja4-fingerprint.md)), and whether the path looks like a
static asset (`session.IsStaticAssetPath`) — powering the admin dashboard's
statistics pages and the `GET <prefix>/api/statistics/*` endpoints.

The write to the database is asynchronous (a goroutine per request), so it
never adds latency to the response the client sees. In production
(`gateway/deps.NewProduction()`), it's additionally batched: `Create` calls
are coalesced in memory and flushed to the database every 100 records or
500ms, whichever comes first, rather than one transaction per request —
this is not configurable via YAML, since there's no real trade-off for an
operator to tune (see `PERFORMANCE_ANALYSIS.md`'s "batching traffic-metrics
writes" section for the measured effect and the small, deliberate
durability trade-off: up to 500ms of the very latest rows can be lost on a
hard crash instead of a graceful shutdown). Test dependencies
(`deps.NewTest()`) are not batched, so test assertions can check a row
immediately after a request without a race.

Geolocation lookups (IP → city/country/coordinates) call an external API
(`iplocate.io` if `geolocation.iplocateApiKey` is configured, otherwise the
free `freeipapi.com`) and are cached — 7 days for a successful lookup, 1
minute for a failed one (an unreachable/rate-limited API doesn't retry on
every single subsequent request from the same IP, but does recover quickly
once it's reachable again). `127.x.x.x`/`localhost` are never looked up.

## Enabling it

There's no dedicated flag for this middleware alone — it's part of the
`analytics` group, on together with [`ja4_fingerprint`](ja4-fingerprint.md)
and [`session_extraction`](session-extraction.md):

```yaml
management:
  analytics: true
```

Or, in an explicit [`middleware:` section](../../README.md#middleware-optional-advanced),
where it can be listed (and disabled) independently of the other two:

```yaml
middleware:
  global:
    - name: ja4_fingerprint
    - name: session_extraction
    - name: traffic_metrics
      trafficMetrics:
        excludeStaticAssets: true
```

See [doc/middleware/README.md#two-ways-to-enable-any-of-these](README.md#two-ways-to-enable-any-of-these)
for which form to use, and the important gotcha: an explicit
`middleware:` section replaces *every* legacy flag at once, not just this
one. It's also the only way to enable this middleware without the other
two "analytics" ones — `analytics: true` always turns on all three
together — and the only way to override `excludeStaticAssets` per-entry
rather than gateway-wide.

## Config options

| Key | Type | Default | Description |
|---|---|---|---|
| `management.excludeStaticAssets` (legacy flag path) or `trafficMetrics.excludeStaticAssets` (a `middleware.global[]` entry's block, explicit path) | `bool` | `false` | Skip recording a row at all for a request whose path looks like a static asset (CSS/JS/images/fonts/... — see `session.IsStaticAssetPath`). Has no effect unless analytics/this middleware is also enabled. Reduces per-request overhead and stats-table volume on asset-heavy sites. Every row still records `IsStaticAsset` regardless of this flag, so the [request-details report](#reports) stays filterable by request type either way — flipping this on doesn't affect rows already recorded, only new ones. |

## Reports

- **`GET <prefix>/api/statistics/requests`** (admin-only) — aggregate
  counts by status, country, device, platform, browser, user, and JA4H
  fingerprint over a date range. Backs the dashboard's main **Statistics**
  page.
- **`GET <prefix>/api/statistics/requests/details`** (admin-only) — the raw
  per-request rows over a date range, with an `is_static` filter
  (`true`/`false`/omitted for both). Backs the **Request Details** page
  (`webapp/src/pages/RequestsDetailsPage.tsx`), which surfaces the filter as
  a "Request type" dropdown next to the date-range picker, plus a
  Static/Dynamic badge column so unfiltered results still show which is
  which.

## Dependencies

Both declared dependencies are **verified reads**, not just ordering:

- **`session_extraction`** — reads the session from the request context
  (set by that middleware) to record `UserID`/`SessionID` on the row.
- **`ja4_fingerprint`** — via `session.NewClientInfo`, reads the JA4H
  fingerprint header that middleware sets, to populate `JA4Fingerprint` on
  the row.

Building an explicit `middleware:` chain with `traffic_metrics` but missing
either dependency fails fast at startup with a clear error, instead of
silently recording metrics with that field empty.

## See also

- [Middleware Architecture](../../README.md#middleware-architecture) — the
  factory/registry system, and how to inspect the running chain.
- [doc/middleware/ja4-fingerprint.md](ja4-fingerprint.md) and
  [doc/middleware/session-extraction.md](session-extraction.md) — the rest
  of the "analytics" group, run immediately before this one.
- `PERFORMANCE_ANALYSIS.md` — the excludeStaticAssets and write-batching
  measurements referenced above, plus the broader performance history of
  this middleware.
