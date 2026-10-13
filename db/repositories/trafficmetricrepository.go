package repositories

import (
	"fmt"
	"log"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/db/queries"
	"gorm.io/gorm"
)

// TrafficMetricRepository interface defines methods for managing request statistics.
type TrafficMetricRepository interface {
	Create(stat *db.TrafficMetric) error
	// CreateBatch stores records in chunked INSERTs (createBatchChunkSize), each in its
	// transaction, so N records cost close to one writer-lock acquisition. A batch
	// larger than one chunk is valid input: the background flusher can fall behind.
	CreateBatch(stats []*db.TrafficMetric) error
	FindByPath(path string, limit int) ([]db.TrafficMetric, error)
	// The methods below all take a TrafficFilter (see trafficfilter.go).
	CountRequests(filter queries.TrafficFilter) (int64, error)
	AverageResponseTimeNs(filter queries.TrafficFilter) (float64, error)
	AverageResponseSize(filter queries.TrafficFilter) (float64, error)
	// CountBy groups the filtered requests by dimension. Keys are the
	// column's values rendered as strings (status codes included).
	CountBy(dimension queries.Dimension, filter queries.TrafficFilter) (map[string]int, error)
	ListRequestDetails(filter queries.TrafficFilter) ([]db.TrafficMetricWithUser, error)
	// GetTimeSeries returns request/visitor counts bucketed over time — see
	// db/queries/timeseries.go for granularity options and range validation
	// (ValidateTimeSeriesRange). filter.Start and filter.End are required.
	GetTimeSeries(filter queries.TrafficFilter, granularity queries.TimeSeriesGranularity) ([]queries.TimeSeriesPoint, error)
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
func (r *TrafficMetricRepositoryDB) Create(stat *db.TrafficMetric) error {
	if err := r.DB.Create(stat).Error; err != nil {
		log.Printf("Error creating request statistic: %v", err)
		return err
	}
	return nil
}

// createBatchChunkSize bounds the rows per INSERT. SQLite fails the whole statement
// above its bound-parameter limit (as low as 999); 30 columns x 100 rows = 3000.
const createBatchChunkSize = 100

// CreateBatch stores multiple request statistic records in as few
// INSERT/transactions as fit under SQLite's bound-parameter limit (see
// createBatchChunkSize) — GORM's CreateInBatches chunks the slice and
// wraps each chunk in its own transaction — rather than one round trip per
// record. This is what BatchingTrafficMetricRepository relies on for its
// whole benefit: on SQLite in particular, every write transaction takes a
// single process-wide writer lock and (outside WAL mode) an fsync, so N
// records committed together cost close to one of those instead of N.
//
// A batch larger than createBatchChunkSize is valid input, not a caller
// error: BatchingTrafficMetricRepository buffers Create calls and flushes
// whatever accumulated, and under a burst the background flush goroutine
// can fall behind Create's callers (Create itself is just an in-memory
// append, so a caller never blocks or gets backpressure). CreateInBatches
// handles that gracefully — several chunked INSERTs — where a single
// unchunked Create(&stats) would either fail outright ("too many SQL
// variables") or, on a build with a higher limit, still hold the writer
// lock for one very large transaction.
func (r *TrafficMetricRepositoryDB) CreateBatch(stats []*db.TrafficMetric) error {
	if len(stats) == 0 {
		return nil
	}
	if err := r.DB.CreateInBatches(&stats, createBatchChunkSize).Error; err != nil {
		log.Printf("Error creating batch of %d request statistics: %v", len(stats), err)
		return err
	}
	return nil
}

// FindByPath retrieves statistics for a specific path with limit.
func (r *TrafficMetricRepositoryDB) FindByPath(path string, limit int) ([]db.TrafficMetric, error) {
	var stats []db.TrafficMetric
	err := r.DB.Where("path = ?", path).Limit(limit).Order("timestamp DESC").Find(&stats).Error
	if err != nil {
		log.Printf("Error finding statistics by path: %v", err)
		return nil, err
	}
	return stats, nil
}

func (r *TrafficMetricRepositoryDB) filtered(filter queries.TrafficFilter) *gorm.DB {
	return filter.Apply(r.DB.Model(&db.TrafficMetric{}))
}

// CountRequests returns the number of requests matching filter.
func (r *TrafficMetricRepositoryDB) CountRequests(filter queries.TrafficFilter) (int64, error) {
	var count int64
	if err := r.filtered(filter).Count(&count).Error; err != nil {
		log.Printf("Error counting requests: %v", err)
		return 0, err
	}
	return count, nil
}

func (r *TrafficMetricRepositoryDB) average(column string, filter queries.TrafficFilter) (float64, error) {
	var result struct{ Average float64 }
	err := r.filtered(filter).Select("COALESCE(AVG(" + column + "), 0) as average").Scan(&result).Error
	if err != nil {
		log.Printf("Error calculating average %s: %v", column, err)
		return 0, err
	}
	return result.Average, nil
}

// AverageResponseTimeNs returns the average response time, in nanoseconds.
func (r *TrafficMetricRepositoryDB) AverageResponseTimeNs(filter queries.TrafficFilter) (float64, error) {
	return r.average("response_time_ns", filter)
}

// AverageResponseSize returns the average response size, in bytes.
func (r *TrafficMetricRepositoryDB) AverageResponseSize(filter queries.TrafficFilter) (float64, error) {
	return r.average("response_size", filter)
}

// CountBy returns request counts per distinct value of dimension.
func (r *TrafficMetricRepositoryDB) CountBy(dimension queries.Dimension, filter queries.TrafficFilter) (map[string]int, error) {
	expr, join, ok := dimension.SQL()
	if !ok {
		return nil, fmt.Errorf("unknown traffic dimension: %q", dimension)
	}
	q := r.filtered(filter).Select("CAST(" + expr + " AS TEXT) as dimension_value, COUNT(*) as count")
	if join != "" {
		q = q.Joins(join)
	}
	var rows []struct {
		DimensionValue string
		Count          int
	}
	if err := q.Group("dimension_value").Scan(&rows).Error; err != nil {
		log.Printf("Error counting requests by %s: %v", dimension, err)
		return nil, err
	}
	counts := make(map[string]int, len(rows))
	for _, row := range rows {
		counts[row.DimensionValue] = row.Count
	}
	return counts, nil
}

// ListRequestDetails returns the requests matching filter, newest first.
func (r *TrafficMetricRepositoryDB) ListRequestDetails(filter queries.TrafficFilter) ([]db.TrafficMetricWithUser, error) {
	var stats []db.TrafficMetricWithUser
	err := r.filtered(filter).Preload("User").Order("timestamp DESC").Find(&stats).Error
	if err != nil {
		log.Printf("Error listing request details: %v", err)
		return nil, err
	}
	return stats, nil
}
