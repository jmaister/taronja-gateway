// Package v0 upgrades a pre-v1.0.0 config file (schema "0.0", no version:
// field) to the 1.0 schema. The config package stamps the version afterwards.
package v0

import (
	"bytes"

	yaml "gopkg.in/yaml.v3"
)

// UpgradeToV1 flattens notification.email.smtp.*, moves the middleware
// settings into the root `middleware` list and applies the 1.0 field renames.
func UpgradeToV1(raw []byte) []byte {
	return editYAML(raw, flattenNotificationEmailSMTP, moveMiddlewareToRoot, renameFields)
}

// flattenNotificationEmailSMTP moves notification.email.smtp.* up to
// notification.email.*; configs without the block are left alone.
func flattenNotificationEmailSMTP(root *yaml.Node) bool {
	email := mappingValue(mappingValue(root, "notification"), "email")
	smtp := removeMappingKey(email, "smtp")
	if smtp == nil || smtp.Kind != yaml.MappingNode {
		return false
	}
	email.Content = append(email.Content, smtp.Content...)
	return true
}

// editYAML applies edits to raw's root mapping through yaml.Node, which keeps
// comments and ${VAR} placeholders. raw is returned byte-for-byte when no edit
// changes anything (or it does not parse); otherwise it is re-encoded with
// 2-space indentation, which may reformat quoting.
func editYAML(raw []byte, edits ...func(root *yaml.Node) bool) []byte {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil || len(doc.Content) == 0 {
		return raw
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return raw
	}
	changed := false
	for _, edit := range edits {
		if edit(root) {
			changed = true
		}
	}
	if !changed {
		return raw
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return raw
	}
	if err := enc.Close(); err != nil {
		return raw
	}
	return buf.Bytes()
}

// mappingValue returns the value node for key in mapping node m, or nil if
// m is nil, isn't a mapping, or has no such key.
func mappingValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// removeMappingKey deletes key (and its value) from mapping node m in
// place, returning the removed value node, or nil if m is nil, isn't a
// mapping, or has no such key.
func removeMappingKey(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			val := m.Content[i+1]
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
			return val
		}
	}
	return nil
}
