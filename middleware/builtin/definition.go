package builtin

import (
	"bytes"
	"fmt"

	"github.com/jmaister/taronja-gateway/auth"
	"github.com/jmaister/taronja-gateway/db/repositories"
	"github.com/jmaister/taronja-gateway/middleware"
	"github.com/jmaister/taronja-gateway/session"
	yaml "gopkg.in/yaml.v3"
)

// GlobalDeps are the runtime services the built-in middleware use.
type GlobalDeps struct {
	SessionStore      session.SessionStore
	TokenService      *auth.TokenService
	TrafficMetricRepo repositories.TrafficMetricRepository
	RateLimiter       *RateLimiter
}

// Definition describes a middleware that can be listed under `middleware:`.
// Each one is declared next to its middleware and listed in definitions.
type Definition struct {
	Name        string
	Description string
	// DependsOn names middleware that must appear earlier in the list.
	DependsOn []string
	// Decode turns the entry's `config:` block (nil when absent) into the
	// options New expects, applying defaults and validating them. It needs no
	// runtime services, so `tg validate` can run it.
	Decode func(raw *yaml.Node) (any, error)
	// New builds the middleware from Decode's result.
	New func(opts any, d GlobalDeps) (middleware.Middleware, error)
	// Health reports runtime state. Optional: nil for stateless middleware.
	Health func(d GlobalDeps) middleware.MiddlewareHealth
}

// definitions lists every built-in middleware.
var definitions = []*Definition{
	&tracingDefinition,
	&compressionDefinition,
	&corsDefinition,
	&rateLimiterDefinition,
	&ja4Definition,
	&sessionExtractionDefinition,
	&trafficMetricsDefinition,
	&loggingDefinition,
}

func findDefinition(name string) *Definition {
	for _, def := range definitions {
		if def.Name == name {
			return def
		}
	}
	return nil
}

func definitionNames() []string {
	names := make([]string, len(definitions))
	for i, def := range definitions {
		names[i] = def.Name
	}
	return names
}

// Validator is implemented by option types that can check themselves.
type Validator interface {
	Validate() error
}

// decodeOptions returns a Definition.Decode that decodes the `config:` block
// over defaults, rejecting unknown keys, and runs Validate when C has one.
func decodeOptions[C any](defaults C) func(raw *yaml.Node) (any, error) {
	return func(raw *yaml.Node) (any, error) {
		cfg := defaults
		if raw != nil && raw.Kind != 0 {
			// yaml.Node.Decode cannot reject unknown keys, so round-trip
			// through a strict decoder.
			data, err := yaml.Marshal(raw)
			if err != nil {
				return nil, err
			}
			dec := yaml.NewDecoder(bytes.NewReader(data))
			dec.KnownFields(true)
			if err := dec.Decode(&cfg); err != nil {
				return nil, err
			}
		}
		if v, ok := any(&cfg).(Validator); ok {
			if err := v.Validate(); err != nil {
				return nil, err
			}
		}
		return cfg, nil
	}
}

// noOptions is the Decode of middleware without options: it accepts only an
// absent or empty `config:` block.
func noOptions(raw *yaml.Node) (any, error) {
	if raw != nil && raw.Kind != 0 && !(raw.Kind == yaml.MappingNode && len(raw.Content) == 0) {
		return nil, fmt.Errorf("takes no config")
	}
	return nil, nil
}

// stateless adapts a middleware that needs neither options nor services to
// Definition.New.
func stateless(mw middleware.Middleware) func(any, GlobalDeps) (middleware.Middleware, error) {
	return func(any, GlobalDeps) (middleware.Middleware, error) { return mw, nil }
}
