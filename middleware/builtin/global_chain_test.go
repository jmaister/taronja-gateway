package builtin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v3"
)

func resolve(t *testing.T, entries ...config.MiddlewareConfig) (*GlobalChain, error) {
	t.Helper()
	return ResolveGlobalChain(&config.GatewayConfig{Middleware: entries})
}

func yamlMiddleware(t *testing.T, src string) []config.MiddlewareConfig {
	t.Helper()
	var entries []config.MiddlewareConfig
	require.NoError(t, yaml.Unmarshal([]byte(src), &entries))
	return entries
}

func TestResolveMiddleware_EmptyByDefault(t *testing.T) {
	chain, err := resolve(t)
	require.NoError(t, err)
	assert.Empty(t, chain.Names())
}

func TestResolveMiddleware_KeepsListOrderAndSkipsDisabled(t *testing.T) {
	chain, err := resolve(t, yamlMiddleware(t, `
- name: logging
- name: compression
  enabled: false
- name: ja4_fingerprint
`)...)
	require.NoError(t, err)
	assert.Equal(t, []string{"logging", "ja4_fingerprint"}, chain.Names())
}

func TestResolveMiddleware_UnknownNameListsTheKnownOnes(t *testing.T) {
	_, err := resolve(t, config.MiddlewareConfig{Name: "not_a_middleware"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown middleware 'not_a_middleware'")
	assert.Contains(t, err.Error(), "rate_limiter")
}

func TestResolveMiddleware_DecodesEachMiddlewaresOptions(t *testing.T) {
	chain, err := resolve(t, yamlMiddleware(t, `
- name: rate_limiter
  config:
    requestsPerMinute: 42
    blockMinutes: 3
- name: ja4_fingerprint
- name: session_extraction
- name: cors
  config:
    allowedOrigins: ["https://a.example"]
- name: tracing
  config:
    endpoint: localhost:4318
- name: traffic_metrics
  config:
    excludeStaticAssets: true
`)...)
	require.NoError(t, err)
	rl, _ := GlobalOptions[RateLimiterConfig](chain, NameRateLimiter)
	assert.Equal(t, RateLimiterConfig{RequestsPerMinute: 42, BlockMinutes: 3}, rl)
	cors, _ := GlobalOptions[CORSConfig](chain, NameCORS)
	assert.Equal(t, CORSConfig{AllowedOrigins: []string{"https://a.example"}}, cors)
	tracing, _ := GlobalOptions[TracingConfig](chain, NameTracing)
	assert.Equal(t, TracingConfig{Endpoint: "localhost:4318"}, tracing)
	tm, _ := GlobalOptions[TrafficMetricsConfig](chain, NameTrafficMetrics)
	assert.Equal(t, TrafficMetricsConfig{ExcludeStaticAssets: true}, tm)
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
			_, err := resolve(t, yamlMiddleware(t, tc.yaml)...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestResolveMiddleware_ChecksDependencies(t *testing.T) {
	_, err := resolve(t, yamlMiddleware(t, "- name: session_extraction\n")...)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "depends on 'ja4_fingerprint'")

	_, err = resolve(t, yamlMiddleware(t, "- name: ja4_fingerprint\n- name: traffic_metrics\n")...)
	assert.ErrorContains(t, err, "depends on 'session_extraction'")

	_, err = resolve(t, yamlMiddleware(t, "- name: ja4_fingerprint\n- name: session_extraction\n- name: traffic_metrics\n")...)
	assert.NoError(t, err, "validation must not need real dependencies")
}

func TestBuild_ReportsMissingRuntimeDependency(t *testing.T) {
	chain, err := resolve(t, yamlMiddleware(t, "- name: ja4_fingerprint\n- name: session_extraction\n")...)
	require.NoError(t, err)
	_, err = chain.Build(GlobalDeps{})
	assert.ErrorContains(t, err, "needs a session store")
}

func TestBuild_BuildsAWorkingChain(t *testing.T) {
	chain, err := resolve(t,
		config.NewMiddlewareConfig(NameCORS, CORSConfig{AllowedOrigins: []string{"https://a.example"}}),
		config.NewMiddlewareConfig(NameLogging, nil),
	)
	require.NoError(t, err)
	assert.True(t, chain.Enabled(NameCORS))
	assert.False(t, chain.Enabled(NameRateLimiter))

	mw, err := chain.Build(GlobalDeps{})
	require.NoError(t, err)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) }))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://a.example")
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)
	assert.Equal(t, http.StatusTeapot, rw.Code)
	assert.Equal(t, "https://a.example", rw.Header().Get("Access-Control-Allow-Origin"))
}

func TestGlobalOptions(t *testing.T) {
	disabled := config.NewMiddlewareConfig(NameTracing, TracingConfig{Endpoint: "x:1"})
	disabled.Enabled = new(bool)
	chain, err := resolve(t,
		config.NewMiddlewareConfig(NameJA4Fingerprint, nil),
		config.NewMiddlewareConfig(NameSessionExtraction, nil),
		config.NewMiddlewareConfig(NameTrafficMetrics, TrafficMetricsConfig{ExcludeStaticAssets: true}),
		disabled,
	)
	require.NoError(t, err)

	tm, enabled := GlobalOptions[TrafficMetricsConfig](chain, NameTrafficMetrics)
	assert.True(t, enabled)
	assert.True(t, tm.ExcludeStaticAssets)

	_, enabled = GlobalOptions[TracingConfig](chain, NameTracing)
	assert.False(t, enabled, "a disabled entry is not in effect")
	assert.False(t, chain.Enabled(NameTracing))

	rl, enabled := GlobalOptions[RateLimiterConfig](chain, NameRateLimiter)
	assert.False(t, enabled)
	assert.Equal(t, RateLimiterConfig{}, rl, "an absent middleware yields zero options")
}
