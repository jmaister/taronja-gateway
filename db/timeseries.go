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

// truncateToBucket returns the UTC start of t's bucket for the given
// granularity. GetTimeSeries uses this to group individually-fetched rows
// in Go — not a SQL GROUP BY on a truncated `timestamp` column, since the
// pure-Go SQLite driver this project uses (modernc.org/sqlite) stores
// time.Time in Go's default string representation ("2026-06-10 08:00:00
// +0000 UTC"), which SQLite's own date/time functions (strftime and
// friends) don't parse — they only recognize a handful of specific
// ISO8601-ish formats. Comparisons like `timestamp BETWEEN ? AND ?` still
// work because the driver's own parameter-binding logic converts both
// sides consistently; a raw SQL function applied directly to the stored
// column bytes doesn't go through that conversion at all, and silently
// returns NULL instead of erroring. Confirmed by hand against a real
// in-memory DB before settling on this Go-side approach — see the
// commit that introduced this file for the exact repro.
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

// bucketAgg accumulates one bucket's worth of rows before being reduced to
// a TimeSeriesPoint — the running totals need real sets for the two
// distinct-count fields (a plain int can't be updated correctly without
// tracking which values it's already seen), unlike the other fields.
type bucketAgg struct {
	requestCount    int
	fingerprints    map[string]struct{}
	users           map[string]struct{}
	errorCount      int
	totalResponseNs int64
}

// GetTimeSeries returns one TimeSeriesPoint per bucket across
// [startDate, endDate] at the given granularity, in chronological order,
// including zero-valued buckets for windows with no recorded requests at
// all — a continuous series is much easier to chart correctly than one
// with gaps every time a quiet minute/hour/day happens to fall in range.
//
// Fetches every matching row's timestamp/fingerprint/user_id/status/
// response-time and buckets them in Go (see truncateToBucket's doc
// comment for why this isn't a SQL GROUP BY) rather than paginating — the
// same "load everything matching the filter" approach
// ListRequestDetails already uses elsewhere in this repository.
//
// Callers must validate the range against ValidateTimeSeriesRange first;
// this method does not itself enforce a maximum number of buckets, so an
// unvalidated huge range could produce an equally huge backfilled slice.
func (r *TrafficMetricRepositoryDB) GetTimeSeries(startDate, endDate time.Time, granularity TimeSeriesGranularity) ([]TimeSeriesPoint, error) {
	if _, ok := maxSpanForGranularity[granularity]; !ok {
		return nil, fmt.Errorf("unknown time series granularity: %q", granularity)
	}

	var rows []struct {
		Timestamp      time.Time
		Fingerprint    string
		UserID         string
		HttpStatus     int
		ResponseTimeNs int64
	}
	err := r.DB.Model(&TrafficMetric{}).
		Select("timestamp, fingerprint, user_id, http_status, response_time_ns").
		Where("timestamp BETWEEN ? AND ?", startDate, endDate).
		Find(&rows).Error
	if err != nil {
		log.Printf("Error getting time series: %v", err)
		return nil, err
	}

	buckets := make(map[time.Time]*bucketAgg)
	for _, row := range rows {
		b := truncateToBucket(row.Timestamp, granularity)
		agg, ok := buckets[b]
		if !ok {
			agg = &bucketAgg{fingerprints: make(map[string]struct{}), users: make(map[string]struct{})}
			buckets[b] = agg
		}
		agg.requestCount++
		if row.Fingerprint != "" {
			agg.fingerprints[row.Fingerprint] = struct{}{}
		}
		if row.UserID != "" {
			agg.users[row.UserID] = struct{}{}
		}
		if row.HttpStatus >= 400 {
			agg.errorCount++
		}
		agg.totalResponseNs += row.ResponseTimeNs
	}

	points := make([]TimeSeriesPoint, 0, len(buckets))
	for b := truncateToBucket(startDate, granularity); !b.After(endDate); b = nextBucket(b, granularity) {
		p := TimeSeriesPoint{Timestamp: b}
		if agg, ok := buckets[b]; ok {
			p.RequestCount = agg.requestCount
			p.UniqueFingerprints = len(agg.fingerprints)
			p.UniqueUsers = len(agg.users)
			p.ErrorCount = agg.errorCount
			if agg.requestCount > 0 {
				p.AverageResponseTimeMs = float64(agg.totalResponseNs) / float64(agg.requestCount) / 1e6
			}
		}
		points = append(points, p)
	}

	return points, nil
}
