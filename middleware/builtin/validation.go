package builtin

import (
	"log"

	"github.com/jmaister/taronja-gateway/config"
)

// ValidateConfigOnly runs the checks that only inspect the config: the config
// package's own Validate, then each middleware's options and dependencies. It
// is what `tg validate` calls, since that opens no database.
func ValidateConfigOnly(config *config.GatewayConfig) error {
	if err := config.Validate(); err != nil {
		return err
	}
	_, err := ResolveGlobalChain(config)
	return err
}

// LogMiddlewareStatus logs the status of all middleware based on configuration
func LogMiddlewareStatus(config *config.GatewayConfig) {
	log.Printf("=== Middleware Status ===")

	enabled := 0
	for _, f := range config.Middleware {
		if f.IsEnabled() {
			log.Printf("✓ Middleware %s: ENABLED", f.Name)
			enabled++
		} else {
			log.Printf("✗ Middleware %s: DISABLED", f.Name)
		}
	}
	if enabled == 0 {
		log.Printf("✗ Middleware: NONE")
	}

	// Route-specific middleware status
	authRoutes := 0
	cacheRoutes := 0
	for _, route := range config.Routes {
		if route.Authenticated {
			authRoutes++
		}
		if route.Options != nil && route.Options.CacheControlSeconds != nil {
			cacheRoutes++
		}
	}

	if authRoutes > 0 {
		log.Printf("✓ Authentication: ENABLED on %d routes", authRoutes)
	} else {
		log.Printf("✗ Authentication: NOT USED")
	}

	if cacheRoutes > 0 {
		log.Printf("✓ Cache Control: ENABLED on %d routes", cacheRoutes)
	} else {
		log.Printf("✗ Cache Control: NOT USED")
	}

	// Admin access status
	if config.Management.Admin.Enabled {
		log.Printf("✓ Admin Access: ENABLED (user: %s)", config.Management.Admin.Username)
	} else {
		log.Printf("✗ Admin Access: DISABLED")
	}

	log.Printf("=========================")
}
