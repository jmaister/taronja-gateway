# tracing

Creates an OpenTelemetry span per request and exports it over OTLP/HTTP
(OpenTelemetry Collector, Jaeger, Tempo, Honeycomb, Grafana Cloud...).
Incoming `traceparent` headers are continued, and proxied requests carry the
trace to backends.

```yaml
middleware:
  - name: tracing
    config:
      endpoint: localhost:4318   # host:port, no scheme
      insecure: true             # plain HTTP, for local collectors
```

| Option | Default | Description |
|---|---|---|
| `endpoint` | — | OTLP/HTTP collector `host:port`. Required. |
| `insecure` | `false` | Use HTTP instead of HTTPS. |

**Depends on:** nothing. Place it first so the span covers the whole request.

Spans are batched (they reach the collector a few seconds later) and flushed
on graceful shutdown. Changing the options needs a restart.

## Try it with Jaeger

```sh
docker run -d --name jaeger -p 16686:16686 -p 4318:4318 jaegertracing/jaeger:latest
./tg run --config sample/config.yaml   # with the tracing entry above uncommented
curl http://localhost:8080/api/v1/posts/1
```

Open <http://localhost:16686>, pick the gateway's `name` as the service: each
proxied request shows a `SERVER` span with a `CLIENT` span for the backend
call.
