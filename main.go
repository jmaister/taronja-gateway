package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/gateway"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
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
			// This error should ideally not happen if MarkFlagRequired works,
			// but good to keep for unexpected issues.
			fmt.Fprintf(os.Stderr, "Error getting config flag: %v\\n", err)
			os.Exit(1)
		}
		// No longer need to check if configFilePath is empty here,
		// MarkFlagRequired handles it.
		runGateway(configFilePath)
	},
}

var addUserCmd = &cobra.Command{
	Use:   "adduser [username] [email] [password]",
	Short: "Create a new user in the DB",
	Long:  `Creates a new user in the database.`,
	Args:  cobra.ExactArgs(3), // Expects exactly three arguments
	Run: func(cmd *cobra.Command, args []string) {
		username := args[0]
		email := args[1]
		password := args[2]
		addUser(username, email, password)
	},
}

func init() {
	// Add flag for config file to runCmd
	runCmd.Flags().String("config", "", "Path to the configuration file")
	if err := runCmd.MarkFlagRequired("config"); err != nil {
		log.Fatalf("Failed to mark 'config' flag as required for runCmd: %v", err)
	}

	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(addUserCmd)
	// Future commands can be added here using rootCmd.AddCommand()
}

// --- Main Function ---

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

	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile) // Include file/line number
	log.Println("Starting API Gateway...")

	// 1. Load Configuration
	config, err := config.LoadConfig(configFilePath)
	if err != nil {
		log.Fatalf("FATAL: Failed to load configuration: %v", err)
	}
	log.Printf("Configuration loaded successfully: %s", config.Name)

	// 2. Create Gateway Instance
	gateway, err := gateway.NewGateway(config)
	if err != nil {
		log.Fatalf("FATAL: Failed to create gateway instance: %v", err)
	}

	// 3. Start the HTTP Server
	log.Printf("API Gateway '%s' listening on %s", config.Name, gateway.Server.Addr)
	log.Printf("Gateway public URL set to: %s", config.Server.URL)
	log.Printf("Management API prefix: %s", config.Management.Prefix)

	err = gateway.Server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		log.Fatalf("FATAL: Failed to start server: %v", err)
	}

	log.Println("API Gateway shut down gracefully.")
}

func addUser(username, email, password string) {
	// 0. Load .env file
	err := godotenv.Load()
	if err != nil {
		log.Printf("Warning: Failed to load .env file: %v", err)
	}

	// 2. Initialize DB connection
	db.Init()

	// 3. Get DB connection
	gormDB := db.GetConnection()

	// 4. Initialize user repository
	userRepo := db.NewDBUserRepository(gormDB) // Corrected: NewDBUserRepository returns 1 value

	// 6. Creating the new user object
	newUser := &db.User{
		Username:       username,
		Email:          email,
		Password:       password, // Pass the plain password, GORM hook will hash it
		EmailConfirmed: false,    // Default new users to not confirmed
	}

	// 7. Inserting the new user
	err = userRepo.CreateUser(newUser)
	if err != nil {
		log.Fatalf("Failed to create user: %v", err)
	}

	log.Printf("User '%s' created successfully with email '%s'.", username, email)
}
