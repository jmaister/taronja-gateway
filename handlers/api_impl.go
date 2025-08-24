package handlers

import (
	"time"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/auth"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/session"
)

// --- Strict API Server Implementation ---

// StrictApiServer provides an implementation of the api.StrictServerInterface.
type StrictApiServer struct {
	// No dependencies needed here if middleware handles session validation
	// and places SessionData in context.
	sessionStore      session.SessionStore
	userRepo          db.UserRepository
	trafficMetricRepo db.TrafficMetricRepository
	tokenRepo         db.TokenRepository
	tokenService      *auth.TokenService
	startTime         time.Time
}

// NewStrictApiServer creates a new StrictApiServer.
func NewStrictApiServer(sessionStore session.SessionStore, userRepo db.UserRepository, trafficMetricRepo db.TrafficMetricRepository, tokenRepo db.TokenRepository, tokenService *auth.TokenService, startTime time.Time) *StrictApiServer {
	return &StrictApiServer{
		sessionStore:      sessionStore,
		userRepo:          userRepo,
		trafficMetricRepo: trafficMetricRepo,
		tokenRepo:         tokenRepo,
		tokenService:      tokenService,
		startTime:         startTime,
	}
}

// Ensure StrictApiServer implements StrictServerInterface
var _ api.StrictServerInterface = (*StrictApiServer)(nil)
