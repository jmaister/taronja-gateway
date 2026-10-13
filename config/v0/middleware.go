package v0

import (
	yaml "gopkg.in/yaml.v3"
)

// moveMiddlewareToRoot rewrites the pre-1.0 middleware settings into the root
// `middleware` list: an old `middleware.global` list keeps its order, otherwise
// the management flags (and the root tracing section) become entries in the
// order the gateway used to run them. The old keys are removed. It reports
// whether the config had any of them.
func moveMiddlewareToRoot(root *yaml.Node) bool {
	management := mappingValue(root, "management")
	var oldSection *yaml.Node
	if m := mappingValue(root, "middleware"); m != nil && m.Kind == yaml.MappingNode {
		oldSection = m
	}
	global := mappingValue(oldSection, "global")
	tracing := mappingValue(root, "tracing")

	legacy := func(key string) *yaml.Node { return mappingValue(management, key) }
	legacyKeys := []string{"logging", "compression", "analytics", "excludeStaticAssets", "rateLimiter", "cors"}

	found := oldSection != nil || tracing != nil
	for _, key := range legacyKeys {
		if legacy(key) != nil {
			found = true
		}
	}
	if !found {
		return false
	}

	var entries []*yaml.Node
	if global != nil && global.Kind == yaml.SequenceNode {
		for _, entry := range global.Content {
			if e := middlewareFromGlobalEntry(entry, legacy, tracing); e != nil {
				entries = append(entries, e)
			}
		}
	} else {
		if isTrue(mappingValue(tracing, "enabled")) {
			entries = append(entries, middlewareNode("tracing", tracingOptions(tracing)))
		}
		if isTrue(legacy("compression")) {
			entries = append(entries, middlewareNode("compression", nil))
		}
		if cors := legacy("cors"); hasItems(mappingValue(cors, "allowedOrigins")) {
			entries = append(entries, middlewareNode("cors", cors))
		}
		if rl := legacy("rateLimiter"); rateLimiterEnabled(rl) {
			entries = append(entries, middlewareNode("rate_limiter", rl))
		}
		if isTrue(legacy("analytics")) {
			entries = append(entries,
				middlewareNode("ja4_fingerprint", nil),
				middlewareNode("session_extraction", nil),
				middlewareNode("traffic_metrics", trafficMetricsOptions(legacy("excludeStaticAssets"))),
			)
		}
		if isTrue(legacy("logging")) {
			entries = append(entries, middlewareNode("logging", nil))
		}
	}

	for _, key := range legacyKeys {
		removeMappingKey(management, key)
	}
	if oldSection != nil {
		removeMappingKey(root, "middleware")
	}
	removeMappingKey(root, "tracing")

	if len(entries) == 0 {
		return true
	}
	if list := mappingValue(root, "middleware"); list != nil && list.Kind == yaml.SequenceNode {
		list.Content = append(list.Content, entries...)
		return true
	}
	root.Content = append(root.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "middleware"},
		&yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Content: entries},
	)
	return true
}

// middlewareFromGlobalEntry converts one middleware.global entry; options it did
// not override come from the matching management setting, as before.
func middlewareFromGlobalEntry(entry *yaml.Node, legacy func(string) *yaml.Node, tracing *yaml.Node) *yaml.Node {
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
	f := middlewareNode(name, options)
	if enabled := mappingValue(entry, "enabled"); enabled != nil {
		f.Content = append(f.Content[:2], append([]*yaml.Node{scalar("enabled"), enabled}, f.Content[2:]...)...)
	}
	return f
}

func middlewareNode(name string, options *yaml.Node) *yaml.Node {
	n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{scalar("name"), scalar(name)}}
	if options != nil && options.Kind == yaml.MappingNode && len(options.Content) > 0 {
		n.Content = append(n.Content, scalar("config"), options)
	}
	return n
}

// tracingOptions is the old tracing section without its enabled switch, which
// listing the middleware now replaces.
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
