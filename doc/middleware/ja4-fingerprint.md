# JA4H Fingerprint

**Middleware name:** `ja4_fingerprint`
**Global chain position:** first of the "analytics" group — before
[`session_extraction`](session-extraction.md) and
[`traffic_metrics`](traffic-metrics.md).
**Depends on:** nothing.
**Enabled by default:** no — part of the `analytics` flag/group (see below).

## What it does

Computes a [JA4H fingerprint](https://github.com/FoxIO-LLC/ja4) for every
request — a hash derived from HTTP-level characteristics (method, header
names, `Accept-Language`, cookie/referer presence). It's stored on the
request as the `X-Taronja-JA4H` header
(`middleware/fingerprint.JA4HHeaderName`) for later middleware and
application code to read via `fingerprint.GetJA4FromRequest(req)`. Nothing
in the reverse proxy strips it, so it currently also reaches whatever
backend the request is proxied to, the same as any other request header —
if you don't want a proxied backend to see it, strip it there.

**It is not a stable per-visitor identifier — the same real client
routinely produces several different JA4H values within a single page
load, and this is expected, not a bug.** Concretely
(`github.com/lum8rjack/go-ja4h`'s actual fields — see its source for the
exact format):

- The first segment includes a **count of the request's HTTP headers**.
  Modern browsers send a different header set for a full page navigation
  (`Sec-Fetch-Dest: document`, `Sec-Fetch-User`, `Upgrade-Insecure-Requests`,
  ...) than for a subresource fetch (image/CSS/JS: different
  `Sec-Fetch-Dest`, no `Sec-Fetch-User`) or an XHR/`fetch()` call
  (`Sec-Fetch-Dest: empty`, often fewer headers overall) — so the count, and
  therefore the fingerprint, changes between them even from the identical
  browser tab in the identical session.
- A second segment hashes the **sorted set of header names present** — same
  cause: any optional header appearing/disappearing between two requests
  (client hints, `X-Requested-With`, etc.) changes this hash too.
- The **HTTP method** (`GET` vs `POST`) and **cookie presence** (before vs.
  after login, or a cross-site request where `SameSite` blocks the cookie)
  are both encoded directly, and each request's **referer presence**
  (absent on a typed URL/bookmark, present after clicking a link, stripped
  entirely by some privacy extensions) flips independently of all of the
  above.

In short: expect to see multiple JA4H values for the same logged-in user in
the request-stats dashboard as a matter of course — a page's HTML document,
its CSS/JS/image subresources, and any API calls it makes can each land in
a different bucket. Don't rely on it as a 1:1 device/session identifier;
[`doc/TODO.md`](../TODO.md)'s "Rate limiter" section tracks the still-open
question of what this fingerprint is actually reliable enough to be used
for (anomaly/bot detection in aggregate is the most defensible use so far,
not per-user identity).

The actual fingerprint calculation (`github.com/lum8rjack/go-ja4h`) is
CPU-heavy enough to matter at scale, so the built-in factory wraps it in an
in-memory cache (`ristretto`, keyed by the request characteristics that
feed into the fingerprint — headers, IP, method, protocol — not the
fingerprint itself) with a 1000-entry capacity and a 5-minute TTL per
entry. This is not currently configurable via YAML; it's a fixed
implementation detail of the built-in `ja4_fingerprint` middleware.

## Enabling it

There's no dedicated flag for this middleware alone — it's part of the
`analytics` group, on together with
[`session_extraction`](session-extraction.md) and
[`traffic_metrics`](traffic-metrics.md):

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
```

See [doc/middleware/README.md#two-ways-to-enable-any-of-these](README.md#two-ways-to-enable-any-of-these)
for which form to use, and the important gotcha: an explicit
`middleware:` section replaces *every* legacy flag at once, not just this
one. It's also the only way to enable this middleware without the other
two "analytics" ones — `analytics: true` always turns on all three
together.

## Config options

None. `GetDefaultConfig()` returns an empty struct — listing `ja4_fingerprint`
in an explicit `middleware:` section just positions/enables it, with nothing
to configure.

## Why other middleware depends on it

[`session_extraction`](session-extraction.md) declares a dependency on
`ja4_fingerprint` for ordering reasons (preserving the gateway's original
hardcoded chain order), though it doesn't read the fingerprint itself.
[`traffic_metrics`](traffic-metrics.md) declares the same dependency for a
real reason: it calls `session.NewClientInfo`, which reads the
`X-Taronja-JA4H` header this middleware sets, to populate the
`JA4Fingerprint` field on every recorded `TrafficMetric` row. Building an
explicit chain with `traffic_metrics` but not `ja4_fingerprint` fails fast
at startup with a clear dependency error instead of silently recording
metrics with an empty fingerprint.

## Two companion fingerprints, computed alongside JA4H

This middleware also sets two more headers on every request — not
separately configurable, not separately enable/disable-able, since both
are cheap enough to always compute once `ja4_fingerprint` is running at
all:

- **`X-Taronja-Stable-Fingerprint`** (`fingerprint.GetStableFingerprintFromRequest`,
  `StableFingerprint` field on `ClientInfo`/`TrafficMetric`) — a
  deliberately reduced-entropy fingerprint built only from request
  properties that don't vary by request type: `User-Agent`,
  `Accept-Encoding`, `Accept-Language`, and the low-entropy User-Agent
  Client Hints (`Sec-Ch-Ua*`). It exists specifically to answer "how do I
  reduce JA4H's volatility" (JA4H's own header-count and header-name-set
  components are what makes it change between a navigation and its
  subresource/API requests — see "What it does" above) without waiting for
  TLS. It is **not** part of the JA4 spec family — a custom,
  project-specific signal, named to avoid implying otherwise — and being
  coarser by design, it's meaningfully easier for a deliberately evasive
  client to fake than either JA4 variant. See
  `middleware/fingerprint/stable.go` for the exact field list and
  rationale.
- **`X-Taronja-JA4-TLS`** (`fingerprint.GetJA4TLSFromRequest`,
  `JA4TLSFingerprint` field) — the real TLS-level JA4 fingerprint (cipher
  suites, extensions, ALPN, TLS version from the `ClientHello`), which is
  the most stable of the three by a wide margin since it's a property of
  the client's TLS stack rather than of any individual HTTP request or
  header set. Only populated when the gateway terminates TLS itself
  (`server.tls.enabled`) — see [TLS / HTTPS](../../README.md#tls--https)'s
  "A free bonus of terminating TLS yourself" section, and
  `gateway/ja4tls.go` for the implementation (it isn't wired through the
  `ja4_fingerprint` middleware/factory at all — it's TLS-connection-level
  plumbing set up alongside `server.tls` itself, not a global middleware
  with its own enable/disable flag).

Rough reliability ranking, most to least stable for the same real client
across a browsing session: TLS JA4 > stable fingerprint > JA4H.

## See also

- [Middleware Architecture](../../README.md#middleware-architecture) — the
  factory/registry system, and how to inspect the running chain.
- [doc/middleware/session-extraction.md](session-extraction.md) and
  [doc/middleware/traffic-metrics.md](traffic-metrics.md) — the rest of the
  "analytics" group, run immediately after this one.
- [README.md's TLS / HTTPS section](../../README.md#tls--https) — TLS JA4
  fingerprinting, the more reliable alternative available whenever the
  gateway terminates TLS itself.
