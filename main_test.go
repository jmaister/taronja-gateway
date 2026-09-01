package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/gateway"
	"github.com/jmaister/taronja-gateway/gateway/deps"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureOutput redirects os.Stdout and os.Stderr for the duration of fn,
// returning what was written to each separately. This exists specifically to
// verify migrateConfigFile's stdout/stderr split: `tg migrate --config x >
// y` only composes cleanly in a shell pipeline if actual config content ever
// only goes to stdout, and everything informational goes to stderr.
func captureOutput(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()

	origStdout, origStderr := os.Stdout, os.Stderr
	defer func() { os.Stdout, os.Stderr = origStdout, origStderr }()

	outR, outW, err := os.Pipe()
	require.NoError(t, err)
	errR, errW, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout, os.Stderr = outW, errW

	fn()

	require.NoError(t, outW.Close())
	require.NoError(t, errW.Close())

	var outBuf, errBuf bytes.Buffer
	_, err = io.Copy(&outBuf, outR)
	require.NoError(t, err)
	_, err = io.Copy(&errBuf, errR)
	require.NoError(t, err)

	return outBuf.String(), errBuf.String()
}

// TestMigrateConfigFile_AbsentVersion_NoteOnStderrOnly is the
// current-schema equivalent of what used to be a genuine migration: with
// config.CurrentConfigVersion at 1 (the first released schema — see its doc
// comment), a config file with no `version:` field at all (fromVersion nil
// — see config.GatewayConfig.Version's doc comment) is accepted as-is, so
// `tg migrate` echoes it unchanged to stdout and notes that on stderr,
// distinctly from the "already version N" note
// TestMigrateConfigFile_AlreadyCurrent_NotePrintedToStderrNotStdout below
// covers for a file with an explicit version: field — there's no longer a
// real "older" config to actually migrate either way.
func TestMigrateConfigFile_AbsentVersion_NoteOnStderrOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	raw := "name: Test\nserver:\n  port: 8080\n"
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o644))

	stdout, stderr := captureOutput(t, func() {
		migrateConfigFile(path)
	})

	assert.Equal(t, raw, stdout, "stdout must be exactly the unchanged config content, nothing else mixed in")
	assert.Contains(t, stderr, "has no declared version")
}

// TestMigrateConfigFile_AlreadyCurrent_NotePrintedToStderrNotStdout guards
// the other half of the split: the "nothing to migrate" note must go to
// stderr, never mixed into stdout, or it would corrupt a redirected file —
// stdout must be exactly the (unchanged) config content on its own.
func TestMigrateConfigFile_AlreadyCurrent_NotePrintedToStderrNotStdout(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	raw := fmt.Sprintf("version: %d\nname: Test\nserver:\n  port: 8080\n", config.CurrentConfigVersion)
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o644))

	stdout, stderr := captureOutput(t, func() {
		migrateConfigFile(path)
	})

	assert.Equal(t, raw, stdout, "stdout must be exactly the unchanged config content, nothing else mixed in")
	assert.Contains(t, stderr, fmt.Sprintf("already version %d", config.CurrentConfigVersion))
}

// TestValidateConfigFile_ValidConfig_PrintsSuccess covers validateConfigFile's
// success path (tg validate). Its failure path — like migrateConfigFile's —
// calls os.Exit(1) directly and isn't covered here for the same reason (see
// the note below); config.LoadConfig's and middleware.ValidateConfigOnly's
// own error cases (what validateConfigFile just forwards) are covered
// directly in config/version_test.go and middleware/validation_test.go.
func TestValidateConfigFile_ValidConfig_PrintsSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	raw := fmt.Sprintf("version: %d\n", config.CurrentConfigVersion) + `name: Test Gateway
server:
  host: 127.0.0.1
  port: 8080
management:
  admin:
    enabled: false
routes:
  - name: root
    from: /
    to: http://localhost:9999
`
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o644))

	stdout, stderr := captureOutput(t, func() {
		validateConfigFile(path)
	})

	assert.Contains(t, stdout, "is valid")
	assert.Contains(t, stdout, fmt.Sprintf("version %d", config.CurrentConfigVersion))
	assert.Contains(t, stdout, "1 route")
	assert.Empty(t, stderr)
}

// TestWatchConfigFile_ReloadsOnWrite is the regression test for --watch:
// saving a new version of the config file (a plain rewrite, the common case
// for both a hand edit and most editors' "safe save" write) must trigger a
// real gateway.ReloadConfig call without anything sending a signal or
// restarting the process.
func TestWatchConfigFile_ReloadsOnWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	write := func(name string) {
		raw := fmt.Sprintf("version: %d\nname: %s\nserver:\n  host: 127.0.0.1\n  port: 8080\n", config.CurrentConfigVersion, name) +
			"management:\n  admin:\n    enabled: false\nroutes: []\n"
		require.NoError(t, os.WriteFile(path, []byte(raw), 0o644))
	}
	write("Before")

	cfg, err := config.LoadConfig(path)
	require.NoError(t, err)
	gw, err := gateway.NewGatewayWithDependencies(cfg, nil, deps.NewTest())
	require.NoError(t, err)
	require.Equal(t, "Before", gw.CurrentConfig().Name)

	stop := make(chan struct{})
	defer close(stop)
	require.NoError(t, watchConfigFile(path, gw, stop))

	write("After")

	deadline := time.Now().Add(5 * time.Second)
	for gw.CurrentConfig().Name != "After" {
		if time.Now().After(deadline) {
			t.Fatalf("watchConfigFile did not reload within the deadline; gateway still has config %q", gw.CurrentConfig().Name)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Note: migrateConfigFile's and validateConfigFile's error paths (a missing/
// unparseable/invalid config file) call os.Exit(1) directly, which would
// kill the test process, so neither is covered here. This file has no other
// tests: main.go otherwise has no existing test coverage in this repo (it's
// a thin Cobra wrapper, manually verified against the built binary), and
// that's unchanged here except for these specific properties.

// TestDotEnvLoadIsFatal is the regression test for a real bug found while
// building examples/docker-demo: runGateway used to call log.Fatal on *any*
// godotenv.Load() error, including the plain-missing-file case — which
// meant the gateway refused to start at all in an environment with no .env
// (a fresh clone, or a container relying on real environment variables
// only, as the Docker demo does), even though .env is meant to be optional
// (addUser already only warned, never fatally, on the same error). Testing
// runGateway itself isn't practical (log.Fatalf calls os.Exit, which would
// kill the test process — see the note above), so this tests the extracted
// decision function directly instead.
func TestDotEnvLoadIsFatal(t *testing.T) {
	assert.False(t, dotEnvLoadIsFatal(nil), "no error at all must never be fatal")

	dir := t.TempDir()
	_, statErr := os.Open(filepath.Join(dir, ".env"))
	require.Error(t, statErr)
	require.True(t, os.IsNotExist(statErr), "test assumption: opening a nonexistent file gives an IsNotExist error")
	assert.False(t, dotEnvLoadIsFatal(statErr), "a merely-missing .env must not be fatal")

	assert.True(t, dotEnvLoadIsFatal(errors.New("permission denied")), "a real error (e.g. malformed .env, unreadable file) must still be fatal")
}
