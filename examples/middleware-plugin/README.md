# Example: a third-party middleware

This is a complete, compiled, tested example of adding custom middleware to
Taronja Gateway without touching the gateway's own source — the deliverable
called for by `doc/refactor01.md` Phase 4 ("Example of 3rd-party middleware").

`requestid.go` implements a small but real middleware: it assigns each
request an ID (reusing an inbound `X-Request-Id` header if the caller already
set one, generating a new one otherwise), attaches it to the request context,
and echoes it on the response header — a common building block for tracing
requests across services.

The only things it needs from the gateway are its two public integration
points:

- `middleware.MiddlewareFactory` — the interface `Factory` implements so the
  registry can discover and build the middleware
- `middleware.HealthChecker` — an *optional* interface `Factory` also
  implements here, purely to show the pattern (this middleware has no real
  failure mode, so it always reports healthy)

`requestid_test.go` proves the integration actually works: it registers
`Factory` into a real `middleware.MiddlewareRegistryV2` and builds a chain
with it, the same way `gateway.go` builds the gateway's own global chain from
its built-in factories (see `middleware/chain.go`'s
`NewGlobalMiddlewareRegistry`).

Read `doc/middleware_development.md` for the full walkthrough this example is
based on, including how to wire a middleware like this one into a real
gateway instance.

Run its tests from the repository root:

```bash
go test ./examples/middleware-plugin/...
```
