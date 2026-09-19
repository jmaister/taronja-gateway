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

// --- parseConfigVersion / compare ---

func TestParseConfigVersion_ParsesMajorMinor(t *testing.T) {
	v, err := parseConfigVersion("1.2")
	require.NoError(t, err)
	assert.Equal(t, configVersion{major: 1, minor: 2}, v)
}

// TestParseConfigVersion_BareMajorMeansDotZero covers backward compatibility
// with every config migrated under this project's original single-integer
// scheme: those already declare e.g. "version: 1", not "version: 1.0", and
// must keep comparing exactly equal to "1.0" — not "older", which would
// wrongly demand a re-migration nothing actually changed for.
func TestParseConfigVersion_BareMajorMeansDotZero(t *testing.T) {
	v, err := parseConfigVersion("1")
	require.NoError(t, err)
	assert.Equal(t, configVersion{major: 1, minor: 0}, v)
}

func TestParseConfigVersion_RejectsThreeComponents(t *testing.T) {
	_, err := parseConfigVersion("1.0.0")
	assert.Error(t, err)
}

func TestParseConfigVersion_RejectsNonNumeric(t *testing.T) {
	_, err := parseConfigVersion("abc")
	assert.Error(t, err)

	_, err = parseConfigVersion("1.abc")
	assert.Error(t, err)
}

func TestParseConfigVersion_RejectsNegative(t *testing.T) {
	_, err := parseConfigVersion("-1")
	assert.Error(t, err)
}

func TestConfigVersion_Compare(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.0", "1.0", 0},
		{"1", "1.0", 0}, // bare major == MAJOR.0
		{"0.9", "1.0", -1},
		{"1.0", "0.9", 1},
		{"1.1", "1.2", -1},
		{"2.0", "1.9", 1}, // major dominates minor
		{"9.0", "10.0", -1},
		{"10.0", "9.0", 1}, // must not sort lexicographically ("10.0" < "9.0" as strings, but not as versions)
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s vs %s", tt.a, tt.b), func(t *testing.T) {
			a, err := parseConfigVersion(tt.a)
			require.NoError(t, err)
			b, err := parseConfigVersion(tt.b)
			require.NoError(t, err)
			assert.Equal(t, tt.want, a.compare(b))
		})
	}
}

func TestCompareConfigVersions(t *testing.T) {
	cmp, err := CompareConfigVersions("1.0", "2.0")
	require.NoError(t, err)
	assert.Equal(t, -1, cmp)

	_, err = CompareConfigVersions("not-a-version", "1.0")
	assert.Error(t, err)
}

// TestConfigVersionSequence_EndsAtCurrent guards the invariant
// configVersionSequence's own doc comment promises: its last entry must
// always equal CurrentConfigVersion, or migrateConfigToCurrent could never
// actually reach CurrentConfigVersion by walking the sequence.
func TestConfigVersionSequence_EndsAtCurrent(t *testing.T) {
	require.NotEmpty(t, configVersionSequence)
	last := configVersionSequence[len(configVersionSequence)-1]
	cmp, err := CompareConfigVersions(last, CurrentConfigVersion)
	require.NoError(t, err)
	assert.Equal(t, 0, cmp, "the last entry of configVersionSequence must equal CurrentConfigVersion")
}

// --- setTopLevelVersionField ---

func TestSetTopLevelVersionField_InsertsWhenAbsent(t *testing.T) {
	raw := []byte("name: Test\nserver:\n  port: 8080\n")
	got := setTopLevelVersionField(raw, "2.0")
	assert.Equal(t, "version: 2.0\nname: Test\nserver:\n  port: 8080\n", string(got))
}

func TestSetTopLevelVersionField_ReplacesWhenPresent(t *testing.T) {
	raw := []byte("version: 1.0\nname: Test\n")
	got := setTopLevelVersionField(raw, "2.0")
	assert.Equal(t, "version: 2.0\nname: Test\n", string(got))
}

func TestSetTopLevelVersionField_DoesNotMatchNestedVersionKey(t *testing.T) {
	// A "version:" key indented under some other section must not be treated
	// as the top-level field — only a literal, unindented "version:" at
	// column 0 counts.
	raw := []byte("name: Test\nsomeSection:\n  version: 99\n")
	got := setTopLevelVersionField(raw, "2.0")
	assert.Equal(t, "version: 2.0\nname: Test\nsomeSection:\n  version: 99\n", string(got))
}

func TestSetTopLevelVersionField_PreservesComments(t *testing.T) {
	raw := []byte("# a helpful comment\nname: Test\nserver:\n  port: 8080 # inline comment\n")
	got := setTopLevelVersionField(raw, "2.0")
	assert.Contains(t, string(got), "# a helpful comment")
	assert.Contains(t, string(got), "# inline comment")
}

// --- versionedConfigPath ---

func TestVersionedConfigPath(t *testing.T) {
	tests := []struct {
		name     string
		original string
		version  string
		expected string
	}{
		{"simple yaml", "config.yaml", "2.0", "config-v2.0.yaml"},
		{"yml extension", "/etc/gateway/prod.yml", "2.0", "/etc/gateway/prod-v2.0.yml"},
		{"relative path", "./sample/config.yaml", "2.0", "sample/config-v2.0.yaml"},
		{"no extension", "config", "2.0", "config-v2.0"},
		{"higher version", "config.yaml", "3.0", "config-v3.0.yaml"},
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

// TestFlattenNotificationEmailSMTP_Uses2SpaceIndent is the regression test
// for a real bug: yaml.Marshal's package-level function has no way to set
// the indent width and defaults to 4 spaces, so re-marshaling through it
// reformatted a migrated config's indentation from 2 spaces (what every
// config this project ships or generates uses) to 4 — every line the
// restructuring touched, not just the notification.email.smtp block
// itself. The fields round-tripping correctly (see
// TestFlattenNotificationEmailSMTP_MovesFieldsUpAndDropsSMTPKey) doesn't
// catch this, since unmarshaling the output is indent-width-agnostic —
// this checks the actual written bytes instead.
func TestFlattenNotificationEmailSMTP_Uses2SpaceIndent(t *testing.T) {
	raw := []byte(`name: Test
notification:
  email:
    enabled: true
    smtp:
      host: smtp.example.com
      port: 587
`)
	got := flattenNotificationEmailSMTP(raw)

	assert.Contains(t, string(got), "notification:\n  email:\n    enabled: true\n    host: smtp.example.com\n    port: 587\n",
		"nested keys must stay 2-space indented, not widen to yaml.v3's 4-space default")
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
	assert.Contains(t, gotStr, "version: 1.0")
	assert.NotContains(t, gotStr, "smtp:")
	assert.Contains(t, gotStr, "host: smtp.example.com")
}

func TestMigrateLegacyToV1_StampsVersionEvenWithoutSMTPBlock(t *testing.T) {
	raw := []byte("name: Test\nserver:\n  port: 8080\n")
	got := migrateLegacyToV1(raw)
	assert.Contains(t, string(got), "version: 1.0")
}

// --- migrateConfigToCurrent ---
//
// Both configMigrations and configVersionSequence are swapped out together
// so these can exercise the step-through-multiple-versions logic with
// synthetic version numbers, independent of the real (one real step,
// keyed legacyConfigVersion — see that map's doc comment) content.

func withSyntheticVersionSequence(t *testing.T, sequence []string, migrations map[string]configMigration) {
	t.Helper()
	originalSeq, originalMig := configVersionSequence, configMigrations
	t.Cleanup(func() {
		configVersionSequence = originalSeq
		configMigrations = originalMig
	})
	configVersionSequence = sequence
	configMigrations = migrations
}

func TestMigrateConfigToCurrent_AppliesOneRegisteredStep(t *testing.T) {
	withSyntheticVersionSequence(t, []string{"5.0", "6.0"}, map[string]configMigration{
		"5.0": func(raw []byte) []byte { return setTopLevelVersionField(raw, "6.0") },
	})

	got, err := migrateConfigToCurrent([]byte("name: Test\n"), "5.0", "6.0")
	require.NoError(t, err)
	assert.Equal(t, "version: 6.0\nname: Test\n", string(got))
}

func TestMigrateConfigToCurrent_ChainsMultipleRegisteredSteps(t *testing.T) {
	withSyntheticVersionSequence(t, []string{"5.0", "6.0", "7.0"}, map[string]configMigration{
		"5.0": func(raw []byte) []byte { return append(raw, []byte("step5-to-6\n")...) },
		"6.0": func(raw []byte) []byte { return append(raw, []byte("step6-to-7\n")...) },
	})

	got, err := migrateConfigToCurrent([]byte("name: Test\n"), "5.0", "7.0")
	require.NoError(t, err)
	assert.Equal(t, "name: Test\nstep5-to-6\nstep6-to-7\n", string(got), "each intervening version's migration must run in order")
}

// TestMigrateConfigToCurrent_UnregisteredStepJustStampsVersion covers the
// normal, expected case configMigrations' doc comment describes: a version
// bump with no actual content change (a new optional field, say) needs no
// migration function at all — the step must still advance the file's
// declared version, with nothing else touched.
func TestMigrateConfigToCurrent_UnregisteredStepJustStampsVersion(t *testing.T) {
	withSyntheticVersionSequence(t, []string{"5.0", "6.0"}, map[string]configMigration{}) // no entry for 5.0

	got, err := migrateConfigToCurrent([]byte("name: Test\n"), "5.0", "6.0")
	require.NoError(t, err)
	assert.Equal(t, "version: 6.0\nname: Test\n", string(got), "a step with no registered migration must still stamp the version forward")
}

func TestMigrateConfigToCurrent_NoOpWhenFromVersionAtOrPastToVersion(t *testing.T) {
	raw := []byte(fmt.Sprintf("version: %s\nname: Test\n", CurrentConfigVersion))
	got, err := migrateConfigToCurrent(raw, CurrentConfigVersion, CurrentConfigVersion)
	require.NoError(t, err)
	assert.Equal(t, string(raw), string(got))
}

// TestMigrateConfigToCurrent_RealLegacyStep is the production case, not a
// synthetic one: migrating from legacyConfigVersion (0.0) to
// CurrentConfigVersion (1.0) today runs the one real registered step,
// migrateLegacyToV1.
func TestMigrateConfigToCurrent_RealLegacyStep(t *testing.T) {
	raw := []byte("name: Test\n")
	got, err := migrateConfigToCurrent(raw, legacyConfigVersion, CurrentConfigVersion)
	require.NoError(t, err)
	assert.Contains(t, string(got), "version: 1.0")
}

func TestMigrateConfigToCurrent_UnrecognizedVersionErrors(t *testing.T) {
	_, err := migrateConfigToCurrent([]byte("name: Test\n"), "0.5", CurrentConfigVersion)
	require.Error(t, err, "a version between two known steps was never a real release and must be reported, not silently ignored")
	assert.Contains(t, err.Error(), "unrecognized config schema version")
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
	raw := fmt.Sprintf("version: %s\n", CurrentConfigVersion) + minimalTestConfigYAML
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o644))

	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	require.NotNil(t, cfg.Version)
	assert.Equal(t, CurrentConfigVersion, *cfg.Version)
}

// TestLoadConfig_BareMajorVersionFile_Succeeds covers backward
// compatibility with every config already migrated under this project's
// original single-integer scheme (declares "version: 1", not "version:
// 1.0") — it must still load as current, not be treated as outdated.
func TestLoadConfig_BareMajorVersionFile_Succeeds(t *testing.T) {
	raw := "version: 1\n" + minimalTestConfigYAML
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o644))

	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	require.NotNil(t, cfg.Version)
	assert.Equal(t, "1", *cfg.Version, "the declared value is preserved verbatim, comparison treats it as 1.0")
}

// TestLoadConfig_NewerVersionThanSupported_Fails covers the same "refuse
// to start" policy as the too-old direction: an older gateway binary has
// no way to know it actually honors every field a newer config relies on,
// so it must not proceed silently — this used to just log a warning and
// run anyway, which is exactly the failure mode a declared schema version
// exists to prevent.
func TestLoadConfig_NewerVersionThanSupported_Fails(t *testing.T) {
	raw := "version: 99.0\n" + minimalTestConfigYAML
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o644))

	_, err := LoadConfig(path)
	require.Error(t, err, "a newer-than-supported version must refuse to start, not silently proceed")
	assert.Contains(t, err.Error(), "declares version 99.0")
	assert.Contains(t, err.Error(), "Upgrade the gateway binary", "the remedy for this direction is upgrading the binary, not tg migrate")
}

func TestLoadConfig_InvalidVersionFormat_Fails(t *testing.T) {
	raw := "version: not-a-version\n" + minimalTestConfigYAML
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o644))

	_, err := LoadConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid version")
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
		"current, explicit version: field": fmt.Sprintf("version: %s\n", CurrentConfigVersion),
		"newer than supported":             "version: 99.0\n",
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
	assert.Nil(t, fromVersion, "a file with no version: field must report fromVersion as nil, not \"0.0\"")

	contentStr := string(content)
	assert.Contains(t, contentStr, "version: 1.0")
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
	assert.Equal(t, "version: 1.0\n"+raw, contentStr, "with nothing to restructure, only the version: line should be added")
	assert.Contains(t, contentStr, "# a comment that must survive")
	assert.Contains(t, contentStr, "${TEST_VERSION_MIGRATION_SECRET}")
	assert.NotContains(t, contentStr, "super-secret-value")
}

func TestMigrateConfigContent_AlreadyCurrent_ReturnsUnchanged(t *testing.T) {
	raw := fmt.Sprintf("version: %s\n", CurrentConfigVersion) + minimalTestConfigYAML
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o644))

	content, fromVersion, err := MigrateConfigContent(path)
	require.NoError(t, err)
	require.NotNil(t, fromVersion)
	assert.Equal(t, CurrentConfigVersion, *fromVersion)
	assert.Equal(t, raw, string(content), "an already-current file's content should be returned unchanged")
}

// TestMigrateConfigContent_BareMajorVersion_ReturnsUnchanged covers the
// same backward-compatibility case as TestLoadConfig_BareMajorVersionFile_Succeeds,
// from tg migrate's side: a config already declaring the old bare "1"
// (equivalent to "1.0") must be recognized as already current, not
// migrated again.
func TestMigrateConfigContent_BareMajorVersion_ReturnsUnchanged(t *testing.T) {
	raw := "version: 1\n" + minimalTestConfigYAML
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o644))

	content, fromVersion, err := MigrateConfigContent(path)
	require.NoError(t, err)
	require.NotNil(t, fromVersion)
	assert.Equal(t, "1", *fromVersion)
	assert.Equal(t, raw, string(content))
}

func TestMigrateConfigContent_NewerThanSupported_ReturnsUnchanged(t *testing.T) {
	raw := "version: 99.0\n" + minimalTestConfigYAML
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o644))

	content, fromVersion, err := MigrateConfigContent(path)
	require.NoError(t, err)
	require.NotNil(t, fromVersion)
	assert.Equal(t, "99.0", *fromVersion)
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

func TestMigrateConfigContent_InvalidVersionFormat_Errors(t *testing.T) {
	raw := "version: not-a-version\n" + minimalTestConfigYAML
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o644))

	_, _, err := MigrateConfigContent(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid version")
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
