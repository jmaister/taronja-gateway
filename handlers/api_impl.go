package handlers

import (
	"time"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/auth"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/middleware"
	"github.com/jmaister/taronja-gateway/notification"
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
	blockedClientRepo db.BlockedClientRepository
	tokenService      *auth.TokenService
	startTime         time.Time
	// rate limiter instance for stats/config endpoints
	rateLimiter *middleware.RateLimiter
	// middleware registry for the middleware status/health/metrics endpoints
	// (see doc/refactor01.md Phase 3). May be nil in tests that don't need it.
	middlewareRegistry *middleware.MiddlewareRegistryV2
	// notificationService backs every /api/notifications* endpoint (see
	// handlers/api_notifications.go). May be nil in tests that don't need
	// it — every notification handler checks for that explicitly and
	// returns 503, the same way GetTelegramLinkCode does for an
	// unconfigured Telegram provider.
	notificationService *notification.Service
}

// NewStrictApiServer creates a new StrictApiServer.
func NewStrictApiServer(sessionStore session.SessionStore, userRepo db.UserRepository, trafficMetricRepo db.TrafficMetricRepository, tokenRepo db.TokenRepository, countersRepo db.CountersRepository, blockedClientRepo db.BlockedClientRepository, tokenService *auth.TokenService, startTime time.Time, rateLimiter *middleware.RateLimiter, middlewareRegistry *middleware.MiddlewareRegistryV2, notificationService *notification.Service) *StrictApiServer {
	return &StrictApiServer{
		sessionStore:        sessionStore,
		userRepo:            userRepo,
		trafficMetricRepo:   trafficMetricRepo,
		tokenRepo:           tokenRepo,
		countersRepo:        countersRepo,
		blockedClientRepo:   blockedClientRepo,
		tokenService:        tokenService,
		startTime:           startTime,
		rateLimiter:         rateLimiter,
		middlewareRegistry:  middlewareRegistry,
		notificationService: notificationService,
	}
}

// Ensure StrictApiServer implements StrictServerInterface
var _ api.StrictServerInterface = (*StrictApiServer)(nil)
