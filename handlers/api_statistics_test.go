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

func setupStatsTestServer() (*StrictApiServer, *db.TrafficMetricRepositoryMemory) {
	sessionRepo := db.NewMemorySessionRepository()
	sessionStore := session.NewSessionStore(sessionRepo)
	userRepo := db.NewMemoryUserRepository()
	userLoginRepo := db.NewUserLoginRepositoryMemory(userRepo)
	statsRepo := db.NewMemoryTrafficMetricRepository(userRepo)
	tokenRepo := db.NewTokenRepositoryMemory()
	tokenService := auth.NewTokenService(tokenRepo, userRepo)

	// For tests, we can use a nil database connection since we're using memory repositories
	startTime := time.Now()

	server := NewStrictApiServer(sessionStore, userRepo, userLoginRepo, statsRepo, tokenRepo, tokenService, startTime)
	return server, statsRepo
}

func TestGetRequestStatistics_Unauthorized(t *testing.T) {
	server, _ := setupStatsTestServer()

	// Test without authentication
	ctx := context.Background()
	request := api.GetRequestStatisticsRequestObject{}

	response, err := server.GetRequestStatistics(ctx, request)
	assert.NoError(t, err)

	// Should return 401 Unauthorized
	_, ok := response.(api.GetRequestStatistics401JSONResponse)
	assert.True(t, ok, "Expected 401 Unauthorized response")
}

func TestGetRequestStatistics_NonAdmin(t *testing.T) {
	server, _ := setupStatsTestServer()

	// Create a non-admin session
	sessionData := &db.Session{
		Token:           "test-token",
		UserID:          "user123",
		Username:        "testuser",
		IsAuthenticated: true,
		IsAdmin:         false, // Not admin
		ValidUntil:      time.Now().Add(time.Hour),
	}

	ctx := context.WithValue(context.Background(), session.SessionKey, sessionData)
	request := api.GetRequestStatisticsRequestObject{}

	response, err := server.GetRequestStatistics(ctx, request)
	assert.NoError(t, err)

	// Should return 401 Unauthorized for non-admin users
	_, ok := response.(api.GetRequestStatistics401JSONResponse)
	assert.True(t, ok, "Expected 401 Unauthorized response for non-admin user")
}

func TestGetRequestStatistics_Success(t *testing.T) {
	server, statsRepo := setupStatsTestServer()

	// Create an admin session
	sessionData := &db.Session{
		Token:           "admin-token",
		UserID:          "admin123",
		Username:        "admin",
		IsAuthenticated: true,
		IsAdmin:         true, // Admin user
		ValidUntil:      time.Now().Add(time.Hour),
	}

	// Add some test traffic metrics
	now := time.Now()
	testMetrics := []*db.TrafficMetric{
		{
			HttpMethod:     "GET",
			Path:           "/api/test",
			HttpStatus:     200,
			ResponseTimeNs: 1000000000, // 1 second in nanoseconds
			ResponseSize:   1024,       // 1KB
			Timestamp:      now.Add(-time.Hour),
			ClientInfo: db.ClientInfo{
				Country:        "US",
				DeviceFamily:   "desktop",
				OSFamily:       "Windows",
				BrowserFamily:  "Chrome",
				JA4Fingerprint: "ge11nn05_9c68f7ca5aaf_d4bd6ad6f3ac",
			},
		},
		{
			HttpMethod:     "POST",
			Path:           "/api/create",
			HttpStatus:     201,
			ResponseTimeNs: 2000000000, // 2 seconds in nanoseconds
			ResponseSize:   2048,       // 2KB
			Timestamp:      now.Add(-30 * time.Minute),
			ClientInfo: db.ClientInfo{
				Country:        "ES",
				DeviceFamily:   "mobile",
				OSFamily:       "Android",
				BrowserFamily:  "Firefox",
				JA4Fingerprint: "ge11nn05_7f3e9c2a1f8b_a9e7b3d4c2f1",
			},
		},
		{
			HttpMethod:     "GET",
			Path:           "/api/error",
			HttpStatus:     404,
			ResponseTimeNs: 500000000, // 0.5 seconds in nanoseconds
			ResponseSize:   512,       // 0.5KB
			Timestamp:      now.Add(-15 * time.Minute),
			ClientInfo: db.ClientInfo{
				Country:        "US",
				DeviceFamily:   "tablet",
				OSFamily:       "iOS",
				BrowserFamily:  "Safari",
				JA4Fingerprint: "ge11nn05_9c68f7ca5aaf_d4bd6ad6f3ac", // Same as first request
			},
		},
	}

	for _, metric := range testMetrics {
		err := statsRepo.Create(metric)
		assert.NoError(t, err)
	}

	ctx := context.WithValue(context.Background(), session.SessionKey, sessionData)

	// Test with date range parameters
	startDate := now.Add(-2 * time.Hour)
	endDate := now
	request := api.GetRequestStatisticsRequestObject{
		Params: api.GetRequestStatisticsParams{
			StartDate: &startDate,
			EndDate:   &endDate,
		},
	}

	response, err := server.GetRequestStatistics(ctx, request)
	assert.NoError(t, err)

	// Should return 200 OK with statistics
	successResponse, ok := response.(api.GetRequestStatistics200JSONResponse)
	assert.True(t, ok, "Expected 200 OK response")

	stats := api.RequestStatistics(successResponse)

	// Verify the statistics
	assert.Equal(t, 3, stats.TotalRequests)
	assert.Contains(t, stats.RequestsByStatus, "200")
	assert.Contains(t, stats.RequestsByStatus, "201")
	assert.Contains(t, stats.RequestsByStatus, "404")
	assert.Equal(t, 1, stats.RequestsByStatus["200"])
	assert.Equal(t, 1, stats.RequestsByStatus["201"])
	assert.Equal(t, 1, stats.RequestsByStatus["404"])

	// Verify average response time (should be around 1.167 seconds = 1167ms)
	expectedAvgTimeMs := float32((1000 + 2000 + 500) / 3) // Average in milliseconds
	assert.InDelta(t, expectedAvgTimeMs, stats.AverageResponseTime, 1.0)

	// Verify average response size
	expectedAvgSize := float32((1024 + 2048 + 512) / 3)
	assert.InDelta(t, expectedAvgSize, stats.AverageResponseSize, 1.0)

	// Verify geographical data
	assert.Contains(t, stats.RequestsByCountry, "US")
	assert.Contains(t, stats.RequestsByCountry, "ES")
	assert.Equal(t, 2, stats.RequestsByCountry["US"])
	assert.Equal(t, 1, stats.RequestsByCountry["ES"])

	// Verify device data
	assert.Contains(t, stats.RequestsByDeviceType, "desktop")
	assert.Contains(t, stats.RequestsByDeviceType, "mobile")
	assert.Contains(t, stats.RequestsByDeviceType, "tablet")

	// Verify platform data
	assert.Contains(t, stats.RequestsByPlatform, "Windows")
	assert.Contains(t, stats.RequestsByPlatform, "Android")
	assert.Contains(t, stats.RequestsByPlatform, "iOS")

	// Verify browser data
	assert.Contains(t, stats.RequestsByBrowser, "Chrome")
	assert.Contains(t, stats.RequestsByBrowser, "Firefox")
	assert.Contains(t, stats.RequestsByBrowser, "Safari")

	// Verify JA4 fingerprint data
	assert.Contains(t, stats.RequestsByJA4Fingerprint, "ge11nn05_9c68f7ca5aaf_d4bd6ad6f3ac")
	assert.Contains(t, stats.RequestsByJA4Fingerprint, "ge11nn05_7f3e9c2a1f8b_a9e7b3d4c2f1")
	assert.Equal(t, 2, stats.RequestsByJA4Fingerprint["ge11nn05_9c68f7ca5aaf_d4bd6ad6f3ac"]) // First and third request
	assert.Equal(t, 1, stats.RequestsByJA4Fingerprint["ge11nn05_7f3e9c2a1f8b_a9e7b3d4c2f1"]) // Second request
}

func TestGetRequestStatistics_EmptyData(t *testing.T) {
	server, _ := setupStatsTestServer()

	// Create an admin session
	sessionData := &db.Session{
		Token:           "admin-token",
		UserID:          "admin123",
		Username:        "admin",
		IsAuthenticated: true,
		IsAdmin:         true,
		ValidUntil:      time.Now().Add(time.Hour),
	}

	ctx := context.WithValue(context.Background(), session.SessionKey, sessionData)
	request := api.GetRequestStatisticsRequestObject{}

	response, err := server.GetRequestStatistics(ctx, request)
	assert.NoError(t, err)

	// Should return 200 OK with empty statistics
	successResponse, ok := response.(api.GetRequestStatistics200JSONResponse)
	assert.True(t, ok, "Expected 200 OK response")

	stats := api.RequestStatistics(successResponse)

	// Verify empty statistics
	assert.Equal(t, 0, stats.TotalRequests)
	assert.Equal(t, float32(0), stats.AverageResponseTime)
	assert.Equal(t, float32(0), stats.AverageResponseSize)
	assert.Empty(t, stats.RequestsByStatus)
	assert.Empty(t, stats.RequestsByCountry)
	assert.Empty(t, stats.RequestsByDeviceType)
	assert.Empty(t, stats.RequestsByPlatform)
	assert.Empty(t, stats.RequestsByBrowser)
	assert.Empty(t, stats.RequestsByJA4Fingerprint)
}

func TestStatisticsShowUsernames(t *testing.T) {
	// Setup repositories
	userRepo := db.NewMemoryUserRepository()
	trafficMetricRepo := db.NewMemoryTrafficMetricRepository(userRepo)
	sessionRepo := db.NewMemorySessionRepository()
	sessionStore := session.NewSessionStore(sessionRepo)
	userLoginRepo := db.NewUserLoginRepositoryMemory(userRepo)
	tokenRepo := db.NewTokenRepositoryMemory()
	tokenService := auth.NewTokenService(tokenRepo, userRepo)

	// Create test server
	startTime := time.Now()
	server := NewStrictApiServer(sessionStore, userRepo, userLoginRepo, trafficMetricRepo, tokenRepo, tokenService, startTime)

	// Create test users
	testUser1 := &db.User{
		ID:       "user-1",
		Username: "alice",
		Email:    "alice@example.com",
		Name:     "Alice Test",
		IsAdmin:  false,
	}
	testUser2 := &db.User{
		ID:       "user-2",
		Username: "bob",
		Email:    "bob@example.com",
		Name:     "Bob Test",
		IsAdmin:  false,
	}
	adminUser := &db.User{
		ID:       "admin-1",
		Username: "admin",
		Email:    "admin@example.com",
		Name:     "Admin User",
		IsAdmin:  true,
	}

	err := userRepo.CreateUser(testUser1)
	assert.NoError(t, err)
	err = userRepo.CreateUser(testUser2)
	assert.NoError(t, err)
	err = userRepo.CreateUser(adminUser)
	assert.NoError(t, err)

	// Create admin session
	adminSession := &db.Session{
		Token:           "admin-session",
		UserID:          adminUser.ID,
		Username:        adminUser.Username,
		Email:           adminUser.Email,
		IsAuthenticated: true,
		IsAdmin:         true,
		Provider:        "test",
	}

	// Create traffic metrics for different users
	now := time.Now()

	// Traffic for alice
	aliceMetric := &db.TrafficMetric{
		HttpMethod:     "GET",
		Path:           "/api/test1",
		HttpStatus:     200,
		ResponseTimeNs: 1000000,
		Timestamp:      now,
		ResponseSize:   100,
		UserID:         testUser1.ID,
		SessionID:      "alice-session",
	}

	// Traffic for bob
	bobMetric := &db.TrafficMetric{
		HttpMethod:     "POST",
		Path:           "/api/test2",
		HttpStatus:     201,
		ResponseTimeNs: 2000000,
		Timestamp:      now,
		ResponseSize:   200,
		UserID:         testUser2.ID,
		SessionID:      "bob-session",
	}

	// Traffic for guest (no user)
	guestMetric := &db.TrafficMetric{
		HttpMethod:     "GET",
		Path:           "/api/public",
		HttpStatus:     200,
		ResponseTimeNs: 500000,
		Timestamp:      now,
		ResponseSize:   50,
		UserID:         "", // No user
		SessionID:      "",
	}

	err = trafficMetricRepo.Create(aliceMetric)
	assert.NoError(t, err)
	err = trafficMetricRepo.Create(bobMetric)
	assert.NoError(t, err)
	err = trafficMetricRepo.Create(guestMetric)
	assert.NoError(t, err)

	t.Run("StatisticsShowUsernamesNotUserIDs", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), session.SessionKey, adminSession)

		startDate := now.Add(-1 * time.Hour)
		endDate := now.Add(1 * time.Hour)

		request := api.GetRequestStatisticsRequestObject{
			Params: api.GetRequestStatisticsParams{
				StartDate: &startDate,
				EndDate:   &endDate,
			},
		}

		response, err := server.GetRequestStatistics(ctx, request)
		assert.NoError(t, err)

		successResponse, ok := response.(api.GetRequestStatistics200JSONResponse)
		assert.True(t, ok)

		// Verify user statistics show usernames, not user IDs
		userStats := successResponse.RequestsByUser
		assert.Contains(t, userStats, "alice")
		assert.Contains(t, userStats, "bob")
		assert.Contains(t, userStats, "guest")
		assert.Equal(t, 1, userStats["alice"])
		assert.Equal(t, 1, userStats["bob"])
		assert.Equal(t, 1, userStats["guest"])

		// Verify user IDs are NOT in the results
		assert.NotContains(t, userStats, "user-1")
		assert.NotContains(t, userStats, "user-2")
	})
}
