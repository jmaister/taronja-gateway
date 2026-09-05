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
// field and its migration machinery were built and tested ahead of ever
// shipping, so what was internally "version 2" during development never
// existed in a released config file. Renumbering it 1 for the first public
// release avoids implying there was ever a real, released "version 1"
// format publicly using this project name to migrate away from — there
// wasn't.
const CurrentConfigVersion = 1

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
// Empty today: CurrentConfigVersion is 1, and there is no version before 1
// to migrate from (see its doc comment) — this is genuinely the first
// released schema. Add the first real entry here (keyed 1, migrating to a
// new version 2) whenever the schema next changes. A migration that needs
// to restructure the document — not just add or update a field — will need
// a different strategy than the line-level text edit setTopLevelVersionField
// uses (e.g. parsing into yaml.Node to edit the AST while still preserving
// comments), since re-marshaling a GatewayConfig struct would drop every
// comment in the file.
var configMigrations = map[int]configMigration{}

// migrateConfigToCurrent steps raw config bytes forward from fromVersion to
// toVersion by applying each intervening version's migration in turn.
// Returns raw unchanged if fromVersion is already at or past toVersion.
// toVersion is a parameter rather than always CurrentConfigVersion so tests
// can exercise the actual step-through-multiple-versions logic with
// synthetic version numbers, independent of how many real migrations
// configMigrations currently holds (none, as of CurrentConfigVersion 1 —
// see its doc comment). MigrateConfigContent, the only production caller,
// always passes CurrentConfigVersion.
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
// none was declared — and returns an error if a declared version is older
// than CurrentConfigVersion: the gateway must not run against an outdated
// config file (see doc/refactor01.md's config versioning section — this
// used to migrate the file automatically instead of refusing to start, but
// a silently-rewritten config someone might not notice was the wrong
// tradeoff). The error message tells the user to run `tg migrate` rather
// than leaving them to guess.
//
// A nil cfg.Version — no `version:` field in the file at all — is accepted
// without comparison, not treated as any particular version number: every
// config file written before v1.0.0 (the `version:` field's own first
// released version) has no version field, so treating its absence as an
// error would refuse to start against every existing user's config. See
// GatewayConfig.Version's doc comment for why this is a distinct state from
// an explicit "version: 1", not the same thing spelled two ways.
//
// A version *newer* than this build supports is logged as a warning, not an
// error — there's no way to downgrade a config, and refusing to start over a
// merely-unrecognized newer field would be more disruptive than useful.
func checkConfigVersion(configPath string, cfg *GatewayConfig) error {
	if cfg.Version == nil {
		log.Printf("Config file version: not declared (current: %d)", CurrentConfigVersion)
		return nil
	}

	fileVersion := *cfg.Version
	log.Printf("Config file version: %d (current: %d)", fileVersion, CurrentConfigVersion)

	if fileVersion > CurrentConfigVersion {
		log.Printf("Warning: config file '%s' declares version %d, newer than this gateway version supports (%d). Proceeding, but some settings may not be recognized.",
			configPath, fileVersion, CurrentConfigVersion)
		return nil
	}
	if fileVersion < CurrentConfigVersion {
		return fmt.Errorf(
			"config file '%s' is version %d, but this gateway requires version %d\n\n"+
				"Run this to upgrade it (it prints the migrated config; redirect it to a file):\n\n"+
				"    tg migrate --config %s > %s\n\n"+
				"Then point --config at the new file.",
			configPath, fileVersion, CurrentConfigVersion, configPath, versionedConfigPath(configPath, CurrentConfigVersion),
		)
	}
	return nil
}

// MigrateConfigContent reads the config file at path and returns its content
// migrated up to CurrentConfigVersion (migrateConfigToCurrent) — or
// unchanged, if it declares no version at all, or is already at
// CurrentConfigVersion or newer. It never writes anything: this is what `tg
// migrate` calls to produce the output it prints to stdout, leaving it up
// to the caller (a shell redirect, in the CLI's case) to decide whether and
// where to save it. See checkConfigVersion for why the gateway doesn't
// migrate a config file automatically or write one on its own anymore.
//
// fromVersion is the file's declared version exactly as read from it — nil
// if it has no `version:` field, which is always treated the same as
// already-current: there's no version before CurrentConfigVersion (1) for
// an undeclared file to be migrated from. Useful for callers that want to
// report whether a migration actually happened.
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

	if probe.Version == nil || *probe.Version >= CurrentConfigVersion {
		return raw, probe.Version, nil
	}

	return migrateConfigToCurrent(raw, *probe.Version, CurrentConfigVersion), probe.Version, nil
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
