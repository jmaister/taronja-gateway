package db

import (
	"log"
	"time"

	"gorm.io/gorm"
)

// TrafficMetricRepository interface defines methods for managing request statistics.
type TrafficMetricRepository interface {
	Create(stat *TrafficMetric) error
	FindByDateRange(startDate, endDate time.Time) ([]TrafficMetric, error)
	FindByPath(path string, limit int) ([]TrafficMetric, error)
	GetAverageResponseTime(startDate, endDate time.Time) (float64, error)
	GetRequestCountByStatus(startDate, endDate time.Time) (map[int]int, error)
	GetTotalRequestCount(startDate, endDate time.Time) (int64, error)
	GetAverageResponseSize(startDate, endDate time.Time) (float64, error)
	GetRequestCountByCountry(startDate, endDate time.Time) (map[string]int, error)
	GetRequestCountByDeviceType(startDate, endDate time.Time) (map[string]int, error)
	GetRequestCountByPlatform(startDate, endDate time.Time) (map[string]int, error)
	GetRequestCountByBrowser(startDate, endDate time.Time) (map[string]int, error)
	GetRequestCountByUser(startDate, endDate time.Time) (map[string]int, error) // NEW
	ListRequestDetails(start, end *time.Time) ([]TrafficMetricWithUser, error)
}

// TrafficMetricRepositoryDB implements TrafficMetricRepository using GORM.
type TrafficMetricRepositoryDB struct {
	DB *gorm.DB
}

// NewTrafficMetricRepository creates a new TrafficMetricRepositoryDB instance.
func NewTrafficMetricRepository(db *gorm.DB) TrafficMetricRepository {
	return &TrafficMetricRepositoryDB{DB: db}
}

// Create stores a new request statistic record.
func (r *TrafficMetricRepositoryDB) Create(stat *TrafficMetric) error {
	if err := r.DB.Create(stat).Error; err != nil {
		log.Printf("Error creating request statistic: %v", err)
		return err
	}
	return nil
}

// FindByDateRange retrieves statistics within a date range.
func (r *TrafficMetricRepositoryDB) FindByDateRange(startDate, endDate time.Time) ([]TrafficMetric, error) {
	var stats []TrafficMetric
	err := r.DB.Where("timestamp BETWEEN ? AND ?", startDate, endDate).Find(&stats).Error
	if err != nil {
		log.Printf("Error finding statistics by date range: %v", err)
		return nil, err
	}
	return stats, nil
}

// FindByPath retrieves statistics for a specific path with limit.
func (r *TrafficMetricRepositoryDB) FindByPath(path string, limit int) ([]TrafficMetric, error) {
	var stats []TrafficMetric
	err := r.DB.Where("path = ?", path).Limit(limit).Order("timestamp DESC").Find(&stats).Error
	if err != nil {
		log.Printf("Error finding statistics by path: %v", err)
		return nil, err
	}
	return stats, nil
}

// GetAverageResponseTime calculates the average response time within a date range.
func (r *TrafficMetricRepositoryDB) GetAverageResponseTime(startDate, endDate time.Time) (float64, error) {
	var result struct {
		Average float64
	}

	err := r.DB.Model(&TrafficMetric{}).
		Select("AVG(response_time_ns) as average").
		Where("timestamp BETWEEN ? AND ?", startDate, endDate).
		Scan(&result).Error

	if err != nil {
		log.Printf("Error calculating average response time: %v", err)
		return 0, err
	}

	return result.Average, nil
}

// GetRequestCountByStatus returns request counts grouped by status code within a date range.
func (r *TrafficMetricRepositoryDB) GetRequestCountByStatus(startDate, endDate time.Time) (map[int]int, error) {
	var results []struct {
		HttpStatus int
		Count      int
	}

	err := r.DB.Model(&TrafficMetric{}).
		Select("http_status, COUNT(*) as count").
		Where("timestamp BETWEEN ? AND ?", startDate, endDate).
		Group("http_status").
		Scan(&results).Error

	if err != nil {
		log.Printf("Error getting request count by status: %v", err)
		return nil, err
	}

	statusCounts := make(map[int]int)
	for _, result := range results {
		statusCounts[result.HttpStatus] = result.Count
	}

	return statusCounts, nil
}

// GetTotalRequestCount returns the total number of requests within a date range.
func (r *TrafficMetricRepositoryDB) GetTotalRequestCount(startDate, endDate time.Time) (int64, error) {
	var count int64
	err := r.DB.Model(&TrafficMetric{}).
		Where("timestamp BETWEEN ? AND ?", startDate, endDate).
		Count(&count).Error

	if err != nil {
		log.Printf("Error getting total request count: %v", err)
		return 0, err
	}

	return count, nil
}

// GetAverageResponseSize calculates the average response size within a date range.
func (r *TrafficMetricRepositoryDB) GetAverageResponseSize(startDate, endDate time.Time) (float64, error) {
	var result struct {
		Average float64
	}

	err := r.DB.Model(&TrafficMetric{}).
		Select("AVG(response_size) as average").
		Where("timestamp BETWEEN ? AND ?", startDate, endDate).
		Scan(&result).Error

	if err != nil {
		log.Printf("Error calculating average response size: %v", err)
		return 0, err
	}

	return result.Average, nil
}

// GetRequestCountByCountry returns request counts grouped by country within a date range.
func (r *TrafficMetricRepositoryDB) GetRequestCountByCountry(startDate, endDate time.Time) (map[string]int, error) {
	var results []struct {
		Country string
		Count   int
	}

	err := r.DB.Model(&TrafficMetric{}).
		Select("country, COUNT(*) as count").
		Where("timestamp BETWEEN ? AND ?", startDate, endDate).
		Group("country").
		Scan(&results).Error

	if err != nil {
		log.Printf("Error getting request count by country: %v", err)
		return nil, err
	}

	countryCounts := make(map[string]int)
	for _, result := range results {
		countryCounts[result.Country] = result.Count
	}

	return countryCounts, nil
}

// GetRequestCountByDeviceType returns request counts grouped by device type within a date range.
func (r *TrafficMetricRepositoryDB) GetRequestCountByDeviceType(startDate, endDate time.Time) (map[string]int, error) {
	var results []struct {
		DeviceFamily string
		Count        int
	}

	err := r.DB.Model(&TrafficMetric{}).
		Select("device_family, COUNT(*) as count").
		Where("timestamp BETWEEN ? AND ?", startDate, endDate).
		Group("device_family").
		Scan(&results).Error

	if err != nil {
		log.Printf("Error getting request count by device type: %v", err)
		return nil, err
	}

	deviceCounts := make(map[string]int)
	for _, result := range results {
		deviceCounts[result.DeviceFamily] = result.Count
	}

	return deviceCounts, nil
}

// GetRequestCountByPlatform returns request counts grouped by platform within a date range.
func (r *TrafficMetricRepositoryDB) GetRequestCountByPlatform(startDate, endDate time.Time) (map[string]int, error) {
	var results []struct {
		OSFamily string
		Count    int
	}

	err := r.DB.Model(&TrafficMetric{}).
		Select("os_family, COUNT(*) as count").
		Where("timestamp BETWEEN ? AND ?", startDate, endDate).
		Group("os_family").
		Scan(&results).Error

	if err != nil {
		log.Printf("Error getting request count by platform: %v", err)
		return nil, err
	}

	platformCounts := make(map[string]int)
	for _, result := range results {
		platformCounts[result.OSFamily] = result.Count
	}

	return platformCounts, nil
}

// GetRequestCountByBrowser returns request counts grouped by browser within a date range.
func (r *TrafficMetricRepositoryDB) GetRequestCountByBrowser(startDate, endDate time.Time) (map[string]int, error) {
	var results []struct {
		BrowserFamily string
		Count         int
	}

	err := r.DB.Model(&TrafficMetric{}).
		Select("browser_family, COUNT(*) as count").
		Where("timestamp BETWEEN ? AND ?", startDate, endDate).
		Group("browser_family").
		Scan(&results).Error

	if err != nil {
		log.Printf("Error getting request count by browser: %v", err)
		return nil, err
	}

	browserCounts := make(map[string]int)
	for _, result := range results {
		browserCounts[result.BrowserFamily] = result.Count
	}

	return browserCounts, nil
}

// GetRequestCountByUser returns request counts grouped by user within a date range.
func (r *TrafficMetricRepositoryDB) GetRequestCountByUser(startDate, endDate time.Time) (map[string]int, error) {
	var results []struct {
		Username string
		Count    int
	}

	// Join with users table to get username from user_id
	err := r.DB.Table("traffic_metrics").
		Select("COALESCE(users.username, 'guest') as username, COUNT(*) as count").
		Joins("LEFT JOIN users ON users.id = traffic_metrics.user_id").
		Where("traffic_metrics.timestamp BETWEEN ? AND ?", startDate, endDate).
		Group("users.username").
		Scan(&results).Error

	if err != nil {
		log.Printf("Error getting request count by user: %v", err)
		return nil, err
	}

	userCounts := make(map[string]int)
	for _, result := range results {
		userCounts[result.Username] = result.Count
	}

	return userCounts, nil
}

// ListRequestDetails returns all request details in a date range (or all if nil)
func (r *TrafficMetricRepositoryDB) ListRequestDetails(start, end *time.Time) ([]TrafficMetricWithUser, error) {
	var stats []TrafficMetricWithUser
	query := r.DB.Model(&TrafficMetric{}).Preload("User")
	if start != nil && end != nil {
		query = query.Where("timestamp BETWEEN ? AND ?", *start, *end)
	} else if start != nil {
		query = query.Where("timestamp >= ?", *start)
	} else if end != nil {
		query = query.Where("timestamp <= ?", *end)
	}
	err := query.Order("timestamp DESC").Find(&stats).Error
	if err != nil {
		log.Printf("Error listing request details: %v", err)
		return nil, err
	}
	return stats, nil
}
