//go:build e2e

// Package e2e holds end-to-end tests that run the real gateway against real
// external services started by e2e/docker-compose.yml. Run them with
// `make e2e`; they are excluded from `make test` by the e2e build tag.
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/gateway"
	"github.com/jmaister/taronja-gateway/gateway/deps"
	"github.com/stretchr/testify/require"
)

const (
	adminUsername = "admin"
	adminPassword = "e2e-admin-password"
)

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

// testGateway is a gateway running in this process on a random local port.
type testGateway struct {
	URL  string
	Deps *deps.Dependencies
}

// startGateway writes configYAML to a file, loads it with config.LoadConfig
// (the same path `tg run` takes) and serves it until the test ends. The YAML
// may use {{URL}} and {{PORT}} for the gateway's own address.
func startGateway(t *testing.T, configYAML string) *testGateway {
	t.Helper()
	listener, _ := listenGateway(t)
	return startGatewayOn(t, listener, configYAML)
}

// listenGateway reserves the gateway's address before it starts, for tests
// that must register its URL somewhere first (e.g. an OIDC redirect URI).
func listenGateway(t *testing.T) (net.Listener, string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	return listener, fmt.Sprintf("http://127.0.0.1:%d", listener.Addr().(*net.TCPAddr).Port)
}

// startGatewayOn is startGateway on a listener from listenGateway.
func startGatewayOn(t *testing.T, listener net.Listener, configYAML string) *testGateway {
	t.Helper()
	port := listener.Addr().(*net.TCPAddr).Port
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	configYAML = strings.ReplaceAll(configYAML, "{{URL}}", baseURL)
	configYAML = strings.ReplaceAll(configYAML, "{{PORT}}", fmt.Sprint(port))
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(configYAML), 0o600))
	cfg, err := config.LoadConfig(configPath)
	require.NoError(t, err)

	d := deps.NewTestWithName(t.Name())
	shutdownNotifications, err := gateway.InitNotifications(context.Background(), cfg.Notification, cfg.Server, d)
	require.NoError(t, err)
	shutdownBilling := gateway.InitBilling(context.Background(), cfg.Billing, cfg.Server, d)

	gw, err := gateway.NewGatewayWithDependencies(cfg, nil, d)
	require.NoError(t, err)
	go func() { _ = gw.Server.Serve(listener) }()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = gw.Server.Shutdown(ctx)
		_ = shutdownBilling(ctx)
		_ = shutdownNotifications(ctx)
	})
	return &testGateway{URL: baseURL, Deps: d}
}

// baseConfig is a minimal valid config with the admin account enabled.
func baseConfig(extra string) string {
	return `version: "1.0"
name: e2e
server:
  host: 127.0.0.1
  port: {{PORT}}
  url: {{URL}}
management:
  admin:
    enabled: true
    username: ` + adminUsername + `
    password: ` + adminPassword + `
    email: admin@example.com
routes:
  - name: home
    from: /
    toFile: ` + mustAbs("../static/login.html") + `
` + extra
}

func mustAbs(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		panic(err)
	}
	return abs
}

// client is an HTTP client with its own cookie jar, i.e. one browser.
type client struct {
	t    *testing.T
	base string
	http *http.Client
}

func newClient(t *testing.T, base string) *client {
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	return &client{t: t, base: base, http: &http.Client{Jar: jar, Timeout: 15 * time.Second}}
}

// login signs in with the basic (username/password) provider.
func (c *client) login(username, password string) {
	c.t.Helper()
	resp, err := c.http.PostForm(c.base+"/_/auth/basic/login", url.Values{"username": {username}, "password": {password}})
	require.NoError(c.t, err)
	defer resp.Body.Close()
	require.Equal(c.t, http.StatusOK, resp.StatusCode, "login should end on a page after its redirect")

	me := c.getJSON("/_/me", http.StatusOK)
	require.Equal(c.t, true, me["authenticated"])
}

func (c *client) do(method, path string, body any) (*http.Response, []byte) {
	c.t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		require.NoError(c.t, err)
		reader = strings.NewReader(string(encoded))
	}
	req, err := http.NewRequest(method, c.base+path, reader)
	require.NoError(c.t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	require.NoError(c.t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(c.t, err)
	return resp, data
}

func (c *client) getJSON(path string, wantStatus int) map[string]any {
	c.t.Helper()
	return c.sendJSON(http.MethodGet, path, nil, wantStatus)
}

func (c *client) sendJSON(method, path string, body any, wantStatus int) map[string]any {
	c.t.Helper()
	resp, data := c.do(method, path, body)
	require.Equal(c.t, wantStatus, resp.StatusCode, "%s %s: %s", method, path, data)
	var out map[string]any
	if len(data) > 0 {
		require.NoError(c.t, json.Unmarshal(data, &out), "%s %s: %s", method, path, data)
	}
	return out
}

func (c *client) getPage(path string) (*http.Response, string) {
	c.t.Helper()
	resp, err := c.http.Get(c.base + path)
	require.NoError(c.t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(c.t, err)
	return resp, string(body)
}

// me returns /_/me for a signed-in client and nil otherwise.
func (c *client) me() map[string]any {
	c.t.Helper()
	resp, body := c.do(http.MethodGet, "/_/me", nil)
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	return mustJSON(c.t, body)
}

// eventually polls check until it returns true or timeout passes.
func eventually(t *testing.T, timeout time.Duration, what string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("timed out after %s waiting for %s", timeout, what)
}

func mustJSON(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, json.Unmarshal(body, &out))
	return out
}
