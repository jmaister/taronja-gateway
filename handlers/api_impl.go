package handlers

import (
	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/session"
)

// --- Strict API Server Implementation ---

// StrictApiServer provides an implementation of the api.StrictServerInterface.
type StrictApiServer struct {
	// No dependencies needed here if middleware handles session validation
	// and places SessionData in context.
	sessionStore session.SessionStore
}

// NewStrictApiServer creates a new StrictApiServer.
func NewStrictApiServer(sessionStore session.SessionStore) *StrictApiServer {
	return &StrictApiServer{
		sessionStore: sessionStore,
	}
}

// Ensure StrictApiServer implements StrictServerInterface
var _ api.StrictServerInterface = (*StrictApiServer)(nil)
