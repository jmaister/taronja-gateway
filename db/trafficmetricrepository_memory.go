package db

import (
	"sort"
	"sync"
	"time"
)

// TrafficMetricRepositoryMemory implements TrafficMetricRepository using in-memory storage for testing.
type TrafficMetricRepositoryMemory struct {
	stats    []TrafficMetric
	mutex    sync.RWMutex
	nextID   uint
	userRepo UserRepository
}

// NewMemoryTrafficMetricRepository creates a new TrafficMetricRepositoryMemory instance.
func NewMemoryTrafficMetricRepository(userRepo UserRepository) *TrafficMetricRepositoryMemory {
	return &TrafficMetricRepositoryMemory{
		stats:    make([]TrafficMetric, 0),
		mutex:    sync.RWMutex{},
		nextID:   1,
		userRepo: userRepo,
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
			total += stat.ResponseTimeNs
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

// GetTotalRequestCount returns the total number of requests within a date range.
func (r *TrafficMetricRepositoryMemory) GetTotalRequestCount(startDate, endDate time.Time) (int64, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	var count int64
	for _, stat := range r.stats {
		if stat.Timestamp.After(startDate) && stat.Timestamp.Before(endDate) {
			count++
		}
	}

	return count, nil
}

// GetAverageResponseSize calculates the average response size within a date range.
func (r *TrafficMetricRepositoryMemory) GetAverageResponseSize(startDate, endDate time.Time) (float64, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	var total int64
	var count int

	for _, stat := range r.stats {
		if stat.Timestamp.After(startDate) && stat.Timestamp.Before(endDate) {
			total += stat.ResponseSize
			count++
		}
	}

	if count == 0 {
		return 0, nil
	}

	return float64(total) / float64(count), nil
}

// GetRequestCountByCountry returns request counts grouped by country within a date range.
func (r *TrafficMetricRepositoryMemory) GetRequestCountByCountry(startDate, endDate time.Time) (map[string]int, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	countryCounts := make(map[string]int)

	for _, stat := range r.stats {
		if stat.Timestamp.After(startDate) && stat.Timestamp.Before(endDate) {
			countryCounts[stat.Country]++
		}
	}

	return countryCounts, nil
}

// GetRequestCountByDeviceType returns request counts grouped by device type within a date range.
func (r *TrafficMetricRepositoryMemory) GetRequestCountByDeviceType(startDate, endDate time.Time) (map[string]int, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	deviceCounts := make(map[string]int)

	for _, stat := range r.stats {
		if stat.Timestamp.After(startDate) && stat.Timestamp.Before(endDate) {
			deviceCounts[stat.DeviceFamily]++
		}
	}

	return deviceCounts, nil
}

// GetRequestCountByPlatform returns request counts grouped by platform within a date range.
func (r *TrafficMetricRepositoryMemory) GetRequestCountByPlatform(startDate, endDate time.Time) (map[string]int, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	platformCounts := make(map[string]int)

	for _, stat := range r.stats {
		if stat.Timestamp.After(startDate) && stat.Timestamp.Before(endDate) {
			platformCounts[stat.OSFamily]++
		}
	}

	return platformCounts, nil
}

// GetRequestCountByBrowser returns request counts grouped by browser within a date range.
func (r *TrafficMetricRepositoryMemory) GetRequestCountByBrowser(startDate, endDate time.Time) (map[string]int, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	browserCounts := make(map[string]int)

	for _, stat := range r.stats {
		if stat.Timestamp.After(startDate) && stat.Timestamp.Before(endDate) {
			browserCounts[stat.BrowserFamily]++
		}
	}

	return browserCounts, nil
}

// GetRequestCountByUser returns request counts grouped by user within a date range.
func (r *TrafficMetricRepositoryMemory) GetRequestCountByUser(startDate, endDate time.Time) (map[string]int, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	userCounts := make(map[string]int)

	for _, stat := range r.stats {
		if stat.Timestamp.After(startDate) && stat.Timestamp.Before(endDate) {
			user := stat.UserID
			if user == "" {
				user = "guest"
			}
			userCounts[user]++
		}
	}

	return userCounts, nil
}

// GetRequestCountByJA4Fingerprint returns request counts grouped by JA4 fingerprint within a date range.
func (r *TrafficMetricRepositoryMemory) GetRequestCountByJA4Fingerprint(startDate, endDate time.Time) (map[string]int, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	ja4Counts := make(map[string]int)

	for _, stat := range r.stats {
		if stat.Timestamp.After(startDate) && stat.Timestamp.Before(endDate) {
			fingerprint := stat.ClientInfo.JA4Fingerprint
			if fingerprint != "" {
				ja4Counts[fingerprint]++
			}
		}
	}

	return ja4Counts, nil
}

// ListRequestDetails returns all request details in a date range (or all if nil)
func (r *TrafficMetricRepositoryMemory) ListRequestDetails(start, end *time.Time) ([]TrafficMetricWithUser, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	var result []TrafficMetricWithUser
	for _, stat := range r.stats {
		include := false
		if start != nil && end != nil {
			if stat.Timestamp.After(*start) && stat.Timestamp.Before(*end) {
				include = true
			}
		} else if start != nil {
			if stat.Timestamp.After(*start) {
				include = true
			}
		} else if end != nil {
			if stat.Timestamp.Before(*end) {
				include = true
			}
		} else {
			include = true
		}

		if include {
			metricWithUser := TrafficMetricWithUser{
				TrafficMetric: stat,
			}

			// Fetch user if UserID is set
			if stat.UserID != "" && r.userRepo != nil {
				user, err := r.userRepo.FindUserByIdOrUsername(stat.UserID, "", "")
				if err == nil && user != nil {
					metricWithUser.User = user
				}
			}

			result = append(result, metricWithUser)
		}
	}
	// Sort by timestamp descending
	sort.Slice(result, func(i, j int) bool {
		return result[i].TrafficMetric.Timestamp.After(result[j].TrafficMetric.Timestamp)
	})
	return result, nil
}
