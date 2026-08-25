package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- setTopLevelVersionField ---

func TestSetTopLevelVersionField_InsertsWhenAbsent(t *testing.T) {
	raw := []byte("name: Test\nserver:\n  port: 8080\n")
	got := setTopLevelVersionField(raw, 2)
	assert.Equal(t, "version: 2\nname: Test\nserver:\n  port: 8080\n", string(got))
}

func TestSetTopLevelVersionField_ReplacesWhenPresent(t *testing.T) {
	raw := []byte("version: 1\nname: Test\n")
	got := setTopLevelVersionField(raw, 2)
	assert.Equal(t, "version: 2\nname: Test\n", string(got))
}

func TestSetTopLevelVersionField_DoesNotMatchNestedVersionKey(t *testing.T) {
	// A "version:" key indented under some other section must not be treated
	// as the top-level field — only a literal, unindented "version:" at
	// column 0 counts.
	raw := []byte("name: Test\nsomeSection:\n  version: 99\n")
	got := setTopLevelVersionField(raw, 2)
	assert.Equal(t, "version: 2\nname: Test\nsomeSection:\n  version: 99\n", string(got))
}

func TestSetTopLevelVersionField_PreservesComments(t *testing.T) {
	raw := []byte("# a helpful comment\nname: Test\nserver:\n  port: 8080 # inline comment\n")
	got := setTopLevelVersionField(raw, 2)
	assert.Contains(t, string(got), "# a helpful comment")
	assert.Contains(t, string(got), "# inline comment")
}

// --- versionedConfigPath ---

func TestVersionedConfigPath(t *testing.T) {
	tests := []struct {
		name     string
		original string
		version  int
		expected string
	}{
		{"simple yaml", "config.yaml", 2, "config-v2.yaml"},
		{"yml extension", "/etc/gateway/prod.yml", 2, "/etc/gateway/prod-v2.yml"},
		{"relative path", "./sample/config.yaml", 2, "sample/config-v2.yaml"},
		{"no extension", "config", 2, "config-v2"},
		{"higher version", "config.yaml", 3, "config-v3.yaml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := versionedConfigPath(tt.original, tt.version)
			assert.Equal(t, filepath.Clean(tt.expected), filepath.Clean(got))
		})
	}
}

// --- migrateConfigToCurrent ---

func TestMigrateConfigToCurrent_AppliesRegisteredSteps(t *testing.T) {
	raw := []byte("name: Test\n")
	got := migrateConfigToCurrent(raw, 1)
	assert.Equal(t, "version: 2\nname: Test\n", string(got))
}

func TestMigrateConfigToCurrent_NoOpAtCurrentVersion(t *testing.T) {
	raw := []byte("version: 2\nname: Test\n")
	got := migrateConfigToCurrent(raw, CurrentConfigVersion)
	assert.Equal(t, string(raw), string(got))
}

// --- LoadConfig integration ---

func TestLoadConfig_LegacyFile_MigratesAndPreservesSecrets(t *testing.T) {
	t.Setenv("TEST_VERSION_MIGRATION_SECRET", "super-secret-value")

	raw := `name: Test Gateway
server:
  host: 127.0.0.1
  port: 8080
  # a comment that must survive migration
management:
  admin:
    enabled: false
authenticationProviders:
  google:
    clientId: ${TEST_VERSION_MIGRATION_SECRET}
routes:
  - name: root
    from: /
    to: http://localhost:9999
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o644))

	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	assert.Equal(t, CurrentConfigVersion, cfg.Version, "in-memory config should behave as the current version")

	// Original file must be untouched.
	originalAfter, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, raw, string(originalAfter), "the original config file must not be modified")

	// Migrated file must exist, contain the new version field, preserve the
	// comment, and — critically — must NOT contain the resolved secret.
	migratedPath := filepath.Join(dir, "config-v2.yaml")
	migrated, err := os.ReadFile(migratedPath)
	require.NoError(t, err, "expected a migrated config-v2.yaml to be created")
	migratedStr := string(migrated)
	assert.Contains(t, migratedStr, "version: 2")
	assert.Contains(t, migratedStr, "# a comment that must survive migration")
	assert.Contains(t, migratedStr, "${TEST_VERSION_MIGRATION_SECRET}", "migrated file must keep the env var placeholder, not resolve it")
	assert.NotContains(t, migratedStr, "super-secret-value", "migrated file must never contain a resolved secret")
}

func TestLoadConfig_CurrentVersionFile_NoMigrationFileCreated(t *testing.T) {
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
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o644))

	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	assert.Equal(t, CurrentConfigVersion, cfg.Version)

	_, err = os.Stat(filepath.Join(dir, "config-v2.yaml"))
	assert.True(t, os.IsNotExist(err), "no migrated file should be created when the config is already current")
}

func TestLoadConfig_MigratedFileAlreadyExists_NotOverwritten(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(minimalTestConfigYAML), 0o644))

	migratedPath := filepath.Join(dir, "config-v2.yaml")
	sentinel := "version: 2\n# hand-edited, do not clobber\nname: Hand Edited\n"
	require.NoError(t, os.WriteFile(migratedPath, []byte(sentinel), 0o644))

	_, err := LoadConfig(path)
	require.NoError(t, err)

	after, err := os.ReadFile(migratedPath)
	require.NoError(t, err)
	assert.Equal(t, sentinel, string(after), "an existing migrated file must not be overwritten")
}

func TestLoadConfig_NewerVersionThanSupported_ProceedsWithWarning(t *testing.T) {
	raw := "version: 99\n" + minimalTestConfigYAML
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o644))

	cfg, err := LoadConfig(path)
	require.NoError(t, err, "a newer-than-supported version should log a warning, not fail")
	assert.Equal(t, 99, cfg.Version, "an unsupported newer version should be left as declared, not overwritten")

	_, err = os.Stat(filepath.Join(dir, "config-v99.yaml"))
	assert.True(t, os.IsNotExist(err), "no migration file should be created for a newer-than-supported version")
}

const minimalTestConfigYAML = `name: Test Gateway
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
