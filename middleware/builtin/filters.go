package builtin

import (
	"fmt"
	"strings"

	"github.com/jmaister/taronja-gateway/auth"
	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/middleware"
	"github.com/jmaister/taronja-gateway/session"
)

// FilterDeps are the runtime services the built-in filters use. Validation
// passes the zero value: no filter is created then.
type FilterDeps struct {
	SessionStore      session.SessionStore
	TokenService      *auth.TokenService
	TrafficMetricRepo db.TrafficMetricRepository
	RateLimiter       *RateLimiter
}

// NewFilterRegistry registers every built-in filter. Adding a filter means
// writing its factory next to its middleware and listing it here.
func NewFilterRegistry(d FilterDeps) *middleware.Registry {
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

// ResolveFilters turns the config's `filters` list into chain specs, skipping
// disabled entries. Each filter decodes and validates its own `config` block.
func ResolveFilters(registry *middleware.Registry, filters []config.FilterConfig) ([]middleware.MiddlewareSpec, error) {
	specs := make([]middleware.MiddlewareSpec, 0, len(filters))
	for _, entry := range filters {
		if !entry.IsEnabled() {
			continue
		}
		factory, ok := registry.Factory(entry.Name)
		if !ok {
			return nil, fmt.Errorf("filters: unknown filter '%s' (known: %s)", entry.Name, strings.Join(registry.Names(), ", "))
		}
		cfg, err := factory.DecodeConfig(entry.Config)
		if err != nil {
			return nil, fmt.Errorf("filters: %s: %w", entry.Name, err)
		}
		specs = append(specs, middleware.MiddlewareSpec{Name: entry.Name, Config: cfg})
	}
	return specs, nil
}

// NewGlobalChain builds the global filter chain from gatewayConfig. The
// registry is returned so callers can expose status, health and metrics.
func NewGlobalChain(gatewayConfig *config.GatewayConfig, d FilterDeps) (*middleware.Registry, *middleware.ChainBuilder, error) {
	registry := NewFilterRegistry(d)
	specs, err := ResolveFilters(registry, gatewayConfig.Filters)
	if err != nil {
		return nil, nil, err
	}
	chain, err := registry.BuildChain(specs)
	if err != nil {
		return nil, nil, err
	}
	return registry, chain, nil
}

// ValidateFilters checks every filter's name, options and dependencies without
// creating any of them.
func ValidateFilters(gatewayConfig *config.GatewayConfig) error {
	registry := NewFilterRegistry(FilterDeps{})
	specs, err := ResolveFilters(registry, gatewayConfig.Filters)
	if err != nil {
		return err
	}
	if err := registry.ValidateSpecs(specs); err != nil {
		return fmt.Errorf("filters: %w", err)
	}
	return nil
}

// FilterEnabled reports whether the named filter is listed and enabled.
func FilterEnabled(gatewayConfig *config.GatewayConfig, name string) bool {
	for _, entry := range gatewayConfig.Filters {
		if entry.Name == name && entry.IsEnabled() {
			return true
		}
	}
	return false
}

// FilterSettings returns the decoded options of the named filter and whether
// it is enabled. Code outside the chain that depends on a filter's options
// (the shared rate limiter, tracing setup, the statistics API) reads them here
// so it always agrees with the chain.
func FilterSettings[C any](gatewayConfig *config.GatewayConfig, name string) (C, bool, error) {
	var zero C
	for _, entry := range gatewayConfig.Filters {
		if entry.Name != name || !entry.IsEnabled() {
			continue
		}
		factory, ok := NewFilterRegistry(FilterDeps{}).Factory(name)
		if !ok {
			return zero, false, fmt.Errorf("filters: unknown filter '%s'", name)
		}
		cfg, err := factory.DecodeConfig(entry.Config)
		if err != nil {
			return zero, false, fmt.Errorf("filters: %s: %w", name, err)
		}
		c, ok := cfg.(C)
		if !ok {
			return zero, false, fmt.Errorf("filters: %s: config is %T, not %T", name, cfg, zero)
		}
		return c, true, nil
	}
	return zero, false, nil
}
