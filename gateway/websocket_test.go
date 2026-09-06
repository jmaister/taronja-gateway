package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/gateway/deps"
	"github.com/stretchr/testify/require"
)

// wsEchoBackend starts a real backend that upgrades every connection to a
// WebSocket and echoes back whatever it receives — enough to prove a
// round-trip actually happened through the gateway, not just that a plain
// HTTP request succeeded.
func wsEchoBackend(t *testing.T) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("backend upgrade failed: %v", err)
			return
		}
		defer conn.Close()
		for {
			msgType, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if err := conn.WriteMessage(msgType, data); err != nil {
				return
			}
		}
	}))
	t.Cleanup(server.Close)
	return server
}

// dialWS connects to a gateway route as a real WebSocket client — the point
// of these tests: httptest.NewRecorder() doesn't implement http.Hijacker,
// so this needs a real listening server (httptest.NewServer(gw.Mux)) and a
// real client dial, exactly the way a browser's WebSocket API would.
func dialWS(t *testing.T, serverURL, path string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(serverURL, "http") + path
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		detail := "no response"
		if resp != nil {
			detail = resp.Status
		}
		t.Fatalf("dialing %s: %v (%s)", wsURL, err, detail)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func echoRoundTrip(t *testing.T, conn *websocket.Conn, message string) {
	t.Helper()
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(message)))
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	_, data, err := conn.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, message, string(data))
}

// TestWebSocketProxy_SingleTarget_EchoesMessages is a regression test for a
// real bug: any request going through LoggingMiddleware or
// TrafficMetricMiddleware — both enabled by every sample config
// (management.logging/analytics) — used to fail to upgrade at all.
// http.ResponseController.Hijack() (which httputil.ReverseProxy's upgrade
// handling uses) only follows a wrapper's http.ResponseWriter chain if the
// wrapper implements Unwrap() http.ResponseWriter; neither
// middleware/logging.go's responseWriter nor
// middleware/trafficmetric.go's responseWriterWithStats did, so Hijack()
// failed with http.ErrNotSupported and every WebSocket upgrade through
// either middleware came back as a plain failed request instead of a
// protocol switch — with no visible symptom short of actually trying it,
// which is exactly why this went unnoticed. Deliberately enables both here
// rather than leaving them off, to exercise the real-world configuration
// instead of the coincidentally-simpler one that would have hidden this.
func TestWebSocketProxy_SingleTarget_EchoesMessages(t *testing.T) {
	backend := wsEchoBackend(t)

	cfg := &config.GatewayConfig{
		Server:     config.ServerConfig{Host: "localhost", Port: 0},
		Management: config.ManagementConfig{Prefix: "/admin", Analytics: true, Logging: true},
		Routes: []config.RouteConfig{
			{Name: "WS", From: "/ws/*", To: config.RouteTargets{backend.URL}},
		},
	}
	gw, err := NewGatewayWithDependencies(cfg, nil, deps.NewTest())
	require.NoError(t, err)

	// gw.Mux is the raw, un-wrapped *http.ServeMux — every other gateway
	// test in this package deliberately uses it to isolate routing/proxy
	// logic from the global middleware chain. That's exactly what this
	// test must NOT do: gw.handler (unexported, same-package access) is
	// what applyConfig actually stores as the real http.Server's Handler —
	// mux wrapped by the full global chain (logging, traffic_metrics, ...
	// see buildRuntime) — so this is what a real running gateway serves
	// through, and the only way to catch a bug that only one of those
	// middlewares causes.
	server := httptest.NewServer(gw.handler)
	defer server.Close()

	conn := dialWS(t, server.URL, "/ws/chat")
	echoRoundTrip(t, conn, "hello over the gateway")
	echoRoundTrip(t, conn, "a second message on the same connection")
}

// TestWebSocketProxy_MultipleTargets_EchoesMessages covers the load-balanced
// path specifically: roundRobinTransport.RoundTrip's multi-target branch
// buffers/replays the request body for retry-on-failure, and this confirms
// that doesn't interfere with the upgrade handshake httputil.ReverseProxy
// performs through it.
func TestWebSocketProxy_MultipleTargets_EchoesMessages(t *testing.T) {
	backend1 := wsEchoBackend(t)
	backend2 := wsEchoBackend(t)

	cfg := &config.GatewayConfig{
		Server:     config.ServerConfig{Host: "localhost", Port: 0},
		Management: config.ManagementConfig{Prefix: "/admin", Analytics: true, Logging: true},
		Routes: []config.RouteConfig{
			{Name: "WS", From: "/ws/*", To: config.RouteTargets{backend1.URL, backend2.URL}},
		},
	}
	gw, err := NewGatewayWithDependencies(cfg, nil, deps.NewTest())
	require.NoError(t, err)

	// gw.Mux is the raw, un-wrapped *http.ServeMux — every other gateway
	// test in this package deliberately uses it to isolate routing/proxy
	// logic from the global middleware chain. That's exactly what this
	// test must NOT do: gw.handler (unexported, same-package access) is
	// what applyConfig actually stores as the real http.Server's Handler —
	// mux wrapped by the full global chain (logging, traffic_metrics, ...
	// see buildRuntime) — so this is what a real running gateway serves
	// through, and the only way to catch a bug that only one of those
	// middlewares causes.
	server := httptest.NewServer(gw.handler)
	defer server.Close()

	// Two separate connections, so round-robin's target selection (one per
	// RoundTrip call, i.e. once per upgrade handshake) has a chance to pick
	// each backend across the pair — either way, both must actually work.
	conn1 := dialWS(t, server.URL, "/ws/chat")
	echoRoundTrip(t, conn1, "message on connection 1")

	conn2 := dialWS(t, server.URL, "/ws/chat")
	echoRoundTrip(t, conn2, "message on connection 2")
}
