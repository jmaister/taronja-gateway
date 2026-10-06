package cli

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/middleware/builtin"
	"github.com/spf13/cobra"
)

func newMiddlewareCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "middleware",
		Short: "Introspect the gateway's global middleware chain",
	}

	list := &cobra.Command{
		Use:   "list",
		Short: "List the middleware of a config file and every available middleware",
		Long: `Prints the middleware chain a config file resolves to, in order, then every
built-in middleware with whether it is active and what it depends on.

Starts no server and opens no database connection.`,
		Run: func(cmd *cobra.Command, args []string) {
			listMiddleware(configPath(cmd))
		},
	}
	addConfigFlag(list, "Path to the configuration file")
	cmd.AddCommand(list)
	return cmd
}

// listMiddleware resolves and validates the chain without creating any middleware, so
// it needs no runtime dependencies.
func listMiddleware(configFilePath string) {
	cfg, err := config.LoadConfig(configFilePath)
	if err != nil {
		fatalf("Failed to load configuration: %v", err)
	}

	registry := builtin.NewGlobalRegistry(builtin.GlobalDeps{})
	specs, err := builtin.ResolveGlobalMiddleware(registry, cfg.Middleware)
	if err != nil {
		fatalf("%v", err)
	}
	if err := registry.ValidateSpecs(specs); err != nil {
		fatalf("middleware: %v", err)
	}

	active := make(map[string]bool, len(specs))
	fmt.Printf("Middleware chain for %q:\n\n", cfg.Name)
	if len(specs) == 0 {
		fmt.Println("  (none: the config has no enabled entries under middleware:)")
	}
	for i, spec := range specs {
		active[spec.Name] = true
		fmt.Printf("  %d. %s\n", i+1, spec.Name)
	}
	fmt.Println()

	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tSTATUS\tDEPENDENCIES\tDESCRIPTION")
	for _, name := range registry.Names() {
		f, _ := registry.Factory(name)
		status := "available"
		if active[name] {
			status = "active"
		}
		dependsOn := "-"
		if deps := f.GetDependencies(); len(deps) > 0 {
			dependsOn = strings.Join(deps, ", ")
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", name, status, dependsOn, f.GetDescription())
	}
	tw.Flush()
}
