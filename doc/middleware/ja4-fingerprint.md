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
order and names, `Accept`/`Accept-Language`/`Accept-Encoding`/`User-Agent`
values, cookie presence) that tends to be stable for a given HTTP client
across requests, independent of IP address. It's stored on the request as
the `X-Taronja-JA4H` header (`middleware/fingerprint.JA4HHeaderName`) for
later middleware and application code to read via
`fingerprint.GetJA4FromRequest(req)` — it is not sent to the backend the
request is proxied to.

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

## See also

- [Middleware Architecture](../../README.md#middleware-architecture) — the
  factory/registry system, and how to inspect the running chain.
- [doc/middleware/session-extraction.md](session-extraction.md) and
  [doc/middleware/traffic-metrics.md](traffic-metrics.md) — the rest of the
  "analytics" group, run immediately after this one.
