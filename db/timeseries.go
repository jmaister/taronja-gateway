package db

import (
	"fmt"
	"log"
	"time"
)

// TimeSeriesGranularity is the bucket size for GetTimeSeries. Kept as a
// plain string type here rather than importing the OpenAPI-generated
// api.TimeSeriesGranularity, which would create a db -> api dependency
// this package doesn't otherwise have — handlers/api_statistics.go
// converts between the two at the boundary.
type TimeSeriesGranularity string

const (
	GranularityMinute TimeSeriesGranularity = "minute"
	GranularityHour   TimeSeriesGranularity = "hour"
	GranularityDay    TimeSeriesGranularity = "day"
	GranularityWeek   TimeSeriesGranularity = "week"
	GranularityMonth  TimeSeriesGranularity = "month"
)

// maxSpanForGranularity caps how large a start/end range each granularity
// accepts, so e.g. a minute-bucketed request over a full year can't
// generate hundreds of thousands of (mostly empty) points. Enforced by
// ValidateTimeSeriesRange, not by GetTimeSeries itself.
var maxSpanForGranularity = map[TimeSeriesGranularity]time.Duration{
	GranularityMinute: 24 * time.Hour,
	GranularityHour:   31 * 24 * time.Hour,
	GranularityDay:    366 * 24 * time.Hour,
	GranularityWeek:   2 * 365 * 24 * time.Hour,
	GranularityMonth:  5 * 365 * 24 * time.Hour,
}

// ValidateTimeSeriesRange reports an error if granularity isn't
// recognized, if end is before start, or if [start, end] exceeds the
// maximum span allowed for that granularity — see maxSpanForGranularity.
func ValidateTimeSeriesRange(start, end time.Time, granularity TimeSeriesGranularity) error {
	maxSpan, ok := maxSpanForGranularity[granularity]
	if !ok {
		return fmt.Errorf("unknown time series granularity: %q", granularity)
	}
	if end.Before(start) {
		return fmt.Errorf("end_date must not be before start_date")
	}
	if end.Sub(start) > maxSpan {
		return fmt.Errorf("start_date/end_date span exceeds the maximum of %s allowed for granularity %q", maxSpan, granularity)
	}
	return nil
}

// sqliteBucketExpr returns the SQL that truncates timeColumn to its bucket start as
// an RFC 3339 UTC string. timeColumn is wrapped in stripUTCFormatSuffix because
// SQLite's date functions cannot parse the driver's time.Time format. Built with
// Sprintf, which is safe: both arguments come from fixed constants in this package.
func sqliteBucketExpr(timeColumn string, g TimeSeriesGranularity) (string, error) {
	col := stripUTCFormatSuffix(timeColumn)
	switch g {
	case GranularityMinute:
		return fmt.Sprintf(`strftime('%%Y-%%m-%%dT%%H:%%M:00Z', %s)`, col), nil
	case GranularityHour:
		return fmt.Sprintf(`strftime('%%Y-%%m-%%dT%%H:00:00Z', %s)`, col), nil
	case GranularityDay:
		return fmt.Sprintf(`strftime('%%Y-%%m-%%dT00:00:00Z', %s)`, col), nil
	case GranularityWeek:
		// 'weekday 0' moves forward to the next Sunday (or stays put if
		// the date already falls on one); subtracting 6 days lands on the
		// preceding Monday — the standard SQLite idiom for "start of the
		// ISO (Monday-first) week containing this date".
		return fmt.Sprintf(`strftime('%%Y-%%m-%%dT00:00:00Z', %s, 'weekday 0', '-6 days')`, col), nil
	case GranularityMonth:
		return fmt.Sprintf(`strftime('%%Y-%%m-01T00:00:00Z', %s)`, col), nil
	default:
		return "", fmt.Errorf("unknown time series granularity: %q", g)
	}
}

// stripUTCFormatSuffix extracts the first 19 characters ("YYYY-MM-DD HH:MM:SS") of
// timeColumn, dropping the offset/zone suffix the driver stores. The value is then
// read as UTC, which is only correct because timestamps are normalized to UTC
// before writing (TrafficMetric.BeforeCreate, session.NewTrafficMetric).
func stripUTCFormatSuffix(timeColumn string) string {
	return fmt.Sprintf("substr(%s, 1, 19)", timeColumn)
}

// truncateToBucket returns the UTC start of t's bucket for the given
// granularity — the Go-side equivalent of sqliteBucketExpr, used only to
// generate the full expected list of bucket boundaries (including empty
// ones) that GetTimeSeries backfills its sparse SQL query results
// against. Not used for the aggregation itself.
func truncateToBucket(t time.Time, g TimeSeriesGranularity) time.Time {
	t = t.UTC()
	switch g {
	case GranularityMinute:
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, time.UTC)
	case GranularityHour:
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, time.UTC)
	case GranularityWeek:
		day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		// Go's time.Weekday: Sunday=0 ... Saturday=6. Back up to the
		// preceding Monday (0 days back if already Monday, 6 if Sunday).
		offset := (int(day.Weekday()) + 6) % 7
		return day.AddDate(0, 0, -offset)
	case GranularityMonth:
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	default: // GranularityDay and any unrecognized value
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	}
}

// nextBucket advances a bucket start t to the next one.
func nextBucket(t time.Time, g TimeSeriesGranularity) time.Time {
	switch g {
	case GranularityMinute:
		return t.Add(time.Minute)
	case GranularityHour:
		return t.Add(time.Hour)
	case GranularityWeek:
		return t.AddDate(0, 0, 7)
	case GranularityMonth:
		return t.AddDate(0, 1, 0)
	default: // GranularityDay
		return t.AddDate(0, 0, 1)
	}
}

// TimeSeriesPoint is one bucket of GetTimeSeries's result.
type TimeSeriesPoint struct {
	Timestamp time.Time
	// RequestCount is the total number of requests recorded in this bucket.
	RequestCount int
	// UniqueFingerprints is the number of distinct ClientInfo.Fingerprint
	// values seen in this bucket — a proxy for unique visitors/devices,
	// including anonymous ones. See doc/middleware/ja4-fingerprint.md for
	// what "fingerprint" means and its reliability caveats.
	UniqueFingerprints int
	// NewVisitors is how many of UniqueFingerprints had their first-ever
	// appearance (across the entire table's history, not just the
	// requested range) fall within this bucket. Always <= UniqueFingerprints.
	NewVisitors int
	// ReturningVisitors is UniqueFingerprints - NewVisitors: fingerprints
	// active in this bucket that were already known from an earlier one
	// (possibly long before the requested range even started).
	ReturningVisitors int
	// UniqueUsers is the number of distinct authenticated user IDs seen in
	// this bucket. Excludes anonymous traffic entirely (an empty UserID
	// never counts as "one more unique user").
	UniqueUsers int
	// ErrorCount is the number of requests with an HTTP status of 400 or
	// higher in this bucket.
	ErrorCount int
	// AverageResponseTimeMs is the average response time, in milliseconds,
	// for requests in this bucket. Zero for an empty (backfilled) bucket.
	AverageResponseTimeMs float64
}

// GetTimeSeries returns one point per bucket in [filter.Start, filter.End],
// chronologically, with zero-valued buckets filled in. Counts come from a single
// grouped query; NewVisitors comes from a second, range-bounded query on the
// fingerprint_first_seen table. Callers must first check the range with
// ValidateTimeSeriesRange.
func (r *TrafficMetricRepositoryDB) GetTimeSeries(filter TrafficFilter, granularity TimeSeriesGranularity) ([]TimeSeriesPoint, error) {
	if filter.Start == nil || filter.End == nil {
		return nil, fmt.Errorf("time series needs both start and end")
	}
	startDate, endDate := filter.Start.UTC(), filter.End.UTC()

	bucketExpr, err := sqliteBucketExpr("timestamp", granularity)
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

	byBucket := make(map[time.Time]TimeSeriesPoint, len(rows))
	for _, row := range rows {
		ts, parseErr := time.Parse(time.RFC3339, row.Bucket)
		if parseErr != nil {
			log.Printf("Error parsing time series bucket %q: %v", row.Bucket, parseErr)
			continue
		}
		newVisitors := newVisitorsByBucket[ts]
		byBucket[ts] = TimeSeriesPoint{
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

	points := make([]TimeSeriesPoint, 0, len(byBucket))
	for b := truncateToBucket(startDate, granularity); !b.After(endDate); b = nextBucket(b, granularity) {
		if p, ok := byBucket[b]; ok {
			points = append(points, p)
		} else {
			points = append(points, TimeSeriesPoint{Timestamp: b})
		}
	}

	return points, nil
}

// getNewVisitorsByBucket counts, per bucket, fingerprints whose first-ever
// appearance falls in it; absent buckets mean 0. The first appearance comes from
// the fingerprint_first_seen table (kept by a trigger on traffic_metrics), so the
// query is bounded by the range. When filter narrows rows beyond the time range,
// a fingerprint only counts if it also has a matching row in the bucket.
func (r *TrafficMetricRepositoryDB) getNewVisitorsByBucket(filter TrafficFilter, startDate, endDate time.Time, granularity TimeSeriesGranularity) (map[time.Time]int, error) {
	bucketExpr, err := sqliteBucketExpr("fs.first_timestamp", granularity)
	if err != nil {
		return nil, err
	}

	q := r.DB.Table("fingerprint_first_seen AS fs").
		Select(bucketExpr+" AS bucket, COUNT(*) AS new_visitors").
		Where("fs.first_timestamp BETWEEN ? AND ?", startDate, endDate)

	rowFilter := filter
	rowFilter.Start, rowFilter.End = nil, nil
	if rowFilter != (TrafficFilter{}) {
		rowBucket, err := sqliteBucketExpr("traffic_metrics.timestamp", granularity)
		if err != nil {
			return nil, err
		}
		matching := filter.Apply(r.DB.Model(&TrafficMetric{}).Select("1").
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
