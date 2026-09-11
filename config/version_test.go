package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v3"
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

// --- flattenNotificationEmailSMTP ---

func TestFlattenNotificationEmailSMTP_MovesFieldsUpAndDropsSMTPKey(t *testing.T) {
	raw := []byte(`name: Test
notification:
  email:
    enabled: true
    smtp:
      host: smtp.example.com
      port: 587
      username: ${SMTP_USERNAME}
      password: ${SMTP_PASSWORD}
      from: noreply@example.com
      fromName: Example
`)
	got := flattenNotificationEmailSMTP(raw)

	var parsed struct {
		Notification struct {
			Email struct {
				Enabled  bool           `yaml:"enabled"`
				Host     string         `yaml:"host"`
				Port     int            `yaml:"port"`
				Username string         `yaml:"username"`
				Password string         `yaml:"password"`
				From     string         `yaml:"from"`
				FromName string         `yaml:"fromName"`
				SMTP     map[string]any `yaml:"smtp"`
			} `yaml:"email"`
		} `yaml:"notification"`
	}
	require.NoError(t, yaml.Unmarshal(got, &parsed))

	assert.True(t, parsed.Notification.Email.Enabled)
	assert.Equal(t, "smtp.example.com", parsed.Notification.Email.Host)
	assert.Equal(t, 587, parsed.Notification.Email.Port)
	assert.Equal(t, "${SMTP_USERNAME}", parsed.Notification.Email.Username, "env var placeholders must survive unresolved")
	assert.Equal(t, "${SMTP_PASSWORD}", parsed.Notification.Email.Password)
	assert.Equal(t, "noreply@example.com", parsed.Notification.Email.From)
	assert.Equal(t, "Example", parsed.Notification.Email.FromName)
	assert.Nil(t, parsed.Notification.Email.SMTP, "the smtp: key itself must be gone, not just emptied")
}

func TestFlattenNotificationEmailSMTP_NoOpWhenNoSMTPBlock(t *testing.T) {
	raw := []byte("name: Test\nnotification:\n  email:\n    enabled: false\n")
	got := flattenNotificationEmailSMTP(raw)
	assert.Equal(t, string(raw), string(got))
}

func TestFlattenNotificationEmailSMTP_NoOpWhenNoNotificationSection(t *testing.T) {
	raw := []byte("name: Test\nserver:\n  port: 8080\n")
	got := flattenNotificationEmailSMTP(raw)
	assert.Equal(t, string(raw), string(got))
}

func TestFlattenNotificationEmailSMTP_NoOpOnMalformedYAML(t *testing.T) {
	raw := []byte("not: valid: yaml: at: all:\n\t- broken")
	got := flattenNotificationEmailSMTP(raw)
	assert.Equal(t, string(raw), string(got), "malformed input must pass through untouched, not be mangled or panic")
}

// --- migrateLegacyToV1 ---

func TestMigrateLegacyToV1_FlattensSMTPAndStampsVersion(t *testing.T) {
	raw := []byte(`name: Test
notification:
  email:
    enabled: true
    smtp:
      host: smtp.example.com
      port: 587
`)
	got := migrateLegacyToV1(raw)
	gotStr := string(got)
	assert.Contains(t, gotStr, "version: 1")
	assert.NotContains(t, gotStr, "smtp:")
	assert.Contains(t, gotStr, "host: smtp.example.com")
}

func TestMigrateLegacyToV1_StampsVersionEvenWithoutSMTPBlock(t *testing.T) {
	raw := []byte("name: Test\nserver:\n  port: 8080\n")
	got := migrateLegacyToV1(raw)
	assert.Contains(t, string(got), "version: 1")
}

// --- migrateConfigToCurrent ---
//
// toVersion is a parameter specifically so these can exercise the
// step-through-multiple-versions logic with synthetic version numbers,
// independent of configMigrations' real (one entry today, keyed
// legacyConfigVersion — see its doc comment) content.

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

// TestMigrateConfigToCurrent_RealLegacyStep is the production case, not a
// synthetic one: migrating from legacyConfigVersion (0) to
// CurrentConfigVersion (1) today runs the one real registered step,
// migrateLegacyToV1.
func TestMigrateConfigToCurrent_RealLegacyStep(t *testing.T) {
	raw := []byte("name: Test\n")
	got := migrateConfigToCurrent(raw, legacyConfigVersion, CurrentConfigVersion)
	assert.Contains(t, string(got), "version: 1")
}

// --- LoadConfig integration ---

// TestLoadConfig_AbsentVersionFile_Fails is the counterpart of what used to
// be TestLoadConfig_AbsentVersionFile_Succeeds: an absent version: field is
// now treated as legacyConfigVersion, strictly below CurrentConfigVersion,
// so LoadConfig refuses to run against it — same as any other outdated
// declared version — rather than silently accepting a config that might
// still have the old notification.email.smtp.* shape. See
// checkConfigVersion's doc comment for why.
func TestLoadConfig_AbsentVersionFile_Fails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(minimalTestConfigYAML), 0o644))

	_, err := LoadConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no declared version")
	assert.Contains(t, err.Error(), "tg migrate")
}

func TestLoadConfig_CurrentVersionFile_Succeeds(t *testing.T) {
	raw := fmt.Sprintf("version: %d\n", CurrentConfigVersion) + minimalTestConfigYAML
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o644))

	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	require.NotNil(t, cfg.Version)
	assert.Equal(t, CurrentConfigVersion, *cfg.Version)
}

func TestLoadConfig_NewerVersionThanSupported_ProceedsWithWarning(t *testing.T) {
	raw := "version: 99\n" + minimalTestConfigYAML
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o644))

	cfg, err := LoadConfig(path)
	require.NoError(t, err, "a newer-than-supported version should log a warning, not fail")
	require.NotNil(t, cfg.Version)
	assert.Equal(t, 99, *cfg.Version, "an unsupported newer version should be left as declared, not overwritten")
}

// TestLoadConfig_NeverWritesFiles is the direct regression test for
// "migration only ever runs via `tg migrate`, never as a side effect of
// LoadConfig" — regardless of whether LoadConfig accepts or refuses a given
// file, the config file's directory must contain exactly the one file this
// test put there, both before and after the call. Complements the
// source-level guarantee (checkConfigVersion, LoadConfig's only version
// handling, never calls migrateConfigToCurrent/MigrateConfigContent — only
// MigrateConfigContent does, and its only caller in the whole module is
// main.go's migrateConfigFile, wired exclusively to the `migrate` Cobra
// command) with something a future regression would actually fail, not
// just something true by inspection today.
func TestLoadConfig_NeverWritesFiles(t *testing.T) {
	versionLines := map[string]string{
		"absent version: field":            "", // no version: line at all -> refused, but must still not write anything
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

// TestMigrateConfigContent_AbsentVersion_FlattensSMTPAndStampsVersion is the
// real-world case that motivated treating an absent version: field as
// legacyConfigVersion instead of "already current": a config with no
// version: field at all, but with the old notification.email.smtp.* shape,
// must actually come out the other side flattened and versioned — not
// printed back unchanged, which would leave email notifications silently
// misconfigured under the current schema (EmailNotificationConfig has no
// smtp field any more).
func TestMigrateConfigContent_AbsentVersion_FlattensSMTPAndStampsVersion(t *testing.T) {
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
notification:
  email:
    enabled: true
    smtp:
      host: smtp.example.com
      port: 587
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
	assert.Nil(t, fromVersion, "a file with no version: field must report fromVersion as nil, not 0")

	contentStr := string(content)
	assert.Contains(t, contentStr, "version: 1")
	assert.NotContains(t, contentStr, "smtp:", "the smtp: nesting must be flattened away")
	assert.Contains(t, contentStr, "host: smtp.example.com")
	assert.Contains(t, contentStr, "${TEST_VERSION_MIGRATION_SECRET}", "content must keep the env var placeholder, not resolve it")
	assert.NotContains(t, contentStr, "super-secret-value", "content must never contain a resolved secret")

	// Original file must be untouched.
	originalAfter, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, raw, string(originalAfter), "the original config file must not be modified")

	// And the migrated output must load successfully as current.
	migratedPath := filepath.Join(dir, "migrated.yaml")
	require.NoError(t, os.WriteFile(migratedPath, content, 0o644))
	cfg, err := LoadConfig(migratedPath)
	require.NoError(t, err)
	require.NotNil(t, cfg.Version)
	assert.Equal(t, CurrentConfigVersion, *cfg.Version)
	assert.True(t, cfg.Notification.Email.IsConfigured() || cfg.Notification.Email.Host == "smtp.example.com", "the flattened host must actually be readable as EmailNotificationConfig.Host now")
}

// TestMigrateConfigContent_AbsentVersion_NoOpFieldsPreserveComments covers a
// config with no version: field and nothing that actually needs
// restructuring (no notification.email.smtp block) — flattenNotificationEmailSMTP
// is a no-op for it, so only setTopLevelVersionField's line-level edit
// applies, which guarantees comments and secret placeholders survive
// byte-for-byte outside the one changed line.
func TestMigrateConfigContent_AbsentVersion_NoOpFieldsPreserveComments(t *testing.T) {
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
	assert.Nil(t, fromVersion)

	contentStr := string(content)
	assert.Equal(t, "version: 1\n"+raw, contentStr, "with nothing to restructure, only the version: line should be added")
	assert.Contains(t, contentStr, "# a comment that must survive")
	assert.Contains(t, contentStr, "${TEST_VERSION_MIGRATION_SECRET}")
	assert.NotContains(t, contentStr, "super-secret-value")
}

func TestMigrateConfigContent_AlreadyCurrent_ReturnsUnchanged(t *testing.T) {
	raw := fmt.Sprintf("version: %d\n", CurrentConfigVersion) + minimalTestConfigYAML
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o644))

	content, fromVersion, err := MigrateConfigContent(path)
	require.NoError(t, err)
	require.NotNil(t, fromVersion)
	assert.Equal(t, CurrentConfigVersion, *fromVersion)
	assert.Equal(t, raw, string(content), "an already-current file's content should be returned unchanged")
}

func TestMigrateConfigContent_NewerThanSupported_ReturnsUnchanged(t *testing.T) {
	raw := "version: 99\n" + minimalTestConfigYAML
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o644))

	content, fromVersion, err := MigrateConfigContent(path)
	require.NoError(t, err)
	require.NotNil(t, fromVersion)
	assert.Equal(t, 99, *fromVersion)
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
