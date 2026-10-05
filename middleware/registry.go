package middleware

import (
	"fmt"
	"log"
	"sort"
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

// Registry builds middleware chains from declarative MiddlewareSpec lists.
// Factories must be registered before BuildChain, which checks that every spec's
// dependencies were built earlier in the same chain.
//
// Every method takes mu: BuildChain may be called again on a live registry while
// the admin endpoints read status and metrics, which would otherwise crash on
// concurrent map access.
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

// BuildChain builds a ChainBuilder from an ordered list of specs, creating each
// middleware from its factory. It errors on an unregistered name or a dependency
// not built by an earlier spec. Each call discards the previous call's
// built/metrics state.
func (r *Registry) BuildChain(specs []MiddlewareSpec) (*ChainBuilder, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.built = make(map[string]bool)
	r.metrics = make(map[string]*middlewareMetricsCounter)

	chain := NewChainBuilder()
	err := r.createAll(specs, func(name string, mw Middleware) {
		// Wrap with request-metrics instrumentation (see metrics.go) before
		// adding to the chain, so GetMetrics/GetAllMetrics can report on it.
		counter := &middlewareMetricsCounter{}
		r.metrics[name] = counter
		chain.Add(instrumentMiddleware(mw, counter))
		r.built[name] = true
		log.Printf("Added middleware to chain: %s", name)
	})
	if err != nil {
		return nil, err
	}
	return chain, nil
}

// NewChain is BuildChain without the status side effects: no instrumentation,
// no log line, and the registry's built/metrics state is left alone. Use it for
// registries that build many small chains (per-route middleware), where
// BuildChain's per-call reset would be meaningless.
func (r *Registry) NewChain(specs []MiddlewareSpec) (*ChainBuilder, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	chain := NewChainBuilder()
	if err := r.createAll(specs, func(_ string, mw Middleware) { chain.Add(mw) }); err != nil {
		return nil, err
	}
	return chain, nil
}

// createAll creates each spec's middleware in order, validating names and
// dependencies, and hands it to add. The caller holds r.mu.
func (r *Registry) createAll(specs []MiddlewareSpec, add func(name string, mw Middleware)) error {
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

		cfg := spec.Config
		if cfg == nil {
			var err error
			if cfg, err = factory.DecodeConfig(nil); err != nil {
				return fmt.Errorf("middleware '%s': %w", spec.Name, err)
			}
		}

		mw, err := factory.Create(cfg)
		if err != nil {
			return fmt.Errorf("failed to create middleware '%s': %w", spec.Name, err)
		}

		add(spec.Name, mw)
		built[spec.Name] = true
	}
	return nil
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

// Factory returns the factory registered under name.
func (r *Registry) Factory(name string) (MiddlewareFactory, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.factories[name]
	return f, ok
}

// Names returns every registered factory name, sorted.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
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
