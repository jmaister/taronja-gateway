// Package notification implements Taronja Gateway's generic notification
// system: one in-app record per notification (db.Notification), delivered
// over zero or more external channels (email, Telegram, ...) through a
// small Provider interface, with support for the user answering directly
// from a channel — a link in an email, a button in Telegram — rather than
// only from the in-app list.
//
// The gateway owns delivery mechanics only. What triggers a notification
// and what it says is entirely the calling app's decision: Type, Title,
// Body, URL, and Metadata are all caller-supplied and opaque to the
// gateway (Type isn't validated against any registry; Metadata is never
// interpreted, only stored and returned). This mirrors this codebase's
// existing OAuth2 provider architecture (providers/providers.go): a small
// shared interface, several independent implementations, one registry.
package notification

import (
	"encoding/json"
)

// Action is one possible answer to a notification — rendered as a button
// in Telegram and as a link in an email. ID is echoed back verbatim in
// Notification.RespondedActionID once chosen, so it should be a stable,
// caller-meaningful value ("approve"/"deny", not an array index that could
// shift meaning if the caller changes what it sends next time.
type Action struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// Style is an optional rendering hint ("primary", "danger", ...),
	// passed through as-is to whatever renders the action — the gateway
	// doesn't enforce a fixed set of values, the same way it doesn't
	// enforce one for Type.
	Style string `json:"style,omitempty"`
}

// EncodeActions marshals actions to the JSON form stored in
// db.Notification.Actions. A nil/empty slice encodes to "", not "[]" or
// "null", so db.Notification.Actions == "" unambiguously means "no
// actions" everywhere it's checked (SQL and Go alike).
func EncodeActions(actions []Action) (string, error) {
	if len(actions) == 0 {
		return "", nil
	}
	b, err := json.Marshal(actions)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// DecodeActions reverses EncodeActions. "" decodes to a nil slice.
func DecodeActions(encoded string) ([]Action, error) {
	if encoded == "" {
		return nil, nil
	}
	var actions []Action
	if err := json.Unmarshal([]byte(encoded), &actions); err != nil {
		return nil, err
	}
	return actions, nil
}

// findAction returns the action with the given ID, or nil if actions has
// none matching — used to validate a response names a real action before
// recording it.
func findAction(actions []Action, id string) *Action {
	for i := range actions {
		if actions[i].ID == id {
			return &actions[i]
		}
	}
	return nil
}

// EncodeMetadata marshals an arbitrary caller-supplied metadata map to the
// JSON form stored in db.Notification.Metadata. A nil/empty map encodes to
// "", matching EncodeActions' convention.
func EncodeMetadata(metadata map[string]interface{}) (string, error) {
	if len(metadata) == 0 {
		return "", nil
	}
	b, err := json.Marshal(metadata)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// DecodeMetadata reverses EncodeMetadata. "" decodes to a nil map.
func DecodeMetadata(encoded string) (map[string]interface{}, error) {
	if encoded == "" {
		return nil, nil
	}
	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(encoded), &metadata); err != nil {
		return nil, err
	}
	return metadata, nil
}
