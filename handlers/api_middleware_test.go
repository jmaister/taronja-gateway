package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/gateway/deps"
	"github.com/jmaister/taronja-gateway/middleware"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupMiddlewareTestServer builds a StrictApiServer wired to a real
// middleware.MiddlewareRegistryV2, with the logging middleware active (built
// into a chain) so status/metrics/health have something concrete to report.
func setupMiddlewareTestServer(t *testing.T) *StrictApiServer {
	t.Helper()
	dependencies := deps.NewTest()

	registry, err := middleware.NewGlobalMiddlewareRegistry(
		dependencies.SessionStore, dependencies.TokenService, dependencies.TrafficMetricRepo, nil,
	)
	require.NoError(t, err)

	gatewayConfig := &config.GatewayConfig{}
	gatewayConfig.Management.Logging = true
	_, err = middleware.BuildGlobalChainFromConfigV2(registry, gatewayConfig)
	require.NoError(t, err)

	return NewStrictApiServer(
		dependencies.SessionStore, dependencies.UserRepo, dependencies.TrafficMetricRepo,
		dependencies.TokenRepo, dependencies.CountersRepo, dependencies.TokenService,
		dependencies.StartTime, nil, registry,
	)
}

func adminContext() context.Context {
	sess := &db.Session{Token: "x", IsAuthenticated: true, IsAdmin: true, ValidUntil: time.Now().Add(time.Hour)}
	return context.WithValue(context.Background(), session.SessionKey, sess)
}

func TestGetMiddlewareStatus_Unauthorized(t *testing.T) {
	s := setupMiddlewareTestServer(t)

	resp, err := s.GetMiddlewareStatus(context.Background(), api.GetMiddlewareStatusRequestObject{})
	require.NoError(t, err)
	_, ok := resp.(api.GetMiddlewareStatus401JSONResponse)
	assert.True(t, ok, "expected 401 for an unauthenticated request")
}

func TestGetMiddlewareStatus_ListsBuiltInMiddleware(t *testing.T) {
	s := setupMiddlewareTestServer(t)

	resp, err := s.GetMiddlewareStatus(adminContext(), api.GetMiddlewareStatusRequestObject{})
	require.NoError(t, err)
	items, ok := resp.(api.GetMiddlewareStatus200JSONResponse)
	require.True(t, ok)
	require.Len(t, items, 8, "expected all 8 built-in global middleware factories")

	byName := make(map[string]api.MiddlewareStatusItem, len(items))
	for _, item := range items {
		byName[item.Name] = item
	}

	logging, ok := byName[config.MiddlewareNameLogging]
	require.True(t, ok)
	assert.True(t, logging.Enabled)
	assert.Equal(t, api.Active, logging.Status)
	assert.Nil(t, logging.Health, "logging has no HealthChecker implementation")

	rateLimiter, ok := byName[config.MiddlewareNameRateLimiter]
	require.True(t, ok)
	assert.False(t, rateLimiter.Enabled, "rate limiter was not enabled in the test config")
	assert.Equal(t, api.Available, rateLimiter.Status)
	require.NotNil(t, rateLimiter.Health, "rate limiter always implements HealthChecker")

	sessionExtraction, ok := byName[config.MiddlewareNameSessionExtraction]
	require.True(t, ok)
	assert.Contains(t, sessionExtraction.Dependencies, config.MiddlewareNameJA4Fingerprint)

	compression, ok := byName[config.MiddlewareNameCompression]
	require.True(t, ok)
	assert.False(t, compression.Enabled, "compression was not enabled in the test config")
	assert.Equal(t, api.Available, compression.Status)

	tracing, ok := byName[config.MiddlewareNameTracing]
	require.True(t, ok)
	assert.False(t, tracing.Enabled, "tracing was not enabled in the test config")
	assert.Equal(t, api.Available, tracing.Status)
}

func TestGetMiddlewareMetrics_Unauthorized(t *testing.T) {
	s := setupMiddlewareTestServer(t)

	resp, err := s.GetMiddlewareMetrics(context.Background(), api.GetMiddlewareMetricsRequestObject{Name: config.MiddlewareNameLogging})
	require.NoError(t, err)
	_, ok := resp.(api.GetMiddlewareMetrics401JSONResponse)
	assert.True(t, ok)
}

func TestGetMiddlewareMetrics_UnknownNameReturns404(t *testing.T) {
	s := setupMiddlewareTestServer(t)

	resp, err := s.GetMiddlewareMetrics(adminContext(), api.GetMiddlewareMetricsRequestObject{Name: "not_a_real_middleware"})
	require.NoError(t, err)
	_, ok := resp.(api.GetMiddlewareMetrics404JSONResponse)
	assert.True(t, ok)
}

func TestGetMiddlewareMetrics_ReportsRecordedRequests(t *testing.T) {
	dependencies := deps.NewTest()

	registry, err := middleware.NewGlobalMiddlewareRegistry(
		dependencies.SessionStore, dependencies.TokenService, dependencies.TrafficMetricRepo, nil,
	)
	require.NoError(t, err)

	gatewayConfig := &config.GatewayConfig{}
	gatewayConfig.Management.Logging = true
	chain, err := middleware.BuildGlobalChainFromConfigV2(registry, gatewayConfig)
	require.NoError(t, err)

	// Drive one request through the built chain so the logging middleware
	// actually records a metric.
	handler := chain.Build(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	s := NewStrictApiServer(
		dependencies.SessionStore, dependencies.UserRepo, dependencies.TrafficMetricRepo,
		dependencies.TokenRepo, dependencies.CountersRepo, dependencies.TokenService,
		dependencies.StartTime, nil, registry,
	)

	resp, err := s.GetMiddlewareMetrics(adminContext(), api.GetMiddlewareMetricsRequestObject{Name: config.MiddlewareNameLogging})
	require.NoError(t, err)
	metrics, ok := resp.(api.GetMiddlewareMetrics200JSONResponse)
	require.True(t, ok)
	assert.Equal(t, int64(1), metrics.RequestCount)
	assert.Equal(t, int64(0), metrics.ErrorCount)
	require.NotNil(t, metrics.LastInvokedAt)
}

func TestGetAllMiddlewareMetrics_Unauthorized(t *testing.T) {
	s := setupMiddlewareTestServer(t)

	resp, err := s.GetAllMiddlewareMetrics(context.Background(), api.GetAllMiddlewareMetricsRequestObject{})
	require.NoError(t, err)
	_, ok := resp.(api.GetAllMiddlewareMetrics401JSONResponse)
	assert.True(t, ok)
}

// TestGetAllMiddlewareMetrics_ReportsOnlyBuiltMiddleware is the Phase 5
// regression test for the bulk metrics endpoint: it must report every
// middleware that was actually built into the chain (with real recorded
// data), and nothing that wasn't.
func TestGetAllMiddlewareMetrics_ReportsOnlyBuiltMiddleware(t *testing.T) {
	dependencies := deps.NewTest()

	registry, err := middleware.NewGlobalMiddlewareRegistry(
		dependencies.SessionStore, dependencies.TokenService, dependencies.TrafficMetricRepo, nil,
	)
	require.NoError(t, err)

	gatewayConfig := &config.GatewayConfig{}
	gatewayConfig.Management.Logging = true // rate_limiter, ja4, etc. stay unbuilt
	chain, err := middleware.BuildGlobalChainFromConfigV2(registry, gatewayConfig)
	require.NoError(t, err)

	handler := chain.Build(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		rw := httptest.NewRecorder()
		handler.ServeHTTP(rw, req)
	}

	s := NewStrictApiServer(
		dependencies.SessionStore, dependencies.UserRepo, dependencies.TrafficMetricRepo,
		dependencies.TokenRepo, dependencies.CountersRepo, dependencies.TokenService,
		dependencies.StartTime, nil, registry,
	)

	resp, err := s.GetAllMiddlewareMetrics(adminContext(), api.GetAllMiddlewareMetricsRequestObject{})
	require.NoError(t, err)
	items, ok := resp.(api.GetAllMiddlewareMetrics200JSONResponse)
	require.True(t, ok)
	require.Len(t, items, 1, "only logging was built into the chain")
	assert.Equal(t, config.MiddlewareNameLogging, items[0].Name)
	assert.Equal(t, int64(2), items[0].RequestCount)
}
