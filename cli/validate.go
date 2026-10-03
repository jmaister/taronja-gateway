package cli

import (
	"fmt"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/middleware/builtin"
	"github.com/spf13/cobra"
)

func newValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate a config file without starting the gateway",
		Long: `Reports whether a config file is valid: it parses, declares a supported
version, has well-formed routes and admin settings, and resolves to a global
middleware chain with no unmet dependencies.

Starts no server and opens no database connection, so it is safe in CI.`,
		Run: func(cmd *cobra.Command, args []string) {
			validateConfigFile(configPath(cmd))
		},
	}
	addConfigFlag(cmd, "Path to the configuration file to validate")
	return cmd
}

func validateConfigFile(configFilePath string) {
	cfg, err := config.LoadConfig(configFilePath)
	if err != nil {
		fatalf("%v", err)
	}

	if err := builtin.ValidateConfigOnly(cfg); err != nil {
		fatalf("%v", err)
	}

	versionDesc := "no declared version"
	if cfg.Version != nil {
		versionDesc = fmt.Sprintf("version %s", *cfg.Version)
	}
	fmt.Printf("'%s' is valid (%s): %d route(s), management prefix %q.\n",
		configFilePath, versionDesc, len(cfg.Routes), cfg.Management.Prefix)
}
