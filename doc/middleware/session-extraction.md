# Session Extraction

**Middleware name:** `session_extraction`
**Global chain position:** second of the "analytics" group — after
[`ja4_fingerprint`](ja4-fingerprint.md), before
[`traffic_metrics`](traffic-metrics.md).
**Depends on:** `ja4_fingerprint` (ordering only — see below).
**Enabled by default:** no — part of the `analytics` flag/group (see below).

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

There's no dedicated flag for this middleware alone — it's part of the
`analytics` group, on together with [`ja4_fingerprint`](ja4-fingerprint.md)
and [`traffic_metrics`](traffic-metrics.md):

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

None. `GetDefaultConfig()` returns an empty struct — listing
`session_extraction` in an explicit `middleware:` section just
positions/enables it, with nothing to configure.

## Dependencies

Declares a dependency on `ja4_fingerprint`, but — unlike
[`traffic_metrics`](traffic-metrics.md)'s dependency on the same
middleware — this one is **ordering-only, not a verified read**:
`SessionExtractionMiddleware` itself never reads the JA4H header. It's kept
because the gateway's original hardcoded chain always ran JA4H fingerprinting
first ("so fingerprint is available for other middlewares"), and removing
the dependency without being certain nothing downstream implicitly relies
on that ordering wasn't worth the risk for a purely cosmetic simplification.
If you're building an explicit `middleware:` chain and want
`session_extraction` without `ja4_fingerprint`, this is safe to do — you'll
just need to list `ja4_fingerprint` too to satisfy the declared dependency,
even though nothing here actually consumes it.

## See also

- [Middleware Architecture](../../README.md#middleware-architecture) — the
  factory/registry system, and how to inspect the running chain.
- [doc/middleware/ja4-fingerprint.md](ja4-fingerprint.md) and
  [doc/middleware/traffic-metrics.md](traffic-metrics.md) — the rest of the
  "analytics" group.
