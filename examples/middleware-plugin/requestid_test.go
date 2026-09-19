package requestid

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmaister/taronja-gateway/middleware"
)

func TestMiddleware_GeneratesIDWhenAbsent(t *testing.T) {
	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if FromContext(r.Context()) == "" {
			t.Fatal("expected a request ID in context")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	if rw.Header().Get(HeaderName) == "" {
		t.Fatal("expected X-Request-Id response header to be set")
	}
}

func TestMiddleware_PropagatesInboundID(t *testing.T) {
	const inboundID = "caller-supplied-id"
	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := FromContext(r.Context()); got != inboundID {
			t.Fatalf("expected inbound ID to be reused, got %q", got)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(HeaderName, inboundID)
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	if got := rw.Header().Get(HeaderName); got != inboundID {
		t.Fatalf("expected response header to echo the inbound ID, got %q", got)
	}
}

// TestFactory_RegistersAndBuildsThroughRealRegistry is the point of this
// example: it proves a third-party middleware integrates with nothing more
// than middleware.NewMiddlewareRegistryV2 and middleware.MiddlewareFactory —
// the exact same public API the gateway's own built-in middleware use (see
// middleware/factory.go).
func TestFactory_RegistersAndBuildsThroughRealRegistry(t *testing.T) {
	registry := middleware.NewMiddlewareRegistryV2()
	if err := registry.RegisterFactory(NewFactory()); err != nil {
		t.Fatalf("RegisterFactory failed: %v", err)
	}

	chain, err := registry.BuildChain([]middleware.MiddlewareSpec{{Name: "request_id"}})
	if err != nil {
		t.Fatalf("BuildChain failed: %v", err)
	}

	called := false
	handler := chain.Build(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	if !called {
		t.Fatal("expected the wrapped handler to run")
	}
	if rw.Header().Get(HeaderName) == "" {
		t.Fatal("expected X-Request-Id to be set by the built chain")
	}

	status := registry.GetStatus()["request_id"]
	if !status.Enabled || status.Status != "active" {
		t.Fatalf("expected request_id to report active, got %+v", status)
	}
	if status.Health == nil || status.Health.Status != "healthy" {
		t.Fatalf("expected a healthy status from Factory.HealthCheck, got %+v", status.Health)
	}
}
