package config

import (
	"fmt"

	yaml "gopkg.in/yaml.v3"
)

// MiddlewareConfig is one entry of the root `middleware` list. Entries run in list
// order; each middleware's options live in its own `config` block, decoded and
// validated by the middleware itself (see doc/middleware/).
type MiddlewareConfig struct {
	Name    string     `yaml:"name"`              // Middleware identifier, e.g. "rate_limiter". Required.
	Enabled *bool      `yaml:"enabled,omitempty"` // Default: true. Set false to keep an entry without running it.
	Config  *yaml.Node `yaml:"config,omitempty"`  // Middleware-specific options. Optional.
}

// UnmarshalYAML keeps `config` as a raw node for the middleware to decode. A
// strict decoder would otherwise try to match its keys against yaml.Node's
// own fields.
func (f *MiddlewareConfig) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: a middleware entry must be a mapping with a name", value.Line)
	}
	*f = MiddlewareConfig{}
	for i := 0; i+1 < len(value.Content); i += 2 {
		key, val := value.Content[i], value.Content[i+1]
		switch key.Value {
		case "name":
			if err := val.Decode(&f.Name); err != nil {
				return err
			}
		case "enabled":
			if err := val.Decode(&f.Enabled); err != nil {
				return err
			}
		case "config":
			f.Config = val
		default:
			return fmt.Errorf("line %d: unknown middleware field %q (expected name, enabled, config)", key.Line, key.Value)
		}
	}
	return nil
}

// IsEnabled reports whether the entry runs; an absent Enabled means true.
func (f MiddlewareConfig) IsEnabled() bool {
	return f.Enabled == nil || *f.Enabled
}

// validateMiddleware checks what the config package can check on its own: every
// entry has a name and none is listed twice. Names and options are checked by
// each middleware itself when the chain is resolved.
func validateMiddleware(entries []MiddlewareConfig) error {
	seen := make(map[string]bool, len(entries))
	for i, f := range entries {
		if f.Name == "" {
			return fmt.Errorf("middleware[%d]: missing 'name'", i)
		}
		if seen[f.Name] {
			return fmt.Errorf("middleware: '%s' is listed more than once", f.Name)
		}
		seen[f.Name] = true
	}
	return nil
}

// NewMiddlewareConfig builds a `middleware` entry for configs assembled in code;
// options (a struct or map, nil for none) become its config block.
func NewMiddlewareConfig(name string, options any) MiddlewareConfig {
	f := MiddlewareConfig{Name: name}
	if options != nil {
		var node yaml.Node
		if err := node.Encode(options); err != nil {
			panic(fmt.Sprintf("middleware %s: encoding options: %v", name, err))
		}
		f.Config = &node
	}
	return f
}
