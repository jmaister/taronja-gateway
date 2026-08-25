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

// BuildGlobalChainFromConfigV2 translates gateway configuration into an
// ordered list of MiddlewareSpec and builds the resulting chain via the
// registry. This mirrors the conditional logic in BuildGlobalChain, but
// expressed declaratively so it can be introspected and tested independently
// of the concrete middleware implementations.
func BuildGlobalChainFromConfigV2(
	registry *MiddlewareRegistryV2,
	gatewayConfig *config.GatewayConfig,
) (*ChainBuilder, error) {
	specs := []MiddlewareSpec{}

	// Rate limiter should run first, even before analytics.
	if gatewayConfig.Management.RateLimiter.IsEnabled() {
		specs = append(specs, MiddlewareSpec{
			Name:   "rate_limiter",
			Config: gatewayConfig.Management.RateLimiter,
		})
	}

	// Analytics group: JA4H fingerprint -> session extraction -> traffic metrics.
	if gatewayConfig.Management.Analytics {
		specs = append(specs, MiddlewareSpec{Name: "ja4_fingerprint"})
		specs = append(specs, MiddlewareSpec{Name: "session_extraction"})
		specs = append(specs, MiddlewareSpec{Name: "traffic_metrics"})
	}

	// Logging.
	if gatewayConfig.Management.Logging {
		specs = append(specs, MiddlewareSpec{Name: "logging"})
	}

	return registry.BuildChain(specs)
}
