package config

import (
	"fmt"
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
//
// toVersion is a parameter specifically so these can exercise the
// step-through-multiple-versions logic with synthetic version numbers,
// independent of configMigrations' real (empty, as of CurrentConfigVersion
// 1 — see its doc comment) content.

func TestMigrateConfigToCurrent_AppliesOneRegisteredStep(t *testing.T) {
	original := configMigrations
	defer func() { configMigrations = original }()
	configMigrations = map[int]configMigration{
		5: func(raw []byte) []byte { return setTopLevelVersionField(raw, 6) },
	}

	got := migrateConfigToCurrent([]byte("name: Test\n"), 5, 6)
	assert.Equal(t, "version: 6\nname: Test\n", string(got))
}

func TestMigrateConfigToCurrent_ChainsMultipleRegisteredSteps(t *testing.T) {
	original := configMigrations
	defer func() { configMigrations = original }()
	configMigrations = map[int]configMigration{
		5: func(raw []byte) []byte { return append(raw, []byte("step5-to-6\n")...) },
		6: func(raw []byte) []byte { return append(raw, []byte("step6-to-7\n")...) },
	}

	got := migrateConfigToCurrent([]byte("name: Test\n"), 5, 7)
	assert.Equal(t, "name: Test\nstep5-to-6\nstep6-to-7\n", string(got), "each intervening version's migration must run in order")
}

func TestMigrateConfigToCurrent_MissingStepStampsVersionForwardAsSafetyNet(t *testing.T) {
	original := configMigrations
	defer func() { configMigrations = original }()
	configMigrations = map[int]configMigration{} // no entry for version 5

	got := migrateConfigToCurrent([]byte("name: Test\n"), 5, 6)
	assert.Equal(t, "version: 6\nname: Test\n", string(got), "a missing migration step must still stamp the version forward, not leave the file unchanged")
}

func TestMigrateConfigToCurrent_NoOpWhenFromVersionAtOrPastToVersion(t *testing.T) {
	raw := []byte(fmt.Sprintf("version: %d\nname: Test\n", CurrentConfigVersion))
	got := migrateConfigToCurrent(raw, CurrentConfigVersion, CurrentConfigVersion)
	assert.Equal(t, string(raw), string(got))
}

// TestMigrateConfigToCurrent_NoOpForRealCurrentVersion is the production
// case, not a synthetic one: CurrentConfigVersion is 1 today and
// configMigrations has no real entries (there is no version before 1 to
// migrate from — this is the first released schema), so migrating a
// version-1 file must be a true no-op.
func TestMigrateConfigToCurrent_NoOpForRealCurrentVersion(t *testing.T) {
	raw := []byte("name: Test\n")
	got := migrateConfigToCurrent(raw, legacyConfigVersion, CurrentConfigVersion)
	assert.Equal(t, string(raw), string(got))
}

// --- LoadConfig integration ---
//
// LoadConfig would refuse to run against a config file older than
// CurrentConfigVersion, rather than silently migrating it — see
// checkConfigVersion — but there's no real "older" file to test that with
// today (CurrentConfigVersion is 1, the first released schema version; see
// its doc comment). Migration itself is a deliberate, explicit step via
// `tg migrate` / config.MigrateConfigFile, tested separately below.

// TestLoadConfig_AbsentVersionFile_TreatedAsCurrent_Succeeds is the
// no-migration-needed counterpart of what used to be
// TestLoadConfig_LegacyFile_FailsWithMigrationAdvice: with
// CurrentConfigVersion at 1 (see its doc comment — this is the first
// released schema, so there's no real older version to refuse), a config
// file with no `version:` field at all is exactly as valid as one that
// declares `version: 1` explicitly — both are legacyConfigVersion, which is
// current.
func TestLoadConfig_AbsentVersionFile_TreatedAsCurrent_Succeeds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(minimalTestConfigYAML), 0o644))

	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	assert.Equal(t, 0, cfg.Version, "an absent version: field must be left as the zero value, not silently stamped")
}

func TestLoadConfig_CurrentVersionFile_Succeeds(t *testing.T) {
	raw := fmt.Sprintf("version: %d\n", CurrentConfigVersion) + minimalTestConfigYAML
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
// LoadConfig" — across every version relationship LoadConfig can see today
// (there's no "older than current" case any more: CurrentConfigVersion is 1,
// and an absent version: field is legacyConfigVersion, also 1 — see both
// constants' doc comments — so "current" now covers what used to be two
// separate cases), the config file's directory must contain exactly the one
// file this test put there, both before and after the call. Complements the
// source-level guarantee (checkConfigVersion, LoadConfig's only version
// handling, never calls migrateConfigToCurrent/MigrateConfigContent — only
// MigrateConfigContent does, and its only caller in the whole module is
// main.go's migrateConfigFile, wired exclusively to the `migrate` Cobra
// command) with something a future regression would actually fail, not just
// something true by inspection today.
func TestLoadConfig_NeverWritesFiles(t *testing.T) {
	versionLines := map[string]string{
		"current, absent version:  field":  "", // no version: line at all -> legacyConfigVersion (1), which is current
		"current, explicit version: field": fmt.Sprintf("version: %d\n", CurrentConfigVersion),
		"newer than supported":             "version: 99\n",
	}

	for name, versionLine := range versionLines {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			require.NoError(t, os.WriteFile(path, []byte(versionLine+minimalTestConfigYAML), 0o644))

			before, err := os.ReadDir(dir)
			require.NoError(t, err)

			// Ignore the return values deliberately: the one thing under test
			// here is that LoadConfig never touches the filesystem beyond
			// reading the file it was given, regardless of outcome.
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

// TestMigrateConfigContent_AbsentVersion_AlreadyCurrent_PreservesEverything
// covers what used to require a real migration to demonstrate: with
// CurrentConfigVersion at 1 (see its doc comment — nothing before 1 exists
// to migrate from), a config file with no `version:` field at all is
// legacyConfigVersion, which is current, so MigrateConfigContent returns it
// completely untouched — the strongest possible guarantee that comments and
// unresolved secret placeholders survive, since nothing rewrites the file
// at all.
func TestMigrateConfigContent_AbsentVersion_AlreadyCurrent_PreservesEverything(t *testing.T) {
	t.Setenv("TEST_VERSION_MIGRATION_SECRET", "super-secret-value")

	raw := `name: Test Gateway
server:
  host: 127.0.0.1
  port: 8080
  # a comment that must survive
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
	assert.Equal(t, raw, string(content), "an absent-version (already current) file must be returned byte-for-byte unchanged")

	contentStr := string(content)
	assert.Contains(t, contentStr, "# a comment that must survive")
	assert.Contains(t, contentStr, "${TEST_VERSION_MIGRATION_SECRET}", "content must keep the env var placeholder, not resolve it")
	assert.NotContains(t, contentStr, "super-secret-value", "content must never contain a resolved secret")

	// Original file must be untouched too.
	originalAfter, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, raw, string(originalAfter), "the original config file must not be modified")

	// And it must load successfully as-is.
	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	assert.Equal(t, 0, cfg.Version)
}

func TestMigrateConfigContent_AlreadyCurrent_ReturnsUnchanged(t *testing.T) {
	raw := fmt.Sprintf("version: %d\n", CurrentConfigVersion) + minimalTestConfigYAML
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
