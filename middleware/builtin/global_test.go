package builtin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v3"
)

func resolve(t *testing.T, entries ...config.MiddlewareConfig) ([]middleware.MiddlewareSpec, error) {
	t.Helper()
	return ResolveGlobalMiddleware(NewGlobalRegistry(GlobalDeps{}), entries)
}

func yamlMiddleware(t *testing.T, src string) []config.MiddlewareConfig {
	t.Helper()
	var entries []config.MiddlewareConfig
	require.NoError(t, yaml.Unmarshal([]byte(src), &entries))
	return entries
}

func specNamesOf(specs []middleware.MiddlewareSpec) []string {
	names := make([]string, len(specs))
	for i, s := range specs {
		names[i] = s.Name
	}
	return names
}

func TestResolveMiddleware_EmptyByDefault(t *testing.T) {
	specs, err := resolve(t)
	require.NoError(t, err)
	assert.Empty(t, specs)
}

func TestResolveMiddleware_KeepsListOrderAndSkipsDisabled(t *testing.T) {
	specs, err := ResolveGlobalMiddleware(NewGlobalRegistry(GlobalDeps{}), yamlMiddleware(t, `
- name: logging
- name: compression
  enabled: false
- name: ja4_fingerprint
`))
	require.NoError(t, err)
	assert.Equal(t, []string{"logging", "ja4_fingerprint"}, specNamesOf(specs))
}

func TestResolveMiddleware_UnknownNameListsTheKnownOnes(t *testing.T) {
	_, err := resolve(t, config.MiddlewareConfig{Name: "not_a_middleware"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown middleware 'not_a_middleware'")
	assert.Contains(t, err.Error(), "rate_limiter")
}

func TestResolveMiddleware_DecodesEachMiddlewaresOptions(t *testing.T) {
	specs, err := ResolveGlobalMiddleware(NewGlobalRegistry(GlobalDeps{}), yamlMiddleware(t, `
- name: rate_limiter
  config:
    requestsPerMinute: 42
    blockMinutes: 3
- name: cors
  config:
    allowedOrigins: ["https://a.example"]
- name: tracing
  config:
    endpoint: localhost:4318
- name: traffic_metrics
  config:
    excludeStaticAssets: true
`))
	require.NoError(t, err)
	assert.Equal(t, RateLimiterConfig{RequestsPerMinute: 42, BlockMinutes: 3}, specs[0].Config)
	assert.Equal(t, CORSConfig{AllowedOrigins: []string{"https://a.example"}}, specs[1].Config)
	assert.Equal(t, TracingConfig{Endpoint: "localhost:4318"}, specs[2].Config)
	assert.Equal(t, TrafficMetricsConfig{ExcludeStaticAssets: true}, specs[3].Config)
}

func TestResolveMiddleware_RejectsBadOptions(t *testing.T) {
	cases := map[string]struct{ yaml, want string }{
		"unknown option": {`
- name: rate_limiter
  config:
    requestsPerMinut: 10`, "field requestsPerMinut not found"},
		"options on a middleware without any": {`
- name: logging
  config:
    level: debug`, "logging: takes no config"},
		"cors wildcard with credentials": {`
- name: cors
  config:
    allowedOrigins: ["*"]
    allowCredentials: true`, "allowCredentials"},
		"rate limiter without block time": {`
- name: rate_limiter
  config:
    requestsPerMinute: 10`, "blockMinutes must be positive"},
		"tracing without endpoint": {`
- name: tracing`, "tracing: endpoint is required"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ResolveGlobalMiddleware(NewGlobalRegistry(GlobalDeps{}), yamlMiddleware(t, tc.yaml))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestValidateMiddleware_ChecksDependencies(t *testing.T) {
	cfg := &config.GatewayConfig{Middleware: yamlMiddleware(t, "- name: session_extraction\n")}
	err := ValidateGlobalMiddleware(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "depends on 'ja4_fingerprint'")

	cfg.Middleware = yamlMiddleware(t, "- name: ja4_fingerprint\n- name: session_extraction\n- name: traffic_metrics\n")
	assert.NoError(t, ValidateGlobalMiddleware(cfg), "validation must not need real dependencies")
}

func TestTrafficMetrics_DependsDirectlyOnJA4Fingerprint(t *testing.T) {
	deps := newTrafficMetricsFactory(nil).GetDependencies()
	assert.Contains(t, deps, NameJA4Fingerprint)
	assert.Contains(t, deps, NameSessionExtraction)
}

func TestNewGlobalChain_ReportsMissingRuntimeDependency(t *testing.T) {
	cfg := &config.GatewayConfig{Middleware: yamlMiddleware(t, "- name: ja4_fingerprint\n- name: session_extraction\n")}
	_, _, err := NewGlobalChain(cfg, GlobalDeps{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "needs a session store")
}

func TestNewGlobalChain_BuildsAWorkingChain(t *testing.T) {
	cfg := &config.GatewayConfig{Middleware: []config.MiddlewareConfig{
		config.NewMiddlewareConfig(NameCORS, CORSConfig{AllowedOrigins: []string{"https://a.example"}}),
		config.NewMiddlewareConfig(NameLogging, nil),
	}}
	registry, chain, err := NewGlobalChain(cfg, GlobalDeps{})
	require.NoError(t, err)
	assert.True(t, registry.GetStatus()[NameCORS].Enabled)
	assert.False(t, registry.GetStatus()[NameRateLimiter].Enabled)

	handler := chain.Build(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) }))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://a.example")
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)
	assert.Equal(t, http.StatusTeapot, rw.Code)
	assert.Equal(t, "https://a.example", rw.Header().Get("Access-Control-Allow-Origin"))
}

func TestMiddlewareSettings(t *testing.T) {
	disabled := config.NewMiddlewareConfig(NameTracing, TracingConfig{Endpoint: "x:1"})
	disabled.Enabled = new(bool)
	cfg := &config.GatewayConfig{Middleware: []config.MiddlewareConfig{
		config.NewMiddlewareConfig(NameTrafficMetrics, TrafficMetricsConfig{ExcludeStaticAssets: true}),
		disabled,
	}}

	tm, enabled, err := MiddlewareSettings[TrafficMetricsConfig](cfg, NameTrafficMetrics)
	require.NoError(t, err)
	assert.True(t, enabled)
	assert.True(t, tm.ExcludeStaticAssets)

	_, enabled, err = MiddlewareSettings[TracingConfig](cfg, NameTracing)
	require.NoError(t, err)
	assert.False(t, enabled, "a disabled entry is not in effect")
	assert.False(t, MiddlewareEnabled(cfg, NameTracing))

	rl, enabled, err := MiddlewareSettings[RateLimiterConfig](cfg, NameRateLimiter)
	require.NoError(t, err)
	assert.False(t, enabled)
	assert.Equal(t, RateLimiterConfig{}, rl, "an absent middleware yields zero options")

	_, _, err = MiddlewareSettings[CORSConfig](cfg, NameTrafficMetrics)
	assert.Error(t, err, "asking for the wrong config type is a programming error")
}
