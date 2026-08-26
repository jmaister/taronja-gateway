package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/fsnotify/fsnotify"
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

// gracefulShutdownTimeout bounds how long runGateway waits, after receiving
// SIGINT/SIGTERM, for in-flight requests to finish before forcing the
// process to exit anyway. Matches the http.Server's own ReadTimeout/
// WriteTimeout (gateway/gateway.go) as the project's established "how long
// is too long for one request" convention.
const gracefulShutdownTimeout = 15 * time.Second

var rootCmd = &cobra.Command{
	Use:   "tg",
	Short: "Taronja Gateway CLI",
	Long:  `A CLI for managing and running the Taronja API Gateway.`,
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the Taronja API Gateway",
	Long: `Starts the Taronja API Gateway using the specified configuration file.

The config file can be reloaded without restarting the process: send the
gateway process SIGHUP, or (unless --watch=false) simply save the file —
both re-read it and, if it's still valid, swap in the new middleware chain,
routes, and rate limiter for requests received from then on. An invalid
edit is logged and ignored; the gateway keeps running its last-good config.`,
	Run: func(cmd *cobra.Command, args []string) {
		configFilePath, err := cmd.Flags().GetString("config")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting config flag: %v\n", err)
			os.Exit(1)
		}
		watchConfig, err := cmd.Flags().GetBool("watch")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting watch flag: %v\n", err)
			os.Exit(1)
		}
		runGateway(configFilePath, watchConfig)
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
	Short: "Print a config file migrated to the version this gateway requires",
	Long: `Reads a config file and prints it migrated to the version this gateway
requires (unchanged if it's already current) on stdout. This command never
writes a file itself — redirect the output to save it:

    tg migrate --config config.yaml > config-v2.yaml

The migration is applied one version step at a time (v1->v2, v2->v3, ...),
same as if you'd run this once per intervening version, so a config several
versions behind is fully upgraded in a single run.

The gateway refuses to start ("run") against an outdated config file
specifically so upgrading it is a deliberate step you take (and can review
the result of) rather than something that happens silently on every
startup. Run this whenever "tg run" tells you to.`,
	Run: func(cmd *cobra.Command, args []string) {
		configFilePath, err := cmd.Flags().GetString("config")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting config flag: %v\n", err)
			os.Exit(1)
		}
		migrateConfigFile(configFilePath)
	},
}

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate a config file without starting the gateway",
	Long: `Loads a config file and reports whether it's valid: parses successfully,
declares a supported schema version, has well-formed routes and admin
settings, and — if it has a middleware: section or the legacy analytics/
logging/rateLimiter/cors flags — resolves to a global middleware chain with
no unmet dependencies.

Does not start the HTTP server, open a database connection, or make any
network calls — safe to run in CI, or before deploying a config change, the
same way "tg middleware list" and "tg migrate" are.`,
	Run: func(cmd *cobra.Command, args []string) {
		configFilePath, err := cmd.Flags().GetString("config")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting config flag: %v\n", err)
			os.Exit(1)
		}
		validateConfigFile(configFilePath)
	},
}

func init() {
	runCmd.Flags().String("config", "", "Path to the configuration file")
	if err := runCmd.MarkFlagRequired("config"); err != nil {
		log.Fatalf("Failed to mark 'config' flag as required for runCmd: %v", err)
	}
	runCmd.Flags().Bool("watch", true, "Automatically reload the config file when it changes on disk (in addition to SIGHUP, which always works)")

	middlewareListCmd.Flags().String("config", "", "Path to the configuration file")
	if err := middlewareListCmd.MarkFlagRequired("config"); err != nil {
		log.Fatalf("Failed to mark 'config' flag as required for middlewareListCmd: %v", err)
	}
	middlewareCmd.AddCommand(middlewareListCmd)

	migrateCmd.Flags().String("config", "", "Path to the configuration file to migrate")
	if err := migrateCmd.MarkFlagRequired("config"); err != nil {
		log.Fatalf("Failed to mark 'config' flag as required for migrateCmd: %v", err)
	}

	validateCmd.Flags().String("config", "", "Path to the configuration file to validate")
	if err := validateCmd.MarkFlagRequired("config"); err != nil {
		log.Fatalf("Failed to mark 'config' flag as required for validateCmd: %v", err)
	}

	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(addUserCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(middlewareCmd)
	rootCmd.AddCommand(migrateCmd)
	rootCmd.AddCommand(validateCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runGateway(configFilePath string, watchConfig bool) {
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

	// Serve in the background so this goroutine can watch for a
	// startup/runtime error, an interrupt/terminate signal, or a reload
	// request — reacting appropriately to each — until shutdown.
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- gateway.Server.ListenAndServe()
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// SIGHUP is the traditional "reload your config" signal (nginx, most
	// other long-running servers). Unlike stop, it never ends the loop
	// below — it just triggers a ReloadConfig call and the gateway keeps
	// running either way.
	reload := make(chan os.Signal, 1)
	signal.Notify(reload, syscall.SIGHUP)

	// The complementary, opt-out way to trigger the same reload: watch the
	// config file itself and reload whenever it's saved, so a local
	// developer doesn't have to find the process and signal it by hand.
	// --watch=false (or a watcher setup failure, e.g. an unusual filesystem)
	// falls back to SIGHUP-only, which always works regardless.
	if watchConfig {
		watcherStop := make(chan struct{})
		defer close(watcherStop)
		if err := watchConfigFile(configFilePath, gateway, watcherStop); err != nil {
			log.Printf("Warning: could not watch '%s' for changes (%v) — reload via SIGHUP still works.", configFilePath, err)
		}
	}

runLoop:
	for {
		select {
		case err := <-serverErr:
			if err != nil && err != http.ErrServerClosed {
				log.Fatalf("FATAL: Failed to start server: %v", err)
			}
			break runLoop
		case sig := <-stop:
			// Drain in-flight requests instead of dropping them, which killing
			// the process outright (the previous behavior — ListenAndServe was
			// simply never asked to stop) would do on every deploy or restart.
			log.Printf("Received %s, shutting down gracefully (waiting up to %s for in-flight requests to finish)...", sig, gracefulShutdownTimeout)
			ctx, cancel := context.WithTimeout(context.Background(), gracefulShutdownTimeout)
			defer cancel()
			if err := gateway.Server.Shutdown(ctx); err != nil {
				log.Printf("Warning: graceful shutdown did not complete cleanly within %s: %v", gracefulShutdownTimeout, err)
			}
			break runLoop
		case <-reload:
			log.Printf("Received SIGHUP, reloading configuration from %s...", configFilePath)
			if err := gateway.ReloadConfig(configFilePath); err != nil {
				log.Printf("Config reload failed, keeping previous configuration: %v", err)
			}
		}
	}

	log.Println("API Gateway shut down gracefully.")
}

// watchConfigFile watches configFilePath for changes and calls
// gw.ReloadConfig whenever it's written, until stop is closed. It watches
// the file's containing directory rather than the file itself — editors and
// deployment tools commonly save by writing a new file and renaming it over
// the original (fsnotify's own documented workaround for this: a watch on
// the file itself would silently stop firing after the first such rename,
// since the watch follows the inode, not the path), and filters events down
// to the target file's own name. Events are debounced with a short timer
// since a single save often produces several rapid events (e.g. a WRITE
// followed by a CHMOD).
func watchConfigFile(configFilePath string, gw *gateway.Gateway, stop <-chan struct{}) error {
	absPath, err := filepath.Abs(configFilePath)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path: %w", err)
	}
	dir := filepath.Dir(absPath)
	name := filepath.Base(absPath)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create file watcher: %w", err)
	}
	if err := watcher.Add(dir); err != nil {
		watcher.Close()
		return fmt.Errorf("failed to watch directory '%s': %w", dir, err)
	}

	log.Printf("Watching '%s' for changes (reload on save; disable with --watch=false).", absPath)

	go func() {
		defer watcher.Close()

		const debounce = 200 * time.Millisecond
		var debounceTimer *time.Timer
		pending := make(chan struct{}, 1)

		for {
			select {
			case <-stop:
				if debounceTimer != nil {
					debounceTimer.Stop()
				}
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if filepath.Base(event.Name) != name {
					continue
				}
				if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
					continue
				}
				if debounceTimer != nil {
					debounceTimer.Stop()
				}
				debounceTimer = time.AfterFunc(debounce, func() {
					select {
					case pending <- struct{}{}:
					default:
					}
				})
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("Warning: config file watcher error: %v", err)
			case <-pending:
				if err := gw.ReloadConfig(configFilePath); err != nil {
					log.Printf("Config reload failed, keeping previous configuration: %v", err)
				}
			}
		}
	}()

	return nil
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

// migrateConfigFile implements `tg migrate`: it prints configFilePath
// migrated to the version this gateway requires (config.CurrentConfigVersion)
// on stdout, stepping through each intervening version's migration in turn
// (config.MigrateConfigContent). It never writes a file itself — the caller
// decides whether and where to save the output, typically via shell
// redirection. This is the only supported way to move an outdated config
// forward — config.LoadConfig (used by "tg run" and "tg middleware list")
// refuses to run against one at all.
func migrateConfigFile(configFilePath string) {
	content, fromVersion, err := config.MigrateConfigContent(configFilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: %v\n", err)
		os.Exit(1)
	}

	// Informational note only for the no-op case: someone redirecting this to
	// a "new" file should know it's actually identical to the source, since
	// that's not otherwise obvious from the output. Otherwise stay quiet on
	// stderr so the command composes cleanly in a pipeline.
	if fromVersion >= config.CurrentConfigVersion {
		fmt.Fprintf(os.Stderr, "Note: '%s' is already version %d (current: %d) — printing it unchanged.\n",
			configFilePath, fromVersion, config.CurrentConfigVersion)
	}

	os.Stdout.Write(content)
}

// validateConfigFile implements `tg validate`: it reports whether
// configFilePath is a valid config the gateway could actually run, without
// starting anything. config.LoadConfig already validates most of the file
// (schema version, server/admin/route settings, the middleware: section's
// names); middleware.ValidateConfigOnly additionally checks the global
// middleware chain's dependency graph (e.g. session_extraction without
// ja4_fingerprint) and a few settings LoadConfig doesn't cover (rate
// limiter/CORS value sanity) — all without needing a database connection or
// any other real dependency, the same way "tg middleware list" doesn't.
func validateConfigFile(configFilePath string) {
	cfg, err := config.LoadConfig(configFilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: %v\n", err)
		os.Exit(1)
	}

	if err := middleware.ValidateConfigOnly(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("'%s' is valid (version %d): %d route(s), management prefix %q.\n",
		configFilePath, cfg.Version, len(cfg.Routes), cfg.Management.Prefix)
}
