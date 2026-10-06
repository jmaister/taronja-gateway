package middleware

import (
	"bytes"
	"fmt"

	yaml "gopkg.in/yaml.v3"
)

// MiddlewareFactory is one middleware: its identity, its configuration and how to
// build it. A Registry composes factories into chains.
type MiddlewareFactory interface {
	// GetName returns the identifier used in the config's `middleware[].name`.
	GetName() string
	// GetDescription returns a short human-readable summary of what the middleware does.
	GetDescription() string
	// GetDependencies returns the names of middleware that must appear earlier in
	// the same chain.
	GetDependencies() []string
	// DecodeConfig turns the middleware's `config:` block (nil when absent) into
	// the value Create expects, applying defaults and validating it.
	DecodeConfig(raw *yaml.Node) (interface{}, error)
	// Create builds the middleware from a value returned by DecodeConfig.
	Create(cfg interface{}) (Middleware, error)
}

// ConcreteFactory is an embeddable base providing the name, description and
// dependencies bookkeeping.
type ConcreteFactory struct {
	name         string
	description  string
	dependencies []string
}

// NewConcreteFactory returns the base for a factory with the given name,
// description and optional dependencies.
func NewConcreteFactory(name, description string, dependencies ...string) ConcreteFactory {
	return ConcreteFactory{name: name, description: description, dependencies: dependencies}
}

func (f *ConcreteFactory) GetName() string           { return f.name }
func (f *ConcreteFactory) GetDescription() string    { return f.description }
func (f *ConcreteFactory) GetDependencies() []string { return f.dependencies }

// Validator is implemented by middleware config types that can check themselves.
// DecodeConfigInto calls it after decoding.
type Validator interface {
	Validate() error
}

// DecodeConfigInto decodes raw over defaults, rejecting unknown keys, and runs
// the result's Validate method when it has one. A nil raw yields defaults.
func DecodeConfigInto[C any](raw *yaml.Node, defaults C) (C, error) {
	cfg := defaults
	if raw != nil && raw.Kind != 0 {
		// yaml.Node.Decode cannot reject unknown keys, so round-trip through a
		// strict decoder.
		data, err := yaml.Marshal(raw)
		if err != nil {
			return cfg, err
		}
		dec := yaml.NewDecoder(bytes.NewReader(data))
		dec.KnownFields(true)
		if err := dec.Decode(&cfg); err != nil {
			return cfg, err
		}
	}
	if v, ok := any(&cfg).(Validator); ok {
		if err := v.Validate(); err != nil {
			return cfg, err
		}
	}
	return cfg, nil
}

// NoConfig is embedded by factories whose middleware takes no options.
type NoConfig struct{}

// DecodeConfig accepts only an absent or empty `config:` block.
func (NoConfig) DecodeConfig(raw *yaml.Node) (interface{}, error) {
	if raw != nil && raw.Kind != 0 && !(raw.Kind == yaml.MappingNode && len(raw.Content) == 0) {
		return nil, fmt.Errorf("takes no config")
	}
	return struct{}{}, nil
}

// ConfigAs returns cfg as C, or an error naming the middleware when Create is
// handed a value its DecodeConfig did not produce.
func ConfigAs[C any](name string, cfg interface{}) (C, error) {
	c, ok := cfg.(C)
	if !ok {
		var zero C
		return zero, fmt.Errorf("%s: invalid config type %T, expected %T", name, cfg, zero)
	}
	return c, nil
}
