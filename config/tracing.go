package config

// TracingConfig configures distributed tracing via OpenTelemetry, exported
// over OTLP/HTTP to a collector (an OpenTelemetry Collector, Jaeger, Tempo,
// Honeycomb, Grafana Cloud, or anything else that speaks OTLP). Disabled by
// default. See doc/middleware/tracing.md and gateway.InitTracing.
type TracingConfig struct {
	// Enabled turns on tracing. Requires Endpoint. Default: false.
	Enabled bool `yaml:"enabled"`
	// Endpoint is the OTLP/HTTP collector's host:port — no scheme, no
	// path (e.g. "localhost:4318", matching otlptracehttp.WithEndpoint's
	// own convention; the exporter appends the standard "/v1/traces" path
	// itself). Required when Enabled.
	Endpoint string `yaml:"endpoint,omitempty"`
	// Insecure sends spans to Endpoint over plain HTTP instead of HTTPS.
	// Default: false (HTTPS) — matching this project's secure-by-default
	// posture elsewhere. Most self-hosted local collectors (a Jaeger or
	// OTel Collector container on the same host or network) don't
	// terminate TLS at all, so this commonly needs setting to true for
	// those; a managed backend reachable over the public internet
	// (Honeycomb, Grafana Cloud, ...) almost always wants it left false.
	Insecure bool `yaml:"insecure,omitempty"`
}
