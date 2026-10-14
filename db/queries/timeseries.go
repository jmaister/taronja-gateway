package queries

import (
	"fmt"
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

// BucketExpr returns the SQL that truncates timeColumn to its bucket start as
// an RFC 3339 UTC string. timeColumn is wrapped in stripUTCFormatSuffix because
// SQLite's date functions cannot parse the driver's time.Time format. Built with
// Sprintf, which is safe: both arguments come from fixed constants in this package.
func BucketExpr(timeColumn string, g TimeSeriesGranularity) (string, error) {
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

// TruncateToBucket returns the UTC start of t's bucket for the given
// granularity — the Go-side equivalent of sqliteBucketExpr, used only to
// generate the full expected list of bucket boundaries (including empty
// ones) that GetTimeSeries backfills its sparse SQL query results
// against. Not used for the aggregation itself.
func TruncateToBucket(t time.Time, g TimeSeriesGranularity) time.Time {
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

// NextBucket advances a bucket start t to the next one.
func NextBucket(t time.Time, g TimeSeriesGranularity) time.Time {
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
