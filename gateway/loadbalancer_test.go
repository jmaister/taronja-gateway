package gateway

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/gateway/deps"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// deadAddr returns an address guaranteed to refuse connections: a listener
// is bound and immediately closed, so nothing is listening there any more.
// Mirrors TestProxyBackendUnreachable_ReturnsBadGatewayInstantly's pattern
// in gateway_test.go.
func deadAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())
	return "http://" + addr
}

// newLoadBalancedTestGateway builds a gateway with a single proxy route
// ("/lb/*" -> targets) and returns it, ready for gw.Mux.ServeHTTP calls —
// no real listener needed, since load balancing/failover happens entirely
// in the outbound direction (proxy.Transport), independent of how the
// inbound request arrives.
func newLoadBalancedTestGateway(t *testing.T, targets []string) *Gateway {
	t.Helper()
	cfg := &config.GatewayConfig{
		Server:     config.ServerConfig{Host: "localhost", Port: 0},
		Management: config.ManagementConfig{Prefix: "/admin"},
		Routes: []config.RouteConfig{
			{
				Name: "Load Balanced",
				From: "/lb/*",
				To:   targets,
			},
		},
	}
	gw, err := NewGatewayWithDependencies(cfg, nil, deps.NewTest())
	require.NoError(t, err)
	return gw
}

func TestProxyRoute_MultipleTargets_RoundRobinsAcrossBackends(t *testing.T) {
	backend1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("backend-1"))
	}))
	defer backend1.Close()
	backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("backend-2"))
	}))
	defer backend2.Close()

	gw := newLoadBalancedTestGateway(t, []string{backend1.URL, backend2.URL})

	// The first request starts round-robin at index 0; each subsequent
	// request advances by one — see roundRobinTransport.RoundTrip's doc
	// comment. With two healthy targets this alternates deterministically.
	want := []string{"backend-1", "backend-2", "backend-1", "backend-2"}
	for i, w := range want {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/lb/ping", nil)
		gw.Mux.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code, "request %d", i)
		assert.Equal(t, w, rr.Body.String(), "request %d should hit %s", i, w)
	}
}

func TestProxyRoute_MultipleTargets_FailsOverToNextBackend(t *testing.T) {
	live := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("live-backend"))
	}))
	defer live.Close()

	// One dead target, one live target. Round-robin alternates which one
	// is tried *first*, but every request must still succeed via the live
	// one — either directly (round-robin started there) or via failover
	// (round-robin started at the dead one, which fails fast, and the
	// transport moves on to the live one within the same request).
	gw := newLoadBalancedTestGateway(t, []string{deadAddr(t), live.URL})

	for i := 0; i < 4; i++ {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/lb/ping", nil)
		gw.Mux.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code, "request %d should succeed via the live backend despite the dead one", i)
		assert.Equal(t, "live-backend", rr.Body.String(), "request %d", i)
	}
}

func TestProxyRoute_MultipleTargets_FailsOverOnPost(t *testing.T) {
	// The failover path buffers and replays the request body (see
	// roundRobinTransport.RoundTrip's doc comment) — this is the one
	// behavior that's genuinely new for a request with a body, so it gets
	// its own test rather than relying on the GET-only coverage above.
	var receivedBody string
	live := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		n, _ := r.Body.Read(buf)
		receivedBody = string(buf[:n])
		w.WriteHeader(http.StatusCreated)
	}))
	defer live.Close()

	gw := newLoadBalancedTestGateway(t, []string{deadAddr(t), live.URL})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/lb/create", strings.NewReader("hello world"))
	req.ContentLength = int64(len("hello world"))
	gw.Mux.ServeHTTP(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code)
	assert.Equal(t, "hello world", receivedBody, "the request body must survive the failover retry intact")
}

func TestProxyRoute_AllTargetsUnreachable_ReturnsBadGatewayInstantly(t *testing.T) {
	gw := newLoadBalancedTestGateway(t, []string{deadAddr(t), deadAddr(t)})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/lb/ping", nil)

	start := time.Now()
	gw.Mux.ServeHTTP(rr, req)
	elapsed := time.Since(start)

	assert.Equal(t, http.StatusBadGateway, rr.Code, "with every backend unreachable, the route must still fail with a clean 502")
	assert.Less(t, elapsed, 2*time.Second, "trying two dead backends in turn must still fail fast, not hang")
}

// TestProxyRoute_MultipleTargets_ConcurrentRequestsDistributeSafely is a
// concurrency stress test for roundRobinTransport's shared atomic counter —
// run with -race, it would catch a data race in the selection logic that
// the sequential round-robin test above can't, since that one never has
// two requests in flight at once.
func TestProxyRoute_MultipleTargets_ConcurrentRequestsDistributeSafely(t *testing.T) {
	var hits1, hits2 atomic.Int64
	backend1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits1.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer backend1.Close()
	backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits2.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer backend2.Close()

	gw := newLoadBalancedTestGateway(t, []string{backend1.URL, backend2.URL})

	const concurrency = 50
	var wg sync.WaitGroup
	var okCount atomic.Int64
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rr := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/lb/ping", nil)
			gw.Mux.ServeHTTP(rr, req)
			if rr.Code == http.StatusOK {
				okCount.Add(1)
			}
		}()
	}
	wg.Wait()

	assert.EqualValues(t, concurrency, okCount.Load(), "every concurrent request must still succeed")
	assert.EqualValues(t, concurrency, hits1.Load()+hits2.Load(), "every request must reach exactly one backend, none lost or double-sent")
	// Not asserting an exact split — concurrent scheduling order isn't
	// deterministic — just that both backends actually got real traffic,
	// i.e. selection isn't silently stuck on one target under load.
	assert.Positive(t, hits1.Load())
	assert.Positive(t, hits2.Load())
}

func TestProxyRoute_SingleTarget_StillWorks(t *testing.T) {
	// Regression guard for the config.RouteTargets type change itself: a
	// single-element To behaves exactly as a bare string always did.
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("only-backend"))
	}))
	defer backend.Close()

	gw := newLoadBalancedTestGateway(t, []string{backend.URL})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/lb/ping", nil)
	gw.Mux.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "only-backend", rr.Body.String())
}
