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
//
// LoadConfig must refuse to run against a config file older than
// CurrentConfigVersion, rather than silently migrating it — see
// checkConfigVersion. Migration itself is a deliberate, explicit step via
// `tg migrate` / config.MigrateConfigFile, tested separately below.

func TestLoadConfig_LegacyFile_FailsWithMigrationAdvice(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(minimalTestConfigYAML), 0o644))

	_, err := LoadConfig(path)
	require.Error(t, err, "LoadConfig must refuse to run against a config file older than CurrentConfigVersion")
	assert.Contains(t, err.Error(), "version 1")
	assert.Contains(t, err.Error(), "requires version 2")
	assert.Contains(t, err.Error(), "tg migrate --config "+path, "the error must tell the user how to fix it")

	// LoadConfig must not have written anything to disk on this path — that's
	// what tg migrate is for now, not an automatic side effect of loading.
	_, statErr := os.Stat(filepath.Join(dir, "config-v2.yaml"))
	assert.True(t, os.IsNotExist(statErr), "LoadConfig must not write a migrated file itself")
}

func TestLoadConfig_CurrentVersionFile_Succeeds(t *testing.T) {
	raw := "version: 2\n" + minimalTestConfigYAML
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o644))

	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	assert.Equal(t, CurrentConfigVersion, cfg.Version)
}

func TestLoadConfig_NewerVersionThanSupported_ProceedsWithWarning(t *testing.T) {
	raw := "version: 99\n" + minimalTestConfigYAML
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o644))

	cfg, err := LoadConfig(path)
	require.NoError(t, err, "a newer-than-supported version should log a warning, not fail")
	assert.Equal(t, 99, cfg.Version, "an unsupported newer version should be left as declared, not overwritten")
}

// --- MigrateConfigFile (tg migrate) ---

func TestMigrateConfigFile_WritesUpgradedCopyAndPreservesSecrets(t *testing.T) {
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

	writtenPath, fromVersion, err := MigrateConfigFile(path, false)
	require.NoError(t, err)
	assert.Equal(t, legacyConfigVersion, fromVersion)
	assert.Equal(t, filepath.Join(dir, "config-v2.yaml"), writtenPath)

	// Original file must be untouched.
	originalAfter, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, raw, string(originalAfter), "the original config file must not be modified")

	// Migrated file must exist, contain the new version field, preserve the
	// comment, and — critically — must NOT contain the resolved secret.
	migrated, err := os.ReadFile(writtenPath)
	require.NoError(t, err)
	migratedStr := string(migrated)
	assert.Contains(t, migratedStr, "version: 2")
	assert.Contains(t, migratedStr, "# a comment that must survive migration")
	assert.Contains(t, migratedStr, "${TEST_VERSION_MIGRATION_SECRET}", "migrated file must keep the env var placeholder, not resolve it")
	assert.NotContains(t, migratedStr, "super-secret-value", "migrated file must never contain a resolved secret")

	// And the migrated file must now load successfully on its own.
	cfg, err := LoadConfig(writtenPath)
	require.NoError(t, err)
	assert.Equal(t, CurrentConfigVersion, cfg.Version)
}

func TestMigrateConfigFile_AlreadyCurrent_NoOp(t *testing.T) {
	raw := "version: 2\n" + minimalTestConfigYAML
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o644))

	writtenPath, fromVersion, err := MigrateConfigFile(path, false)
	require.NoError(t, err)
	assert.Equal(t, "", writtenPath, "nothing should be written when the file is already current")
	assert.Equal(t, CurrentConfigVersion, fromVersion)

	_, statErr := os.Stat(filepath.Join(dir, "config-v2.yaml"))
	assert.True(t, os.IsNotExist(statErr))
}

func TestMigrateConfigFile_ExistingTargetWithoutForce_Errors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(minimalTestConfigYAML), 0o644))

	migratedPath := filepath.Join(dir, "config-v2.yaml")
	sentinel := "version: 2\n# hand-edited, do not clobber\nname: Hand Edited\n"
	require.NoError(t, os.WriteFile(migratedPath, []byte(sentinel), 0o644))

	writtenPath, _, err := MigrateConfigFile(path, false)
	require.Error(t, err, "must refuse to overwrite an existing migrated file without --force")
	assert.Contains(t, err.Error(), "--force")
	assert.Equal(t, "", writtenPath)

	after, err := os.ReadFile(migratedPath)
	require.NoError(t, err)
	assert.Equal(t, sentinel, string(after), "an existing migrated file must not be overwritten without --force")
}

func TestMigrateConfigFile_ExistingTargetWithForce_Overwrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(minimalTestConfigYAML), 0o644))

	migratedPath := filepath.Join(dir, "config-v2.yaml")
	require.NoError(t, os.WriteFile(migratedPath, []byte("version: 2\nname: Stale\n"), 0o644))

	writtenPath, fromVersion, err := MigrateConfigFile(path, true)
	require.NoError(t, err)
	assert.Equal(t, migratedPath, writtenPath)
	assert.Equal(t, legacyConfigVersion, fromVersion)

	after, err := os.ReadFile(migratedPath)
	require.NoError(t, err)
	assert.Contains(t, string(after), "Test Gateway", "--force must overwrite the stale migrated file")
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
