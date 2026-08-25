package middleware

import (
	"fmt"
	"log"

	"github.com/jmaister/taronja-gateway/config"
)

// MiddlewareSpec describes one middleware to build: which factory to use (by
// name) and the configuration to pass to it. A slice of specs is an ordered,
// declarative description of a middleware chain.
type MiddlewareSpec struct {
	Name   string      `json:"name" yaml:"name"`
	Config interface{} `json:"config" yaml:"config"`
}

// MiddlewareStatus describes the status of a middleware known to a MiddlewareRegistryV2.
type MiddlewareStatus struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Status       string   `json:"status"` // "active" or "available"
	Enabled      bool     `json:"enabled"`
	Dependencies []string `json:"dependencies"`
}

// MiddlewareRegistryV2 builds middleware chains from declarative MiddlewareSpec
// lists. Factories must be registered before BuildChain is called; BuildChain
// validates that every spec's dependencies were already satisfied earlier in
// the same chain and returns a ready-to-use ChainBuilder.
type MiddlewareRegistryV2 struct {
	factories map[string]MiddlewareFactory
	built     map[string]bool
}

// NewMiddlewareRegistryV2 creates an empty registry with no factories registered.
func NewMiddlewareRegistryV2() *MiddlewareRegistryV2 {
	return &MiddlewareRegistryV2{
		factories: make(map[string]MiddlewareFactory),
		built:     make(map[string]bool),
	}
}

// RegisterFactory registers a middleware factory under its own name. Returns
// an error if a factory with the same name is already registered.
func (r *MiddlewareRegistryV2) RegisterFactory(factory MiddlewareFactory) error {
	name := factory.GetName()
	if _, exists := r.factories[name]; exists {
		return fmt.Errorf("middleware factory '%s' already registered", name)
	}
	r.factories[name] = factory
	log.Printf("Registered middleware factory: %s", name)
	return nil
}

// BuildChain builds a ChainBuilder from an ordered list of MiddlewareSpec.
// For each spec, it looks up the registered factory, verifies all of the
// factory's declared dependencies were built by an earlier spec in the same
// call, creates the middleware instance, and appends it to the chain.
//
// Returns an error if a spec names an unregistered middleware or if a
// dependency is not satisfied by an earlier spec.
func (r *MiddlewareRegistryV2) BuildChain(specs []MiddlewareSpec) (*ChainBuilder, error) {
	chain := NewChainBuilder()
	built := make(map[string]bool)

	for _, spec := range specs {
		factory, exists := r.factories[spec.Name]
		if !exists {
			return nil, fmt.Errorf("unknown middleware: %s", spec.Name)
		}

		// Validate dependencies are satisfied by earlier specs in this chain.
		for _, dep := range factory.GetDependencies() {
			if !built[dep] {
				return nil, fmt.Errorf(
					"middleware '%s' depends on '%s' which is not enabled",
					spec.Name, dep,
				)
			}
		}

		cfg := spec.Config
		if cfg == nil {
			cfg = factory.GetDefaultConfig()
		}

		mw, err := factory.Create(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create middleware '%s': %w", spec.Name, err)
		}

		chain.Add(mw)
		built[spec.Name] = true
		r.built[spec.Name] = true
		log.Printf("Added middleware to chain: %s", spec.Name)
	}

	return chain, nil
}

// GetStatus returns the status of every registered factory: "active" if it
// was included in the most recent BuildChain call, "available" otherwise.
func (r *MiddlewareRegistryV2) GetStatus() map[string]MiddlewareStatus {
	status := make(map[string]MiddlewareStatus, len(r.factories))

	for name, factory := range r.factories {
		enabled := r.built[name]
		s := MiddlewareStatus{
			Name:         name,
			Description:  factory.GetDescription(),
			Enabled:      enabled,
			Dependencies: factory.GetDependencies(),
		}
		if enabled {
			s.Status = "active"
		} else {
			s.Status = "available"
		}
		status[name] = s
	}

	return status
}

// ValidateSpecs checks that every spec names a registered factory and that
// its dependencies are satisfied by an earlier spec in the same list —
// without creating any middleware instances. This is the same dependency
// graph check BuildChain performs, exposed separately so configuration can be
// validated at startup before real dependencies (session store, DB
// repositories, rate limiter instance, ...) are available to actually build
// the chain.
func (r *MiddlewareRegistryV2) ValidateSpecs(specs []MiddlewareSpec) error {
	built := make(map[string]bool)
	for _, spec := range specs {
		factory, exists := r.factories[spec.Name]
		if !exists {
			return fmt.Errorf("unknown middleware: %s", spec.Name)
		}
		for _, dep := range factory.GetDependencies() {
			if !built[dep] {
				return fmt.Errorf(
					"middleware '%s' depends on '%s' which is not enabled",
					spec.Name, dep,
				)
			}
		}
		built[spec.Name] = true
	}
	return nil
}

// referenceGlobalFactories returns one instance of every built-in global
// middleware factory, with no real dependencies wired in (nil session store,
// nil repositories, ...). It exists purely so MiddlewareSpec lists can be
// validated (names + dependency graph) via ValidateGlobalChainSpecs without
// needing live dependencies — those factories' Create() is never called for
// validation, only GetName()/GetDependencies().
func referenceGlobalFactories() []MiddlewareFactory {
	return []MiddlewareFactory{
		NewRateLimiterFactory(nil),
		NewJA4Factory(),
		NewSessionExtractionFactory(nil, nil),
		NewTrafficMetricsFactory(nil),
		NewLoggingFactory(),
	}
}

// ValidateGlobalChainSpecs validates specs (typically produced by
// ResolveGlobalChainSpecs) against the built-in global middleware factories:
// every name must be recognized and every dependency must be satisfied by an
// earlier spec. It does not require or use real middleware dependencies.
func ValidateGlobalChainSpecs(specs []MiddlewareSpec) error {
	registry := NewMiddlewareRegistryV2()
	for _, f := range referenceGlobalFactories() {
		if err := registry.RegisterFactory(f); err != nil {
			return err
		}
	}
	return registry.ValidateSpecs(specs)
}

// ResolveGlobalChainSpecs translates gateway configuration into an ordered
// list of MiddlewareSpec describing the global chain.
//
// If gatewayConfig.Middleware.Global is non-empty (an explicit `middleware:`
// section is present in the config file), it is used directly: each entry
// becomes a spec, in the order listed, skipping entries with Enabled=false.
// This is the Phase 2 declarative path (see doc/refactor01.md).
//
// Otherwise, the legacy management.analytics / management.logging /
// management.rateLimiter flags are translated into the equivalent specs —
// identical to Phase 1 / the original hardcoded BuildGlobalChain — so
// existing config files keep working unchanged.
func ResolveGlobalChainSpecs(gatewayConfig *config.GatewayConfig) ([]MiddlewareSpec, error) {
	if len(gatewayConfig.Middleware.Global) > 0 {
		return specsFromMiddlewareSection(gatewayConfig)
	}
	return legacySpecsFromConfig(gatewayConfig), nil
}

// specsFromMiddlewareSection builds specs from an explicit
// gatewayConfig.Middleware.Global list.
func specsFromMiddlewareSection(gatewayConfig *config.GatewayConfig) ([]MiddlewareSpec, error) {
	specs := make([]MiddlewareSpec, 0, len(gatewayConfig.Middleware.Global))

	for _, entry := range gatewayConfig.Middleware.Global {
		if !config.IsMiddlewareNameKnown(entry.Name) {
			// config.LoadConfig already rejects unknown names at load time; this
			// guards GatewayConfig values built programmatically (e.g. in tests)
			// that bypass LoadConfig.
			return nil, fmt.Errorf("middleware.global: unknown middleware '%s'", entry.Name)
		}
		if !entry.IsEnabled() {
			continue
		}

		spec := MiddlewareSpec{Name: entry.Name}
		if entry.Name == config.MiddlewareNameRateLimiter {
			if entry.RateLimiter != nil {
				spec.Config = *entry.RateLimiter
			} else {
				spec.Config = gatewayConfig.Management.RateLimiter
			}
		}
		specs = append(specs, spec)
	}

	return specs, nil
}

// legacySpecsFromConfig derives specs from the pre-Phase-2 configuration
// flags: management.rateLimiter, management.analytics, management.logging.
func legacySpecsFromConfig(gatewayConfig *config.GatewayConfig) []MiddlewareSpec {
	specs := []MiddlewareSpec{}

	// Rate limiter should run first, even before analytics.
	if gatewayConfig.Management.RateLimiter.IsEnabled() {
		specs = append(specs, MiddlewareSpec{
			Name:   config.MiddlewareNameRateLimiter,
			Config: gatewayConfig.Management.RateLimiter,
		})
	}

	// Analytics group: JA4H fingerprint -> session extraction -> traffic metrics.
	if gatewayConfig.Management.Analytics {
		specs = append(specs, MiddlewareSpec{Name: config.MiddlewareNameJA4Fingerprint})
		specs = append(specs, MiddlewareSpec{Name: config.MiddlewareNameSessionExtraction})
		specs = append(specs, MiddlewareSpec{Name: config.MiddlewareNameTrafficMetrics})
	}

	// Logging.
	if gatewayConfig.Management.Logging {
		specs = append(specs, MiddlewareSpec{Name: config.MiddlewareNameLogging})
	}

	return specs
}

// BuildGlobalChainFromConfigV2 resolves gateway configuration to an ordered
// list of MiddlewareSpec (via ResolveGlobalChainSpecs) and builds the
// resulting chain via the registry.
func BuildGlobalChainFromConfigV2(
	registry *MiddlewareRegistryV2,
	gatewayConfig *config.GatewayConfig,
) (*ChainBuilder, error) {
	specs, err := ResolveGlobalChainSpecs(gatewayConfig)
	if err != nil {
		return nil, err
	}
	return registry.BuildChain(specs)
}
