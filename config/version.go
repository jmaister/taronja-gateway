package config

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	yaml "gopkg.in/yaml.v3"
)

// CurrentConfigVersion is the config schema version this build requires, as
// MAJOR.MINOR. When bumping it, append the new version to configVersionSequence
// and add a configMigrations entry only if content must be transformed.
const CurrentConfigVersion = "1.0"

// legacyConfigVersion is what an absent `version:` field means: any pre-v1.0.0
// config. It is older than current because some of those configs need real
// migration (notification.email.smtp.* was flattened; see migrateLegacyToV1).
const legacyConfigVersion = "0.0"

// configVersion is a parsed MAJOR/MINOR pair; raw strings must not be compared
// ("10.0" sorts before "9.0").
type configVersion struct {
	major, minor int
}

// parseConfigVersion parses "MAJOR" or "MAJOR.MINOR". A bare "1" equals "1.0"
// so configs from the old single-integer scheme still compare equal.
func parseConfigVersion(s string) (configVersion, error) {
	parts := strings.Split(s, ".")
	if len(parts) > 2 {
		return configVersion{}, fmt.Errorf("invalid config version %q: expected MAJOR or MAJOR.MINOR (e.g. %q), not a third component", s, CurrentConfigVersion)
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil || major < 0 {
		return configVersion{}, fmt.Errorf("invalid config version %q: %q is not a non-negative integer", s, parts[0])
	}
	minor := 0
	if len(parts) == 2 {
		minor, err = strconv.Atoi(parts[1])
		if err != nil || minor < 0 {
			return configVersion{}, fmt.Errorf("invalid config version %q: %q is not a non-negative integer", s, parts[1])
		}
	}
	return configVersion{major: major, minor: minor}, nil
}

// compare returns -1, 0, or 1 as v is less than, equal to, or greater than
// other.
func (v configVersion) compare(other configVersion) int {
	if v.major != other.major {
		if v.major < other.major {
			return -1
		}
		return 1
	}
	if v.minor != other.minor {
		if v.minor < other.minor {
			return -1
		}
		return 1
	}
	return 0
}

// mustParseConfigVersion parses s, panicking on failure — used only for
// version strings this project itself controls at compile time
// (CurrentConfigVersion, legacyConfigVersion, configVersionSequence's
// entries), never for anything read from a config file. A panic here means
// one of those constants is malformed, a bug to fix at the source, not a
// runtime condition any caller could sensibly recover from.
func mustParseConfigVersion(s string) configVersion {
	v, err := parseConfigVersion(s)
	if err != nil {
		panic(fmt.Sprintf("config: internal config version constant %v", err))
	}
	return v
}

// CompareConfigVersions returns -1, 0 or 1 as a is less than, equal to or greater
// than b, or an error if either fails to parse.
func CompareConfigVersions(a, b string) (int, error) {
	pa, err := parseConfigVersion(a)
	if err != nil {
		return 0, err
	}
	pb, err := parseConfigVersion(b)
	if err != nil {
		return 0, err
	}
	return pa.compare(pb), nil
}

// declaredOrLegacyVersion returns v dereferenced, or legacyConfigVersion if
// v is nil (no `version:` field in the file at all) — the one place that
// mapping is made, so every caller that needs to compare or migrate from a
// file's version agrees on what "undeclared" means instead of each
// special-casing nil separately.
func declaredOrLegacyVersion(v *string) string {
	if v == nil {
		return legacyConfigVersion
	}
	return *v
}

// configVersionSequence lists every config version in ascending order, starting
// with legacyConfigVersion. Its last entry must equal CurrentConfigVersion
// (enforced by a test).
var configVersionSequence = []string{legacyConfigVersion, CurrentConfigVersion}

// configMigration upgrades raw config bytes by one version. It works on bytes, not
// the parsed struct, to keep comments, ordering and ${VAR} placeholders.
type configMigration func(raw []byte) []byte

// configMigrations maps a version to the migration that upgrades it to the next.
// A step with no entry just gets the new version stamped on. Today only
// legacyConfigVersion has one (migrateLegacyToV1).
var configMigrations = map[string]configMigration{
	legacyConfigVersion: migrateLegacyToV1,
}

// migrateLegacyToV1 upgrades a pre-v1.0.0 config to 1.0: flattens
// notification.email.smtp.*, moves the middleware settings into the root `middleware`
// list, then stamps version: 1.0.
func migrateLegacyToV1(raw []byte) []byte {
	return setTopLevelVersionField(editYAML(raw, flattenNotificationEmailSMTPNode, moveMiddlewareToRoot), "1.0")
}

// flattenNotificationEmailSMTP moves notification.email.smtp.* up to
// notification.email.*; configs without the block pass through untouched.
func flattenNotificationEmailSMTP(raw []byte) []byte {
	return editYAML(raw, flattenNotificationEmailSMTPNode)
}

func flattenNotificationEmailSMTPNode(root *yaml.Node) bool {
	email := mappingValue(mappingValue(root, "notification"), "email")
	smtp := removeMappingKey(email, "smtp")
	if smtp == nil || smtp.Kind != yaml.MappingNode {
		return false
	}
	email.Content = append(email.Content, smtp.Content...)
	return true
}

// editYAML applies edits to raw's root mapping through yaml.Node, which keeps
// comments and ${VAR} placeholders. raw is returned byte-for-byte when no edit
// changes anything (or it does not parse); otherwise it is re-encoded with
// 2-space indentation, which may reformat quoting.
func editYAML(raw []byte, edits ...func(root *yaml.Node) bool) []byte {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil || len(doc.Content) == 0 {
		return raw
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return raw
	}
	changed := false
	for _, edit := range edits {
		if edit(root) {
			changed = true
		}
	}
	if !changed {
		return raw
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return raw
	}
	if err := enc.Close(); err != nil {
		return raw
	}
	return buf.Bytes()
}

// mappingValue returns the value node for key in mapping node m, or nil if
// m is nil, isn't a mapping, or has no such key.
func mappingValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// removeMappingKey deletes key (and its value) from mapping node m in
// place, returning the removed value node, or nil if m is nil, isn't a
// mapping, or has no such key.
func removeMappingKey(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			val := m.Content[i+1]
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
			return val
		}
	}
	return nil
}

// indexOfConfigVersion returns v's position in configVersionSequence,
// matched by parsed numeric value (not raw string equality — see
// parseConfigVersion for why "1" and "1.0" must match the same entry), or
// an error if v doesn't parse or isn't a version this build recognizes at
// all (something in between two known versions, which should never happen
// for a version this project itself ever wrote to a file).
func indexOfConfigVersion(v string) (int, error) {
	parsed, err := parseConfigVersion(v)
	if err != nil {
		return -1, err
	}
	for i, s := range configVersionSequence {
		if mustParseConfigVersion(s).compare(parsed) == 0 {
			return i, nil
		}
	}
	return -1, fmt.Errorf("unrecognized config schema version %q", v)
}

// migrateConfigToCurrent applies each step from fromVersion to toVersion, or
// returns raw unchanged if already there. It errors on an unknown version.
// toVersion is a parameter so tests can use a synthetic sequence.
func migrateConfigToCurrent(raw []byte, fromVersion, toVersion string) ([]byte, error) {
	fromIdx, err := indexOfConfigVersion(fromVersion)
	if err != nil {
		return nil, err
	}
	toIdx, err := indexOfConfigVersion(toVersion)
	if err != nil {
		return nil, err
	}
	for i := fromIdx; i < toIdx; i++ {
		stepVersion, nextVersion := configVersionSequence[i], configVersionSequence[i+1]
		if migrate, ok := configMigrations[stepVersion]; ok {
			raw = migrate(raw)
			continue
		}
		raw = setTopLevelVersionField(raw, nextVersion)
	}
	return raw, nil
}

// topLevelVersionLine matches a `version:` key at column 0 (i.e. a
// top-level GatewayConfig field, not a same-named key nested under some
// other section).
var topLevelVersionLine = regexp.MustCompile(`(?m)^version:.*$`)

// setTopLevelVersionField returns raw with its top-level `version:` field
// set to version — replacing the existing line if one is present, otherwise
// inserting a new line at the very top of the file. Every other line is
// left untouched, so comments and formatting elsewhere in the file survive
// intact.
func setTopLevelVersionField(raw []byte, version string) []byte {
	line := fmt.Sprintf("version: %s", version)
	if topLevelVersionLine.Match(raw) {
		return topLevelVersionLine.ReplaceAll(raw, []byte(line))
	}
	return append([]byte(line+"\n"), raw...)
}

// checkConfigVersion errors unless the declared version equals
// CurrentConfigVersion exactly. An absent version counts as legacy, not current,
// because pre-v1.0.0 configs may need migration. A newer version also refuses to
// start, since this binary cannot know it honors every field; the fix is
// upgrading the binary, not `tg migrate`.
func checkConfigVersion(configPath string, cfg *GatewayConfig) error {
	fileVersionStr := declaredOrLegacyVersion(cfg.Version)
	if cfg.Version == nil {
		log.Printf("Config file version: not declared, treated as pre-v1.0.0 (current: %s)", CurrentConfigVersion)
	} else {
		log.Printf("Config file version: %s (current: %s)", fileVersionStr, CurrentConfigVersion)
	}

	fileVersion, err := parseConfigVersion(fileVersionStr)
	if err != nil {
		return fmt.Errorf("config file '%s' has an invalid version: %w", configPath, err)
	}

	switch fileVersion.compare(mustParseConfigVersion(CurrentConfigVersion)) {
	case 1: // newer than this build supports
		return fmt.Errorf(
			"config file '%s' declares version %s, newer than this gateway version supports (%s)\n\n"+
				"This gateway binary predates that config schema version and can't guarantee it\n"+
				"honors every setting the file relies on. Upgrade the gateway binary to one that\n"+
				"supports config schema version %s or newer, then try again.",
			configPath, fileVersionStr, CurrentConfigVersion, fileVersionStr,
		)
	case -1: // older than required
		declared := fmt.Sprintf("is version %s", fileVersionStr)
		if cfg.Version == nil {
			declared = "has no declared version (treated as pre-v1.0.0)"
		}
		return fmt.Errorf(
			"config file '%s' %s, but this gateway requires version %s\n\n"+
				"Run this to upgrade it (it prints the migrated config; redirect it to a file):\n\n"+
				"    tg migrate --config %s > %s\n\n"+
				"Then point --config at the new file.",
			configPath, declared, CurrentConfigVersion, configPath, versionedConfigPath(configPath, CurrentConfigVersion),
		)
	}
	return nil
}

// MigrateConfigContent returns the file at path migrated to CurrentConfigVersion
// (unchanged if already current or newer) and never writes anything.
// fromVersion is the declared version, nil if the file has none.
func MigrateConfigContent(path string) (content []byte, fromVersion *string, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read config file '%s': %w", path, err)
	}

	// Only the version field is needed here; full parsing/validation happens
	// later, in LoadConfig, once the migrated content is actually loaded.
	var probe struct {
		Version *string `yaml:"version"`
	}
	if err := yaml.Unmarshal(raw, &probe); err != nil {
		return nil, nil, fmt.Errorf("failed to parse config file '%s': %w", path, err)
	}

	effectiveVersionStr := declaredOrLegacyVersion(probe.Version)
	effectiveVersion, err := parseConfigVersion(effectiveVersionStr)
	if err != nil {
		return nil, nil, fmt.Errorf("config file '%s' has an invalid version: %w", path, err)
	}
	if effectiveVersion.compare(mustParseConfigVersion(CurrentConfigVersion)) >= 0 {
		return raw, probe.Version, nil
	}

	migrated, err := migrateConfigToCurrent(raw, effectiveVersionStr, CurrentConfigVersion)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to migrate config file '%s': %w", path, err)
	}
	return migrated, probe.Version, nil
}

// versionedConfigPath suggests a filename for a migrated config, e.g.
// "config.yaml" becomes "config-v1.0.yaml". Used only in messages.
func versionedConfigPath(originalPath string, version string) string {
	dir := filepath.Dir(originalPath)
	ext := filepath.Ext(originalPath)
	base := strings.TrimSuffix(filepath.Base(originalPath), ext)
	return filepath.Join(dir, fmt.Sprintf("%s-v%s%s", base, version, ext))
}
