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

// CurrentConfigVersion is the config schema version this build of the
// gateway expects a config file to declare via its top-level `version:`
// field, in MAJOR.MINOR format (see parseConfigVersion) — e.g. "1.0", not
// "1.0.0": a config file's schema doesn't need a third, patch-level
// component, since every actual change to it is either "the structure
// changed, bump MAJOR and add a migration" or "a field was added/removed
// with no migration needed, bump MINOR" — see configVersionSequence and
// configMigrations for how those two cases are told apart. Bump this, add
// the new version to the END of configVersionSequence, and add a
// corresponding entry to configMigrations if (and only if) the change
// needs real content transformed, not just the version line stamped
// forward.
//
// This is "1.0" — not "0.1" or "2.0" — as of the gateway's v1.0.0 release:
// the `version:` field itself was built and tested ahead of ever shipping,
// so no released config file ever declared an explicit version to migrate
// away from. That does NOT mean an undeclared version is already current,
// though — see legacyConfigVersion, which is what an absent field actually
// means.
const CurrentConfigVersion = "1.0"

// legacyConfigVersion is the version an absent `version:` field is treated
// as: every config file written before v1.0.0 (when the field was
// introduced), regardless of which pre-1.0.0 release wrote it. It's
// "0.0" — strictly below CurrentConfigVersion — not because a real
// "version 0.0" was ever declared anywhere, but because that's genuinely a
// different, older config shape with real content to migrate: v0.0.24 (the
// last tag before this field existed) shipped notification.email.smtp.* as
// a nested block, later flattened to notification.email.* directly (see
// migrateLegacyToV1) with nothing else changed. A config missing the
// version: field could be from any release before that flattening, so it's
// treated as needing that migration — running it again on a file that
// never had the nested smtp: block to begin with is a safe no-op (see
// flattenNotificationEmailSMTP). Deliberately its own named constant
// rather than a literal "0.0" inline: it must keep meaning exactly this
// even after a future schema change moves CurrentConfigVersion further.
const legacyConfigVersion = "0.0"

// configVersion is a config schema version broken into its MAJOR/MINOR
// components for numeric comparison — see parseConfigVersion for how a
// declared version string becomes one of these, and compare for how two
// are ordered. Comparing the raw declared strings directly (e.g. via
// strings.Compare) would be wrong the moment either component reaches two
// digits ("10.0" sorts before "9.0" lexicographically) — parsing first,
// like any other version scheme, avoids that.
type configVersion struct {
	major, minor int
}

// parseConfigVersion parses s as a config schema version: "MAJOR" or
// "MAJOR.MINOR", each component a non-negative integer. A bare "MAJOR"
// (no dot) is treated as "MAJOR.0" — accepted specifically because every
// config migrated under this project's original, pre-semver single-integer
// scheme already declares e.g. "version: 1", not "version: 1.0", and those
// files must keep comparing exactly equal to "1.0" without needing yet
// another migration just to add the ".0". A version with more than two
// components, a negative number, or a non-numeric component is rejected —
// this project's config versions are deliberately not full three-component
// semver (see CurrentConfigVersion's doc comment for why a patch component
// would never mean anything here).
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

// CompareConfigVersions compares two config schema version strings
// (MAJOR or MAJOR.MINOR, see parseConfigVersion), returning -1, 0, or 1 as
// a is less than, equal to, or greater than b, or an error if either
// fails to parse. Exported for main.go's `tg migrate` command, which needs
// to tell whether a file's declared version is already at or past
// CurrentConfigVersion without duplicating this package's version-parsing
// rules.
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

// configVersionSequence lists every config schema version that has ever
// existed, in ascending order: legacyConfigVersion (0.0, what an absent
// version: field means) first, then every version CurrentConfigVersion has
// ever been bumped to since, in the order it happened. migrateConfigToCurrent
// walks this list step by step from a file's version to CurrentConfigVersion
// — see that function and configMigrations for what happens at each step.
// The last entry must always equal CurrentConfigVersion; TestConfigVersionSequence_EndsAtCurrent
// enforces this so the two can never silently drift apart.
//
// Add the next real version to the end of this list whenever
// CurrentConfigVersion changes — whether or not that version actually
// needs an entry in configMigrations (see that map's doc comment for why
// not every step does).
var configVersionSequence = []string{legacyConfigVersion, CurrentConfigVersion}

// configMigration transforms the raw YAML bytes of a config file written for
// one version into the equivalent content for the next version up. It
// operates on raw bytes, not the parsed GatewayConfig struct, specifically
// so it can preserve everything a full unmarshal-then-marshal round trip
// would silently drop — comments, blank lines, key ordering, and
// environment-variable placeholders like ${VAR_NAME} (which must never be
// resolved into a file written back to disk).
type configMigration func(raw []byte) []byte

// configMigrations maps a version in configVersionSequence to the
// migration that upgrades a config written for that version to the next
// one in the sequence. Not every version in configVersionSequence needs an
// entry here — a version bump with no actual content change (a new
// optional field with a sensible zero-value default, say) doesn't need one
// at all: migrateConfigToCurrent stamps the new version straight onto the
// file with no other change whenever a step has no registered migration.
// This is the normal, expected case for most version bumps, not a
// fallback for a bug — real content migrations (a field renamed or moved,
// like legacyConfigVersion's below) are the exception, not the rule.
//
// legacyConfigVersion (0.0) is the only entry today, migrating every
// pre-v1.0.0 config up to version 1.0 — see migrateLegacyToV1.
var configMigrations = map[string]configMigration{
	legacyConfigVersion: migrateLegacyToV1,
}

// migrateLegacyToV1 upgrades a pre-v1.0.0 config (legacyConfigVersion) to
// version 1.0: flattens notification.email.smtp.* to notification.email.*
// (see flattenNotificationEmailSMTP — the one real content change between
// the two), then stamps the file with an explicit version: 1.0.
func migrateLegacyToV1(raw []byte) []byte {
	return setTopLevelVersionField(flattenNotificationEmailSMTP(raw), "1.0")
}

// flattenNotificationEmailSMTP rewrites a config's notification.email.smtp
// block, if present, into fields directly on notification.email — the
// shape EmailNotificationConfig has used since the generic notification
// system replaced the original, v0.0.24-era email-only implementation.
// Every field smtp.* named (host, port, username, password, from,
// fromName) is renamed straight to email.* with no other change; a config
// with no notification.email.smtp block at all (most of them — email
// notifications were opt-in even before v1.0.0) passes through this
// function untouched.
//
// Unlike setTopLevelVersionField's line-level text edit, this genuinely
// restructures the document, which needs parsing — done here via
// yaml.Node so comments, key order, and unresolved ${VAR_NAME} placeholders
// elsewhere in the file survive, the same guarantee configMigration's own
// doc comment promises. The cost: yaml.v3 may reformat quoting/indentation
// on re-marshal in ways a byte-for-byte diff would notice, even though the
// content is unchanged — an unavoidable tradeoff of actually moving keys
// around, not something a text-level edit like setTopLevelVersionField
// could do at all.
func flattenNotificationEmailSMTP(raw []byte) []byte {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil || len(doc.Content) == 0 {
		// Malformed or empty YAML: leave it exactly as-is. LoadConfig will
		// report a real parse error on it shortly after anyway — this isn't
		// the place to fail (or worse, half-mangle) it.
		return raw
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return raw
	}

	email := mappingValue(mappingValue(root, "notification"), "email")
	smtp := removeMappingKey(email, "smtp")
	if smtp == nil || smtp.Kind != yaml.MappingNode {
		return raw // no notification/email/smtp section at all — nothing to do
	}
	email.Content = append(email.Content, smtp.Content...)

	// yaml.Marshal's package-level function has no way to set the
	// indent width and defaults to 4 spaces — inconsistent with every
	// config file this project ships or generates (2 spaces, the
	// conventional YAML style). Going through an *Encoder with
	// SetIndent(2) instead keeps a migrated config's indentation
	// matching what it had before this function touched it.
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return raw // shouldn't happen; fail safe to the original bytes
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

// migrateConfigToCurrent steps raw config bytes forward from fromVersion to
// toVersion, one entry of configVersionSequence at a time, applying each
// step's registered migration (configMigrations) in turn — or, for a step
// with none registered, just stamping the next version onto the file with
// no other change (see configMigrations' doc comment for why that's the
// normal case, not a fallback). Returns raw unchanged if fromVersion is
// already at or past toVersion. Errors if either version isn't one this
// build recognizes (see indexOfConfigVersion).
//
// toVersion is a parameter rather than always CurrentConfigVersion so tests
// can exercise the step-through-multiple-versions logic with a synthetic
// configVersionSequence/configMigrations too, not just the one real step
// they hold today. MigrateConfigContent, the only production caller,
// always passes CurrentConfigVersion.
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

// checkConfigVersion logs cfg's declared config schema version — or that
// none was declared, in which case it's treated as legacyConfigVersion, not
// ignored — and returns an error if that doesn't exactly match
// CurrentConfigVersion, in either direction: the gateway must not run
// against a config file it can't be sure it fully understands. The error
// message tells the user what to do — run `tg migrate` for an older file,
// upgrade the gateway binary for a newer one — rather than leaving them to
// guess.
//
// An undeclared version is NOT accepted as "already fine": every config
// file written before v1.0.0 has no version field at all, and some of
// those really do need the one real migration that exists today (the
// notification.email.smtp.* flattening — see migrateLegacyToV1) to keep
// working correctly under this schema. Treating "no version field" the
// same as "already current" would mean that flattening never runs for
// exactly the files that need it, silently leaving email notifications
// misconfigured with no error anywhere. See GatewayConfig.Version's doc
// comment for why nil and an explicit "version: 1.0" are still tracked as
// distinct states even though both compare the same way here.
//
// A version *newer* than this build supports refuses to start too, not
// just a warning: an older binary has no way to know it actually honors
// every field a newer config relies on, so proceeding risks silently
// ignoring a setting the config author expected to take effect — exactly
// the failure mode a declared schema version exists to catch. There's no
// `tg migrate`-style fix for this direction, though — a migration only
// ever moves a config forward, so nothing can downgrade one — the only
// real remedy is upgrading the gateway binary itself to one that
// recognizes the file's version.
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

// MigrateConfigContent reads the config file at path and returns its content
// migrated up to CurrentConfigVersion (migrateConfigToCurrent) — unchanged
// only if it's already at CurrentConfigVersion or newer. It never writes
// anything: this is what `tg migrate` calls to produce the output it prints
// to stdout, leaving it up to the caller (a shell redirect, in the CLI's
// case) to decide whether and where to save it. See checkConfigVersion for
// why the gateway doesn't migrate a config file automatically or write one
// on its own anymore.
//
// fromVersion is the file's declared version exactly as read from it — nil
// if it has no `version:` field. That's still migrated (from
// legacyConfigVersion, internally), it's just reported to the caller as nil
// rather than "0.0", so a caller distinguishing "this file predates
// versioning entirely" from "this file explicitly declared some old
// version" (main.go's migrateConfigFile does, for its own message) can
// tell them apart.
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

// versionedConfigPath returns a conventional suggested filename for a
// migrated config, used only in messages (checkConfigVersion's error, the
// CLI's usage text) since `tg migrate` no longer writes a file itself: the
// same directory and extension as originalPath, with the target version's
// suffix appended to the base filename — e.g. "config.yaml" becomes
// "config-v1.0.yaml" for version "1.0", "/etc/gateway/prod.yml" becomes
// "/etc/gateway/prod-v1.0.yml". Nothing stops a user from redirecting `tg
// migrate`'s output to a different name.
func versionedConfigPath(originalPath string, version string) string {
	dir := filepath.Dir(originalPath)
	ext := filepath.Ext(originalPath)
	base := strings.TrimSuffix(filepath.Base(originalPath), ext)
	return filepath.Join(dir, fmt.Sprintf("%s-v%s%s", base, version, ext))
}
