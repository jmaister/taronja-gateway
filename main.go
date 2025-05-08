package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/gateway"
	"github.com/joho/godotenv"
)

// --- Main Function ---

func main() {
	err := godotenv.Load() // 👈 load .env file
	if err != nil {
		log.Fatal(err)
	}

	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile) // Include file/line number
	log.Println("Starting API Gateway...")

	// 1. Load Configuration
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run . <path/to/config.yaml>")
		os.Exit(1)
	}
	configFilePath := os.Args[1]
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
