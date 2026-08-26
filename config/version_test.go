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

// --- effectiveConfigVersion ---

func TestEffectiveConfigVersion(t *testing.T) {
	assert.Equal(t, legacyConfigVersion, effectiveConfigVersion(0), "zero/absent must map to legacyConfigVersion")
	assert.Equal(t, 1, effectiveConfigVersion(1))
	assert.Equal(t, 2, effectiveConfigVersion(2))
	assert.Equal(t, 99, effectiveConfigVersion(99), "any explicit positive value must pass through unchanged")
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

// TestLoadConfig_NeverWritesFiles is the direct regression test for
// "migration only ever runs via `tg migrate`, never as a side effect of
// LoadConfig" — across every version relationship LoadConfig can see (older,
// current, newer), the config file's directory must contain exactly the one
// file this test put there, both before and after the call. Complements the
// source-level guarantee (checkConfigVersion, LoadConfig's only version
// handling, never calls migrateConfigToCurrent/migrateV1ToV2/
// MigrateConfigContent — only MigrateConfigContent does, and its only
// caller in the whole module is main.go's migrateConfigFile, wired
// exclusively to the `migrate` Cobra command) with something a future
// regression would actually fail, not just something true by inspection
// today.
func TestLoadConfig_NeverWritesFiles(t *testing.T) {
	versionLines := map[string]string{
		"older than current":   "", // no version: line at all -> legacyConfigVersion (1)
		"current":              "version: 2\n",
		"newer than supported": "version: 99\n",
	}

	for name, versionLine := range versionLines {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			require.NoError(t, os.WriteFile(path, []byte(versionLine+minimalTestConfigYAML), 0o644))

			before, err := os.ReadDir(dir)
			require.NoError(t, err)

			// Ignore the return values deliberately: whether LoadConfig
			// succeeds or fails (it fails for "older than current"), the one
			// thing under test here is that it never touches the filesystem
			// beyond reading the file it was given.
			_, _ = LoadConfig(path)

			after, err := os.ReadDir(dir)
			require.NoError(t, err)

			require.Len(t, after, len(before), "LoadConfig must not create, rename, or delete any file in the config's directory")
			assert.Equal(t, before[0].Name(), after[0].Name())
		})
	}
}

// --- MigrateConfigContent (tg migrate) ---
//
// MigrateConfigContent never writes anything — it just returns the migrated
// bytes for the caller (the CLI, printing to stdout) to do with as it
// pleases. There's no "already exists" case to guard here anymore: the user
// controls the destination via shell redirection, the same as any other
// stdout-producing command.

func TestMigrateConfigContent_ReturnsMigratedContentAndPreservesSecrets(t *testing.T) {
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

	content, fromVersion, err := MigrateConfigContent(path)
	require.NoError(t, err)
	assert.Equal(t, legacyConfigVersion, fromVersion)

	// Original file must be untouched.
	originalAfter, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, raw, string(originalAfter), "the original config file must not be modified")

	// Returned content must contain the new version field, preserve the
	// comment, and — critically — must NOT contain the resolved secret.
	contentStr := string(content)
	assert.Contains(t, contentStr, "version: 2")
	assert.Contains(t, contentStr, "# a comment that must survive migration")
	assert.Contains(t, contentStr, "${TEST_VERSION_MIGRATION_SECRET}", "migrated content must keep the env var placeholder, not resolve it")
	assert.NotContains(t, contentStr, "super-secret-value", "migrated content must never contain a resolved secret")

	// And writing it out must produce a file that loads successfully on its own.
	migratedPath := filepath.Join(dir, "config-v2.yaml")
	require.NoError(t, os.WriteFile(migratedPath, content, 0o644))
	cfg, err := LoadConfig(migratedPath)
	require.NoError(t, err)
	assert.Equal(t, CurrentConfigVersion, cfg.Version)
}

func TestMigrateConfigContent_AlreadyCurrent_ReturnsUnchanged(t *testing.T) {
	raw := "version: 2\n" + minimalTestConfigYAML
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o644))

	content, fromVersion, err := MigrateConfigContent(path)
	require.NoError(t, err)
	assert.Equal(t, CurrentConfigVersion, fromVersion)
	assert.Equal(t, raw, string(content), "an already-current file's content should be returned unchanged")
}

func TestMigrateConfigContent_NewerThanSupported_ReturnsUnchanged(t *testing.T) {
	raw := "version: 99\n" + minimalTestConfigYAML
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o644))

	content, fromVersion, err := MigrateConfigContent(path)
	require.NoError(t, err)
	assert.Equal(t, 99, fromVersion)
	assert.Equal(t, raw, string(content), "a newer-than-supported file's content should be returned unchanged, not downgraded")
}

func TestMigrateConfigContent_MissingFile_Errors(t *testing.T) {
	_, _, err := MigrateConfigContent(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read config file")
}

func TestMigrateConfigContent_InvalidYAML_Errors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	// Not valid YAML: a top-level scalar can't unmarshal into the version probe struct.
	require.NoError(t, os.WriteFile(path, []byte("not: valid: yaml: at: all:\n\t- broken"), 0o644))

	_, _, err := MigrateConfigContent(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse config file")
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
