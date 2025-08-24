package main

import (
	"fmt"
	"log"

	"github.com/jmaister/taronja-gateway/config"
)

func main() {
	cfg, err := config.LoadConfig("../sample/config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	fmt.Printf("Successfully loaded configuration: %s\n", cfg.Name)
	fmt.Printf("Number of routes: %d\n", len(cfg.Routes))

	// Test our new cache control functionality
	for _, route := range cfg.Routes {
		fmt.Printf("\nRoute: %s\n", route.Name)
		fmt.Printf("  From: %s\n", route.From)
		if route.ShouldSetCacheHeader() {
			fmt.Printf("  Cache-Control: %s\n", route.GetCacheControlHeader())
		} else {
			fmt.Printf("  Cache-Control: (not set)\n")
		}
	}
}
