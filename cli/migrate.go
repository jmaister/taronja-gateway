package cli

import (
	"fmt"
	"os"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/spf13/cobra"
)

func newMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Print a config file migrated to the version this gateway requires",
		Long: `Prints a config file migrated to the version this gateway requires (unchanged
if already current) on stdout. It never writes a file; redirect to save:

    tg migrate --config config.yaml > config-v2.yaml

"tg run" refuses an outdated config so the upgrade is a deliberate step.`,
		Run: func(cmd *cobra.Command, args []string) {
			migrateConfigFile(configPath(cmd))
		},
	}
	addConfigFlag(cmd, "Path to the configuration file to migrate")
	return cmd
}

// migrateConfigFile keeps stdout a clean config document; notes go to stderr.
func migrateConfigFile(configFilePath string) {
	content, fromVersion, err := config.MigrateConfigContent(configFilePath)
	if err != nil {
		fatalf("%v", err)
	}

	// A parse failure here falls through to the "migrated" note; it is informational only.
	alreadyCurrent := fromVersion != nil
	if alreadyCurrent {
		if cmp, err := config.CompareConfigVersions(*fromVersion, config.CurrentConfigVersion); err != nil || cmp < 0 {
			alreadyCurrent = false
		}
	}
	switch {
	case alreadyCurrent:
		fmt.Fprintf(os.Stderr, "Note: '%s' is already version %s (current: %s) — printing it unchanged.\n",
			configFilePath, *fromVersion, config.CurrentConfigVersion)
	case fromVersion == nil:
		fmt.Fprintf(os.Stderr, "'%s' had no declared version (treated as pre-v1.0.0) — migrated to version %s.\n",
			configFilePath, config.CurrentConfigVersion)
	default:
		fmt.Fprintf(os.Stderr, "'%s' migrated from version %s to %s.\n",
			configFilePath, *fromVersion, config.CurrentConfigVersion)
	}

	os.Stdout.Write(content)
}
