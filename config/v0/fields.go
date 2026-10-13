package v0

import yaml "gopkg.in/yaml.v3"

// renameFields applies the 1.0 field changes outside the middleware list:
// management.prefix is gone (fixed at "/_"), management.session.secondsDuration
// becomes durationSeconds, and each route drops `static` (implied by
// toFolder/toFile) and turns `authentication: {enabled: X}` into
// `authenticated: X`.
func renameFields(root *yaml.Node) bool {
	changed := false
	management := mappingValue(root, "management")
	if removeMappingKey(management, "prefix") != nil {
		changed = true
	}
	if session := mappingValue(management, "session"); renameMappingKey(session, "secondsDuration", "durationSeconds") {
		changed = true
	}

	routes := mappingValue(root, "routes")
	if routes == nil || routes.Kind != yaml.SequenceNode {
		return changed
	}
	for _, route := range routes.Content {
		if removeMappingKey(route, "static") != nil {
			changed = true
		}
		auth := mappingValue(route, "authentication")
		if auth == nil {
			continue
		}
		enabled := mappingValue(auth, "enabled")
		removeMappingKey(route, "authentication")
		changed = true
		if isTrue(enabled) {
			route.Content = append(route.Content, scalar("authenticated"), &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"})
		}
	}
	return changed
}

// renameMappingKey renames key in mapping m, keeping its position and value.
func renameMappingKey(m *yaml.Node, from, to string) bool {
	if m == nil || m.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == from {
			m.Content[i].Value = to
			return true
		}
	}
	return false
}
