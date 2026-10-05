package config

import (
	"fmt"

	yaml "gopkg.in/yaml.v3"
)

// FilterConfig is one entry of the root `filters` list. Filters run in list
// order; each filter's options live in its own `config` block, decoded and
// validated by the filter itself (see doc/middleware/).
type FilterConfig struct {
	Name    string     `yaml:"name"`              // Filter identifier, e.g. "rate_limiter". Required.
	Enabled *bool      `yaml:"enabled,omitempty"` // Default: true. Set false to keep an entry without running it.
	Config  *yaml.Node `yaml:"config,omitempty"`  // Filter-specific options. Optional.
}

// UnmarshalYAML keeps `config` as a raw node for the filter to decode. A
// strict decoder would otherwise try to match its keys against yaml.Node's
// own fields.
func (f *FilterConfig) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: a filter must be a mapping with a name", value.Line)
	}
	*f = FilterConfig{}
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
			return fmt.Errorf("line %d: unknown filter field %q (expected name, enabled, config)", key.Line, key.Value)
		}
	}
	return nil
}

// IsEnabled reports whether the entry runs; an absent Enabled means true.
func (f FilterConfig) IsEnabled() bool {
	return f.Enabled == nil || *f.Enabled
}

// validateFilters checks what the config package can check on its own: every
// entry has a name and none is listed twice. Names and options are checked by
// the filters themselves when the chain is resolved.
func validateFilters(filters []FilterConfig) error {
	seen := make(map[string]bool, len(filters))
	for i, f := range filters {
		if f.Name == "" {
			return fmt.Errorf("filters[%d]: missing 'name'", i)
		}
		if seen[f.Name] {
			return fmt.Errorf("filters: '%s' is listed more than once", f.Name)
		}
		seen[f.Name] = true
	}
	return nil
}

// NewFilterConfig builds a `filters` entry for configs assembled in code;
// options (a struct or map, nil for none) become its config block.
func NewFilterConfig(name string, options any) FilterConfig {
	f := FilterConfig{Name: name}
	if options != nil {
		var node yaml.Node
		if err := node.Encode(options); err != nil {
			panic(fmt.Sprintf("filter %s: encoding options: %v", name, err))
		}
		f.Config = &node
	}
	return f
}
