# Tracing

**Middleware name:** `tracing`
**Global chain position:** first — even before [`compression`](compression.md),
the previous outermost middleware. It needs to see the request before
anything else touches it (to extract an incoming W3C `traceparent` header
as early as possible), and its span should cover the whole request
lifecycle, compression time included.
**Depends on:** nothing.
**Enabled by default:** no.

## What it does

Creates an OpenTelemetry span per request, using
[`go.opentelemetry.io/contrib`'s `otelhttp`](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp) —
the standard, upstream-maintained instrumentation package, not a hand-rolled
span wrapper — and exports it over OTLP/HTTP to whatever collector
`tracing.endpoint` points at (an OpenTelemetry Collector, Jaeger, Tempo,
Honeycomb, Grafana Cloud, or anything else that speaks OTLP).

Two things make this a *distributed* trace, not just a log line with extra
steps:

- **Incoming trace continuation.** If the request already carries a W3C
  `traceparent` header (from a client, or from whatever's in front of this
  gateway), the span this middleware creates is a *child* of that trace,
  not a new, disconnected one.
- **Outbound propagation to proxied backends.** When tracing is enabled,
  every reverse-proxy route's transport is wrapped with `otelhttp.
  NewTransport` too (see `gateway/gateway.go`'s `createProxyHandlerFunc`),
  which starts a child span for the backend call and injects the current
  trace context into the outbound request's own `traceparent` header — so
  a backend that's also instrumented sees itself as part of the same
  trace, not a fresh one.

The span records standard HTTP semantic-convention attributes (method,
route, status code, request/response size, ...) automatically; there's no
way to add custom attributes from config — that would mean either a
scripting hook or a fixed, opinionated set of extra fields, neither of
which exists today.

## Enabling it

Disabled by default, and unlike every other middleware in this reference,
its configuration lives at the top level of the config file
(`tracing:`), not under `management:` — because it isn't really a
dashboard/API concern, it's "where does telemetry about this whole gateway
go":

```yaml
tracing:
  enabled: true
  endpoint: localhost:4318   # OTLP/HTTP collector host:port, no scheme
  insecure: true             # plain HTTP to endpoint, not HTTPS
```

Or, in an explicit [`middleware:` section](../../README.md#middleware-optional-advanced):

```yaml
middleware:
  global:
    - name: tracing
```

Note that the `middleware:` section only controls the middleware's
*position* in the chain — the actual exporter setup (`endpoint`,
`insecure`) still comes from the top-level `tracing:` section either way;
there's no per-entry override for it (see `doc/middleware/README.md`'s
note on this). See
[doc/middleware/README.md#two-ways-to-enable-any-of-these](README.md#two-ways-to-enable-any-of-these)
for the general legacy-flag-vs-`middleware:`-section comparison, and the
important gotcha: an explicit `middleware:` section replaces *every*
legacy flag at once, not just this one.

## Try it locally

The quickest way to see real traces from a real gateway, with a UI, is
[Jaeger](https://www.jaegertracing.io/)'s all-in-one image — one
container gives you an OTLP/HTTP receiver and a web UI to browse traces
in, no separate collector or database to set up. Verified end to end
against this project's actual `tg` binary; any other OTLP-speaking
backend (an OpenTelemetry Collector, Tempo, Honeycomb, Grafana Cloud, ...)
works the same way, just pointed at a different `endpoint`.

1. **Start Jaeger.** `4318` is the OTLP/HTTP port `tracing.endpoint`
   needs; `16686` is the web UI.

   ```sh
   docker run -d --name jaeger \
     -p 16686:16686 \
     -p 4318:4318 \
     jaegertracing/jaeger:latest
   ```

2. **Point a config at it and run the gateway.** `sample/config.yaml`
   already has a commented-out `tracing:` block near the top — uncomment
   it (or add the same three lines to any config with at least one proxy
   route; tracing a route that never proxies anywhere only ever produces
   the one inbound span, not the distributed pair described below):

   ```yaml
   tracing:
     enabled: true
     endpoint: localhost:4318
     insecure: true
   ```

   ```sh
   make build
   ./tg run --config sample/config.yaml
   ```

3. **Send some traffic.** Any request through a route that proxies
   somewhere works — e.g. with the sample config's `/api/v1/*` route:

   ```sh
   curl http://localhost:8080/api/v1/posts/1
   ```

4. **Open the Jaeger UI** at <http://localhost:16686>, pick the
   gateway's `name` (from its config) in the **Service** dropdown, and
   click **Find Traces**. Each request shows up as one trace with (at
   least) two spans: a `SERVER` span for the inbound request and a
   `CLIENT` span nested under it for the outbound call to the backend —
   click into one to see the full waterfall, status code, and timing for
   both hops. Traces land within a few seconds of the request, not
   instantly — `sdktrace.WithBatcher` batches spans rather than exporting
   each one immediately (see "Notes" below), so give it a moment before
   assuming nothing arrived.

   Prefer the raw data over a UI? Jaeger's query API works too:
   `curl 'http://localhost:16686/api/traces?service=<your-gateway-name>'`.

5. **Clean up:** `docker stop jaeger && docker rm jaeger`.

Want this proven automatically instead of by hand? See
`gateway/tracing_test.go`'s
`TestGatewayTracing_DistributedSpanLinkageOverRealOTLP` — it does the
same round trip (proxy a request, check the exported spans) against a
throwaway fake collector, on every test run, with no Docker or manual
steps involved:

```sh
go test ./gateway/... -run TestGatewayTracing_DistributedSpanLinkageOverRealOTLP -v
```

## Config options

| Field | Required | Default | Description |
|---|---|---|---|
| `enabled` | No | `false` | Turns tracing on. Requires `endpoint`. |
| `endpoint` | Yes, when `enabled` | — | The OTLP/HTTP collector's `host:port` — no scheme, no path (e.g. `localhost:4318`). `otlptracehttp` appends the standard `/v1/traces` path itself. |
| `insecure` | No | `false` | Send spans over plain HTTP instead of HTTPS to `endpoint`. Most self-hosted local collectors (a Jaeger or OTel Collector container on the same host or network) don't terminate TLS at all, so this commonly needs setting to `true` for those; a managed backend reachable over the public internet (Honeycomb, Grafana Cloud, ...) almost always wants it left `false`. |

`enabled: true` with no `endpoint` fails config validation (and `tg
validate`) immediately, the same way an incomplete `server.tls` section
does — there's nothing more to check locally than the config's own shape;
actually reaching the collector is a network operation that can only
happen at real gateway startup.

## Notes

- **Fixed at startup, like TLS.** The OTLP exporter is constructed once,
  from whatever `tracing.*` said at the time (`gateway.InitTracing`,
  called from `main.go` before the gateway itself is built). A config
  reload (SIGHUP or file-watch) that changes `tracing.enabled`/`endpoint`/
  `insecure` is stored but has no effect until a full restart — the
  gateway logs a warning when this happens, the same as it does for a
  `server.tls` change on reload.
- **Testing this doesn't need a real collector.** Unit tests
  (`middleware/tracing_test.go`) use the OpenTelemetry SDK's own
  in-memory exporter (`go.opentelemetry.io/otel/sdk/trace/tracetest`) to
  assert on span names, attributes, and parent/child relationships with
  no network involved at all. `gateway/tracing_test.go` goes further and
  verifies real OTLP/HTTP export and real backend header propagation
  against a plain `httptest.Server` standing in for a collector — enough
  to catch a wiring mistake without needing Jaeger, an OTel Collector, or
  Docker in CI. `TestGatewayTracing_DistributedSpanLinkageOverRealOTLP`
  in that file goes one step further still: its fake collector actually
  decodes the real OTLP protobuf export (`go.opentelemetry.io/proto/otlp`)
  and asserts the inbound request's `SERVER` span and the outbound proxy
  call's `CLIENT` span share one trace ID with a real parent/child link
  between them — and that the exact `traceparent` header the backend
  received names that specific child span, not just "some non-empty
  header." Run it directly with
  `go test ./gateway/... -run TestGatewayTracing_DistributedSpanLinkageOverRealOTLP -v`
  to see the whole chain proven end to end on demand.
- **Graceful shutdown flushes pending spans.** `gateway.InitTracing`
  returns a shutdown function `main.go` calls after the server stops
  accepting new requests — without it, a span for the very last requests
  handled before shutdown could sit lost in the batch exporter's internal
  queue when the process exits, never reaching the collector.
- Because it's the very outermost middleware, its span's reported
  duration includes every other middleware's processing time —
  compression, rate limiting, everything — which is the point: it's meant
  to reflect what a client actually experienced end to end.

## See also

- [Middleware Architecture](../../README.md#middleware-architecture) — the
  factory/registry system all of these middlewares are built on, and how to
  inspect the running chain.
- [doc/middleware/compression.md](compression.md) — runs immediately after
  this one in the default chain.
