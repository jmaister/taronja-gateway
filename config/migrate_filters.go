package config

import (
	yaml "gopkg.in/yaml.v3"
)

// moveFiltersToRoot rewrites the pre-1.0 filter settings into the root
// `filters` list: an explicit middleware.global list keeps its order, otherwise
// the management flags (and the root tracing section) map to filters in the
// order the gateway used to run them. The old keys are removed. It reports
// whether the config had any of them.
func moveFiltersToRoot(root *yaml.Node) bool {
	management := mappingValue(root, "management")
	middlewareSection := mappingValue(root, "middleware")
	global := mappingValue(middlewareSection, "global")
	tracing := mappingValue(root, "tracing")

	legacy := func(key string) *yaml.Node { return mappingValue(management, key) }
	legacyKeys := []string{"logging", "compression", "analytics", "excludeStaticAssets", "rateLimiter", "cors"}

	found := middlewareSection != nil || tracing != nil
	for _, key := range legacyKeys {
		if legacy(key) != nil {
			found = true
		}
	}
	if !found {
		return false
	}

	var filters []*yaml.Node
	if global != nil && global.Kind == yaml.SequenceNode {
		for _, entry := range global.Content {
			if f := filterFromGlobalEntry(entry, legacy, tracing); f != nil {
				filters = append(filters, f)
			}
		}
	} else {
		if isTrue(mappingValue(tracing, "enabled")) {
			filters = append(filters, filterNode("tracing", tracingOptions(tracing)))
		}
		if isTrue(legacy("compression")) {
			filters = append(filters, filterNode("compression", nil))
		}
		if cors := legacy("cors"); hasItems(mappingValue(cors, "allowedOrigins")) {
			filters = append(filters, filterNode("cors", cors))
		}
		if rl := legacy("rateLimiter"); rateLimiterEnabled(rl) {
			filters = append(filters, filterNode("rate_limiter", rl))
		}
		if isTrue(legacy("analytics")) {
			filters = append(filters,
				filterNode("ja4_fingerprint", nil),
				filterNode("session_extraction", nil),
				filterNode("traffic_metrics", trafficMetricsOptions(legacy("excludeStaticAssets"))),
			)
		}
		if isTrue(legacy("logging")) {
			filters = append(filters, filterNode("logging", nil))
		}
	}

	for _, key := range legacyKeys {
		removeMappingKey(management, key)
	}
	removeMappingKey(root, "middleware")
	removeMappingKey(root, "tracing")

	if len(filters) > 0 {
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "filters"},
			&yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Content: filters},
		)
	}
	return true
}

// filterFromGlobalEntry converts one middleware.global entry; options it did
// not override come from the matching management setting, as before.
func filterFromGlobalEntry(entry *yaml.Node, legacy func(string) *yaml.Node, tracing *yaml.Node) *yaml.Node {
	nameNode := mappingValue(entry, "name")
	if nameNode == nil {
		return nil
	}
	name := nameNode.Value
	var options *yaml.Node
	switch name {
	case "rate_limiter":
		options = firstNonNil(mappingValue(entry, "rateLimiter"), legacy("rateLimiter"))
	case "cors":
		options = firstNonNil(mappingValue(entry, "cors"), legacy("cors"))
	case "traffic_metrics":
		options = firstNonNil(mappingValue(entry, "trafficMetrics"), trafficMetricsOptions(legacy("excludeStaticAssets")))
	case "tracing":
		options = tracingOptions(tracing)
	}
	f := filterNode(name, options)
	if enabled := mappingValue(entry, "enabled"); enabled != nil {
		f.Content = append(f.Content[:2], append([]*yaml.Node{scalar("enabled"), enabled}, f.Content[2:]...)...)
	}
	return f
}

func filterNode(name string, options *yaml.Node) *yaml.Node {
	n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{scalar("name"), scalar(name)}}
	if options != nil && options.Kind == yaml.MappingNode && len(options.Content) > 0 {
		n.Content = append(n.Content, scalar("config"), options)
	}
	return n
}

// tracingOptions is the old tracing section without its enabled switch, which
// listing the filter now replaces.
func tracingOptions(tracing *yaml.Node) *yaml.Node {
	if tracing == nil || tracing.Kind != yaml.MappingNode {
		return nil
	}
	removeMappingKey(tracing, "enabled")
	return tracing
}

func trafficMetricsOptions(exclude *yaml.Node) *yaml.Node {
	if !isTrue(exclude) {
		return nil
	}
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{scalar("excludeStaticAssets"), exclude}}
}

// rateLimiterEnabled mirrors the old "any limit is set" rule. A block that
// cannot be decoded (e.g. ${VAR} placeholders) is kept rather than dropped.
func rateLimiterEnabled(rl *yaml.Node) bool {
	if rl == nil {
		return false
	}
	var probe struct {
		RequestsPerMinute int `yaml:"requestsPerMinute"`
		MaxErrors         int `yaml:"maxErrors"`
		VulnerabilityScan struct {
			URLs   []string `yaml:"urls"`
			Max404 int      `yaml:"max404"`
		} `yaml:"vulnerabilityScan"`
	}
	if err := rl.Decode(&probe); err != nil {
		return true
	}
	return probe.RequestsPerMinute > 0 || probe.MaxErrors > 0 ||
		probe.VulnerabilityScan.Max404 > 0 || len(probe.VulnerabilityScan.URLs) > 0
}

func isTrue(n *yaml.Node) bool {
	var b bool
	return n != nil && n.Kind == yaml.ScalarNode && n.Decode(&b) == nil && b
}

func hasItems(n *yaml.Node) bool {
	return n != nil && n.Kind == yaml.SequenceNode && len(n.Content) > 0
}

func firstNonNil(nodes ...*yaml.Node) *yaml.Node {
	for _, n := range nodes {
		if n != nil {
			return n
		}
	}
	return nil
}

func scalar(v string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v}
}
