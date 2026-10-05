package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureOutput redirects os.Stdout/os.Stderr while fn runs and returns each.
func captureOutput(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()

	origStdout, origStderr := os.Stdout, os.Stderr
	defer func() { os.Stdout, os.Stderr = origStdout, origStderr }()

	outR, outW, err := os.Pipe()
	require.NoError(t, err)
	errR, errW, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout, os.Stderr = outW, errW

	// Drain concurrently: large writes (the OpenAPI spec) deadlock a write-then-read on Windows.
	var outBuf, errBuf bytes.Buffer
	done := make(chan error, 2)
	go func() {
		_, copyErr := io.Copy(&outBuf, outR)
		done <- copyErr
	}()
	go func() {
		_, copyErr := io.Copy(&errBuf, errR)
		done <- copyErr
	}()

	fn()

	require.NoError(t, outW.Close())
	require.NoError(t, errW.Close())
	require.NoError(t, <-done)
	require.NoError(t, <-done)

	return outBuf.String(), errBuf.String()
}

// A config with no version is migrated, not treated as current.
func TestMigrateConfigFile_AbsentVersion_ActuallyMigrates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	raw := "name: Test\nserver:\n  port: 8080\n"
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o644))

	stdout, stderr := captureOutput(t, func() {
		migrateConfigFile(path)
	})

	assert.Contains(t, stdout, fmt.Sprintf("version: %s", config.CurrentConfigVersion), "stdout must contain the migrated, now-versioned content")
	assert.Contains(t, stdout, "name: Test", "the rest of the file's content must survive")
	assert.Contains(t, stderr, "no declared version", "a real migration still gets an informational note, distinct from the no-op notes below")
}

// The "already current" note goes to stderr so a redirected stdout stays a clean config.
func TestMigrateConfigFile_AlreadyCurrent_NotePrintedToStderrNotStdout(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	raw := fmt.Sprintf("version: %s\nname: Test\nserver:\n  port: 8080\n", config.CurrentConfigVersion)
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o644))

	stdout, stderr := captureOutput(t, func() {
		migrateConfigFile(path)
	})

	assert.Equal(t, raw, stdout, "stdout must be exactly the unchanged config content, nothing else mixed in")
	assert.Contains(t, stderr, fmt.Sprintf("already version %s", config.CurrentConfigVersion))
}

func TestValidateConfigFile_ValidConfig_PrintsSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	raw := fmt.Sprintf("version: %s\n", config.CurrentConfigVersion) + `name: Test Gateway
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
	assert.Contains(t, stdout, fmt.Sprintf("version %s", config.CurrentConfigVersion))
	assert.Contains(t, stdout, "1 route")
	assert.Empty(t, stderr)
}

// stdout must be exactly the embedded spec so redirecting it yields a valid file.
func TestPrintOpenAPISpec_WritesEmbeddedYamlToStdout(t *testing.T) {
	stdout, stderr := captureOutput(t, printOpenAPISpec)

	assert.Empty(t, stderr)
	assert.Equal(t, string(api.OpenApiSpecYaml), stdout)
	assert.True(t, strings.HasPrefix(stdout, "openapi:"), "stdout must be a YAML OpenAPI document, not a log line")
	assert.Contains(t, stdout, "\n  /openapi.yaml:")
}

// --config belongs on run/migrate/validate/filters list, never on the root (a required root flag broke every subcommand).
func TestRunCommandOwnsConfigFlag(t *testing.T) {
	rootCmd := newRootCmd(&app{})
	assert.Nil(t, rootCmd.Flags().Lookup("config"), "root must not own --config")

	run, _, err := rootCmd.Find([]string{"run"})
	require.NoError(t, err)
	require.NotNil(t, run.Flags().Lookup("config"))

	cmd, _, err := rootCmd.Find([]string{"openapi"})
	require.NoError(t, err)
	assert.Equal(t, "openapi", cmd.Name())
	assert.Nil(t, cmd.Flags().Lookup("config"), "openapi must not require --config")
}

// A missing .env must not stop startup; a malformed or unreadable one must.
func TestDotEnvLoadIsFatal(t *testing.T) {
	assert.False(t, dotEnvLoadIsFatal(nil), "no error at all must never be fatal")

	dir := t.TempDir()
	_, statErr := os.Open(filepath.Join(dir, ".env"))
	require.Error(t, statErr)
	require.True(t, os.IsNotExist(statErr), "test assumption: opening a nonexistent file gives an IsNotExist error")
	assert.False(t, dotEnvLoadIsFatal(statErr), "a merely-missing .env must not be fatal")

	assert.True(t, dotEnvLoadIsFatal(errors.New("permission denied")), "a real error (e.g. malformed .env, unreadable file) must still be fatal")
}
