package deps

import (
	"time"

	"github.com/jmaister/taronja-gateway/auth"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/session"
	"gorm.io/gorm"
)

// Dependencies holds all application dependencies
type Dependencies struct {
	// Database connection
	DB *gorm.DB

	// Repositories
	UserRepo          db.UserRepository
	SessionRepo       db.SessionRepository
	TrafficMetricRepo db.TrafficMetricRepository
	TokenRepo         db.TokenRepository
	CountersRepo      db.CountersRepository
	BlockedClientRepo db.BlockedClientRepository

	// Services
	SessionStore session.SessionStore
	TokenService *auth.TokenService

	// Application state
	StartTime time.Time
}

// trafficMetricsBatchSize and trafficMetricsFlushInterval configure the
// db.BatchingTrafficMetricRepository NewProduction wraps TrafficMetricRepo
// in. Not user-configurable (unlike management.excludeStaticAssets): this
// is a pure implementation-detail efficiency change with no user-visible
// behavior difference beyond "up to this much recent traffic-metrics data
// can be lost on a hard crash instead of a graceful shutdown", so there's
// no meaningful trade-off for an operator to tune — 500ms/100 records is a
// small enough window that it isn't one in practice.
const (
	trafficMetricsBatchSize     = 100
	trafficMetricsFlushInterval = 500 * time.Millisecond
)

// Close releases resources that need an explicit, orderly shutdown rather
// than just being dropped — today, just flushing TrafficMetricRepo's
// buffered-but-not-yet-written records if it's batching (NewProduction
// wraps it in one; NewTest/NewTestWithName don't, so this is a no-op for
// test dependencies). Call once, after the HTTP server has stopped
// accepting new requests (e.g. after http.Server.Shutdown returns) so nothing
// is still calling Create concurrently.
func (d *Dependencies) Close() {
	if closer, ok := d.TrafficMetricRepo.(interface{ Close() }); ok {
		closer.Close()
	}
}

// NewProduction creates dependencies configured for production use
func NewProduction() *Dependencies {
	// Initialize database
	db.Init()
	gormDB := db.GetConnection()

	// Create repositories using database implementations
	userRepo := db.NewDBUserRepository(gormDB)
	sessionRepo := db.NewSessionRepositoryDB(gormDB)
	// Wrapped in a batcher: see the trafficMetricsBatchSize doc comment and
	// PERFORMANCE_ANALYSIS.md for why. Dependencies.Close flushes it on
	// shutdown.
	trafficMetricRepo := db.NewBatchingTrafficMetricRepository(
		db.NewTrafficMetricRepository(gormDB),
		trafficMetricsBatchSize,
		trafficMetricsFlushInterval,
	)
	tokenRepo := db.NewTokenRepositoryDB(gormDB)
	countersRepo := db.NewDBCountersRepository(gormDB)
	blockedClientRepo := db.NewBlockedClientRepositoryDB(gormDB)

	// Create session store with 24 hour duration
	sessionStore := session.NewSessionStore(sessionRepo, 24*time.Hour)

	// Create token service
	tokenService := auth.NewTokenService(tokenRepo, userRepo)

	return &Dependencies{
		DB:                gormDB,
		UserRepo:          userRepo,
		SessionRepo:       sessionRepo,
		TrafficMetricRepo: trafficMetricRepo,
		TokenRepo:         tokenRepo,
		CountersRepo:      countersRepo,
		BlockedClientRepo: blockedClientRepo,
		SessionStore:      sessionStore,
		TokenService:      tokenService,
		StartTime:         time.Now(),
	}
}

// NewTest creates dependencies configured for testing with a test database
func NewTest() *Dependencies {
	return NewTestWithName("test-dependencies")
}

// NewTestWithName creates dependencies configured for testing with a test database using a specific name
func NewTestWithName(testName string) *Dependencies {
	// Initialize test database with unique name for test isolation
	db.SetupTestDB(testName)
	gormDB := db.GetConnection()

	// Create repositories using database implementations (not memory!)
	userRepo := db.NewDBUserRepository(gormDB)
	sessionRepo := db.NewSessionRepositoryDB(gormDB)
	trafficMetricRepo := db.NewTrafficMetricRepository(gormDB)
	tokenRepo := db.NewTokenRepositoryDB(gormDB)
	countersRepo := db.NewDBCountersRepository(gormDB)
	blockedClientRepo := db.NewBlockedClientRepositoryDB(gormDB)

	// Create session store with 1 hour duration for tests
	sessionStore := session.NewSessionStore(sessionRepo, 1*time.Hour)

	// Create token service
	tokenService := auth.NewTokenService(tokenRepo, userRepo)

	return &Dependencies{
		DB:                gormDB,
		UserRepo:          userRepo,
		SessionRepo:       sessionRepo,
		TrafficMetricRepo: trafficMetricRepo,
		TokenRepo:         tokenRepo,
		CountersRepo:      countersRepo,
		BlockedClientRepo: blockedClientRepo,
		SessionStore:      sessionStore,
		TokenService:      tokenService,
		StartTime:         time.Now(),
	}
}
