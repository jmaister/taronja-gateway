package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/stretchr/testify/assert"
)

func intPtr(i int) *int {
	return &i
}

func TestHttpCacheMiddleware_NoCacheControlSet(t *testing.T) {
	// Route with no options - no Cache-Control header should be set
	routeConfig := config.RouteConfig{
		Name: "no-options-route",
		From: "/test",
		To:   "http://example.com",
	}

	cacheMiddleware := NewHttpCacheMiddleware()
	handler := cacheMiddleware.CacheControlMiddlewareFunc(routeConfig)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Empty(t, rr.Header().Get("Cache-Control"), "Cache-Control header should not be set when options is nil")
}

func TestHttpCacheMiddleware_NilCacheControlSeconds(t *testing.T) {
	// Route with options but nil CacheControlSeconds - no Cache-Control header should be set
	routeConfig := config.RouteConfig{
		Name:    "nil-cache-route",
		From:    "/test",
		To:      "http://example.com",
		Options: &config.RouteOptions{CacheControlSeconds: nil},
	}

	cacheMiddleware := NewHttpCacheMiddleware()
	handler := cacheMiddleware.CacheControlMiddlewareFunc(routeConfig)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Empty(t, rr.Header().Get("Cache-Control"), "Cache-Control header should not be set when CacheControlSeconds is nil")
}

func TestHttpCacheMiddleware_ZeroCacheControlSeconds(t *testing.T) {
	// Route with CacheControlSeconds=0 should set Cache-Control: no-cache
	routeConfig := config.RouteConfig{
		Name: "no-cache-route",
		From: "/test",
		To:   "http://example.com",
		Options: &config.RouteOptions{
			CacheControlSeconds: intPtr(0),
		},
	}

	cacheMiddleware := NewHttpCacheMiddleware()
	handler := cacheMiddleware.CacheControlMiddlewareFunc(routeConfig)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "no-cache", rr.Header().Get("Cache-Control"), "Cache-Control header should be 'no-cache' when CacheControlSeconds=0")
}

func TestHttpCacheMiddleware_PositiveCacheControlSeconds(t *testing.T) {
	// Route with CacheControlSeconds=3600 should set Cache-Control: max-age=3600
	routeConfig := config.RouteConfig{
		Name: "cached-route",
		From: "/test",
		To:   "http://example.com",
		Options: &config.RouteOptions{
			CacheControlSeconds: intPtr(3600),
		},
	}

	cacheMiddleware := NewHttpCacheMiddleware()
	handler := cacheMiddleware.CacheControlMiddlewareFunc(routeConfig)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "max-age=3600", rr.Header().Get("Cache-Control"), "Cache-Control header should be 'max-age=3600'")
}

func TestHttpCacheMiddleware_NegativeCacheControlSeconds(t *testing.T) {
	// Route with negative CacheControlSeconds should not set Cache-Control header
	routeConfig := config.RouteConfig{
		Name: "negative-cache-route",
		From: "/test",
		To:   "http://example.com",
		Options: &config.RouteOptions{
			CacheControlSeconds: intPtr(-1),
		},
	}

	cacheMiddleware := NewHttpCacheMiddleware()
	handler := cacheMiddleware.CacheControlMiddlewareFunc(routeConfig)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Empty(t, rr.Header().Get("Cache-Control"), "Cache-Control header should not be set when CacheControlSeconds is negative")
}

func TestHttpCacheMiddleware_NextHandlerIsCalled(t *testing.T) {
	// Verify that the next handler is always called
	routeConfig := config.RouteConfig{
		Name: "test-route",
		From: "/test",
		To:   "http://example.com",
		Options: &config.RouteOptions{
			CacheControlSeconds: intPtr(300),
		},
	}

	nextCalled := false
	cacheMiddleware := NewHttpCacheMiddleware()
	handler := cacheMiddleware.CacheControlMiddlewareFunc(routeConfig)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.True(t, nextCalled, "next handler should always be called")
	assert.Equal(t, "max-age=300", rr.Header().Get("Cache-Control"))
}
