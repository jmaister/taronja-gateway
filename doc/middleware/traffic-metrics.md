# Traffic Metrics

**Middleware name:** `traffic_metrics`
**Suggested position:** last of the "analytics" group — after
[`ja4_fingerprint`](ja4-fingerprint.md) and
[`session_extraction`](session-extraction.md), before
[`logging`](logging.md).
**Depends on:** `session_extraction`, `ja4_fingerprint` (both verified
reads — see below).
**Enabled by default:** no; list it under `middleware:`.

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
operator to tune (see `doc/PERFORMANCE_ANALYSIS.md`'s "batching traffic-metrics
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

The dashboard's statistics need all three analytics middleware, in this order:

```yaml
middleware:
  - name: ja4_fingerprint
  - name: session_extraction
  - name: traffic_metrics
```

To skip static assets:

```yaml
  - name: traffic_metrics
    config:
      excludeStaticAssets: true
```

## Config options

| Key | Type | Default | Description |
|---|---|---|---|
| `excludeStaticAssets` | `bool` | `false` | Skip recording a row at all for a request whose path looks like a static asset (CSS/JS/images/fonts/... — see `session.IsStaticAssetPath`). Has no effect unless analytics/this middleware is also enabled. Reduces per-request overhead and stats-table volume on asset-heavy sites. Rows still record `IsStaticAsset`. When this is on, the statistics endpoints ignore the `is_static` filter (no static rows are recorded), the summary reports `staticAssetsExcluded: true`, and the dashboard hides the "Request type" dropdown. The setting is meant to be chosen before data is collected: turning it on later keeps the static rows already stored but no longer lets the dashboard filter them. |

## Reports

- **`GET <prefix>/api/statistics/requests`** (admin-only) — aggregate
  counts by status, country, device, platform, browser, user, and JA4H
  fingerprint over a date range. Backs the dashboard's main **Statistics**
  page.
- **`GET <prefix>/api/statistics/requests/details`** (admin-only) — the raw
  per-request rows over a date range, with an `is_static` filter
  (`true`/`false`/omitted for both). Backs the **Request Details** page
  (`webapp/src/pages/RequestsDetailsPage.tsx`), which surfaces the filter as
  a "Request type" dropdown next to the date-range picker (hidden when
  `excludeStaticAssets` is on), plus a Static/Dynamic badge column so
  unfiltered results still show which is which.

### New vs. returning visitors

The time series (`GET <prefix>/api/statistics/timeseries`) splits each
bucket's unique fingerprints into new and returning visitors. A fingerprint is
new in the bucket that holds its first-ever request, however old that is.

That first request cannot be found by scanning only the requested date range,
so the gateway keeps a `fingerprint_first_seen` table (fingerprint, earliest
timestamp). Migration `0002_fingerprint_first_seen` creates it and backfills it
from `traffic_metrics`. An `AFTER INSERT` trigger on `traffic_metrics` keeps it
current, so single and batch inserts both update it without any Go code.

- The query only reads fingerprints whose first request falls inside the
  requested range, so its cost shrinks with the range.
- With a filter such as country or browser, a fingerprint counts as new in a
  bucket only if it also has a matching request in that bucket. `NewVisitors`
  never exceeds `UniqueFingerprints`.
- Rows without a fingerprint are not tracked.
- The table keeps first-seen dates even if old `traffic_metrics` rows are
  deleted later, so a deleted visitor still counts as "seen before".

## Dependencies

Both declared dependencies are **verified reads**, not just ordering:

- **`session_extraction`** — reads the session from the request context
  (set by that middleware) to record `UserID`/`SessionID` on the row.
- **`ja4_fingerprint`** — via `session.NewClientInfo`, reads the JA4H
  fingerprint header that middleware sets, to populate `JA4Fingerprint` on
  the row.

Listing `traffic_metrics` without either dependency earlier in `middleware:`
fails at startup with a clear error, instead of silently recording metrics
with that field empty.

## See also

- [Middleware Architecture](../../README.md#middleware-architecture) — the
  factory/registry system, and how to inspect the running chain.
- [doc/middleware/ja4-fingerprint.md](ja4-fingerprint.md) and
  [doc/middleware/session-extraction.md](session-extraction.md) — the rest
  of the "analytics" group, run immediately before this one.
- `doc/PERFORMANCE_ANALYSIS.md` — the excludeStaticAssets and write-batching
  measurements referenced above, plus the broader performance history of
  this middleware.
