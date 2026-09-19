# CORS

**Middleware name:** `cors`
**Global chain position:** second, right after `compression` (which only
wraps the response writer and never rejects a request) — and first among
the middlewares that can actually reject or answer a request, since a CORS
preflight (`OPTIONS`) request is never meant to reach application logic,
rate limiting included.
**Depends on:** nothing.
**Enabled by default:** no.

## What it does

Adds `Access-Control-Allow-*` response headers so a browser running a
frontend on a different origin than the gateway is allowed to call the
management API directly. The gateway's own bundled admin dashboard is
always served same-origin, so this middleware exists purely for the case of
a separately-hosted frontend calling the gateway's API.

For a preflight `OPTIONS` request, it responds immediately with
`Access-Control-Allow-Methods`/`-Headers`/`-Max-Age` and a `204`, without
the request ever reaching the route it targets or the rest of the global
chain — a preflight carries no real application intent, so there's nothing
further for it to do once it's answered.

For every other request, it inspects the `Origin` header: if there's no
`Origin` (a same-origin request) or the origin isn't in the allowed list,
it adds nothing and passes the request through unchanged — this is not an
enforcement mechanism (browsers enforce CORS, not servers) so an
unrecognized origin is not blocked here, just not granted CORS headers. For
an allowed origin, it sets `Access-Control-Allow-Origin` (either `*` or the
specific origin, echoed back with a `Vary: Origin` — see the wildcard rule
below) and, if configured, `Access-Control-Allow-Credentials: true`.

## Enabling it

Disabled by default: with no `allowedOrigins` configured, `IsEnabled()` is
false and the middleware is a pure pass-through that adds no headers at
all — identical to the gateway's behavior before CORS support existed.

```yaml
management:
  cors:
    allowedOrigins: ["https://app.example.com"]
```

Or, in an explicit [`middleware:` section](../../README.md#middleware-optional-advanced):

```yaml
middleware:
  global:
    - name: cors
      cors:
        allowedOrigins: ["https://app.example.com"]
```

See [doc/middleware/README.md#two-ways-to-enable-any-of-these](README.md#two-ways-to-enable-any-of-these)
for which form to use, and the important gotcha: an explicit
`middleware:` section replaces *every* legacy flag at once, not just this
one.

## Config options

All under `management.cors` (legacy flag path) or a `cors:` block on the
`middleware.global[]` entry (explicit path) — same fields either way.

| Key | Type | Default | Description |
|---|---|---|---|
| `allowedOrigins` | `[]string` | `[]` (disabled) | Exact origins (`scheme://host[:port]`) allowed to make cross-origin requests. A literal `"*"` allows any origin, but only when `allowCredentials` is not also `true` — the CORS spec forbids that combination and browsers reject it outright, so the gateway rejects it at config load time instead of shipping a setup that silently doesn't work. |
| `allowCredentials` | `bool` | `false` | Sets `Access-Control-Allow-Credentials: true`, letting browsers send cookies on cross-origin requests. Requires `allowedOrigins` to be an explicit list (see the `*` restriction above). |
| `allowedMethods` | `[]string` | `GET, POST, PUT, PATCH, DELETE, OPTIONS` | Methods allowed in a preflight response. |
| `allowedHeaders` | `[]string` | `Content-Type, Authorization` | Request headers allowed in a preflight response. |
| `maxAgeSeconds` | `int` | `600` (10 minutes) | How long a browser may cache a preflight response (`Access-Control-Max-Age`) before sending another one. |

## Notes

- When `allowedOrigins` is a wildcard `"*"` and credentials aren't
  requested, the response echoes back a literal `Access-Control-Allow-Origin: *`.
  Otherwise — an explicit origin list, or credentials involved — it echoes
  the specific request's `Origin` back (never `*`) and adds `Vary: Origin`,
  since the response then depends on which origin asked.
- This middleware only affects the **management API** (everything under
  `management.prefix`, e.g. `/_/api/...`). It has no effect on your own
  proxied/static routes — add your own CORS handling upstream, or on the
  backend being proxied to, if those need it.

## See also

- [Middleware Architecture](../../README.md#middleware-architecture) — the
  factory/registry system all of these middlewares are built on, and how to
  inspect the running chain.
- [doc/middleware/rate-limiter.md](rate-limiter.md) — runs immediately
  after this one in the default chain.
