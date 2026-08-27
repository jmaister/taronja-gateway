package gateway

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/gateway/deps"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeReloadTestConfig writes a minimal, valid (version: 2) config file to
// a temp directory and returns its path, so ReloadConfig — which always
// re-reads from disk via config.LoadConfig, exactly like "tg run" — has
// something real to load.
func writeReloadTestConfig(t *testing.T, dir string, extraYAML string) string {
	t.Helper()
	base := `
version: 2
name: Reload Test Gateway
server:
  host: 127.0.0.1
  port: 8080
management:
  prefix: /_
  admin:
    enabled: false
routes:
  - name: v1
    from: /api/*
    to: http://127.0.0.1:1
`
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(base+extraYAML), 0o644))
	return path
}

// TestReloadConfig_SwapsRoutes proves a reload actually takes effect: a
// route added only in the second version of the file is unreachable before
// ReloadConfig and reachable after, without restarting the gateway.
func TestReloadConfig_SwapsRoutes(t *testing.T) {
	dir := t.TempDir()
	path := writeReloadTestConfig(t, dir, "")

	cfg, err := config.LoadConfig(path)
	require.NoError(t, err)

	gw, err := NewGatewayWithDependencies(cfg, nil, deps.NewTest())
	require.NoError(t, err)

	// New route isn't registered yet.
	req := httptest.NewRequest("GET", "/new-route", nil)
	rr := httptest.NewRecorder()
	gw.Mux.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusNotFound, rr.Code, "route from the not-yet-loaded config must not exist before reload")

	// Rewrite the file with an added route, then reload.
	require.NoError(t, os.WriteFile(path, []byte(`
version: 2
name: Reload Test Gateway
server:
  host: 127.0.0.1
  port: 8080
management:
  prefix: /_
  admin:
    enabled: false
routes:
  - name: v1
    from: /api/*
    to: http://127.0.0.1:1
  - name: new-route
    from: /new-route
    to: http://127.0.0.1:1
`), 0o644))

	require.NoError(t, gw.ReloadConfig(path))

	rr = httptest.NewRecorder()
	gw.Mux.ServeHTTP(rr, req)
	assert.NotEqual(t, http.StatusNotFound, rr.Code, "route added by the reloaded config should now be registered (proxy failure is fine, 404 is not)")

	assert.Equal(t, "Reload Test Gateway", gw.GatewayConfig.Name)
	assert.Len(t, gw.GatewayConfig.Routes, 2)
}

// TestReloadConfig_InvalidConfigLeavesRunningConfigUnchanged is the
// regression this feature exists to make safe: a bad edit to the config
// file (here, an unsupported schema version) must not interrupt or corrupt
// an already-running gateway — ReloadConfig should report the error and
// leave the previous, still-valid generation serving exactly as before.
func TestReloadConfig_InvalidConfigLeavesRunningConfigUnchanged(t *testing.T) {
	dir := t.TempDir()
	path := writeReloadTestConfig(t, dir, "")

	cfg, err := config.LoadConfig(path)
	require.NoError(t, err)

	gw, err := NewGatewayWithDependencies(cfg, nil, deps.NewTest())
	require.NoError(t, err)

	originalConfig := gw.GatewayConfig
	originalMux := gw.Mux

	// Overwrite with a config declaring an unsupported (too old) version —
	// config.LoadConfig refuses this outright, same as it would for "tg run".
	require.NoError(t, os.WriteFile(path, []byte(`
version: 1
name: Broken Reload
server:
  host: 127.0.0.1
  port: 8080
routes: []
`), 0o644))

	err = gw.ReloadConfig(path)
	require.Error(t, err, "reloading an outdated/invalid config must fail, not silently apply")

	assert.Same(t, originalConfig, gw.GatewayConfig, "GatewayConfig must be untouched after a failed reload")
	assert.Same(t, originalMux, gw.Mux, "Mux must be untouched after a failed reload")

	// The gateway must still actually serve the original config's routes.
	req := httptest.NewRequest("GET", "/api/anything", nil)
	rr := httptest.NewRecorder()
	gw.Mux.ServeHTTP(rr, req)
	assert.NotEqual(t, http.StatusNotFound, rr.Code, "original route must still be registered after a failed reload")
}

// TestReloadConfig_RateLimiterPicksUpNewLimits proves the rate limiter
// instance handed to the admin/status API is also swapped on reload, not
// just routes — regression coverage for the same
// shared-instance-must-be-rebuilt class of bug the pre-existing
// TestCreateHTTPServer_RateLimiterHonorsPerEntryOverride guards for initial
// construction.
func TestReloadConfig_RateLimiterPicksUpNewLimits(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
version: 2
name: Reload Test Gateway
server:
  host: 127.0.0.1
  port: 8080
management:
  prefix: /_
  admin:
    enabled: false
  rateLimiter:
    requestsPerMinute: 10
    blockMinutes: 5
routes:
  - name: v1
    from: /api/*
    to: http://127.0.0.1:1
`), 0o644))

	cfg, err := config.LoadConfig(path)
	require.NoError(t, err)

	gw, err := NewGatewayWithDependencies(cfg, nil, deps.NewTest())
	require.NoError(t, err)
	require.Equal(t, 10, gw.RateLimiter.Config().RequestsPerMinute)

	require.NoError(t, os.WriteFile(path, []byte(`
version: 2
name: Reload Test Gateway
server:
  host: 127.0.0.1
  port: 8080
management:
  prefix: /_
  admin:
    enabled: false
  rateLimiter:
    requestsPerMinute: 99
    blockMinutes: 5
routes:
  - name: v1
    from: /api/*
    to: http://127.0.0.1:1
`), 0o644))

	require.NoError(t, gw.ReloadConfig(path))
	assert.Equal(t, 99, gw.RateLimiter.Config().RequestsPerMinute, "reload must rebuild the rate limiter from the new limits, not keep serving the old instance")
}

// TestReloadConfig_ConcurrentWithLiveRequests is a data-race regression
// test: the login page handler (registerLoginRoutes) reads the gateway's
// current config on every request, live, which is exactly the kind of read
// that must be synchronized against ReloadConfig's write of the same field
// — see currentConfig's doc comment in reload.go. Run with `go test -race`;
// without currentConfig's locking this reliably reports a DATA RACE between
// this test's request goroutines and its reload goroutine.
func TestReloadConfig_ConcurrentWithLiveRequests(t *testing.T) {
	dir := t.TempDir()
	path := writeReloadTestConfig(t, dir, "")

	cfg, err := config.LoadConfig(path)
	require.NoError(t, err)

	gw, err := NewGatewayWithDependencies(cfg, nil, deps.NewTest())
	require.NoError(t, err)

	// gw.Server.Handler is the real, once-only-assigned entry point live
	// traffic goes through (see reload.go's reloadableHandler) — unlike
	// gw.Mux, which is reassigned on every reload, so hitting it directly
	// here would race against ReloadConfig for a reason that has nothing to
	// do with what this test is actually checking.
	handler := gw.Server.Handler

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 50; i++ {
			req := httptest.NewRequest("GET", "/_/login", nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
		}
	}()

	for i := 0; i < 20; i++ {
		require.NoError(t, gw.ReloadConfig(path))
	}
	<-done
}

// TestReloadConfig_PicksUpEditedDotEnvValue proves ReloadConfig's
// reloadDotEnv step actually matters: the config file's text never changes
// here, only the .env value a `${GW_NAME}` placeholder in it expands to —
// without re-reading .env with overwrite semantics on every reload, this
// would still resolve to the stale value from process startup.
func TestReloadConfig_PicksUpEditedDotEnvValue(t *testing.T) {
	dir := t.TempDir()

	// godotenv.Overload (called by reloadDotEnv, with no explicit filename,
	// same as main.go's own startup godotenv.Load) always reads "./.env" —
	// there's no path parameter to point it elsewhere, so this needs to
	// actually be the working directory for the reload below.
	origWD, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { require.NoError(t, os.Chdir(origWD)) })

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("GW_NAME=Before\n"), 0o644))

	// Simulates what main.go's own startup godotenv.Load already put in the
	// process environment before the gateway's first config.LoadConfig —
	// this test isn't calling that startup path, so it sets the equivalent
	// baseline by hand.
	require.NoError(t, os.Setenv("GW_NAME", "Before"))
	t.Cleanup(func() { require.NoError(t, os.Unsetenv("GW_NAME")) })

	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
version: 2
name: ${GW_NAME}
server:
  host: 127.0.0.1
  port: 8080
management:
  admin:
    enabled: false
routes: []
`), 0o644))

	cfg, err := config.LoadConfig(path)
	require.NoError(t, err)
	require.Equal(t, "Before", cfg.Name)

	gw, err := NewGatewayWithDependencies(cfg, nil, deps.NewTest())
	require.NoError(t, err)

	// The config file's own text is untouched — only .env changes.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("GW_NAME=After\n"), 0o644))

	require.NoError(t, gw.ReloadConfig(path))
	assert.Equal(t, "After", gw.GatewayConfig.Name,
		"reload must re-expand ${GW_NAME} against the freshly-edited .env value, not whatever the process had at startup")
}

// TestReloadConfig_WarnsOnHostOrPortChange covers applyConfig's
// warnIfImmutableFieldsChanged: editing server.host/port and reloading must
// not be silently ignored — it should log clearly that the new value was
// stored but has no effect until a real restart, since the running listener
// can't rebind itself.
func TestReloadConfig_WarnsOnHostOrPortChange(t *testing.T) {
	dir := t.TempDir()
	path := writeReloadTestConfig(t, dir, "") // server.port: 8080

	cfg, err := config.LoadConfig(path)
	require.NoError(t, err)
	gw, err := NewGatewayWithDependencies(cfg, nil, deps.NewTest())
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(path, []byte(`
version: 2
name: Reload Test Gateway
server:
  host: 127.0.0.1
  port: 9999
management:
  prefix: /_
  admin:
    enabled: false
routes:
  - name: v1
    from: /api/*
    to: http://127.0.0.1:1
`), 0o644))

	var logBuf bytes.Buffer
	origOutput := log.Writer()
	log.SetOutput(&logBuf)
	defer log.SetOutput(origOutput)

	require.NoError(t, gw.ReloadConfig(path))

	logged := logBuf.String()
	assert.Contains(t, logged, "cannot rebind without a full restart")
	assert.Contains(t, logged, "8080", "should name the port actually still listening")
	assert.Contains(t, logged, "9999", "should name the new, not-yet-effective port from the file")

	// The stored config still reflects the file, even though the listener
	// didn't move — GatewayConfig has no "partially applied" representation.
	assert.Equal(t, 9999, gw.GatewayConfig.Server.Port)
}
