package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestInstrumentMiddleware_PassesRequestThrough(t *testing.T) {
	counter := &middlewareMetricsCounter{}
	var identity Middleware = func(next http.Handler) http.Handler { return next }
	wrapped := instrumentMiddleware(identity, counter)

	called := false
	handler := wrapped(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusTeapot)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	if !called {
		t.Fatal("expected the wrapped handler to run")
	}
	if rw.Code != http.StatusTeapot {
		t.Fatalf("expected status to pass through unchanged, got %d", rw.Code)
	}

	snap := counter.snapshot("test")
	if snap.RequestCount != 1 {
		t.Fatalf("expected 1 recorded request, got %d", snap.RequestCount)
	}
}

func TestMiddlewareMetricsCounter_AverageDuration(t *testing.T) {
	c := &middlewareMetricsCounter{}
	c.record(http.StatusOK, 10*time.Millisecond)
	c.record(http.StatusOK, 30*time.Millisecond)

	snap := c.snapshot("test")
	if snap.RequestCount != 2 {
		t.Fatalf("expected 2 requests, got %d", snap.RequestCount)
	}
	if snap.AverageDurationMs < 19.9 || snap.AverageDurationMs > 20.1 {
		t.Fatalf("expected average duration ~20ms, got %f", snap.AverageDurationMs)
	}
}
