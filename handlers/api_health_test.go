package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/gateway/deps"
	"github.com/stretchr/testify/assert"
)

func TestHealthCheck(t *testing.T) {
	// Create test dependencies - uses test database instead of memory repositories
	// Reset any existing DB connection to ensure clean state
	db.ResetConnection()
	testDeps := deps.NewTest()

	// Create StrictApiServer using dependencies
	s := NewStrictApiServer(
		testDeps.SessionStore,
		testDeps.UserRepo,
		testDeps.TrafficMetricRepo,
		testDeps.TokenRepo,
		testDeps.CountersRepo,
		testDeps.TokenService,
		testDeps.StartTime,
	)

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
