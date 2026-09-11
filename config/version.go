package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	yaml "gopkg.in/yaml.v3"
)

// CurrentConfigVersion is the config schema version this build of the
// gateway expects a config file to declare via its top-level `version:`
// field. Bump it, and add a corresponding entry to configMigrations,
// whenever a config schema change should be reflected in the version a
// config file declares.
//
// This is 1 — not 2 — as of the gateway's v1.0.0 release: the `version:`
// field itself was built and tested ahead of ever shipping, so no released
// config file ever declared an explicit "version: 1" to migrate away from.
// That does NOT mean an undeclared version is already current, though —
// see legacyConfigVersion, which is what an absent field actually means.
const CurrentConfigVersion = 1

// legacyConfigVersion is the version an absent `version:` field is treated
// as: every config file written before v1.0.0 (when the field was
// introduced), regardless of which pre-1.0.0 release wrote it. It's
// numbered 0 — strictly below CurrentConfigVersion — not because a real
// "version 0" was ever declared anywhere, but because that's genuinely a
// different, older config shape with real content to migrate: v0.0.24 (the
// last tag before this field existed) shipped notification.email.smtp.* as
// a nested block, later flattened to notification.email.* directly (see
// migrateLegacyToV1) with nothing else changed. A config missing the
// version: field could be from any release before that flattening, so it's
// treated as needing that migration — running it again on a file that
// never had the nested smtp: block to begin with is a safe no-op (see
// flattenNotificationEmailSMTP). Deliberately its own named constant
// rather than a literal 0 inline: it must keep meaning exactly this even
// after a future schema change moves CurrentConfigVersion past 1.
const legacyConfigVersion = 0

// declaredOrLegacyVersion returns v dereferenced, or legacyConfigVersion if
// v is nil (no `version:` field in the file at all) — the one place that
// mapping is made, so every caller that needs to compare or migrate from a
// file's version agrees on what "undeclared" means instead of each
// special-casing nil separately.
func declaredOrLegacyVersion(v *int) int {
	if v == nil {
		return legacyConfigVersion
	}
	return *v
}

// configMigration transforms the raw YAML bytes of a config file written for
// one version into the equivalent content for the next version up. It
// operates on raw bytes, not the parsed GatewayConfig struct, specifically
// so it can preserve everything a full unmarshal-then-marshal round trip
// would silently drop — comments, blank lines, key ordering, and
// environment-variable placeholders like ${VAR_NAME} (which must never be
// resolved into a file written back to disk).
type configMigration func(raw []byte) []byte

// configMigrations maps a version to the migration that upgrades a config
// written for that version to version+1. Every version below
// CurrentConfigVersion must have an entry here (migrateConfigToCurrent falls
// back to just stamping the version forward if one is ever missing, but
// that's a bug-safety net, not something to rely on).
//
// legacyConfigVersion (0) is the only entry today, migrating every
// pre-v1.0.0 config up to version 1 — see migrateLegacyToV1. Add the next
// real entry here (keyed 1, migrating to a new version 2) whenever the
// schema next changes.
var configMigrations = map[int]configMigration{
	legacyConfigVersion: migrateLegacyToV1,
}

// migrateLegacyToV1 upgrades a pre-v1.0.0 config (legacyConfigVersion) to
// version 1: flattens notification.email.smtp.* to notification.email.*
// (see flattenNotificationEmailSMTP — the one real content change between
// the two), then stamps the file with an explicit version: 1.
func migrateLegacyToV1(raw []byte) []byte {
	return setTopLevelVersionField(flattenNotificationEmailSMTP(raw), 1)
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

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return raw // shouldn't happen; fail safe to the original bytes
	}
	return out
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

// migrateConfigToCurrent steps raw config bytes forward from fromVersion to
// toVersion by applying each intervening version's migration in turn.
// Returns raw unchanged if fromVersion is already at or past toVersion.
// toVersion is a parameter rather than always CurrentConfigVersion so tests
// can exercise the step-through-multiple-versions logic with synthetic
// version numbers too, not just the one real step configMigrations holds
// today. MigrateConfigContent, the only production caller, always passes
// CurrentConfigVersion.
func migrateConfigToCurrent(raw []byte, fromVersion, toVersion int) []byte {
	for v := fromVersion; v < toVersion; v++ {
		migrate, ok := configMigrations[v]
		if !ok {
			// No migration registered for this step. This shouldn't happen if
			// configMigrations is kept in sync with CurrentConfigVersion, but
			// fail safe by at least stamping the version forward rather than
			// leaving the rest of the upgrade undone.
			raw = setTopLevelVersionField(raw, v+1)
			continue
		}
		raw = migrate(raw)
	}
	return raw
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
func setTopLevelVersionField(raw []byte, version int) []byte {
	line := fmt.Sprintf("version: %d", version)
	if topLevelVersionLine.Match(raw) {
		return topLevelVersionLine.ReplaceAll(raw, []byte(line))
	}
	return append([]byte(line+"\n"), raw...)
}

// checkConfigVersion logs cfg's declared config schema version — or that
// none was declared, in which case it's treated as legacyConfigVersion, not
// ignored — and returns an error if that's older than CurrentConfigVersion:
// the gateway must not run against an outdated config file (see
// doc/refactor01.md's config versioning section — this used to migrate the
// file automatically instead of refusing to start, but a silently-rewritten
// config someone might not notice was the wrong tradeoff). The error
// message tells the user to run `tg migrate` rather than leaving them to
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
// comment for why nil and an explicit "version: 1" are still tracked as
// distinct states even though both compare the same way here.
//
// A version *newer* than this build supports is logged as a warning, not an
// error — there's no way to downgrade a config, and refusing to start over a
// merely-unrecognized newer field would be more disruptive than useful.
func checkConfigVersion(configPath string, cfg *GatewayConfig) error {
	fileVersion := declaredOrLegacyVersion(cfg.Version)
	if cfg.Version == nil {
		log.Printf("Config file version: not declared, treated as pre-v1.0.0 (current: %d)", CurrentConfigVersion)
	} else {
		log.Printf("Config file version: %d (current: %d)", fileVersion, CurrentConfigVersion)
	}

	if fileVersion > CurrentConfigVersion {
		log.Printf("Warning: config file '%s' declares version %d, newer than this gateway version supports (%d). Proceeding, but some settings may not be recognized.",
			configPath, fileVersion, CurrentConfigVersion)
		return nil
	}
	if fileVersion < CurrentConfigVersion {
		declared := fmt.Sprintf("is version %d", fileVersion)
		if cfg.Version == nil {
			declared = "has no declared version (treated as pre-v1.0.0)"
		}
		return fmt.Errorf(
			"config file '%s' %s, but this gateway requires version %d\n\n"+
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
// rather than 0, so a caller distinguishing "this file predates versioning
// entirely" from "this file explicitly declared some old number" (main.go's
// migrateConfigFile does, for its own message) can tell them apart.
func MigrateConfigContent(path string) (content []byte, fromVersion *int, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read config file '%s': %w", path, err)
	}

	// Only the version field is needed here; full parsing/validation happens
	// later, in LoadConfig, once the migrated content is actually loaded.
	var probe struct {
		Version *int `yaml:"version"`
	}
	if err := yaml.Unmarshal(raw, &probe); err != nil {
		return nil, nil, fmt.Errorf("failed to parse config file '%s': %w", path, err)
	}

	effectiveVersion := declaredOrLegacyVersion(probe.Version)
	if effectiveVersion >= CurrentConfigVersion {
		return raw, probe.Version, nil
	}

	return migrateConfigToCurrent(raw, effectiveVersion, CurrentConfigVersion), probe.Version, nil
}

// versionedConfigPath returns a conventional suggested filename for a
// migrated config, used only in messages (checkConfigVersion's error, the
// CLI's usage text) since `tg migrate` no longer writes a file itself: the
// same directory and extension as originalPath, with the target version's
// suffix appended to the base filename — e.g. "config.yaml" becomes
// "config-v2.yaml" for version 2, "/etc/gateway/prod.yml" becomes
// "/etc/gateway/prod-v2.yml". Nothing stops a user from redirecting `tg
// migrate`'s output to a different name.
func versionedConfigPath(originalPath string, version int) string {
	dir := filepath.Dir(originalPath)
	ext := filepath.Ext(originalPath)
	base := strings.TrimSuffix(filepath.Base(originalPath), ext)
	return filepath.Join(dir, fmt.Sprintf("%s-v%d%s", base, version, ext))
}
