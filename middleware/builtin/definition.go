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
type Definition struct {
	Name        string
	Description string
	DependsOn   []string                                                    // must appear earlier in the list
	Decode      func(raw *yaml.Node) (any, error)                           // `config:` block to validated options
	New         func(opts any, d GlobalDeps) (middleware.Middleware, error) // builds it from Decode's result
	Health      func(d GlobalDeps) middleware.MiddlewareHealth              // optional
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

// decodeOptions decodes over defaults, rejects unknown keys and runs Validate.
func decodeOptions[C any](defaults C) func(raw *yaml.Node) (any, error) {
	return func(raw *yaml.Node) (any, error) {
		cfg := defaults
		if raw != nil && raw.Kind != 0 {
			// yaml.Node.Decode cannot reject unknown keys: re-decode strictly.
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

// noOptions rejects any non-empty `config:` block.
func noOptions(raw *yaml.Node) (any, error) {
	if raw != nil && raw.Kind != 0 && !(raw.Kind == yaml.MappingNode && len(raw.Content) == 0) {
		return nil, fmt.Errorf("takes no config")
	}
	return nil, nil
}

// stateless is the New of middleware without options or services.
func stateless(mw middleware.Middleware) func(any, GlobalDeps) (middleware.Middleware, error) {
	return func(any, GlobalDeps) (middleware.Middleware, error) { return mw, nil }
}
