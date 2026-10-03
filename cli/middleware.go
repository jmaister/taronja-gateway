package cli

import (
	"fmt"
	"os"
	"sort"
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
		Short: "List global middleware and their status for a config file",
		Long: `Prints every built-in global middleware for a config file: its position in
the resolved chain, whether it is active, its dependencies and its health.

Starts no server and opens no database connection.`,
		Run: func(cmd *cobra.Command, args []string) {
			listMiddleware(configPath(cmd))
		},
	}
	addConfigFlag(list, "Path to the configuration file")
	cmd.AddCommand(list)
	return cmd
}

// listMiddleware builds the chain with nil dependencies: introspection only
// needs each factory's metadata, never its runtime collaborators.
func listMiddleware(configFilePath string) {
	cfg, err := config.LoadConfig(configFilePath)
	if err != nil {
		fatalf("Failed to load configuration: %v", err)
	}

	specs, err := builtin.ResolveGlobalChainSpecs(cfg)
	if err != nil {
		fatalf("Failed to resolve middleware chain: %v", err)
	}

	registry, _, err := builtin.NewGlobalChain(cfg, nil, nil, nil, nil)
	if err != nil {
		fatalf("Failed to build middleware chain: %v", err)
	}

	fmt.Printf("Global middleware chain for %q:\n\n", cfg.Name)
	if len(specs) == 0 {
		fmt.Println("  (none active — no middleware: section and no management.analytics/logging/rateLimiter flags enabled)")
	} else {
		for i, spec := range specs {
			fmt.Printf("  %d. %s\n", i+1, spec.Name)
		}
	}
	fmt.Println()

	status := registry.GetStatus()
	names := make([]string, 0, len(status))
	for name := range status {
		names = append(names, name)
	}
	sort.Strings(names)

	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tSTATUS\tHEALTH\tDEPENDENCIES\tDESCRIPTION")
	for _, name := range names {
		s := status[name]
		dependsOn := "-"
		if len(s.Dependencies) > 0 {
			dependsOn = strings.Join(s.Dependencies, ", ")
		}
		health := "-"
		if s.Health != nil {
			health = s.Health.Status
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", s.Name, s.Status, health, dependsOn, s.Description)
	}
	tw.Flush()
}
