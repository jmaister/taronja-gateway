package handlers

import (
	"github.com/jmaister/taronja-gateway/billing"
	"time"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/auth"
	"github.com/jmaister/taronja-gateway/db/repositories"
	"github.com/jmaister/taronja-gateway/featureflag"
	"github.com/jmaister/taronja-gateway/middleware/builtin"
	"github.com/jmaister/taronja-gateway/notification"
	"github.com/jmaister/taronja-gateway/session"
)

type StrictApiServer struct {
	// No dependencies needed here if middleware handles session validation
	// and places SessionData in context.
	sessionStore      session.SessionStore
	userRepo          repositories.UserRepository
	trafficMetricRepo repositories.TrafficMetricRepository
	tokenRepo         repositories.TokenRepository
	countersRepo      repositories.CountersRepository
	blockedClientRepo repositories.BlockedClientRepository
	tokenService      *auth.TokenService
	startTime         time.Time
	// rate limiter instance for stats/config endpoints
	rateLimiter *builtin.RateLimiter
	// global middleware chain for the middleware status/health/metrics
	// endpoints. May be nil in tests that don't need it.
	middlewareChain *builtin.GlobalChain
	// notificationService backs every /api/notifications* endpoint (see
	// handlers/api_notifications.go). May be nil in tests that don't need
	// it — every notification handler checks for that explicitly and
	// returns 503, the same way GetTelegramLinkCode does for an
	// unconfigured Telegram provider.
	notificationService *notification.Service
	// featureFlags backs /flags and /api/feature-flags*. Set with
	// SetFeatureFlags; nil in tests that don't call those endpoints.
	featureFlags *featureflag.Service
	// userPhones stores phone numbers (the sms/whatsapp address). Set with
	// SetUserPhones; nil in tests that don't use phones.
	userPhones repositories.UserPhoneRepository
	// organizations backs /api/organizations* and the organizations in
	// /me; nil when not set (those endpoints answer 503).
	organizations repositories.OrganizationRepository
	// billing backs every billing and entitlement endpoint; nil when billing
	// is disabled (they answer 503).
	billing *billing.Service
	// excludeStaticAssets mirrors the traffic_metrics middleware option so the
	// summary can tell the UI that static-asset requests are not recorded.
	excludeStaticAssets bool
}

// NewStrictApiServer creates a new StrictApiServer.
func NewStrictApiServer(sessionStore session.SessionStore, userRepo repositories.UserRepository, trafficMetricRepo repositories.TrafficMetricRepository, tokenRepo repositories.TokenRepository, countersRepo repositories.CountersRepository, blockedClientRepo repositories.BlockedClientRepository, tokenService *auth.TokenService, startTime time.Time, rateLimiter *builtin.RateLimiter, middlewareChain *builtin.GlobalChain, notificationService *notification.Service) *StrictApiServer {
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
		middlewareChain:     middlewareChain,
		notificationService: notificationService,
	}
}

// Ensure StrictApiServer implements StrictServerInterface
var _ api.StrictServerInterface = (*StrictApiServer)(nil)

// SetExcludeStaticAssets records the traffic_metrics excludeStaticAssets option for the statistics API.
func (s *StrictApiServer) SetExcludeStaticAssets(exclude bool) {
	s.excludeStaticAssets = exclude
}

// SetFeatureFlags sets the feature flag service for the feature flag endpoints.
func (s *StrictApiServer) SetFeatureFlags(featureFlags *featureflag.Service) {
	s.featureFlags = featureFlags
}

// SetOrganizations sets the organization store.
func (s *StrictApiServer) SetOrganizations(orgs repositories.OrganizationRepository) {
	s.organizations = orgs
}

// SetBilling sets the billing service.
func (s *StrictApiServer) SetBilling(svc *billing.Service) {
	s.billing = svc
}

// SetUserPhones sets the phone number store the user endpoints read and write.
func (s *StrictApiServer) SetUserPhones(userPhones repositories.UserPhoneRepository) {
	s.userPhones = userPhones
}
