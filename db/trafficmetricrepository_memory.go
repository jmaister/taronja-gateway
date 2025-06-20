package db

import (
	"sort"
	"sync"
	"time"
)

// TrafficMetricRepositoryMemory implements TrafficMetricRepository using in-memory storage for testing.
type TrafficMetricRepositoryMemory struct {
	stats  []TrafficMetric
	mutex  sync.RWMutex
	nextID uint
}

// NewMemoryTrafficMetricRepository creates a new TrafficMetricRepositoryMemory instance.
func NewMemoryTrafficMetricRepository() *TrafficMetricRepositoryMemory {
	return &TrafficMetricRepositoryMemory{
		stats:  make([]TrafficMetric, 0),
		mutex:  sync.RWMutex{},
		nextID: 1,
	}
}

// Create stores a new request statistic record in memory.
func (r *TrafficMetricRepositoryMemory) Create(stat *TrafficMetric) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Set ID and timestamps
	stat.ID = r.nextID
	r.nextID++
	now := time.Now()
	stat.CreatedAt = now
	stat.UpdatedAt = now

	// Make a copy to avoid issues with pointer sharing
	statCopy := *stat
	r.stats = append(r.stats, statCopy)

	return nil
}

// FindByDateRange retrieves statistics within a date range.
func (r *TrafficMetricRepositoryMemory) FindByDateRange(startDate, endDate time.Time) ([]TrafficMetric, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	var result []TrafficMetric
	for _, stat := range r.stats {
		if stat.Timestamp.After(startDate) && stat.Timestamp.Before(endDate) {
			result = append(result, stat)
		}
	}

	return result, nil
}

// FindByPath retrieves statistics for a specific path with limit.
func (r *TrafficMetricRepositoryMemory) FindByPath(path string, limit int) ([]TrafficMetric, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	var matching []TrafficMetric
	for _, stat := range r.stats {
		if stat.Path == path {
			matching = append(matching, stat)
		}
	}

	// Sort by timestamp descending
	sort.Slice(matching, func(i, j int) bool {
		return matching[i].Timestamp.After(matching[j].Timestamp)
	})

	// Apply limit
	if limit > 0 && len(matching) > limit {
		matching = matching[:limit]
	}

	return matching, nil
}

// GetAverageResponseTime calculates the average response time within a date range.
func (r *TrafficMetricRepositoryMemory) GetAverageResponseTime(startDate, endDate time.Time) (float64, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	var total int64
	var count int

	for _, stat := range r.stats {
		if stat.Timestamp.After(startDate) && stat.Timestamp.Before(endDate) {
			total += stat.ResponseTimeMs
			count++
		}
	}

	if count == 0 {
		return 0, nil
	}

	return float64(total) / float64(count), nil
}

// GetRequestCountByStatus returns request counts grouped by status code within a date range.
func (r *TrafficMetricRepositoryMemory) GetRequestCountByStatus(startDate, endDate time.Time) (map[int]int, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	statusCounts := make(map[int]int)

	for _, stat := range r.stats {
		if stat.Timestamp.After(startDate) && stat.Timestamp.Before(endDate) {
			statusCounts[stat.HttpStatus]++
		}
	}

	return statusCounts, nil
}
