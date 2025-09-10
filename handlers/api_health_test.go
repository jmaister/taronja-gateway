package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/auth"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/stretchr/testify/assert"
)

func TestHealthCheck(t *testing.T) {
	// Create a new StrictApiServer instance with all required dependencies
	userRepo := db.NewMemoryUserRepository()
	sessionRepo := db.NewMemorySessionRepository()
	sessionStore := session.NewSessionStore(sessionRepo, 24*time.Hour)
	trafficMetricRepo := db.NewMemoryTrafficMetricRepository(userRepo)
	tokenRepo := db.NewTokenRepositoryMemory()
	tokenService := auth.NewTokenService(tokenRepo, userRepo)

	// For tests, we can use a nil database connection since we're using memory repositories
	startTime := time.Now()

	s := NewStrictApiServer(sessionStore, userRepo, trafficMetricRepo, tokenRepo, tokenService, startTime)

	t.Run("SuccessfulHealthCheck", func(t *testing.T) {
		// Setup: Create a health check request
		ctx := context.Background()
		req := api.HealthCheckRequestObject{}

		// Record the time before the call
		beforeCall := time.Now()

		// Execute the health check
		resp, err := s.HealthCheck(ctx, req)

		// Record the time after the call
		afterCall := time.Now()

		// Assert: No error should occur
		assert.NoError(t, err)
		assert.NotNil(t, resp)

		// Assert: Response should be of the correct type
		healthResp, ok := resp.(api.HealthCheck200JSONResponse)
		assert.True(t, ok, "Response should be HealthCheck200JSONResponse")

		// Assert: Status should be "ok"
		assert.Equal(t, "ok", healthResp.Status)

		// Assert: Uptime should be present and non-empty
		assert.NotEmpty(t, healthResp.Uptime)

		// Assert: Timestamp should be within a reasonable range (between before and after the call)
		assert.True(t, healthResp.Timestamp.After(beforeCall) || healthResp.Timestamp.Equal(beforeCall),
			"Timestamp should be after or equal to the time before the call")
		assert.True(t, healthResp.Timestamp.Before(afterCall) || healthResp.Timestamp.Equal(afterCall),
			"Timestamp should be before or equal to the time after the call")

		// Assert: Timestamp should not be zero
		assert.False(t, healthResp.Timestamp.IsZero(), "Timestamp should not be zero")
	})

}
