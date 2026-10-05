# Session Extraction

**Filter name:** `session_extraction`
**Suggested position:** second of the "analytics" group — after
[`ja4_fingerprint`](ja4-fingerprint.md), before
[`traffic_metrics`](traffic-metrics.md).
**Depends on:** `ja4_fingerprint` (ordering only — see below).
**Enabled by default:** no; list it under `filters:`.

## What it does

Resolves the current caller's identity — from a session cookie or a bearer
token, via the same `ValidateSessionFromRequest` logic every other
authentication path in the gateway uses — and, if one is found, attaches it
to the request context (`session.SessionKey`) for downstream code to read.

Critically, **this middleware never enforces authentication.** It runs on
every request regardless of whether the matched route requires
authentication, and an unauthenticated request passes through exactly as
before — no redirect, no `401`. Its only job is making the session
available to whatever runs next in the same request, most importantly
[`traffic_metrics`](traffic-metrics.md), so that a metrics row can record
who made the request even though the route itself doesn't require login
(the route's own `authentication.enabled: true`, handled separately by
`middleware.AuthMiddleware`, is what actually blocks/redirects
unauthenticated requests to protected routes).

## Enabling it

The dashboard's statistics need all three analytics filters, in this order:

```yaml
filters:
  - name: ja4_fingerprint
  - name: session_extraction
  - name: traffic_metrics
```

It needs `ja4_fingerprint` earlier in the list.

## Config options

None. A `config:` block on this filter is rejected.

## Dependencies

Declares a dependency on `ja4_fingerprint`, but — unlike
[`traffic_metrics`](traffic-metrics.md)'s dependency on the same
middleware — this one is **ordering-only, not a verified read**:
`SessionExtractionMiddleware` itself never reads the JA4H header. It's kept
because the gateway's original hardcoded chain always ran JA4H fingerprinting
first ("so fingerprint is available for other middlewares"), and removing
the dependency without being certain nothing downstream implicitly relies
on that ordering wasn't worth the risk for a purely cosmetic simplification.
So listing `session_extraction` always needs `ja4_fingerprint` before it,
even though nothing here consumes the fingerprint.

## See also

- [Filter Architecture](../../README.md#filter-architecture) — the
  factory/registry system, and how to inspect the running chain.
- [doc/middleware/ja4-fingerprint.md](ja4-fingerprint.md) and
  [doc/middleware/traffic-metrics.md](traffic-metrics.md) — the rest of the
  "analytics" group.
