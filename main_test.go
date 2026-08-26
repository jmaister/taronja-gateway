package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

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

// TestMigrateConfigFile_LegacyConfig_WritesOnlyContentToStdout is the core
// regression test for `tg migrate`'s pipe-friendliness: a genuine migration
// must write nothing but the migrated config to stdout, since that's what
// `tg migrate --config x.yaml > x-v2.yaml` captures.
func TestMigrateConfigFile_LegacyConfig_WritesOnlyContentToStdout(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("name: Test\nserver:\n  port: 8080\n"), 0o644))

	stdout, stderr := captureOutput(t, func() {
		migrateConfigFile(path)
	})

	assert.Contains(t, stdout, "version: 2", "migrated content must be on stdout")
	assert.Contains(t, stdout, "name: Test")
	assert.Empty(t, stderr, "a genuine migration should print nothing to stderr")
}

// TestMigrateConfigFile_AlreadyCurrent_NotePrintedToStderrNotStdout guards
// the other half of the split: the "nothing to migrate" note must go to
// stderr, never mixed into stdout, or it would corrupt a redirected file —
// stdout must be exactly the (unchanged) config content on its own.
func TestMigrateConfigFile_AlreadyCurrent_NotePrintedToStderrNotStdout(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	raw := "version: 2\nname: Test\nserver:\n  port: 8080\n"
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o644))

	stdout, stderr := captureOutput(t, func() {
		migrateConfigFile(path)
	})

	assert.Equal(t, raw, stdout, "stdout must be exactly the unchanged config content, nothing else mixed in")
	assert.Contains(t, stderr, "already version 2")
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
	raw := `version: 2
name: Test Gateway
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
	assert.Contains(t, stdout, "version 2")
	assert.Contains(t, stdout, "1 route")
	assert.Empty(t, stderr)
}

// Note: migrateConfigFile's and validateConfigFile's error paths (a missing/
// unparseable/invalid config file) call os.Exit(1) directly, which would
// kill the test process, so neither is covered here. This file has no other
// tests: main.go otherwise has no existing test coverage in this repo (it's
// a thin Cobra wrapper, manually verified against the built binary), and
// that's unchanged here except for these specific properties.
