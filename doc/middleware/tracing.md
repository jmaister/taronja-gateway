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

## Try it locally

Each backend below listens for OTLP/HTTP on `localhost:4318`, so the gateway
config is the same for all of them:

```yaml
middleware:
  - name: tracing
    config:
      endpoint: localhost:4318
      insecure: true
```

Start the backend, run the gateway (`./tg run --config sample/config.yaml`
with the tracing entry uncommented), then send a request through a proxy
route: `curl http://localhost:8080/api/v1/posts/1`. Each request produces a
`SERVER` span and a `CLIENT` span for the backend call; look for the
gateway's `name` as the service.

### Jaeger

```sh
docker run -d --name jaeger -p 16686:16686 -p 4318:4318 jaegertracing/jaeger:latest
```

UI: <http://localhost:16686>.

### Grafana (Tempo)

```sh
docker run -d --name lgtm -p 3000:3000 -p 4318:4318 grafana/otel-lgtm:latest
```

UI: <http://localhost:3000> (`admin` / `admin`), **Explore** → **Tempo**. New
traces become searchable after about 30 seconds.

### OpenObserve

OpenObserve needs a login header and its own URL path, which the `tracing`
middleware does not send, so an OpenTelemetry Collector sits in between and
adds them.

`otel-collector.yaml` (the header is `base64("root@example.com:Complexpass#123")`):

```yaml
receivers:
  otlp:
    protocols:
      http:
        endpoint: 0.0.0.0:4318
exporters:
  otlphttp/openobserve:
    endpoint: http://openobserve:5080/api/default
    headers:
      Authorization: Basic cm9vdEBleGFtcGxlLmNvbTpDb21wbGV4cGFzcyMxMjM=
service:
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [otlphttp/openobserve]
```

```sh
docker network create tracing
docker run -d --name openobserve --network tracing -p 5080:5080 \
  -e ZO_DATA_DIR=/data -e ZO_ROOT_USER_EMAIL=root@example.com \
  -e ZO_ROOT_USER_PASSWORD='Complexpass#123' \
  o2cr.ai/openobserve/openobserve-enterprise:latest
docker run -d --name otelcol --network tracing -p 4318:4318 \
  -v "$PWD/otel-collector.yaml:/etc/otelcol-contrib/config.yaml" \
  otel/opentelemetry-collector-contrib:latest
```

UI: <http://localhost:5080> (`root@example.com` / `Complexpass#123`), **Traces**.

The same collector setup works for any backend that needs an API key or token
(hosted services such as Honeycomb or Grafana Cloud): change the exporter's
`endpoint` and `headers`.

### SigNoz

SigNoz installs through its own tool, Foundry; follow
[signoz.io/docs/install/docker](https://signoz.io/docs/install/docker/). It
receives OTLP/HTTP on `localhost:4318` without a login, so the config above
works unchanged. Its UI also uses port 8080, so run the gateway on another
port (`server.port: 9080`) and open SigNoz at <http://localhost:8080>,
**Traces**.
