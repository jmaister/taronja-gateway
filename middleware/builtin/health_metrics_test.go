package builtin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loggingChain builds a chain with only logging enabled around handler.
func loggingChain(t *testing.T, status int) (*GlobalChain, http.Handler) {
	t.Helper()
	chain, err := ResolveGlobalChain(&config.GatewayConfig{Middleware: []config.MiddlewareConfig{config.NewMiddlewareConfig(NameLogging, nil)}})
	require.NoError(t, err)
	mw, err := chain.Build(GlobalDeps{})
	require.NoError(t, err)
	return chain, mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
}

func serve(handler http.Handler, n int) {
	for i := 0; i < n; i++ {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ping", nil))
	}
}

// --- Health checks ---

func TestRateLimiterHealth(t *testing.T) {
	assert.Equal(t, "unknown", rateLimiterHealth(GlobalDeps{}).Status)

	rl := NewRateLimiter(RateLimiterConfig{RequestsPerMinute: 100, MaxErrors: 5, BlockMinutes: 5}, nil)
	health := rateLimiterHealth(GlobalDeps{RateLimiter: rl})
	assert.Equal(t, "healthy", health.Status)
	assert.NotEmpty(t, health.Message)
}

func TestStatus_IncludesHealthOnlyWhenImplemented(t *testing.T) {
	chain, _ := loggingChain(t, http.StatusOK)
	byName := map[string]MiddlewareStatus{}
	for _, s := range chain.Status() {
		byName[s.Name] = s
	}
	require.Len(t, byName, len(definitions))

	assert.NotNil(t, byName[NameRateLimiter].Health)
	assert.Nil(t, byName[NameLogging].Health)
	assert.Equal(t, "active", byName[NameLogging].Status)
	assert.Equal(t, "available", byName[NameRateLimiter].Status)
	assert.NotNil(t, byName[NameCORS].Dependencies, "no dependencies is an empty list, not null")
}

// --- Metrics ---

func TestMetrics_UnknownAndNotEnabled(t *testing.T) {
	chain, _ := loggingChain(t, http.StatusOK)

	_, ok := chain.Metrics("does_not_exist")
	assert.False(t, ok)

	snap, ok := chain.Metrics(NameRateLimiter)
	assert.True(t, ok)
	assert.Zero(t, snap.RequestCount)
	assert.Nil(t, snap.LastInvokedAt)
}

func TestMetrics_RecordedAcrossRequests(t *testing.T) {
	chain, handler := loggingChain(t, http.StatusOK)
	serve(handler, 3)

	snap, ok := chain.Metrics(NameLogging)
	require.True(t, ok)
	assert.Equal(t, int64(3), snap.RequestCount)
	assert.Zero(t, snap.ErrorCount)
	assert.NotNil(t, snap.LastInvokedAt)
}

func TestMetrics_CountsServerErrors(t *testing.T) {
	chain, handler := loggingChain(t, http.StatusInternalServerError)
	serve(handler, 1)

	snap, _ := chain.Metrics(NameLogging)
	assert.Equal(t, int64(1), snap.RequestCount)
	assert.Equal(t, int64(1), snap.ErrorCount)
}

func TestAllMetrics_OnlyIncludesEnabledMiddleware(t *testing.T) {
	chain, _ := loggingChain(t, http.StatusOK)
	all := chain.AllMetrics()
	require.Len(t, all, 1)
	assert.Equal(t, NameLogging, all[0].Name)
}
