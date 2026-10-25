package cli

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/gateway"
	"github.com/jmaister/taronja-gateway/gateway/deps"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

// gracefulShutdownTimeout bounds how long in-flight requests get to finish
// after SIGINT/SIGTERM; it matches the http.Server read/write timeouts.
const gracefulShutdownTimeout = 15 * time.Second

func newRunCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the Taronja API Gateway",
		Long:  "Starts the Taronja API Gateway using the specified configuration file.",
		Run: func(cmd *cobra.Command, args []string) {
			a.runGateway(configPath(cmd))
		},
	}
	addConfigFlag(cmd, "Path to the configuration file")
	return cmd
}

// dotEnvLoadIsFatal reports whether a godotenv.Load error should abort startup:
// a missing .env is fine, an unreadable or malformed one is not.
func dotEnvLoadIsFatal(err error) bool {
	return err != nil && !os.IsNotExist(err)
}

func (a *app) runGateway(configFilePath string) {
	if err := godotenv.Load(); dotEnvLoadIsFatal(err) {
		log.Fatalf("FATAL: failed to load .env: %v", err)
	}

	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Printf("Starting Taronja Gateway v%s...", a.build.Version)

	cfg, err := config.LoadConfig(configFilePath)
	if err != nil {
		log.Fatalf("FATAL: Failed to load configuration: %v", err)
	}
	log.Printf("Configuration loaded successfully: %s", cfg.Name)

	session.SetGeolocationConfig(&cfg.Geolocation)

	shutdownTracing, err := gateway.InitTracing(context.Background(), cfg)
	if err != nil {
		log.Fatalf("FATAL: failed to initialize tracing: %v", err)
	}

	gatewayDeps := deps.NewProduction()

	// Must run before NewGatewayWithDependencies, which reads the notification service.
	shutdownNotifications, err := gateway.InitNotifications(context.Background(), cfg.Notification, cfg.Server, gatewayDeps)
	if err != nil {
		log.Fatalf("FATAL: failed to initialize notifications: %v", err)
	}

	// Must run before NewGatewayWithDependencies, which registers the billing routes.
	shutdownBilling := gateway.InitBilling(context.Background(), cfg.Billing, cfg.Server, gatewayDeps)

	gw, err := gateway.NewGatewayWithDependencies(cfg, a.webappFS, gatewayDeps)
	if err != nil {
		log.Fatalf("FATAL: Failed to create gateway instance: %v", err)
	}

	gw.Version = a.build.Version

	log.Printf("API Gateway '%s' listening on %s", cfg.Name, gw.Server.Addr)
	log.Printf("Gateway public URL set to: %s", cfg.Server.URL)
	log.Printf("Management API prefix: %s", config.ManagementPrefix)

	cfg.AuthenticationProviders.PrintOAuthCallbackURLs(cfg.Server.URL, config.ManagementPrefix)

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- gw.Server.ListenAndServe()
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// An unhandled SIGHUP would kill the process without a graceful shutdown.
	signal.Ignore(syscall.SIGHUP)

	select {
	case err := <-serverErr:
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("FATAL: Failed to start server: %v", err)
		}
	case sig := <-stop:
		log.Printf("Received %s, shutting down gracefully (waiting up to %s for in-flight requests to finish)...", sig, gracefulShutdownTimeout)
		ctx, cancel := context.WithTimeout(context.Background(), gracefulShutdownTimeout)
		defer cancel()
		if err := gw.Server.Shutdown(ctx); err != nil {
			log.Printf("Warning: graceful shutdown did not complete cleanly within %s: %v", gracefulShutdownTimeout, err)
		}
	}

	// No request is in flight any more, so the metrics batcher can be flushed safely.
	gw.Dependencies.Close()

	tracingCtx, tracingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer tracingCancel()
	if err := shutdownTracing(tracingCtx); err != nil {
		log.Printf("Warning: failed to flush tracing exporter cleanly: %v", err)
	}

	billingCtx, billingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer billingCancel()
	err = shutdownBilling(billingCtx)
	if err != nil {
		log.Printf("Warning: failed to stop the billing workers cleanly: %v", err)
	}

	notificationsCtx, notificationsCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer notificationsCancel()
	if err := shutdownNotifications(notificationsCtx); err != nil {
		log.Printf("Warning: failed to stop notification delivery cleanly: %v", err)
	}

	log.Println("API Gateway shut down gracefully.")
}
