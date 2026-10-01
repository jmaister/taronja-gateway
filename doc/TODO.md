

# TODO tasks for the project

## Remove prefix config

We assume "_" is the prefix for all gateway routes, no need to configure it.
Using /_tg/ might be a good one so we see the name of the project everywhere.

## Middleware looks repeated

These are the logs from the starting application, looks like middleware gets registered twice

2026/09/24 00:40:09 registry_v2.go:75: Registered middleware factory: compression
2026/09/24 00:40:09 registry_v2.go:75: Registered middleware factory: cors
2026/09/24 00:40:09 registry_v2.go:75: Registered middleware factory: rate_limiter
2026/09/24 00:40:09 registry_v2.go:75: Registered middleware factory: ja4_fingerprint
2026/09/24 00:40:09 registry_v2.go:75: Registered middleware factory: session_extraction
2026/09/24 00:40:09 registry_v2.go:75: Registered middleware factory: traffic_metrics
2026/09/24 00:40:09 registry_v2.go:75: Registered middleware factory: logging
2026/09/24 00:40:09 registry_v2.go:75: Registered middleware factory: tracing
2026/09/24 00:40:09 registry_v2.go:75: Registered middleware factory: ja4_fingerprint
2026/09/24 00:40:09 registry_v2.go:75: Registered middleware factory: session_extraction
2026/09/24 00:40:09 registry_v2.go:75: Registered middleware factory: traffic_metrics
2026/09/24 00:40:09 registry_v2.go:75: Registered middleware factory: logging
2026/09/24 00:40:09 registry_v2.go:75: Registered middleware factory: tracing
2026/09/24 00:40:09 registry_v2.go:75: Registered middleware factory: compression
2026/09/24 00:40:09 registry_v2.go:75: Registered middleware factory: cors
2026/09/24 00:40:09 registry_v2.go:75: Registered middleware factory: rate_limiter
2026/09/24 00:40:09 validation.go:357: All middleware validation completed successfully

## TO FIX

- middleware/fingerprint/ja4.go shows two different headers for the JA4 or fingerprint, we must use only one header name
- chain.go: NewGlobalMiddlewareRegistry and BuildGlobalChainV2, we must have only one method 
- middleware module: separate middleware handling (factory, chain, registry,...) from the middleware implementations (cors, compression, ...), also do not name RegistryV2, just call it Registry
- integration_ja4h_test.go really needed? if so, can be moved to a different place?
- main.go has a GOTO!!!!!!!!!! remove it!
- remove watch/reload/etc... just stop the app and start again, saves too much code
- PERFORMANCE_ANALYSIS.md, move it to docs/
- README.md is huge! summarize and move to docs/middleware/*.md, put links


# Health check 

GET /_/health - Returns 200 OK if the service is running

Health check configuration for the routes configured in the gateway:
- Add a URL to check
- Check interval
- Timeout
- Expected response code
- Show the results in /_/health



## Password

* Add password strength validation with rules
    * Minimum length
    * Special characters
    * Uppercase letters
* Implement password reset functionality
* Maximum password attempts before lockout
    * Unlock account button
* Implement password expiration policy
* 2FA (Two-Factor Authentication)
    * Email-based 2FA?
    * SMS-based 2FA?
    * Authenticator app-based 2FA


# Constants data

## Countries

* Add a list of countries with their ISO codes
* Flag images for each country
* Country names in multiple languages
* Country calling codes
* Country time zones
* Country languages
* Country currencies

/countries -> [{...}, {...}, ...]
/countries/{countryCode} -> {
  "name": "Spain",
  "flag": "/_/countries/ES/flag",
  "isoCode": "ES",
  "iso3Code": "ESP",
  "callingCode": "+34",
  "timeZone": "Europe/Madrid",
  "languages": ["Spanish", "Catalan", "Galician", "Basque", "Valencian"],
  "currency": "EUR"
}
/countries/{countryCode}/flag -> image of the flag

## Languages

* Add a list of languages with their ISO codes
* Language names in multiple languages
* Language codes (ISO 639-1, ISO 639-2, ISO 639-3)

/languages -> [{...}, {...}, ...]
/languages/{languageCode} -> {
    "name": "Spanish",
    "isoCode": "es",
    "iso2Code": "es",
    "iso3Code": "spa",
    "nativeName": "Español",
    "rtl": false,
    "flag": "/_/languages/es/flag"
}
/languages/{languageCode}/flag -> image of the flag

## Timezones

* Add a list of time zones with their names and offsets
* Time zone names in multiple languages
* Time zone offsets in seconds
* Time zoned data: https://github.com/dmfilipenko/timezones.json/blob/master/timezones.json


## Storage

* Add a storage system for user-uploaded files
* Implement file versioning
* Allow users to manage their files (upload, delete, rename)
* Integrate with cloud storage providers (e.g., AWS S3, Google Cloud Storage)


# Gateway feature gaps (vs. Kong/Traefik/nginx/Envoy/Tyk/KrakenD/APISIX/AWS API Gateway)

Deep-dive comparison done 2026-08-28, checked against the actual code (not
just recollection). Grouped by how load-bearing each feature is elsewhere;
we're working through these one at a time — see status notes.

## Tier 1 — near-universal, currently absent

- [ ] **Upstream health checks (active + passive)** for the load balancer
      (`gateway/loadbalancer.go`). Today it only reacts to a failed connection
      *during* a request — no background probing, no ejection of a backend
      that's merely slow/5xx-ing. Natural precursor to the circuit breaker
      already on the README roadmap.
- [ ] **Circuit breaker** (already flagged 🚧 in README) — smaller lift than
      full health checks: "stop trying this backend for N seconds after M
      failures," reusing the round-robin transport's per-target failure count.
- [ ] **Per-route timeouts.** No `Timeout` field in `config.RouteConfig`, and
      no deadline set on the proxy's transport — a hung backend can hold a
      request open indefinitely.
- [ ] **Horizontal scalability of state.** Rate limiter is a plain in-memory
      map; sessions live in SQLite with no Redis/shared-cache option anywhere
      (`grep -r redis` turns up nothing). Running >1 taronja replica today
      gives each instance its own rate-limit counters. Biggest architectural
      gap of the list — needs a deliberate pluggable-store decision, not a
      quick add.

## Tier 2 — very common, moderate lift, in-scope

- [ ] **IP allow/deny lists and geo-blocking.** We already compute
      geolocation for analytics (`session/ipgeo.go`) but nothing *acts* on it.
- [ ] **Security response headers middleware** (HSTS, X-Frame-Options,
      X-Content-Type-Options, CSP) — same shape as `cors.go`.
- [ ] **Request body size limits** — no `MaxBytesReader`/content-length cap
      anywhere, including on the load balancer's body-buffering retry path.
- [ ] **Dynamic upstream discovery** (DNS SRV, Consul, Kubernetes
      Endpoints/EndpointSlice) instead of a static `to:` list.


