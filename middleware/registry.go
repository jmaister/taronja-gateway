package middleware

import (
	"fmt"
	"log"
	"sync"
)

// MiddlewareSpec describes one middleware to build: which factory to use (by
// name) and the configuration to pass to it. A slice of specs is an ordered,
// declarative description of a middleware chain.
type MiddlewareSpec struct {
	Name   string      `json:"name" yaml:"name"`
	Config interface{} `json:"config" yaml:"config"`
}

// MiddlewareStatus describes the status of a middleware known to a Registry.
type MiddlewareStatus struct {
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	Status       string            `json:"status"` // "active" or "available"
	Enabled      bool              `json:"enabled"`
	Dependencies []string          `json:"dependencies"`
	Health       *MiddlewareHealth `json:"health,omitempty"` // nil if the factory doesn't implement HealthChecker
}

// Registry builds middleware chains from declarative MiddlewareSpec
// lists. Factories must be registered before BuildChain is called; BuildChain
// validates that every spec's dependencies were already satisfied earlier in
// the same chain and returns a ready-to-use ChainBuilder.
//
// Every method takes mu — currently, in this codebase's actual gateway/
// setup.go, a registry is always fresh-built (builtin.NewGlobalChain),
// have BuildChain called on it once, and only then published where a
// concurrent GetStatus/GetMetrics/GetAllMetrics call (the admin dashboard's
// middleware-status/metrics endpoints) could ever reach it — so no two
// goroutines actually touch factories/built/metrics at once today. But
// BuildChain's own doc comment documents calling it again on an
// already-published, already-live registry (e.g. rebuilding one chain's
// specs in place, rather than replacing the whole registry) as supported,
// legitimate reuse — a future caller relying on that documented contract
// while requests are concurrently hitting the admin endpoints above would
// hit Go's classic unsynchronized-concurrent-map-access crash
// ("fatal error: concurrent map read and map write"), not a data race
// subtle enough to go unnoticed. Guarding every access is the difference
// between a documented capability and a landmine sitting behind it.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]MiddlewareFactory
	built     map[string]bool
	metrics   map[string]*middlewareMetricsCounter
}

// NewRegistry creates an empty registry with no factories registered.
func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]MiddlewareFactory),
		built:     make(map[string]bool),
		metrics:   make(map[string]*middlewareMetricsCounter),
	}
}

// RegisterFactory registers a middleware factory under its own name. Returns
// an error if a factory with the same name is already registered.
func (r *Registry) RegisterFactory(factory MiddlewareFactory) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := factory.GetName()
	if _, exists := r.factories[name]; exists {
		return fmt.Errorf("middleware factory '%s' already registered", name)
	}
	r.factories[name] = factory
	return nil
}

// BuildChain builds a ChainBuilder from an ordered list of MiddlewareSpec.
// For each spec, it looks up the registered factory, verifies all of the
// factory's declared dependencies were built by an earlier spec in the same
// call, creates the middleware instance, and appends it to the chain.
//
// Returns an error if a spec names an unregistered middleware or if a
// dependency is not satisfied by an earlier spec.
//
// Calling BuildChain again on the same registry
// discards the built/metrics state of any previous call first, so GetStatus
// and GetMetrics/GetAllMetrics always reflect only the most recently built
// chain rather than accumulating "active" middleware across calls.
func (r *Registry) BuildChain(specs []MiddlewareSpec) (*ChainBuilder, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.built = make(map[string]bool)
	r.metrics = make(map[string]*middlewareMetricsCounter)

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

		// Wrap with request-metrics instrumentation (see metrics.go) before
		// adding to the chain, so GetMetrics/GetAllMetrics can report on it.
		counter := &middlewareMetricsCounter{}
		r.metrics[spec.Name] = counter
		chain.Add(instrumentMiddleware(mw, counter))

		built[spec.Name] = true
		r.built[spec.Name] = true
		log.Printf("Added middleware to chain: %s", spec.Name)
	}

	return chain, nil
}

// GetStatus returns the status of every registered factory: "active" if it
// was included in the most recent BuildChain call, "available" otherwise.
func (r *Registry) GetStatus() map[string]MiddlewareStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	status := make(map[string]MiddlewareStatus, len(r.factories))

	for name, factory := range r.factories {
		enabled := r.built[name]
		// Dependencies is declared as a required (non-nullable) array in the
		// OpenAPI schema, but factories with no dependencies (e.g. CORSFactory)
		// leave their dependencies field as a nil slice, which encoding/json
		// marshals as `null` rather than `[]` since the field has no
		// omitempty. That broke the admin console: item.dependencies.length
		// throws on null. Normalize nil to an empty, non-nil slice here so
		// GetStatus's contract always matches the schema.
		deps := factory.GetDependencies()
		if deps == nil {
			deps = []string{}
		}
		s := MiddlewareStatus{
			Name:         name,
			Description:  factory.GetDescription(),
			Enabled:      enabled,
			Dependencies: deps,
		}
		if enabled {
			s.Status = "active"
		} else {
			s.Status = "available"
		}
		if hc, ok := factory.(HealthChecker); ok {
			h := hc.HealthCheck()
			s.Health = &h
		}
		status[name] = s
	}

	return status
}

// Factories returns every factory currently registered, in no particular
// order (they're stored in a map, keyed by name — see RegisterFactory).
func (r *Registry) Factories() []MiddlewareFactory {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]MiddlewareFactory, 0, len(r.factories))
	for _, f := range r.factories {
		out = append(out, f)
	}
	return out
}

// ValidateSpecs checks that every spec names a registered factory and that
// its dependencies are satisfied by an earlier spec in the same list —
// without creating any middleware instances. This is the same dependency
// graph check BuildChain performs, exposed separately so configuration can be
// validated at startup before real dependencies (session store, DB
// repositories, rate limiter instance, ...) are available to actually build
// the chain.
func (r *Registry) ValidateSpecs(specs []MiddlewareSpec) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

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
