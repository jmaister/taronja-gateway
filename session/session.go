package session

// contextKey is a custom type for context keys to avoid collisions.
type contextKey string

// SessionKey is the key used to store session in context.
const SessionKey contextKey = "session"
