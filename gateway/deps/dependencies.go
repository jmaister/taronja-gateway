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
	CreditsRepo       db.CreditsRepository

	// Services
	SessionStore session.SessionStore
	TokenService *auth.TokenService

	// Application state
	StartTime time.Time
}

// NewProduction creates dependencies configured for production use
func NewProduction() *Dependencies {
	// Initialize database
	db.Init()
	gormDB := db.GetConnection()

	// Create repositories using database implementations
	userRepo := db.NewDBUserRepository(gormDB)
	sessionRepo := db.NewSessionRepositoryDB(gormDB)
	trafficMetricRepo := db.NewTrafficMetricRepository(gormDB)
	tokenRepo := db.NewTokenRepositoryDB(gormDB)
	creditsRepo := db.NewDBCreditsRepository(gormDB)

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
		CreditsRepo:       creditsRepo,
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
	creditsRepo := db.NewDBCreditsRepository(gormDB)

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
		CreditsRepo:       creditsRepo,
		SessionStore:      sessionStore,
		TokenService:      tokenService,
		StartTime:         time.Now(),
	}
}
