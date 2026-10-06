package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/db"
)

// GetRequestStatistics implements the API endpoint for retrieving request statistics.
func (s *StrictApiServer) GetRequestStatistics(ctx context.Context, request api.GetRequestStatisticsRequestObject) (api.GetRequestStatisticsResponseObject, error) {
	if _, ok := requireAdminSession(ctx); !ok {
		return api.GetRequestStatistics401JSONResponse{}, nil
	}

	filter := withDefaultRange(s.filterFrom(trafficFilterParams(request.Params)), func(end time.Time) time.Time {
		return end.AddDate(0, 0, -30)
	})

	stats, err := s.collectRequestStatistics(filter)
	if err != nil {
		log.Printf("Error getting request statistics: %v", err)
		return api.GetRequestStatistics500JSONResponse{}, nil
	}
	stats.StaticAssetsExcluded = &s.excludeStaticAssets
	return api.GetRequestStatistics200JSONResponse(stats), nil
}

func (s *StrictApiServer) collectRequestStatistics(filter db.TrafficFilter) (api.RequestStatistics, error) {
	repo := s.trafficMetricRepo
	var firstErr error
	check := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	countBy := func(dimension db.Dimension) map[string]int {
		counts, err := repo.CountBy(dimension, filter)
		check(err)
		return dropEmptyKeys(counts)
	}

	total, err := repo.CountRequests(filter)
	check(err)
	avgResponseTimeNs, err := repo.AverageResponseTimeNs(filter)
	check(err)
	avgResponseSize, err := repo.AverageResponseSize(filter)
	check(err)

	return api.RequestStatistics{
		TotalRequests:             int(total),
		RequestsByStatus:          countBy(db.DimensionStatus),
		AverageResponseTime:       float32(avgResponseTimeNs / 1e6),
		AverageResponseSize:       float32(avgResponseSize),
		RequestsByCountry:         countBy(db.DimensionCountry),
		RequestsByDeviceType:      countBy(db.DimensionDeviceType),
		RequestsByPlatform:        countBy(db.DimensionPlatform),
		RequestsByBrowser:         countBy(db.DimensionBrowser),
		RequestsByUser:            countBy(db.DimensionUser),
		RequestsByFingerprint:     countBy(db.DimensionFingerprint),
		RequestsByFingerprintType: countBy(db.DimensionFingerprintType),
	}, firstErr
}

// geoPoint returns coordinates as pointers, or nils when country is empty.
// Latitude/Longitude are plain float64 columns, so zero cannot tell a real
// equator/prime-meridian point from "never looked up"; Country is only ever
// set alongside coordinates by a successful geo lookup.
func geoPoint(country string, latitude, longitude float64) (lat, lon *float32) {
	if country == "" {
		return nil, nil
	}
	la, lo := float32(latitude), float32(longitude)
	return &la, &lo
}

func requestDetailFromMetric(m db.TrafficMetricWithUser) api.RequestDetail {
	t := m.TrafficMetric
	var username, userID *string
	if m.User != nil {
		username = &m.User.Username
	}
	if t.UserID != "" {
		userID = &t.UserID
	}
	latitude, longitude := geoPoint(t.Country, t.Latitude, t.Longitude)

	return api.RequestDetail{
		Id:              fmt.Sprintf("%v", t.ID),
		Timestamp:       t.Timestamp,
		Path:            t.Path,
		UserId:          userID,
		Username:        username,
		StatusCode:      t.HttpStatus,
		ResponseTime:    float32(t.ResponseTimeNs) / 1e6, // ns to ms
		ResponseSize:    float32(t.ResponseSize),
		Country:         t.Country,
		City:            t.City,
		Latitude:        latitude,
		Longitude:       longitude,
		DeviceType:      t.DeviceFamily,
		Platform:        t.OSFamily,
		PlatformVersion: t.OSVersion,
		Browser:         t.BrowserFamily,
		BrowserVersion:  t.BrowserVersion,
		IsStatic:        t.IsStaticAsset,
		Fingerprint:     t.Fingerprint,
		FingerprintType: api.RequestDetailFingerprintType(t.FingerprintType),
	}
}

// GetRequestDetails implements GET /_/api/statistics/requests/details
func (s *StrictApiServer) GetRequestDetails(ctx context.Context, req api.GetRequestDetailsRequestObject) (api.GetRequestDetailsResponseObject, error) {
	if _, ok := requireAdminSession(ctx); !ok {
		return api.GetRequestDetails401JSONResponse{}, nil
	}
	metrics, err := s.trafficMetricRepo.ListRequestDetails(s.filterFrom(trafficFilterParams(req.Params)))
	if err != nil {
		return nil, err
	}
	details := make([]api.RequestDetail, 0, len(metrics))
	for _, m := range metrics {
		details = append(details, requestDetailFromMetric(m))
	}
	return api.GetRequestDetails200JSONResponse{Requests: details}, nil
}

// defaultTimeSeriesGranularity is used when the request omits granularity.
const defaultTimeSeriesGranularity = api.Day

// defaultTimeSeriesStart is the start of the default range for granularity:
// a day for "day" and finer, and a span proportional to the bucket size for
// week and month (a one-day month series would be a single point).
func defaultTimeSeriesStart(granularity api.TimeSeriesGranularity, end time.Time) time.Time {
	switch granularity {
	case api.Week:
		return end.AddDate(0, 0, -7*12)
	case api.Month:
		return end.AddDate(-1, 0, 0)
	}
	return end.Add(-24 * time.Hour)
}

// GetRequestTimeSeries implements GET /_/api/statistics/timeseries —
// request/visitor counts bucketed over time. See db/timeseries.go for the
// bucketing and range validation.
func (s *StrictApiServer) GetRequestTimeSeries(ctx context.Context, request api.GetRequestTimeSeriesRequestObject) (api.GetRequestTimeSeriesResponseObject, error) {
	if _, ok := requireAdminSession(ctx); !ok {
		return api.GetRequestTimeSeries401JSONResponse{}, nil
	}

	granularity := defaultTimeSeriesGranularity
	if request.Params.Granularity != nil {
		granularity = *request.Params.Granularity
	}

	filter := withDefaultRange(s.filterFrom(trafficFilterParamsFromTimeSeries(request.Params)), func(end time.Time) time.Time {
		return defaultTimeSeriesStart(granularity, end)
	})

	dbGranularity := db.TimeSeriesGranularity(granularity)
	if err := db.ValidateTimeSeriesRange(*filter.Start, *filter.End, dbGranularity); err != nil {
		return api.GetRequestTimeSeries400JSONResponse{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		}, nil
	}

	dbPoints, err := s.trafficMetricRepo.GetTimeSeries(filter, dbGranularity)
	if err != nil {
		log.Printf("Error getting request time series: %v", err)
		return api.GetRequestTimeSeries500JSONResponse{}, nil
	}

	points := make([]api.TimeSeriesPoint, 0, len(dbPoints))
	for _, p := range dbPoints {
		points = append(points, api.TimeSeriesPoint{
			Timestamp:           p.Timestamp,
			RequestCount:        p.RequestCount,
			UniqueFingerprints:  p.UniqueFingerprints,
			NewVisitors:         p.NewVisitors,
			ReturningVisitors:   p.ReturningVisitors,
			UniqueUsers:         p.UniqueUsers,
			ErrorCount:          p.ErrorCount,
			AverageResponseTime: float32(p.AverageResponseTimeMs),
		})
	}

	return api.GetRequestTimeSeries200JSONResponse{
		Granularity: granularity,
		Points:      points,
	}, nil
}

// GetRateLimiterStats implements GET /_/api/statistics/rate-limiter
func (s *StrictApiServer) GetRateLimiterStats(ctx context.Context, req api.GetRateLimiterStatsRequestObject) (api.GetRateLimiterStatsResponseObject, error) {
	if _, ok := requireAdminSession(ctx); !ok {
		return api.GetRateLimiterStats401JSONResponse{}, nil
	}
	if s.rateLimiter == nil {
		return api.GetRateLimiterStats200JSONResponse(api.RateLimiterStats{}), nil
	}
	// convert middleware stats to API structs
	raw := s.rateLimiter.Stats()
	apiStats := make(api.RateLimiterStats, 0, len(raw))
	for _, st := range raw {
		apiStats = append(apiStats, api.RateLimiterStat{
			Ip:           st.IP,
			Requests:     st.Requests,
			Errors:       st.Errors,
			Scan404:      st.Scan404,
			BlockedUntil: st.BlockedUntil,
		})
	}
	return api.GetRateLimiterStats200JSONResponse(apiStats), nil
}

// GetBlockedClients implements GET /_/api/rate-limiter/blocked
func (s *StrictApiServer) GetBlockedClients(ctx context.Context, req api.GetBlockedClientsRequestObject) (api.GetBlockedClientsResponseObject, error) {
	if _, ok := requireAdminSession(ctx); !ok {
		return api.GetBlockedClients401JSONResponse{}, nil
	}
	if s.blockedClientRepo == nil {
		return api.GetBlockedClients200JSONResponse{Items: []api.BlockedClient{}}, nil
	}

	var filter db.BlockedClientFilter
	if req.Params.Ip != nil {
		filter.IP = *req.Params.Ip
	}
	if req.Params.Country != nil {
		filter.Country = *req.Params.Country
	}
	limit := 50 // default, matching GetUserCounterHistory's convention
	if req.Params.Limit != nil {
		limit = *req.Params.Limit
	}
	offset := 0
	if req.Params.Offset != nil {
		offset = *req.Params.Offset
	}

	items, total, err := s.blockedClientRepo.List(filter, limit, offset)
	if err != nil {
		log.Printf("GetBlockedClients: failed to list blocked clients: %v", err)
		return api.GetBlockedClients500JSONResponse{}, nil
	}

	apiItems := make([]api.BlockedClient, 0, len(items))
	for _, bc := range items {
		item := api.BlockedClient{
			Id:           strconv.FormatUint(uint64(bc.ID), 10),
			IpAddress:    bc.IPAddress,
			Reason:       api.BlockedClientReason(bc.Reason),
			TriggerCount: bc.TriggerCount,
			BlockedAt:    bc.BlockedAt,
			BlockedUntil: bc.BlockedUntil,
		}
		if bc.Path != "" {
			item.Path = &bc.Path
		}
		if bc.UserAgent != "" {
			item.UserAgent = &bc.UserAgent
		}
		if bc.Country != "" {
			item.Country = &bc.Country
		}
		if bc.CountryCode != "" {
			item.CountryCode = &bc.CountryCode
		}
		if bc.City != "" {
			item.City = &bc.City
		}
		item.Latitude, item.Longitude = geoPoint(bc.Country, bc.Latitude, bc.Longitude)
		if bc.Fingerprint != "" {
			item.Fingerprint = &bc.Fingerprint
		}
		if bc.FingerprintType != "" {
			item.FingerprintType = &bc.FingerprintType
		}
		apiItems = append(apiItems, item)
	}

	return api.GetBlockedClients200JSONResponse{
		Items:      apiItems,
		TotalCount: int(total),
		Limit:      limit,
		Offset:     offset,
	}, nil
}

// GetRateLimiterConfig implements GET /_/api/config/rate-limiter
func (s *StrictApiServer) GetRateLimiterConfig(ctx context.Context, req api.GetRateLimiterConfigRequestObject) (api.GetRateLimiterConfigResponseObject, error) {
	if _, ok := requireAdminSession(ctx); !ok {
		return api.GetRateLimiterConfig401JSONResponse{}, nil
	}
	if s.rateLimiter == nil {
		return api.GetRateLimiterConfig200JSONResponse(api.RateLimiterConfigResponse{}), nil
	}
	cfg := s.rateLimiter.Config()
	reqPerMin := cfg.RequestsPerMinute
	maxErrors := cfg.MaxErrors
	blockMins := cfg.BlockMinutes
	resp := api.RateLimiterConfigResponse{
		RequestsPerMinute: &reqPerMin,
		MaxErrors:         &maxErrors,
		BlockMinutes:      &blockMins,
	}
	if len(cfg.VulnerabilityScan.URLs) > 0 || cfg.VulnerabilityScan.Max404 > 0 {
		vs := &struct {
			BlockMinutes int      `json:"blockMinutes"`
			Max404       int      `json:"max404"`
			Urls         []string `json:"urls"`
		}{
			BlockMinutes: cfg.VulnerabilityScan.BlockMinutes,
			Max404:       cfg.VulnerabilityScan.Max404,
			Urls:         cfg.VulnerabilityScan.URLs,
		}
		resp.VulnerabilityScan = vs
	}
	return api.GetRateLimiterConfig200JSONResponse(resp), nil
}
