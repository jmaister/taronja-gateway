package middleware

import (
	"fmt"
	"log"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/gateway/deps"
)

// ValidationError represents a middleware validation error
type ValidationError struct {
	Middleware string
	Message    string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("middleware %s: %s", e.Middleware, e.Message)
}

// ValidateDependencies validates that all required dependencies are available
func ValidateDependencies(deps *deps.Dependencies, config *config.GatewayConfig) error {
	if deps == nil {
		return &ValidationError{
			Middleware: "global",
			Message:    "dependencies cannot be nil",
		}
	}

	if config == nil {
		return &ValidationError{
			Middleware: "global",
			Message:    "gateway configuration is required",
		}
	}

	return nil
}

// ValidateAnalyticsMiddleware validates analytics middleware configuration and dependencies
func ValidateAnalyticsMiddleware(deps *deps.Dependencies, config *config.GatewayConfig) error {
	if !config.Management.Analytics {
		// Analytics not enabled, no validation needed
		return nil
	}

	log.Printf("Validating analytics middleware dependencies...")

	// Validate SessionStore for session extraction
	if deps.SessionStore == nil {
		return &ValidationError{
			Middleware: "session_extraction",
			Message:    "session store is required when analytics is enabled",
		}
	}

	// Validate TokenService for session extraction
	if deps.TokenService == nil {
		return &ValidationError{
			Middleware: "session_extraction",
			Message:    "token service is required when analytics is enabled",
		}
	}

	// Validate TrafficMetricRepository for traffic metrics
	if deps.TrafficMetricRepo == nil {
		return &ValidationError{
			Middleware: "traffic_metrics",
			Message:    "traffic metric repository is required when analytics is enabled",
		}
	}

	log.Printf("Analytics middleware dependencies validated successfully")
	return nil
}

// ValidateAuthenticationMiddleware validates authentication middleware configuration and dependencies
func ValidateAuthenticationMiddleware(deps *deps.Dependencies, config *config.GatewayConfig) error {
	// Check if any routes require authentication
	hasAuthRoutes := false
	for _, route := range config.Routes {
		if route.Authentication.Enabled {
			hasAuthRoutes = true
			break
		}
	}

	if !hasAuthRoutes {
		// No auth routes, no validation needed
		return nil
	}

	log.Printf("Validating authentication middleware dependencies...")

	// Validate SessionStore for authentication
	if deps.SessionStore == nil {
		return &ValidationError{
			Middleware: "authentication",
			Message:    "session store is required when authentication is enabled on routes",
		}
	}

	// Validate TokenService for authentication
	if deps.TokenService == nil {
		return &ValidationError{
			Middleware: "authentication",
			Message:    "token service is required when authentication is enabled on routes",
		}
	}

	// Validate UserRepository for authentication
	if deps.UserRepo == nil {
		return &ValidationError{
			Middleware: "authentication",
			Message:    "user repository is required when authentication is enabled on routes",
		}
	}

	// Validate TokenRepository for authentication
	if deps.TokenRepo == nil {
		return &ValidationError{
			Middleware: "authentication",
			Message:    "token repository is required when authentication is enabled on routes",
		}
	}

	// Validate management prefix for redirects
	if config.Management.Prefix == "" {
		return &ValidationError{
			Middleware: "authentication",
			Message:    "management prefix is required when authentication is enabled",
		}
	}

	log.Printf("Authentication middleware dependencies validated successfully")
	return nil
}

// ValidateAdminAccess validates admin access configuration
func ValidateAdminAccess(deps *deps.Dependencies, config *config.GatewayConfig) error {
	if !config.Management.Admin.Enabled {
		// Admin not enabled, no validation needed
		return nil
	}

	log.Printf("Validating admin access configuration...")

	// Validate admin credentials
	if config.Management.Admin.Username == "" {
		return &ValidationError{
			Middleware: "admin",
			Message:    "admin username is required when admin is enabled",
		}
	}

	if config.Management.Admin.Password == "" {
		return &ValidationError{
			Middleware: "admin",
			Message:    "admin password is required when admin is enabled",
		}
	}

	/*
		if deps.GatewayConfig.Management.Admin.Email == "" {
			return &ValidationError{
				Middleware: "admin",
				Message:    "admin email is required when admin is enabled",
			}
		}
	*/

	log.Printf("Admin access configuration validated successfully")
	return nil
}

// ValidateRouteConfiguration validates route-specific middleware configuration
func ValidateRouteConfiguration(deps *deps.Dependencies, config *config.GatewayConfig) error {
	log.Printf("Validating route middleware configuration...")

	for _, route := range config.Routes {
		// Validate static routes
		if route.Static {
			if route.ToFile == "" && route.To == "" && route.ToFolder == "" {
				return &ValidationError{
					Middleware: "static",
					Message:    fmt.Sprintf("static route '%s' must have either ToFile, ToFolder, or To configured", route.Name),
				}
			}
		} else {
			// Validate proxy routes
			if route.To == "" {
				return &ValidationError{
					Middleware: "proxy",
					Message:    fmt.Sprintf("proxy route '%s' must have To URL configured", route.Name),
				}
			}
		}

		// Validate cache configuration
		if route.Options != nil && route.Options.CacheControlSeconds != nil {
			if *route.Options.CacheControlSeconds < 0 {
				return &ValidationError{
					Middleware: "cache",
					Message:    fmt.Sprintf("route '%s' has invalid cache control seconds: %d", route.Name, *route.Options.CacheControlSeconds),
				}
			}
		}
	}

	log.Printf("Route middleware configuration validated successfully")
	return nil
}

// ValidateAllMiddleware validates all middleware configuration and dependencies
func ValidateAllMiddleware(deps *deps.Dependencies, config *config.GatewayConfig) error {
	log.Printf("Starting comprehensive middleware validation...")

	// Validate basic dependencies
	if err := ValidateDependencies(deps, config); err != nil {
		return err
	}

	// Validate analytics middleware
	if err := ValidateAnalyticsMiddleware(deps, config); err != nil {
		return err
	}

	// Validate authentication middleware
	if err := ValidateAuthenticationMiddleware(deps, config); err != nil {
		return err
	}

	// Validate admin access
	if err := ValidateAdminAccess(deps, config); err != nil {
		return err
	}

	// Validate route configuration
	if err := ValidateRouteConfiguration(deps, config); err != nil {
		return err
	}

	log.Printf("All middleware validation completed successfully")
	return nil
}

// LogMiddlewareStatus logs the status of all middleware based on configuration
func LogMiddlewareStatus(config *config.GatewayConfig) {
	log.Printf("=== Middleware Status ===")

	// Global middleware status
	if config.Management.Analytics {
		log.Printf("✓ JA4H Fingerprinting: ENABLED")
		log.Printf("✓ Session Extraction: ENABLED")
		log.Printf("✓ Traffic Metrics: ENABLED")
	} else {
		log.Printf("✗ Analytics Middleware: DISABLED")
	}

	if config.Management.Logging {
		log.Printf("✓ Request Logging: ENABLED")
	} else {
		log.Printf("✗ Request Logging: DISABLED")
	}

	// Route-specific middleware status
	authRoutes := 0
	cacheRoutes := 0
	for _, route := range config.Routes {
		if route.Authentication.Enabled {
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
