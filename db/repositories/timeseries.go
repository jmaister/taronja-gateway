package repositories

import (
	"fmt"
	"log"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/db/queries"
)

// GetTimeSeries returns one point per bucket in [filter.Start, filter.End],
// chronologically, with zero-valued buckets filled in. Counts come from a single
// grouped query; NewVisitors comes from a second, range-bounded query on the
// fingerprint_first_seen table. Callers must first check the range with
// ValidateTimeSeriesRange.
func (r *TrafficMetricRepositoryDB) GetTimeSeries(filter queries.TrafficFilter, granularity queries.TimeSeriesGranularity) ([]queries.TimeSeriesPoint, error) {
	if filter.Start == nil || filter.End == nil {
		return nil, fmt.Errorf("time series needs both start and end")
	}
	startDate, endDate := filter.Start.UTC(), filter.End.UTC()

	bucketExpr, err := queries.BucketExpr("timestamp", granularity)
	if err != nil {
		return nil, err
	}

	var rows []struct {
		Bucket             string
		RequestCount       int
		UniqueFingerprints int
		UniqueUsers        int
		ErrorCount         int
		AvgResponseTimeNs  float64
	}
	// NULLIF(col, '') turns an empty string into SQL NULL first, so
	// COUNT(DISTINCT ...) — which already ignores NULLs — doesn't count
	// "no fingerprint"/"not authenticated" as one more distinct value.
	selectExpr := fmt.Sprintf(`%s as bucket,
		COUNT(*) as request_count,
		COUNT(DISTINCT NULLIF(fingerprint, '')) as unique_fingerprints,
		COUNT(DISTINCT NULLIF(user_id, '')) as unique_users,
		SUM(CASE WHEN http_status >= 400 THEN 1 ELSE 0 END) as error_count,
		AVG(response_time_ns) as avg_response_time_ns`, bucketExpr)
	err = r.filtered(filter).
		Select(selectExpr).
		Group("bucket").
		Order("bucket").
		Scan(&rows).Error
	if err != nil {
		log.Printf("Error getting time series: %v", err)
		return nil, err
	}

	newVisitorsByBucket, err := r.getNewVisitorsByBucket(filter, startDate, endDate, granularity)
	if err != nil {
		return nil, err
	}

	byBucket := make(map[time.Time]queries.TimeSeriesPoint, len(rows))
	for _, row := range rows {
		ts, parseErr := time.Parse(time.RFC3339, row.Bucket)
		if parseErr != nil {
			log.Printf("Error parsing time series bucket %q: %v", row.Bucket, parseErr)
			continue
		}
		newVisitors := newVisitorsByBucket[ts]
		byBucket[ts] = queries.TimeSeriesPoint{
			Timestamp:             ts,
			RequestCount:          row.RequestCount,
			UniqueFingerprints:    row.UniqueFingerprints,
			NewVisitors:           newVisitors,
			ReturningVisitors:     row.UniqueFingerprints - newVisitors,
			UniqueUsers:           row.UniqueUsers,
			ErrorCount:            row.ErrorCount,
			AverageResponseTimeMs: row.AvgResponseTimeNs / 1e6, // ns -> ms
		}
	}

	points := make([]queries.TimeSeriesPoint, 0, len(byBucket))
	for b := queries.TruncateToBucket(startDate, granularity); !b.After(endDate); b = queries.NextBucket(b, granularity) {
		if p, ok := byBucket[b]; ok {
			points = append(points, p)
		} else {
			points = append(points, queries.TimeSeriesPoint{Timestamp: b})
		}
	}

	return points, nil
}

// getNewVisitorsByBucket counts, per bucket, fingerprints whose first-ever
// appearance falls in it; absent buckets mean 0. The first appearance comes from
// the fingerprint_first_seen table (kept by a trigger on traffic_metrics), so the
// query is bounded by the range. When filter narrows rows beyond the time range,
// a fingerprint only counts if it also has a matching row in the bucket.
func (r *TrafficMetricRepositoryDB) getNewVisitorsByBucket(filter queries.TrafficFilter, startDate, endDate time.Time, granularity queries.TimeSeriesGranularity) (map[time.Time]int, error) {
	bucketExpr, err := queries.BucketExpr("fs.first_timestamp", granularity)
	if err != nil {
		return nil, err
	}

	q := r.DB.Table("fingerprint_first_seen AS fs").
		Select(bucketExpr+" AS bucket, COUNT(*) AS new_visitors").
		Where("fs.first_timestamp BETWEEN ? AND ?", startDate, endDate)

	rowFilter := filter
	rowFilter.Start, rowFilter.End = nil, nil
	if rowFilter != (queries.TrafficFilter{}) {
		rowBucket, err := queries.BucketExpr("traffic_metrics.timestamp", granularity)
		if err != nil {
			return nil, err
		}
		matching := filter.Apply(r.DB.Model(&db.TrafficMetric{}).Select("1").
			Where("traffic_metrics.fingerprint = fs.fingerprint").
			Where(rowBucket + " = " + bucketExpr))
		q = q.Where("EXISTS (?)", matching)
	}

	var rows []struct {
		Bucket      string
		NewVisitors int
	}
	if err := q.Group("bucket").Scan(&rows).Error; err != nil {
		log.Printf("Error getting new visitors by bucket: %v", err)
		return nil, err
	}

	result := make(map[time.Time]int, len(rows))
	for _, row := range rows {
		ts, parseErr := time.Parse(time.RFC3339, row.Bucket)
		if parseErr != nil {
			log.Printf("Error parsing new-visitors bucket %q: %v", row.Bucket, parseErr)
			continue
		}
		result[ts] = row.NewVisitors
	}
	return result, nil
}
