package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/stretchr/testify/assert"
)

func setupStatsTestServer() (*StrictApiServer, *db.TrafficMetricRepositoryMemory) {
	sessionRepo := db.NewMemorySessionRepository()
	sessionStore := session.NewSessionStore(sessionRepo)
	userRepo := db.NewMemoryUserRepository()
	statsRepo := db.NewMemoryTrafficMetricRepository(userRepo)

	server := NewStrictApiServer(sessionStore, userRepo, statsRepo)
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
				Country:       "US",
				DeviceFamily:  "desktop",
				OSFamily:      "Windows",
				BrowserFamily: "Chrome",
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
				Country:       "ES",
				DeviceFamily:  "mobile",
				OSFamily:      "Android",
				BrowserFamily: "Firefox",
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
				Country:       "US",
				DeviceFamily:  "tablet",
				OSFamily:      "iOS",
				BrowserFamily: "Safari",
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
}
