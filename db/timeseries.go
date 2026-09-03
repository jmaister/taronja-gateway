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

// sqliteBucketExpr returns the SQL expression that truncates timeColumn
// (a column or expression yielding a value in the format
// stripUTCFormatSuffix expects) down to the start of its bucket for the
// given granularity, formatted as an RFC 3339 UTC string.
//
// timeColumn is always wrapped in stripUTCFormatSuffix first — see that
// function's doc comment for why: the pure-Go SQLite driver this project
// uses (modernc.org/sqlite) stores a Go time.Time as its default
// .String() representation ("2026-06-10 08:00:00 +0000 UTC"), which none
// of SQLite's date/time functions (strftime and friends) parse as-is.
//
// Safe to build with fmt.Sprintf directly into a SELECT (not a bound
// parameter) because granularity is always one of the fixed constants
// above and timeColumn is always one of a small set of column names this
// package controls — never raw user input.
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

// stripUTCFormatSuffix returns a SQL expression that extracts just the
// "YYYY-MM-DD HH:MM:SS" prefix (the first 19 characters) of timeColumn —
// one of the date-time formats SQLite's own functions do recognize —
// discarding the trailing offset/zone-name suffix
// (" +0000 UTC"/" +0200 CEST"/...) that modernc.org/sqlite's stored
// representation of a Go time.Time always has.
//
// This treats the extracted "YYYY-MM-DD HH:MM:SS" as UTC wall-clock time
// directly, with no offset conversion — correct only because every
// TrafficMetric.Timestamp is normalized to UTC before it's ever written
// (see TrafficMetric.BeforeCreate and session.NewTrafficMetric). A
// non-UTC stored timestamp would silently bucket at its own local
// wall-clock hour instead of the equivalent UTC one; confirmed by hand
// against a real DB before relying on this, alongside confirming the raw
// stored format itself (this function's whole reason to exist) the same
// way — see the commit that introduced this file for both repros.
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

// GetTimeSeries returns one TimeSeriesPoint per bucket across
// [startDate, endDate] at the given granularity, in chronological order,
// including zero-valued buckets for windows with no recorded requests at
// all — a continuous series is much easier to chart correctly than one
// with gaps every time a quiet minute/hour/day happens to fall in range.
//
// The per-bucket counts (requests, distinct fingerprints, distinct users,
// errors, average response time) are computed with a single grouped SQL
// query — not by fetching matching rows and aggregating them in Go, which
// an earlier version of this method did and which doesn't scale: it pulls
// every matching row's data across the wire and holds it (plus a
// per-bucket hash set for each distinct-count field) in application
// memory, when the database can compute the same result set-side, transfer
// only the (small, one-row-per-bucket) aggregated result, and use an index
// on top of it.
//
// NewVisitors needs a second query: "how many fingerprints active in this
// bucket were seen for the very first time, ever" isn't answerable from
// rows inside [startDate, endDate] alone — a fingerprint's true first
// appearance can be arbitrarily far in the past. That query
// (MIN(timestamp) GROUP BY fingerprint over the whole table) is why
// ClientInfo.Fingerprint is indexed.
//
// Callers must validate the range against ValidateTimeSeriesRange first;
// this method does not itself enforce a maximum number of buckets, so an
// unvalidated huge range could produce an equally huge backfilled slice.
func (r *TrafficMetricRepositoryDB) GetTimeSeries(startDate, endDate time.Time, granularity TimeSeriesGranularity) ([]TimeSeriesPoint, error) {
	// See trafficmetricrepository.go's FindByDateRange comment for why:
	// `timestamp BETWEEN ? AND ?` is a text comparison against
	// modernc.org/sqlite's stored representation, which TrafficMetric.BeforeCreate
	// always normalizes to UTC — the query bounds need the same treatment,
	// or a non-UTC caller (e.g. a naive time.Now()) would compare against
	// the wrong string and silently match nothing (or the wrong rows).
	startDate, endDate = startDate.UTC(), endDate.UTC()

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
	err = r.DB.Model(&TrafficMetric{}).
		Select(selectExpr).
		Where("timestamp BETWEEN ? AND ?", startDate, endDate).
		Group("bucket").
		Order("bucket").
		Scan(&rows).Error
	if err != nil {
		log.Printf("Error getting time series: %v", err)
		return nil, err
	}

	newVisitorsByBucket, err := r.getNewVisitorsByBucket(startDate, endDate, granularity)
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

// getNewVisitorsByBucket returns, for each bucket in [startDate, endDate],
// how many distinct fingerprints had their first-ever appearance (across
// the whole TrafficMetric table's history, not just this range) fall in
// that bucket. Buckets with zero new visitors are simply absent from the
// result map — GetTimeSeries's backfill loop treats a missing key as 0.
//
// The inner query (MIN(timestamp) GROUP BY fingerprint) necessarily scans
// the entire table's fingerprint/timestamp columns, not just the
// requested range — a fingerprint's true first-ever appearance can't be
// known from a date-filtered scan alone. This is the one part of
// GetTimeSeries whose cost doesn't shrink with a narrower requested
// range, which is why ClientInfo.Fingerprint is indexed: the index lets
// SQLite compute the per-fingerprint minimum without reading full row
// data for every historical request.
func (r *TrafficMetricRepositoryDB) getNewVisitorsByBucket(startDate, endDate time.Time, granularity TimeSeriesGranularity) (map[time.Time]int, error) {
	bucketExpr, err := sqliteBucketExpr("first_ts", granularity)
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`
		WITH first_seen AS (
			SELECT fingerprint, MIN(timestamp) as first_ts
			FROM traffic_metrics
			WHERE fingerprint != ''
			GROUP BY fingerprint
		)
		SELECT %s as bucket, COUNT(*) as new_visitors
		FROM first_seen
		WHERE first_ts BETWEEN ? AND ?
		GROUP BY bucket`, bucketExpr)

	var rows []struct {
		Bucket      string
		NewVisitors int
	}
	if err := r.DB.Raw(query, startDate, endDate).Scan(&rows).Error; err != nil {
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
