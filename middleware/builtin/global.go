package builtin

import (
	"fmt"
	"strings"

	"github.com/jmaister/taronja-gateway/auth"
	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db/repositories"
	"github.com/jmaister/taronja-gateway/middleware"
	"github.com/jmaister/taronja-gateway/session"
)

// GlobalDeps are the runtime services the built-in middleware use. Validation
// passes the zero value: no middleware is created then.
type GlobalDeps struct {
	SessionStore      session.SessionStore
	TokenService      *auth.TokenService
	TrafficMetricRepo repositories.TrafficMetricRepository
	RateLimiter       *RateLimiter
}

// NewGlobalRegistry registers every built-in middleware. Adding a middleware means
// writing its factory next to its middleware and listing it here.
func NewGlobalRegistry(d GlobalDeps) *middleware.Registry {
	registry := middleware.NewRegistry()
	var tokenService session.TokenService
	if d.TokenService != nil {
		tokenService = d.TokenService
	}
	for _, f := range []middleware.MiddlewareFactory{
		newTracingFactory(),
		newCompressionFactory(),
		newCORSFactory(),
		newRateLimiterFactory(d.RateLimiter),
		newJA4Factory(),
		newSessionExtractionFactory(d.SessionStore, tokenService),
		newTrafficMetricsFactory(d.TrafficMetricRepo),
		newLoggingFactory(),
	} {
		if err := registry.RegisterFactory(f); err != nil {
			panic(err) // names are constants; a duplicate is a programming error
		}
	}
	return registry
}

// ResolveGlobalMiddleware turns the config's `middleware` list into chain specs, skipping
// disabled entries. Each middleware decodes and validates its own `config` block.
func ResolveGlobalMiddleware(registry *middleware.Registry, entries []config.MiddlewareConfig) ([]middleware.MiddlewareSpec, error) {
	specs := make([]middleware.MiddlewareSpec, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsEnabled() {
			continue
		}
		factory, ok := registry.Factory(entry.Name)
		if !ok {
			return nil, fmt.Errorf("middleware: unknown middleware '%s' (known: %s)", entry.Name, strings.Join(registry.Names(), ", "))
		}
		cfg, err := factory.DecodeConfig(entry.Config)
		if err != nil {
			return nil, fmt.Errorf("middleware: %s: %w", entry.Name, err)
		}
		specs = append(specs, middleware.MiddlewareSpec{Name: entry.Name, Config: cfg})
	}
	return specs, nil
}

// NewGlobalChain builds the global middleware chain from gatewayConfig. The
// registry is returned so callers can expose status, health and metrics.
func NewGlobalChain(gatewayConfig *config.GatewayConfig, d GlobalDeps) (*middleware.Registry, *middleware.ChainBuilder, error) {
	registry := NewGlobalRegistry(d)
	specs, err := ResolveGlobalMiddleware(registry, gatewayConfig.Middleware)
	if err != nil {
		return nil, nil, err
	}
	chain, err := registry.BuildChain(specs)
	if err != nil {
		return nil, nil, err
	}
	return registry, chain, nil
}

// ValidateGlobalMiddleware checks every middleware's name, options and dependencies without
// creating any of them.
func ValidateGlobalMiddleware(gatewayConfig *config.GatewayConfig) error {
	registry := NewGlobalRegistry(GlobalDeps{})
	specs, err := ResolveGlobalMiddleware(registry, gatewayConfig.Middleware)
	if err != nil {
		return err
	}
	if err := registry.ValidateSpecs(specs); err != nil {
		return fmt.Errorf("middleware: %w", err)
	}
	return nil
}

// MiddlewareEnabled reports whether the named middleware is listed and enabled.
func MiddlewareEnabled(gatewayConfig *config.GatewayConfig, name string) bool {
	for _, entry := range gatewayConfig.Middleware {
		if entry.Name == name && entry.IsEnabled() {
			return true
		}
	}
	return false
}

// MiddlewareSettings returns the decoded options of the named middleware and whether
// it is enabled. Code outside the chain that depends on a middleware's options
// (the shared rate limiter, tracing setup, the statistics API) reads them here
// so it always agrees with the chain.
func MiddlewareSettings[C any](gatewayConfig *config.GatewayConfig, name string) (C, bool, error) {
	var zero C
	for _, entry := range gatewayConfig.Middleware {
		if entry.Name != name || !entry.IsEnabled() {
			continue
		}
		factory, ok := NewGlobalRegistry(GlobalDeps{}).Factory(name)
		if !ok {
			return zero, false, fmt.Errorf("middleware: unknown middleware '%s'", name)
		}
		cfg, err := factory.DecodeConfig(entry.Config)
		if err != nil {
			return zero, false, fmt.Errorf("middleware: %s: %w", name, err)
		}
		c, ok := cfg.(C)
		if !ok {
			return zero, false, fmt.Errorf("middleware: %s: config is %T, not %T", name, cfg, zero)
		}
		return c, true, nil
	}
	return zero, false, nil
}
