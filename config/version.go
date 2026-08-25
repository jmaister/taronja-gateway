package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// CurrentConfigVersion is the config schema version this build of the
// gateway expects a config file to declare via its top-level `version:`
// field. Bump it, and add a corresponding entry to configMigrations,
// whenever a config schema change should be reflected in the version a
// config file declares.
const CurrentConfigVersion = 2

// legacyConfigVersion is the implicit version of every config file written
// before the `version:` field existed — i.e. every config file that
// predates this feature. LoadConfig treats an absent (zero-value) Version
// as this, not as an error: the field is new, not required.
const legacyConfigVersion = 1

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
// migrateV1ToV2 is the only step today, since the version field itself is
// the only schema change so far. A future migration that needs to
// restructure the document — not just add or update a field — will need a
// different strategy than the line-level text edit setTopLevelVersionField
// uses (e.g. parsing into yaml.Node to edit the AST while still preserving
// comments), since re-marshaling a GatewayConfig struct would drop every
// comment in the file.
var configMigrations = map[int]configMigration{
	1: migrateV1ToV2,
}

// migrateV1ToV2 stamps an explicit `version: 2` field onto a config file
// that predates the version field entirely, or that (hypothetically, since
// nothing ever wrote one) still explicitly declares `version: 1`.
func migrateV1ToV2(raw []byte) []byte {
	return setTopLevelVersionField(raw, 2)
}

// migrateConfigToCurrent steps raw config bytes forward from fromVersion to
// CurrentConfigVersion by applying each intervening version's migration in
// turn. Returns raw unchanged if fromVersion is already current (or newer).
func migrateConfigToCurrent(raw []byte, fromVersion int) []byte {
	for v := fromVersion; v < CurrentConfigVersion; v++ {
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

// applyConfigVersioning determines the config file's effective version (an
// absent/zero Version is treated as legacyConfigVersion), always logs it,
// and — if it predates CurrentConfigVersion — migrates it and writes the
// result to a sibling "-vN" file (versionedConfigPath) without touching the
// original. cfg.Version is set to CurrentConfigVersion in this case too, so
// the rest of the running gateway always behaves per the current schema
// regardless of which file it was actually loaded from.
//
// raw must be the config file's original bytes, from before ${VAR}
// expansion — a migrated file must never have resolved secrets baked into
// it (see LoadConfig, which expands env vars into a separate copy before
// unmarshaling for exactly this reason).
//
// This never fails LoadConfig: a migrated file that can't be written (e.g.
// a read-only config directory) is logged as a warning, not an error, since
// the running gateway doesn't depend on that file existing — it already has
// what it needs in cfg.
func applyConfigVersioning(configPath string, raw []byte, cfg *GatewayConfig) {
	fileVersion := cfg.Version
	if fileVersion == 0 {
		fileVersion = legacyConfigVersion
	}
	log.Printf("Config file version: %d (current: %d)", fileVersion, CurrentConfigVersion)

	switch {
	case fileVersion > CurrentConfigVersion:
		log.Printf("Warning: config file '%s' declares version %d, newer than this gateway version supports (%d). Proceeding, but some settings may not be recognized.",
			configPath, fileVersion, CurrentConfigVersion)
		return
	case fileVersion == CurrentConfigVersion:
		return
	}

	// fileVersion < CurrentConfigVersion: the running gateway still behaves
	// per the current schema from here on, regardless of what's on disk.
	cfg.Version = CurrentConfigVersion

	targetPath := versionedConfigPath(configPath, CurrentConfigVersion)
	if _, err := os.Stat(targetPath); err == nil {
		log.Printf("Config file '%s' is version %d (current: %d). A migrated copy already exists at '%s' — leaving it as-is. Consider switching to it.",
			configPath, fileVersion, CurrentConfigVersion, targetPath)
		return
	}

	migrated := migrateConfigToCurrent(raw, fileVersion)
	if err := os.WriteFile(targetPath, migrated, 0o644); err != nil {
		log.Printf("Warning: config file '%s' is version %d (current: %d), but failed to write a migrated copy to '%s': %v. Continuing with the loaded config as-is; consider adding `version: %d` to '%s' manually.",
			configPath, fileVersion, CurrentConfigVersion, targetPath, err, CurrentConfigVersion, configPath)
		return
	}
	log.Printf("Config file '%s' is version %d (current: %d). Wrote a migrated copy to '%s' — consider switching to it; the original was left unchanged.",
		configPath, fileVersion, CurrentConfigVersion, targetPath)
}

// versionedConfigPath returns the path a migrated config should be written
// to: the same directory and extension as originalPath, with the target
// version's suffix appended to the base filename — e.g. "config.yaml"
// becomes "config-v2.yaml" for version 2, "/etc/gateway/prod.yml" becomes
// "/etc/gateway/prod-v2.yml".
func versionedConfigPath(originalPath string, version int) string {
	dir := filepath.Dir(originalPath)
	ext := filepath.Ext(originalPath)
	base := strings.TrimSuffix(filepath.Base(originalPath), ext)
	return filepath.Join(dir, fmt.Sprintf("%s-v%d%s", base, version, ext))
}
