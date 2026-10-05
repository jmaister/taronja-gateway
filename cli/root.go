// Package cli defines the tg command-line interface.
package cli

import (
	"embed"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// BuildInfo is the release metadata injected at link time by GoReleaser.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
	OS      string
	Arch    string
}

type app struct {
	build    BuildInfo
	webappFS *embed.FS
}

// Execute runs the tg command line and exits the process on error.
func Execute(build BuildInfo, webappFS *embed.FS) {
	if err := newRootCmd(&app{build: build, webappFS: webappFS}).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd(a *app) *cobra.Command {
	root := &cobra.Command{
		Use:   "tg",
		Short: "Taronja Gateway CLI",
		Long:  "A CLI for managing and running the Taronja API Gateway.",
	}
	root.AddCommand(
		newRunCmd(a),
		newAddUserCmd(),
		newVersionCmd(a),
		newFiltersCmd(),
		newMigrateCmd(),
		newValidateCmd(),
		newOpenAPICmd(),
	)
	return root
}

// addConfigFlag registers the required --config flag on cmd.
func addConfigFlag(cmd *cobra.Command, usage string) {
	cmd.Flags().String("config", "", usage)
	if err := cmd.MarkFlagRequired("config"); err != nil {
		panic(fmt.Sprintf("cli: mark --config required on %q: %v", cmd.Name(), err))
	}
}

// configPath returns the value of cmd's --config flag, exiting if it can't be read.
func configPath(cmd *cobra.Command) string {
	path, err := cmd.Flags().GetString("config")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting config flag: %v\n", err)
		os.Exit(1)
	}
	return path
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "FATAL: "+format+"\n", args...)
	os.Exit(1)
}
