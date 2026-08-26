package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestCORSMiddleware_DisabledIsPassthrough(t *testing.T) {
	handler := CORSMiddleware(config.CORSConfig{})(newTestHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://example.com")
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	assert.Equal(t, http.StatusOK, rw.Code)
	assert.Empty(t, rw.Header().Get("Access-Control-Allow-Origin"), "disabled CORS must add no headers at all")
}

func TestCORSMiddleware_NoOriginHeaderIsPassthrough(t *testing.T) {
	handler := CORSMiddleware(config.CORSConfig{AllowedOrigins: []string{"https://example.com"}})(newTestHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil) // no Origin header: not a cross-origin request
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	assert.Equal(t, http.StatusOK, rw.Code)
	assert.Empty(t, rw.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORSMiddleware_AllowedOriginGetsHeader(t *testing.T) {
	called := false
	handler := CORSMiddleware(config.CORSConfig{AllowedOrigins: []string{"https://allowed.example.com"}})(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://allowed.example.com")
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	assert.True(t, called, "a real (non-preflight) request must still reach the wrapped handler")
	assert.Equal(t, "https://allowed.example.com", rw.Header().Get("Access-Control-Allow-Origin"))
	assert.Contains(t, rw.Header().Values("Vary"), "Origin")
}

func TestCORSMiddleware_DisallowedOriginGetsNoHeader(t *testing.T) {
	called := false
	handler := CORSMiddleware(config.CORSConfig{AllowedOrigins: []string{"https://allowed.example.com"}})(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://not-allowed.example.com")
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	assert.True(t, called, "the request still reaches the handler; the browser enforces same-origin, not this middleware")
	assert.Empty(t, rw.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORSMiddleware_WildcardWithoutCredentials(t *testing.T) {
	handler := CORSMiddleware(config.CORSConfig{AllowedOrigins: []string{"*"}})(newTestHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://anything.example.com")
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	assert.Equal(t, "*", rw.Header().Get("Access-Control-Allow-Origin"))
	assert.Empty(t, rw.Header().Get("Access-Control-Allow-Credentials"))
}

func TestCORSMiddleware_CredentialsEchoesSpecificOrigin(t *testing.T) {
	handler := CORSMiddleware(config.CORSConfig{
		AllowedOrigins:   []string{"https://allowed.example.com"},
		AllowCredentials: true,
	})(newTestHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://allowed.example.com")
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	assert.Equal(t, "https://allowed.example.com", rw.Header().Get("Access-Control-Allow-Origin"), "must echo the specific origin, never *, when credentials are involved")
	assert.Equal(t, "true", rw.Header().Get("Access-Control-Allow-Credentials"))
}

func TestCORSMiddleware_PreflightShortCircuits(t *testing.T) {
	called := false
	handler := CORSMiddleware(config.CORSConfig{
		AllowedOrigins: []string{"https://allowed.example.com"},
		AllowedMethods: []string{"GET", "POST"},
		AllowedHeaders: []string{"Content-Type"},
		MaxAgeSeconds:  120,
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "https://allowed.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	assert.False(t, called, "a preflight request must never reach the wrapped handler")
	assert.Equal(t, http.StatusNoContent, rw.Code)
	assert.Equal(t, "GET, POST", rw.Header().Get("Access-Control-Allow-Methods"))
	assert.Equal(t, "Content-Type", rw.Header().Get("Access-Control-Allow-Headers"))
	assert.Equal(t, "120", rw.Header().Get("Access-Control-Max-Age"))
}

func TestCORSMiddleware_DefaultsWhenUnset(t *testing.T) {
	handler := CORSMiddleware(config.CORSConfig{AllowedOrigins: []string{"https://allowed.example.com"}})(newTestHandler())

	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "https://allowed.example.com")
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	assert.Equal(t, "GET, POST, PUT, PATCH, DELETE, OPTIONS", rw.Header().Get("Access-Control-Allow-Methods"))
	assert.Equal(t, "Content-Type, Authorization", rw.Header().Get("Access-Control-Allow-Headers"))
	assert.Equal(t, "600", rw.Header().Get("Access-Control-Max-Age"))
}

// --- CORSFactory ---

func TestCORSFactory_RegistersAndBuildsThroughRealRegistry(t *testing.T) {
	registry := NewMiddlewareRegistryV2()
	require.NoError(t, registry.RegisterFactory(NewCORSFactory()))

	chain, err := registry.BuildChain([]MiddlewareSpec{
		{Name: "cors", Config: config.CORSConfig{AllowedOrigins: []string{"https://allowed.example.com"}}},
	})
	require.NoError(t, err)

	handler := chain.Build(newTestHandler())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://allowed.example.com")
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	assert.Equal(t, "https://allowed.example.com", rw.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORSFactory_RejectsWrongConfigType(t *testing.T) {
	f := NewCORSFactory()
	_, err := f.Create("not a CORSConfig")
	require.Error(t, err)
}
