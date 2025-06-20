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
		Select("AVG(response_time_ms) as average").
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
		Status int
		Count  int
	}

	err := r.DB.Model(&TrafficMetric{}).
		Select("status, COUNT(*) as count").
		Where("timestamp BETWEEN ? AND ?", startDate, endDate).
		Group("status").
		Scan(&results).Error

	if err != nil {
		log.Printf("Error getting request count by status: %v", err)
		return nil, err
	}

	statusCounts := make(map[int]int)
	for _, result := range results {
		statusCounts[result.Status] = result.Count
	}

	return statusCounts, nil
}
