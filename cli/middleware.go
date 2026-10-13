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

	chain, err := builtin.ResolveGlobalChain(cfg)
	if err != nil {
		fatalf("%v", err)
	}

	fmt.Printf("Middleware chain for %q:\n\n", cfg.Name)
	names := chain.Names()
	if len(names) == 0 {
		fmt.Println("  (none: the config has no enabled entries under middleware:)")
	}
	for i, name := range names {
		fmt.Printf("  %d. %s\n", i+1, name)
	}
	fmt.Println()

	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tSTATUS\tDEPENDENCIES\tDESCRIPTION")
	for _, st := range chain.Status() {
		dependsOn := "-"
		if len(st.Dependencies) > 0 {
			dependsOn = strings.Join(st.Dependencies, ", ")
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", st.Name, st.Status, dependsOn, st.Description)
	}
	tw.Flush()
}
