# Logging

**Middleware name:** `logging`
**Suggested position:** last — after everything else, including the
"analytics" group.
**Depends on:** nothing.
**Enabled by default:** no; list it under `middleware:`.

## What it does

Logs one line per request to the gateway's standard log output, after the
request has been fully handled (so the logged status code and duration
reflect what the client actually got):

```
2025-01-15T10:30:00.123Z - 192.0.2.1:54321 "GET /api/users" 200 12.34ms
```

Format: timestamp (`2006-01-02T15:04:05.000Z07:00`), `r.RemoteAddr`, method
and path, response status code, response time in milliseconds. This is a
fixed, plain-text format today — there's no structured (JSON) logging mode,
log level, or header allow-listing yet (see
[`doc/refactor01.md`](../refactor01.md)'s Phase 5 notes for why that's a
deliberately deferred piece of future work, not an oversight).

## Enabling it

```yaml
middleware:
  - name: logging
```

## Config options

None. A `config:` block on this middleware is rejected.

## See also

- [Middleware Architecture](../../README.md#middleware-architecture) — the
  factory/registry system, and how to inspect the running chain.
- [doc/middleware/traffic-metrics.md](traffic-metrics.md) — the structured,
  queryable alternative to this plain-text log line, if you need to middleware
  or aggregate request history rather than just tail it.
