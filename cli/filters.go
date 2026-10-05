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

func newFiltersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "filters",
		Short: "Introspect the gateway's global filter chain",
	}

	list := &cobra.Command{
		Use:   "list",
		Short: "List the filters of a config file and every available filter",
		Long: `Prints the filter chain a config file resolves to, in order, then every
built-in filter with whether it is active and what it depends on.

Starts no server and opens no database connection.`,
		Run: func(cmd *cobra.Command, args []string) {
			listFilters(configPath(cmd))
		},
	}
	addConfigFlag(list, "Path to the configuration file")
	cmd.AddCommand(list)
	return cmd
}

// listFilters resolves and validates the chain without creating any filter, so
// it needs no runtime dependencies.
func listFilters(configFilePath string) {
	cfg, err := config.LoadConfig(configFilePath)
	if err != nil {
		fatalf("Failed to load configuration: %v", err)
	}

	registry := builtin.NewFilterRegistry(builtin.FilterDeps{})
	specs, err := builtin.ResolveFilters(registry, cfg.Filters)
	if err != nil {
		fatalf("%v", err)
	}
	if err := registry.ValidateSpecs(specs); err != nil {
		fatalf("filters: %v", err)
	}

	active := make(map[string]bool, len(specs))
	fmt.Printf("Filter chain for %q:\n\n", cfg.Name)
	if len(specs) == 0 {
		fmt.Println("  (none: the config has no enabled entries under filters:)")
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
