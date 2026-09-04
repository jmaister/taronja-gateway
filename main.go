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

// dotEnvLoadIsFatal reports whether an error from godotenv.Load() should
// stop the gateway from starting: a missing .env is never fatal — see
// runGateway's call site — only a present-but-unreadable/malformed one is.
func dotEnvLoadIsFatal(err error) bool {
	return err != nil && !os.IsNotExist(err)
}

func runGateway(configFilePath string, watchConfig bool) {
	// A missing .env is fine — plenty of deployments (this project's own
	// Docker demo among them) rely on real environment variables only and
	// never have one; only a present-but-unreadable/malformed one is worth
	// stopping for. Same distinction gateway/reload.go's reloadDotEnv makes
	// for every later reload, and the one addUser already made below — this
	// was the one call site treating "no .env" as fatal.
	if err := godotenv.Load(); dotEnvLoadIsFatal(err) {
		log.Fatalf("FATAL: failed to load .env: %v", err)
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

	switch {
	case config.Server.TLS.Enabled && config.Server.TLS.ACME != nil:
		log.Printf("API Gateway '%s' listening on %s (TLS via ACME for domain(s): %s; certificates cached under %s)",
			config.Name, gateway.Server.Addr, strings.Join(config.Server.TLS.ACME.Domains, ", "), config.Server.TLS.ACME.CacheDir)
		if config.Server.TLS.ACME.DirectoryURL != "" {
			log.Printf("Using non-default ACME directory URL: %s (remove server.tls.acme.directoryURL once testing is done, to get a browser-trusted certificate)", config.Server.TLS.ACME.DirectoryURL)
		}
	case config.Server.TLS.Enabled:
		log.Printf("API Gateway '%s' listening on %s (TLS)", config.Name, gateway.Server.Addr)
	default:
		log.Printf("API Gateway '%s' listening on %s", config.Name, gateway.Server.Addr)
	}
	log.Printf("Gateway public URL set to: %s", config.Server.URL)
	log.Printf("Management API prefix: %s", config.Management.Prefix)

	// Print OAuth callback URLs if configured
	config.AuthenticationProviders.PrintOAuthCallbackURLs(config.Server.URL, config.Management.Prefix)

	// Serve in the background so this goroutine can watch for a
	// startup/runtime error, an interrupt/terminate signal, or a reload
	// request — reacting appropriately to each — until shutdown. Buffered
	// for 2: with TLS enabled, gateway.RedirectServer sends into the same
	// channel too (see below), and a clean shutdown can leave one send
	// sitting unread once runLoop breaks on the first — harmless since the
	// channel is buffered and never read past that.
	serverErr := make(chan error, 2)
	go func() {
		if config.Server.TLS.Enabled {
			// Empty certFile/keyFile: the cert comes from
			// gateway.Server.TLSConfig.GetCertificate (see gateway/tls.go's
			// certReloader), not from files ListenAndServeTLS itself opens.
			serverErr <- gateway.Server.ListenAndServeTLS("", "")
		} else {
			serverErr <- gateway.Server.ListenAndServe()
		}
	}()

	// The plain-HTTP redirect listener (server.tls.redirectPort, default 80)
	// is just as much a startup requirement as the main listener when TLS is
	// enabled — if it can't bind (e.g. port 80 already in use, or requires a
	// privilege this process doesn't have), that's worth failing loudly for
	// rather than silently running without the redirect the config asked
	// for. Set server.tls.redirectPort: 0 to opt out of it entirely instead.
	if gateway.RedirectServer != nil {
		log.Printf("HTTP->HTTPS redirect listening on %s", gateway.RedirectServer.Addr)
		go func() {
			serverErr <- gateway.RedirectServer.ListenAndServe()
		}()
	}

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

		// The cert/key files' *content* (e.g. a renewal tool replacing them
		// in place) hot-reloads independently of the config file itself —
		// see Gateway.ReloadTLSCertificate's doc comment for why this is a
		// separate concern from config reload entirely, not just another
		// case watchConfigFile happens to handle. Not applicable to ACME —
		// there's no static cert/key file to watch, since the gateway's own
		// autocert.Manager obtains and renews the certificate itself.
		if config.Server.TLS.Enabled && config.Server.TLS.ACME == nil {
			certWatcherStop := make(chan struct{})
			defer close(certWatcherStop)
			if err := watchCertFiles(config.Server.TLS.CertFile, config.Server.TLS.KeyFile, gateway, certWatcherStop); err != nil {
				log.Printf("Warning: could not watch TLS cert/key files for changes (%v) — restart the gateway to pick up a renewed certificate.", err)
			}
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
			if gateway.RedirectServer != nil {
				if err := gateway.RedirectServer.Shutdown(ctx); err != nil {
					log.Printf("Warning: HTTP->HTTPS redirect listener did not shut down cleanly within %s: %v", gracefulShutdownTimeout, err)
				}
			}
			break runLoop
		case <-reload:
			log.Printf("Received SIGHUP, reloading configuration from %s...", configFilePath)
			if err := gateway.ReloadConfig(configFilePath); err != nil {
				log.Printf("Config reload failed, keeping previous configuration: %v", err)
			}
		}
	}

	// The server has stopped accepting new requests either way (a clean
	// Shutdown, or ListenAndServe returning on its own) — nothing is still
	// calling Dependencies.TrafficMetricRepo.Create concurrently by this
	// point, so it's safe to flush and stop its batching goroutine (see
	// Dependencies.Close and PERFORMANCE_ANALYSIS.md).
	gateway.Dependencies.Close()

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

// watchCertFiles watches certFile and keyFile (config.LoadConfig has
// already resolved both to absolute paths) and calls
// gw.ReloadTLSCertificate whenever either changes, so a renewal tool (e.g.
// certbot) replacing them in place takes effect without a restart. Same
// directory-watch-plus-debounce approach as watchConfigFile, for the same
// reason: watching the file itself, rather than its containing directory,
// silently stops working after the first write-then-rename a renewal tool
// typically does.
func watchCertFiles(certFile, keyFile string, gw *gateway.Gateway, stop <-chan struct{}) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create file watcher: %w", err)
	}

	// certFile and keyFile are very often the same directory (sometimes the
	// same file, for a combined cert+key PEM) — watch each directory once.
	dirs := map[string]bool{filepath.Dir(certFile): true, filepath.Dir(keyFile): true}
	for dir := range dirs {
		if err := watcher.Add(dir); err != nil {
			watcher.Close()
			return fmt.Errorf("failed to watch directory '%s': %w", dir, err)
		}
	}
	names := map[string]bool{filepath.Base(certFile): true, filepath.Base(keyFile): true}

	log.Printf("Watching TLS cert/key files ('%s', '%s') for changes.", certFile, keyFile)

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
				if !names[filepath.Base(event.Name)] {
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
				log.Printf("Warning: TLS cert/key file watcher error: %v", err)
			case <-pending:
				if err := gw.ReloadTLSCertificate(); err != nil {
					log.Printf("TLS certificate reload failed, keeping previous certificate: %v", err)
				} else {
					log.Printf("TLS certificate reloaded from '%s'/'%s'.", certFile, keyFile)
				}
			}
		}
	}()

	return nil
}

func addUser(username, email, password string) {
	// Load .env file — a missing one is fine, see dotEnvLoadIsFatal.
	if err := godotenv.Load(); dotEnvLoadIsFatal(err) {
		log.Printf("Warning: Failed to load .env file: %v", err)
	}

	appDependencies := deps.NewProduction()

	newUser := &db.User{
		Username:       username,
		Email:          email,
		Password:       password,
		EmailConfirmed: false,
	}

	if err := appDependencies.UserRepo.CreateUser(newUser); err != nil {
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
	// stderr so the command composes cleanly in a pipeline. fromVersion is
	// nil when the file has no version: field at all — see
	// config.GatewayConfig.Version's doc comment for why that's reported
	// distinctly rather than as "version 1".
	switch {
	case fromVersion == nil:
		fmt.Fprintf(os.Stderr, "Note: '%s' has no declared version (current: %d) — printing it unchanged.\n",
			configFilePath, config.CurrentConfigVersion)
	case *fromVersion >= config.CurrentConfigVersion:
		fmt.Fprintf(os.Stderr, "Note: '%s' is already version %d (current: %d) — printing it unchanged.\n",
			configFilePath, *fromVersion, config.CurrentConfigVersion)
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

	versionDesc := "no declared version"
	if cfg.Version != nil {
		versionDesc = fmt.Sprintf("version %d", *cfg.Version)
	}
	fmt.Printf("'%s' is valid (%s): %d route(s), management prefix %q.\n",
		configFilePath, versionDesc, len(cfg.Routes), cfg.Management.Prefix)
}
