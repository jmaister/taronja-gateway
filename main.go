package main

import (
	"embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/gateway"
	"github.com/jmaister/taronja-gateway/gateway/deps"
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

func init() {
	runCmd.Flags().String("config", "", "Path to the configuration file")
	if err := runCmd.MarkFlagRequired("config"); err != nil {
		log.Fatalf("Failed to mark 'config' flag as required for runCmd: %v", err)
	}

	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(addUserCmd)
	rootCmd.AddCommand(versionCmd)
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
