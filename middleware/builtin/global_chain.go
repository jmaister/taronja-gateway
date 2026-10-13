package builtin

import (
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/middleware"
)

// GlobalChain is the config's `middleware:` list resolved against the
// built-in definitions: the enabled entries, in order, with their options
// decoded. Resolving needs no runtime services; Build then creates the
// middleware. Build runs once, before serving, so the admin API reads the
// chain without locking.
type GlobalChain struct {
	entries []globalEntry
	deps    GlobalDeps
}

type globalEntry struct {
	def     *Definition
	options any
	metrics *middleware.Counter
}

// MiddlewareStatus describes one built-in middleware for the admin API.
type MiddlewareStatus struct {
	Name         string
	Description  string
	Status       string // "active" or "available"
	Enabled      bool
	Dependencies []string
	Health       *middleware.MiddlewareHealth // nil when the middleware has no health check
}

// ResolveGlobalChain checks every enabled entry's name, options and
// dependencies.
func ResolveGlobalChain(cfg *config.GatewayConfig) (*GlobalChain, error) {
	chain := &GlobalChain{}
	for _, entry := range cfg.Middleware {
		if !entry.IsEnabled() {
			continue
		}
		def := findDefinition(entry.Name)
		if def == nil {
			return nil, fmt.Errorf("middleware: unknown middleware '%s' (known: %s)", entry.Name, strings.Join(definitionNames(), ", "))
		}
		for _, dep := range def.DependsOn {
			if !chain.Enabled(dep) {
				return nil, fmt.Errorf("middleware: '%s' depends on '%s', which must be enabled earlier in the list", def.Name, dep)
			}
		}
		options, err := def.Decode(entry.Config)
		if err != nil {
			return nil, fmt.Errorf("middleware: %s: %w", def.Name, err)
		}
		chain.entries = append(chain.entries, globalEntry{def: def, options: options, metrics: &middleware.Counter{}})
	}
	return chain, nil
}

// Build creates every middleware and returns them as one, each instrumented
// with its request metrics.
func (c *GlobalChain) Build(d GlobalDeps) (middleware.Middleware, error) {
	c.deps = d
	mws := make([]middleware.Middleware, 0, len(c.entries))
	for _, e := range c.entries {
		mw, err := e.def.New(e.options, d)
		if err != nil {
			return nil, fmt.Errorf("middleware: %s: %w", e.def.Name, err)
		}
		mws = append(mws, e.metrics.Instrument(mw))
		log.Printf("Added middleware to chain: %s", e.def.Name)
	}
	return func(next http.Handler) http.Handler { return middleware.Chain(next, mws...) }, nil
}

// Names returns the enabled middleware, in chain order.
func (c *GlobalChain) Names() []string {
	names := make([]string, len(c.entries))
	for i, e := range c.entries {
		names[i] = e.def.Name
	}
	return names
}

// Enabled reports whether the named middleware is in the chain.
func (c *GlobalChain) Enabled(name string) bool {
	return c.entry(name) != nil
}

func (c *GlobalChain) entry(name string) *globalEntry {
	for i := range c.entries {
		if c.entries[i].def.Name == name {
			return &c.entries[i]
		}
	}
	return nil
}

// GlobalOptions returns the named middleware's decoded options and whether it
// is enabled; the zero C when it is not. Code outside the chain that depends
// on a middleware's options (the shared rate limiter, tracing setup, the
// statistics API) reads them here so it always agrees with the chain.
func GlobalOptions[C any](c *GlobalChain, name string) (C, bool) {
	var zero C
	e := c.entry(name)
	if e == nil {
		return zero, false
	}
	return e.options.(C), true
}

// Status describes every built-in middleware, sorted by name.
func (c *GlobalChain) Status() []MiddlewareStatus {
	out := make([]MiddlewareStatus, 0, len(definitions))
	for _, def := range definitions {
		deps := def.DependsOn
		if deps == nil {
			deps = []string{} // the API schema requires an array, not null
		}
		s := MiddlewareStatus{
			Name:         def.Name,
			Description:  def.Description,
			Status:       "available",
			Enabled:      c.Enabled(def.Name),
			Dependencies: deps,
		}
		if s.Enabled {
			s.Status = "active"
		}
		if def.Health != nil {
			h := def.Health(c.deps)
			s.Health = &h
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Metrics returns the request metrics of the named middleware. ok is false
// when no built-in middleware has that name; one that is not enabled reports
// zeros.
func (c *GlobalChain) Metrics(name string) (snap middleware.MiddlewareMetricsSnapshot, ok bool) {
	if findDefinition(name) == nil {
		return snap, false
	}
	if e := c.entry(name); e != nil {
		return e.metrics.Snapshot(name), true
	}
	return middleware.MiddlewareMetricsSnapshot{Name: name}, true
}

// AllMetrics returns the request metrics of every enabled middleware, sorted
// by name.
func (c *GlobalChain) AllMetrics() []middleware.MiddlewareMetricsSnapshot {
	out := make([]middleware.MiddlewareMetricsSnapshot, 0, len(c.entries))
	for _, e := range c.entries {
		out = append(out, e.metrics.Snapshot(e.def.Name))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
