package handlers

import (
	"time"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/auth"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/middleware"
	"github.com/jmaister/taronja-gateway/session"
)

type StrictApiServer struct {
	// No dependencies needed here if middleware handles session validation
	// and places SessionData in context.
	sessionStore      session.SessionStore
	userRepo          db.UserRepository
	trafficMetricRepo db.TrafficMetricRepository
	tokenRepo         db.TokenRepository
	countersRepo      db.CountersRepository
	tokenService      *auth.TokenService
	startTime         time.Time
	// rate limiter instance for stats/config endpoints
	rateLimiter *middleware.RateLimiter
}

// NewStrictApiServer creates a new StrictApiServer.
func NewStrictApiServer(sessionStore session.SessionStore, userRepo db.UserRepository, trafficMetricRepo db.TrafficMetricRepository, tokenRepo db.TokenRepository, countersRepo db.CountersRepository, tokenService *auth.TokenService, startTime time.Time, rateLimiter *middleware.RateLimiter) *StrictApiServer {
	return &StrictApiServer{
		sessionStore:      sessionStore,
		userRepo:          userRepo,
		trafficMetricRepo: trafficMetricRepo,
		tokenRepo:         tokenRepo,
		countersRepo:      countersRepo,
		tokenService:      tokenService,
		startTime:         startTime,
		rateLimiter:       rateLimiter,
	}
}

// Ensure StrictApiServer implements StrictServerInterface
var _ api.StrictServerInterface = (*StrictApiServer)(nil)
