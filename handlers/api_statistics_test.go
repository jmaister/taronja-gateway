package handlers

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/gateway/deps"
	"github.com/jmaister/taronja-gateway/middleware"
	"github.com/jmaister/taronja-gateway/middleware/fingerprint"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupStatsTestServer() (*StrictApiServer, db.TrafficMetricRepository) {
	// Generate a unique test name to ensure database isolation
	testName := fmt.Sprintf("stats_test_%d", time.Now().UnixNano())

	// Use the modern dependency injection approach with isolated database
	dependencies := deps.NewTestWithName(testName)

	server := NewStrictApiServer(
		dependencies.SessionStore,
		dependencies.UserRepo,
		dependencies.TrafficMetricRepo,
		dependencies.TokenRepo,
		dependencies.CountersRepo,
		dependencies.BlockedClientRepo,
		dependencies.TokenService,
		dependencies.StartTime,
		nil, // no rate limiter for basic stats tests
		nil, // no middleware registry for basic stats tests
	)
	return server, dependencies.TrafficMetricRepo
}

func TestGetRequestStatistics_Unauthorized(t *testing.T) {
	defer db.ResetConnection()

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
	defer db.ResetConnection()

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
				Fingerprint:   "ge11nn05_9c68f7ca5aaf_d4bd6ad6f3ac", FingerprintType: fingerprint.TypeJA4H,
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
				Fingerprint:   "ge11nn05_7f3e9c2a1f8b_a9e7b3d4c2f1", FingerprintType: fingerprint.TypeJA4H,
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
				Fingerprint:   "ge11nn05_9c68f7ca5aaf_d4bd6ad6f3ac", FingerprintType: fingerprint.TypeJA4H, // Same as first request
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

	// Verify fingerprint data
	assert.Contains(t, stats.RequestsByFingerprint, "ge11nn05_9c68f7ca5aaf_d4bd6ad6f3ac")
	assert.Contains(t, stats.RequestsByFingerprint, "ge11nn05_7f3e9c2a1f8b_a9e7b3d4c2f1")
	assert.Equal(t, 2, stats.RequestsByFingerprint["ge11nn05_9c68f7ca5aaf_d4bd6ad6f3ac"]) // First and third request
	assert.Equal(t, 1, stats.RequestsByFingerprint["ge11nn05_7f3e9c2a1f8b_a9e7b3d4c2f1"]) // Second request

	// All three seeded rows used fingerprint.TypeJA4H
	assert.Equal(t, 3, stats.RequestsByFingerprintType[fingerprint.TypeJA4H])
}

func TestStatisticsShowUsernames(t *testing.T) {
	// Generate a unique test name to ensure database isolation
	testName := fmt.Sprintf("usernames_test_%d", time.Now().UnixNano())

	// Use the modern dependency injection approach with isolated database
	dependencies := deps.NewTestWithName(testName)

	// Create test server
	server := NewStrictApiServer(
		dependencies.SessionStore,
		dependencies.UserRepo,
		dependencies.TrafficMetricRepo,
		dependencies.TokenRepo,
		dependencies.CountersRepo,
		dependencies.BlockedClientRepo,
		dependencies.TokenService,
		dependencies.StartTime,
		nil,
		nil,
	)

	// Create test users
	testUser1 := &db.User{
		ID:       "user-1",
		Username: "alice",
		Email:    "alice@example.com",
		Name:     "Alice Test",
		Provider: "test",
	}
	testUser2 := &db.User{
		ID:       "user-2",
		Username: "bob",
		Email:    "bob@example.com",
		Name:     "Bob Test",
		Provider: "test",
	}
	adminUser := &db.User{
		ID:       "admin-1",
		Username: "admin",
		Email:    "admin@example.com",
		Name:     "Admin User",
		Provider: "test",
	}

	err := dependencies.UserRepo.CreateUser(testUser1)
	assert.NoError(t, err)
	err = dependencies.UserRepo.CreateUser(testUser2)
	assert.NoError(t, err)
	err = dependencies.UserRepo.CreateUser(adminUser)
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

	err = dependencies.TrafficMetricRepo.Create(aliceMetric)
	assert.NoError(t, err)
	err = dependencies.TrafficMetricRepo.Create(bobMetric)
	assert.NoError(t, err)
	err = dependencies.TrafficMetricRepo.Create(guestMetric)
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

func TestGetRequestDetails_IsStaticFilter(t *testing.T) {
	server, trafficMetricRepo := setupStatsTestServer()

	adminSession := &db.Session{
		Token:           "admin-session",
		UserID:          "admin-1",
		Username:        "admin",
		IsAuthenticated: true,
		IsAdmin:         true,
		Provider:        "test",
	}

	now := time.Now()

	staticMetric := &db.TrafficMetric{
		HttpMethod:     "GET",
		Path:           "/_/static/app.js",
		HttpStatus:     200,
		ResponseTimeNs: 100000,
		Timestamp:      now,
		IsStaticAsset:  true,
	}
	apiMetric := &db.TrafficMetric{
		HttpMethod:     "GET",
		Path:           "/api/widgets",
		HttpStatus:     200,
		ResponseTimeNs: 200000,
		Timestamp:      now,
		IsStaticAsset:  false,
	}
	assert.NoError(t, trafficMetricRepo.Create(staticMetric))
	assert.NoError(t, trafficMetricRepo.Create(apiMetric))

	ctx := context.WithValue(context.Background(), session.SessionKey, adminSession)
	startDate := now.Add(-1 * time.Hour)
	endDate := now.Add(1 * time.Hour)

	getDetails := func(isStatic *bool) []api.RequestDetail {
		request := api.GetRequestDetailsRequestObject{
			Params: api.GetRequestDetailsParams{
				StartDate: &startDate,
				EndDate:   &endDate,
				IsStatic:  isStatic,
			},
		}
		response, err := server.GetRequestDetails(ctx, request)
		assert.NoError(t, err)
		success, ok := response.(api.GetRequestDetails200JSONResponse)
		assert.True(t, ok)
		return success.Requests
	}

	t.Run("no filter returns both", func(t *testing.T) {
		requests := getDetails(nil)
		assert.Len(t, requests, 2)
	})

	t.Run("is_static=true returns only the static asset request", func(t *testing.T) {
		yes := true
		requests := getDetails(&yes)
		if assert.Len(t, requests, 1) {
			assert.Equal(t, "/_/static/app.js", requests[0].Path)
			assert.True(t, requests[0].IsStatic)
		}
	})

	t.Run("is_static=false returns only the non-static request", func(t *testing.T) {
		no := false
		requests := getDetails(&no)
		if assert.Len(t, requests, 1) {
			assert.Equal(t, "/api/widgets", requests[0].Path)
			assert.False(t, requests[0].IsStatic)
		}
	})
}

func TestRateLimiterEndpoints(t *testing.T) {
	// create server with a limiter
	cfg := &config.RateLimiterConfig{RequestsPerMinute: 5, MaxErrors: 0, BlockMinutes: 1}
	rl := middleware.NewRateLimiter(*cfg, nil)
	dependencies := deps.NewTest()
	s := NewStrictApiServer(dependencies.SessionStore, dependencies.UserRepo, dependencies.TrafficMetricRepo, dependencies.TokenRepo, dependencies.CountersRepo, dependencies.BlockedClientRepo, dependencies.TokenService, dependencies.StartTime, rl, nil)
	// admin session
	sess := &db.Session{Token: "x", IsAuthenticated: true, IsAdmin: true, ValidUntil: time.Now().Add(time.Hour)}
	ctx := context.WithValue(context.Background(), session.SessionKey, sess)
	// stats should initially be empty
	resp, err := s.GetRateLimiterStats(ctx, api.GetRateLimiterStatsRequestObject{})
	assert.NoError(t, err)
	statsResp, ok := resp.(api.GetRateLimiterStats200JSONResponse)
	assert.True(t, ok)
	assert.Empty(t, statsResp)
	// config endpoint
	cfgResp, err := s.GetRateLimiterConfig(ctx, api.GetRateLimiterConfigRequestObject{})
	assert.NoError(t, err)
	conf, ok := cfgResp.(api.GetRateLimiterConfig200JSONResponse)
	assert.True(t, ok)
	assert.NotNil(t, conf.RequestsPerMinute)
	assert.Equal(t, cfg.RequestsPerMinute, *conf.RequestsPerMinute)
}

func TestGetBlockedClients_Unauthorized(t *testing.T) {
	dependencies := deps.NewTest()
	s := NewStrictApiServer(dependencies.SessionStore, dependencies.UserRepo, dependencies.TrafficMetricRepo, dependencies.TokenRepo, dependencies.CountersRepo, dependencies.BlockedClientRepo, dependencies.TokenService, dependencies.StartTime, nil, nil)

	resp, err := s.GetBlockedClients(context.Background(), api.GetBlockedClientsRequestObject{})
	require.NoError(t, err)
	_, ok := resp.(api.GetBlockedClients401JSONResponse)
	assert.True(t, ok)
}

func TestGetBlockedClients_ListsAndFilters(t *testing.T) {
	dependencies := deps.NewTest()
	s := NewStrictApiServer(dependencies.SessionStore, dependencies.UserRepo, dependencies.TrafficMetricRepo, dependencies.TokenRepo, dependencies.CountersRepo, dependencies.BlockedClientRepo, dependencies.TokenService, dependencies.StartTime, nil, nil)
	sess := &db.Session{Token: "x", IsAuthenticated: true, IsAdmin: true, ValidUntil: time.Now().Add(time.Hour)}
	ctx := context.WithValue(context.Background(), session.SessionKey, sess)

	now := time.Now()
	require.NoError(t, dependencies.BlockedClientRepo.Create(&db.BlockedClient{
		Reason: db.BlockReasonRateLimit, TriggerCount: 105,
		BlockedAt: now, BlockedUntil: now.Add(time.Hour),
		ClientInfo: db.ClientInfo{IPAddress: "203.0.113.9", UserAgent: "curl/8.0"},
	}))
	require.NoError(t, dependencies.BlockedClientRepo.Create(&db.BlockedClient{
		Reason: db.BlockReasonVulnerabilityScan, Path: "/admin.php", TriggerCount: 4,
		BlockedAt: now, BlockedUntil: now.Add(time.Hour),
		ClientInfo: db.ClientInfo{IPAddress: "203.0.113.10"},
	}))
	require.NoError(t, dependencies.BlockedClientRepo.Create(&db.BlockedClient{
		Reason: db.BlockReasonMaxErrors, TriggerCount: 21,
		BlockedAt: now, BlockedUntil: now.Add(time.Hour),
		ClientInfo: db.ClientInfo{IPAddress: "203.0.113.11", Country: "United States", Latitude: 39.0438, Longitude: -77.4874},
	}))

	t.Run("lists everything with no filter", func(t *testing.T) {
		resp, err := s.GetBlockedClients(ctx, api.GetBlockedClientsRequestObject{})
		require.NoError(t, err)
		body, ok := resp.(api.GetBlockedClients200JSONResponse)
		require.True(t, ok)
		assert.Equal(t, 3, body.TotalCount)
		require.Len(t, body.Items, 3)
	})

	t.Run("includes geo coordinates when recorded, for the attacker map", func(t *testing.T) {
		ip := "203.0.113.11"
		resp, err := s.GetBlockedClients(ctx, api.GetBlockedClientsRequestObject{Params: api.GetBlockedClientsParams{Ip: &ip}})
		require.NoError(t, err)
		body, ok := resp.(api.GetBlockedClients200JSONResponse)
		require.True(t, ok)
		require.Len(t, body.Items, 1)
		require.NotNil(t, body.Items[0].Latitude)
		require.NotNil(t, body.Items[0].Longitude)
		assert.InDelta(t, 39.0438, *body.Items[0].Latitude, 0.001)
		assert.InDelta(t, -77.4874, *body.Items[0].Longitude, 0.001)
		require.NotNil(t, body.Items[0].Country)
		assert.Equal(t, "United States", *body.Items[0].Country)
	})

	t.Run("filters by ip", func(t *testing.T) {
		ip := "203.0.113.9"
		resp, err := s.GetBlockedClients(ctx, api.GetBlockedClientsRequestObject{Params: api.GetBlockedClientsParams{Ip: &ip}})
		require.NoError(t, err)
		body, ok := resp.(api.GetBlockedClients200JSONResponse)
		require.True(t, ok)
		require.Len(t, body.Items, 1)
		assert.Equal(t, "203.0.113.9", body.Items[0].IpAddress)
		assert.Equal(t, api.BlockedClientReason(db.BlockReasonRateLimit), body.Items[0].Reason)
		assert.Equal(t, 105, body.Items[0].TriggerCount)
		require.NotNil(t, body.Items[0].UserAgent)
		assert.Equal(t, "curl/8.0", *body.Items[0].UserAgent)
		assert.Nil(t, body.Items[0].Path, "rate_limit doesn't set a path")
	})

	t.Run("scan block includes the path", func(t *testing.T) {
		ip := "203.0.113.10"
		resp, err := s.GetBlockedClients(ctx, api.GetBlockedClientsRequestObject{Params: api.GetBlockedClientsParams{Ip: &ip}})
		require.NoError(t, err)
		body, ok := resp.(api.GetBlockedClients200JSONResponse)
		require.True(t, ok)
		require.Len(t, body.Items, 1)
		require.NotNil(t, body.Items[0].Path)
		assert.Equal(t, "/admin.php", *body.Items[0].Path)
	})
}

func TestGetRequestTimeSeries_Unauthorized(t *testing.T) {
	defer db.ResetConnection()
	server, _ := setupStatsTestServer()

	response, err := server.GetRequestTimeSeries(context.Background(), api.GetRequestTimeSeriesRequestObject{})
	assert.NoError(t, err)
	_, ok := response.(api.GetRequestTimeSeries401JSONResponse)
	assert.True(t, ok, "expected 401 for an unauthenticated request")
}

func TestGetRequestTimeSeries_InvalidGranularity(t *testing.T) {
	server, _ := setupStatsTestServer()
	adminSession := &db.Session{Token: "admin", IsAuthenticated: true, IsAdmin: true}
	ctx := context.WithValue(context.Background(), session.SessionKey, adminSession)

	badGranularity := api.TimeSeriesGranularity("fortnight")
	response, err := server.GetRequestTimeSeries(ctx, api.GetRequestTimeSeriesRequestObject{
		Params: api.GetRequestTimeSeriesParams{Granularity: &badGranularity},
	})
	assert.NoError(t, err)
	_, ok := response.(api.GetRequestTimeSeries400JSONResponse)
	assert.True(t, ok, "expected 400 for an unrecognized granularity")
}

func TestGetRequestTimeSeries_SpanExceedsGranularityCap(t *testing.T) {
	server, _ := setupStatsTestServer()
	adminSession := &db.Session{Token: "admin", IsAuthenticated: true, IsAdmin: true}
	ctx := context.WithValue(context.Background(), session.SessionKey, adminSession)

	minuteGranularity := api.Minute
	start := time.Now().Add(-48 * time.Hour) // exceeds minute's 24h cap
	end := time.Now()
	response, err := server.GetRequestTimeSeries(ctx, api.GetRequestTimeSeriesRequestObject{
		Params: api.GetRequestTimeSeriesParams{
			Granularity: &minuteGranularity,
			StartDate:   &start,
			EndDate:     &end,
		},
	})
	assert.NoError(t, err)
	_, ok := response.(api.GetRequestTimeSeries400JSONResponse)
	assert.True(t, ok, "expected 400 when the span exceeds minute granularity's cap")
}

func TestGetRequestTimeSeries_Success(t *testing.T) {
	server, trafficMetricRepo := setupStatsTestServer()
	adminSession := &db.Session{Token: "admin", IsAuthenticated: true, IsAdmin: true}
	ctx := context.WithValue(context.Background(), session.SessionKey, adminSession)

	day := time.Now().Truncate(24 * time.Hour) // today at 00:00 local, well within range either way
	require.NoError(t, trafficMetricRepo.Create(&db.TrafficMetric{
		HttpMethod: "GET", Path: "/x", HttpStatus: 200, ResponseTimeNs: 1_000_000,
		Timestamp:  day.Add(2 * time.Hour),
		ClientInfo: db.ClientInfo{Fingerprint: "fp-a"},
	}))
	require.NoError(t, trafficMetricRepo.Create(&db.TrafficMetric{
		HttpMethod: "GET", Path: "/x", HttpStatus: 500, ResponseTimeNs: 3_000_000,
		Timestamp:  day.Add(2*time.Hour + 10*time.Minute),
		ClientInfo: db.ClientInfo{Fingerprint: "fp-b"},
	}))

	hourGranularity := api.Hour
	start := day
	end := day.Add(23 * time.Hour)
	response, err := server.GetRequestTimeSeries(ctx, api.GetRequestTimeSeriesRequestObject{
		Params: api.GetRequestTimeSeriesParams{
			Granularity: &hourGranularity,
			StartDate:   &start,
			EndDate:     &end,
		},
	})
	require.NoError(t, err)
	success, ok := response.(api.GetRequestTimeSeries200JSONResponse)
	require.True(t, ok, "expected 200")
	assert.Equal(t, api.Hour, success.Granularity)
	assert.Len(t, success.Points, 24, "one point per hour from 00:00 to 23:00 inclusive")

	var hour2 *api.TimeSeriesPoint
	for i := range success.Points {
		if success.Points[i].Timestamp.Hour() == 2 {
			hour2 = &success.Points[i]
			break
		}
	}
	require.NotNil(t, hour2, "the 02:00 bucket must be present")
	assert.Equal(t, 2, hour2.RequestCount)
	assert.Equal(t, 2, hour2.UniqueFingerprints)
	assert.Equal(t, 2, hour2.NewVisitors, "both fingerprints have no prior history at all")
	assert.Equal(t, 0, hour2.ReturningVisitors)
	assert.Equal(t, 1, hour2.ErrorCount)
	assert.InDelta(t, 2.0, hour2.AverageResponseTime, 0.01, "average of 1ms and 3ms")
}
