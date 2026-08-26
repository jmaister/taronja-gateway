package main

import (
	"embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/gateway"
	"github.com/jmaister/taronja-gateway/gateway/deps"
	"github.com/jmaister/taronja-gateway/middleware"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

//go:embed webapp/dist
var webappEmbedFS embed.FS

// Version information (injected by GoReleaser)
var (
	version   = "Dev"
	commit    = "none"
	date      = time.Now().Format(time.RFC3339)
	buildOS   = "unknown"
	buildArch = "unknown"
)

var rootCmd = &cobra.Command{
	Use:   "tg",
	Short: "Taronja Gateway CLI",
	Long:  `A CLI for managing and running the Taronja API Gateway.`,
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the Taronja API Gateway",
	Long:  `Starts the Taronja API Gateway using the specified configuration file.`,
	Run: func(cmd *cobra.Command, args []string) {
		configFilePath, err := cmd.Flags().GetString("config")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting config flag: %v\n", err)
			os.Exit(1)
		}
		runGateway(configFilePath)
	},
}

var addUserCmd = &cobra.Command{
	Use:   "adduser [username] [email] [password]",
	Short: "Create a new user in the DB",
	Long:  `Creates a new user in the database.`,
	Args:  cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		username := args[0]
		email := args[1]
		password := args[2]
		addUser(username, email, password)
	},
}

var middlewareCmd = &cobra.Command{
	Use:   "middleware",
	Short: "Introspect the gateway's global middleware chain",
	Long:  `Commands for inspecting the global middleware chain defined by a config file, without starting the gateway.`,
}

var middlewareListCmd = &cobra.Command{
	Use:   "list",
	Short: "List global middleware and their status for a config file",
	Long: `Loads a gateway config file and prints every built-in global middleware:
its position in the resolved chain, whether it's active or merely available,
its dependencies, and (where implemented) its health.

Does not start the HTTP server, open a database connection, or make any
network calls — safe to run against a real config file to check what a
change would do before deploying it.`,
	Run: func(cmd *cobra.Command, args []string) {
		configFilePath, err := cmd.Flags().GetString("config")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting config flag: %v\n", err)
			os.Exit(1)
		}
		listMiddleware(configFilePath)
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Display version information",
	Long:  `Shows the version, build date, commit hash, build OS, and architecture of the application.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Taronja Gateway v%s\n", version)
		fmt.Printf("  Commit: %s\n", commit)
		fmt.Printf("  Built: %s\n", date)
		fmt.Printf("  OS/Arch: %s/%s\n", buildOS, buildArch)
	},
}

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate a config file to the version this gateway requires",
	Long: `Reads a config file and, if it's older than the version this gateway
requires, migrates it and writes the result to a new file named with a
"-vN" suffix (e.g. config.yaml -> config-v2.yaml). The original file is
never modified.

The migration is applied one version step at a time (v1->v2, v2->v3, ...),
same as if you'd run this once per intervening version.

The gateway refuses to start ("run") against an outdated config file
specifically so this migration is a deliberate step you take (and can
review the result of) rather than something that happens silently on
every startup. Run this whenever "tg run" tells you to.`,
	Run: func(cmd *cobra.Command, args []string) {
		configFilePath, err := cmd.Flags().GetString("config")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting config flag: %v\n", err)
			os.Exit(1)
		}
		force, err := cmd.Flags().GetBool("force")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting force flag: %v\n", err)
			os.Exit(1)
		}
		migrateConfigFile(configFilePath, force)
	},
}

func init() {
	runCmd.Flags().String("config", "", "Path to the configuration file")
	if err := runCmd.MarkFlagRequired("config"); err != nil {
		log.Fatalf("Failed to mark 'config' flag as required for runCmd: %v", err)
	}

	middlewareListCmd.Flags().String("config", "", "Path to the configuration file")
	if err := middlewareListCmd.MarkFlagRequired("config"); err != nil {
		log.Fatalf("Failed to mark 'config' flag as required for middlewareListCmd: %v", err)
	}
	middlewareCmd.AddCommand(middlewareListCmd)

	migrateCmd.Flags().String("config", "", "Path to the configuration file to migrate")
	if err := migrateCmd.MarkFlagRequired("config"); err != nil {
		log.Fatalf("Failed to mark 'config' flag as required for migrateCmd: %v", err)
	}
	migrateCmd.Flags().Bool("force", false, "Overwrite the migrated file if it already exists")

	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(addUserCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(middlewareCmd)
	rootCmd.AddCommand(migrateCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runGateway(configFilePath string) {
	err := godotenv.Load() // 👈 load .env file
	if err != nil {
		log.Fatal(err)
	}

	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Printf("Starting Taronja Gateway v%s...", version)

	config, err := config.LoadConfig(configFilePath)
	if err != nil {
		log.Fatalf("FATAL: Failed to load configuration: %v", err)
	}
	log.Printf("Configuration loaded successfully: %s", config.Name)

	session.SetGeolocationConfig(&config.Geolocation)

	// Initialize dependencies for production
	gatewayDeps := deps.NewProduction()

	gateway, err := gateway.NewGatewayWithDependencies(config, &webappEmbedFS, gatewayDeps)
	if err != nil {
		log.Fatalf("FATAL: Failed to create gateway instance: %v", err)
	}

	log.Printf("API Gateway '%s' listening on %s", config.Name, gateway.Server.Addr)
	log.Printf("Gateway public URL set to: %s", config.Server.URL)
	log.Printf("Management API prefix: %s", config.Management.Prefix)

	// Print OAuth callback URLs if configured
	config.AuthenticationProviders.PrintOAuthCallbackURLs(config.Server.URL, config.Management.Prefix)

	err = gateway.Server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		log.Fatalf("FATAL: Failed to start server: %v", err)
	}

	log.Println("API Gateway shut down gracefully.")
}

func addUser(username, email, password string) {
	// Load .env file
	err := godotenv.Load()
	if err != nil {
		log.Printf("Warning: Failed to load .env file: %v", err)
	}

	appDependencies := deps.NewProduction()

	newUser := &db.User{
		Username:       username,
		Email:          email,
		Password:       password,
		EmailConfirmed: false,
	}

	err = appDependencies.UserRepo.CreateUser(newUser)
	if err != nil {
		log.Fatalf("Failed to create user: %v", err)
	}

	log.Printf("User '%s' created successfully with email '%s'.", username, email)
}

// listMiddleware loads a config file and prints the resolved global middleware
// chain (see doc/refactor01.md Phase 4). It builds a MiddlewareRegistryV2 with
// no real dependencies (nil session store, repositories, rate limiter) since
// introspection only needs each factory's name/description/dependencies —
// Create() never actually invokes them — so this never opens a database
// connection or starts a server.
func listMiddleware(configFilePath string) {
	cfg, err := config.LoadConfig(configFilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	registry, err := middleware.NewGlobalMiddlewareRegistry(nil, nil, nil, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: Failed to build middleware registry: %v\n", err)
		os.Exit(1)
	}

	specs, err := middleware.ResolveGlobalChainSpecs(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: Failed to resolve middleware chain: %v\n", err)
		os.Exit(1)
	}

	if _, err := middleware.BuildGlobalChainFromConfigV2(registry, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: Failed to build middleware chain: %v\n", err)
		os.Exit(1)
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

// migrateConfigFile implements `tg migrate`: it upgrades a config file to
// the version this gateway requires (config.CurrentConfigVersion),
// stepping through each intervening version's migration in turn
// (config.MigrateConfigFile), and writes the result to a new "-vN" file
// without touching the original. This is the only supported way to move an
// outdated config forward — config.LoadConfig (used by "tg run" and
// "tg middleware list") refuses to run against one at all.
func migrateConfigFile(configFilePath string, force bool) {
	writtenPath, fromVersion, err := config.MigrateConfigFile(configFilePath, force)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: %v\n", err)
		os.Exit(1)
	}

	if writtenPath == "" {
		fmt.Printf("'%s' is already version %d (current: %d) — nothing to migrate.\n",
			configFilePath, fromVersion, config.CurrentConfigVersion)
		return
	}

	fmt.Printf("Migrated '%s' (version %d) to '%s' (version %d).\n",
		configFilePath, fromVersion, writtenPath, config.CurrentConfigVersion)
	fmt.Printf("The original file was left unchanged. Point --config at the new file when you're ready:\n\n    tg run --config %s\n\n", writtenPath)
}
