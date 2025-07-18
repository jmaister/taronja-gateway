package handlers

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/session"
)

// GetRequestStatistics implements the API endpoint for retrieving request statistics.
func (s *StrictApiServer) GetRequestStatistics(ctx context.Context, request api.GetRequestStatisticsRequestObject) (api.GetRequestStatisticsResponseObject, error) {
	// Check if user is authenticated and is admin
	sessionData, ok := ctx.Value(session.SessionKey).(*db.Session)
	if !ok || sessionData == nil || !sessionData.IsAuthenticated {
		return api.GetRequestStatistics401JSONResponse{}, nil
	}

	// Only admin users can access statistics
	if !sessionData.IsAdmin {
		return api.GetRequestStatistics401JSONResponse{}, nil
	}

	// Set default date range if not provided
	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -30) // Default to last 30 days

	// Parse optional date parameters
	if request.Params.StartDate != nil {
		startDate = *request.Params.StartDate
	}
	if request.Params.EndDate != nil {
		endDate = *request.Params.EndDate
	}

	// Get total request count
	totalRequests, err := s.trafficMetricRepo.GetTotalRequestCount(startDate, endDate)
	if err != nil {
		log.Printf("Error getting total request count: %v", err)
		return api.GetRequestStatistics500JSONResponse{}, nil
	}

	// Get requests by status
	requestsByStatus, err := s.trafficMetricRepo.GetRequestCountByStatus(startDate, endDate)
	if err != nil {
		log.Printf("Error getting requests by status: %v", err)
		return api.GetRequestStatistics500JSONResponse{}, nil
	}

	// Get average response time (convert from nanoseconds to milliseconds)
	avgResponseTimeNs, err := s.trafficMetricRepo.GetAverageResponseTime(startDate, endDate)
	if err != nil {
		log.Printf("Error getting average response time: %v", err)
		return api.GetRequestStatistics500JSONResponse{}, nil
	}
	avgResponseTimeMs := avgResponseTimeNs / 1_000_000 // Convert to milliseconds

	// Get average response size
	avgResponseSize, err := s.trafficMetricRepo.GetAverageResponseSize(startDate, endDate)
	if err != nil {
		log.Printf("Error getting average response size: %v", err)
		return api.GetRequestStatistics500JSONResponse{}, nil
	}

	// Get requests by country
	requestsByCountry, err := s.trafficMetricRepo.GetRequestCountByCountry(startDate, endDate)
	if err != nil {
		log.Printf("Error getting requests by country: %v", err)
		return api.GetRequestStatistics500JSONResponse{}, nil
	}

	// Get requests by device type
	requestsByDeviceType, err := s.trafficMetricRepo.GetRequestCountByDeviceType(startDate, endDate)
	if err != nil {
		log.Printf("Error getting requests by device type: %v", err)
		return api.GetRequestStatistics500JSONResponse{}, nil
	}

	// Get requests by platform
	requestsByPlatform, err := s.trafficMetricRepo.GetRequestCountByPlatform(startDate, endDate)
	if err != nil {
		log.Printf("Error getting requests by platform: %v", err)
		return api.GetRequestStatistics500JSONResponse{}, nil
	}

	// Get requests by browser
	requestsByBrowser, err := s.trafficMetricRepo.GetRequestCountByBrowser(startDate, endDate)
	if err != nil {
		log.Printf("Error getting requests by browser: %v", err)
		return api.GetRequestStatistics500JSONResponse{}, nil
	}

	// Get requests by user
	requestsByUser, err := s.trafficMetricRepo.GetRequestCountByUser(startDate, endDate)
	if err != nil {
		log.Printf("Error getting requests by user: %v", err)
		return api.GetRequestStatistics500JSONResponse{}, nil
	}

	// Get requests by JA4 fingerprint
	requestsByJA4Fingerprint, err := s.trafficMetricRepo.GetRequestCountByJA4Fingerprint(startDate, endDate)
	if err != nil {
		log.Printf("Error getting requests by JA4 fingerprint: %v", err)
		return api.GetRequestStatistics500JSONResponse{}, nil
	}

	// Convert maps to the expected format for the API response
	statusMap := make(map[string]int)
	for status, count := range requestsByStatus {
		statusMap[fmt.Sprintf("%d", status)] = count
	}

	countryMap := make(map[string]int)
	for country, count := range requestsByCountry {
		if country != "" { // Skip empty country codes
			countryMap[country] = count
		}
	}

	deviceMap := make(map[string]int)
	for device, count := range requestsByDeviceType {
		if device != "" { // Skip empty device types
			deviceMap[device] = count
		}
	}

	platformMap := make(map[string]int)
	for platform, count := range requestsByPlatform {
		if platform != "" { // Skip empty platforms
			platformMap[platform] = count
		}
	}

	browserMap := make(map[string]int)
	for browser, count := range requestsByBrowser {
		if browser != "" { // Skip empty browsers
			browserMap[browser] = count
		}
	}

	userMap := make(map[string]int)
	for user, count := range requestsByUser {
		if user != "" {
			userMap[user] = count
		}
	}

	ja4FingerprintMap := make(map[string]int)
	for ja4Fingerprint, count := range requestsByJA4Fingerprint {
		if ja4Fingerprint != "" {
			ja4FingerprintMap[ja4Fingerprint] = count
		}
	}

	// Create the response
	response := api.RequestStatistics{
		TotalRequests:            int(totalRequests),
		RequestsByStatus:         statusMap,
		AverageResponseTime:      float32(avgResponseTimeMs),
		AverageResponseSize:      float32(avgResponseSize),
		RequestsByCountry:        countryMap,
		RequestsByDeviceType:     deviceMap,
		RequestsByPlatform:       platformMap,
		RequestsByBrowser:        browserMap,
		RequestsByUser:           userMap,
		RequestsByJA4Fingerprint: ja4FingerprintMap,
	}

	return api.GetRequestStatistics200JSONResponse(response), nil
}

// GetRequestDetails implements GET /_/api/statistics/requests/details
func (s *StrictApiServer) GetRequestDetails(ctx context.Context, req api.GetRequestDetailsRequestObject) (api.GetRequestDetailsResponseObject, error) {
	// Check if user is authenticated and is admin
	sessionData, ok := ctx.Value(session.SessionKey).(*db.Session)
	if !ok || sessionData == nil || !sessionData.IsAuthenticated {
		return api.GetRequestDetails401JSONResponse{}, nil
	}
	if !sessionData.IsAdmin {
		return api.GetRequestDetails401JSONResponse{}, nil
	}
	var start, end *time.Time
	if req.Params.StartDate != nil {
		start = req.Params.StartDate
	}
	if req.Params.EndDate != nil {
		end = req.Params.EndDate
	}
	metrics, err := s.trafficMetricRepo.ListRequestDetails(start, end)
	if err != nil {
		return nil, err
	}
	var details []api.RequestDetail
	for _, m := range metrics {
		var username *string
		if m.User != nil {
			username = &m.User.Username
		}

		var userID *string
		if m.TrafficMetric.UserID != "" {
			userID = &m.TrafficMetric.UserID
		}

		var latitude, longitude *float32
		if m.TrafficMetric.Latitude != 0 {
			lat := float32(m.TrafficMetric.Latitude)
			latitude = &lat
		}
		if m.TrafficMetric.Longitude != 0 {
			lon := float32(m.TrafficMetric.Longitude)
			longitude = &lon
		}

		details = append(details, api.RequestDetail{
			Id:              fmt.Sprintf("%v", m.TrafficMetric.ID),
			Timestamp:       m.TrafficMetric.Timestamp,
			Path:            m.TrafficMetric.Path,
			UserId:          userID,
			Username:        username,
			StatusCode:      m.TrafficMetric.HttpStatus,
			ResponseTime:    float32(m.TrafficMetric.ResponseTimeNs) / 1e6, // convert ns to ms
			ResponseSize:    float32(m.TrafficMetric.ResponseSize),
			Country:         m.TrafficMetric.Country,
			City:            m.TrafficMetric.City,
			Latitude:        latitude,
			Longitude:       longitude,
			DeviceType:      m.TrafficMetric.DeviceFamily,
			Platform:        m.TrafficMetric.OSFamily,
			PlatformVersion: m.TrafficMetric.OSVersion,
			Browser:         m.TrafficMetric.BrowserFamily,
			BrowserVersion:  m.TrafficMetric.BrowserVersion,
		})
	}
	return api.GetRequestDetails200JSONResponse{Requests: details}, nil
}
