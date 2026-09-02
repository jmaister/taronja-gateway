package db

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateTimeSeriesRange(t *testing.T) {
	base := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		start       time.Time
		end         time.Time
		granularity TimeSeriesGranularity
		wantErr     bool
	}{
		{"valid minute range", base, base.Add(23 * time.Hour), GranularityMinute, false},
		{"minute range too long", base, base.Add(25 * time.Hour), GranularityMinute, true},
		{"valid day range", base, base.AddDate(0, 0, 300), GranularityDay, false},
		{"day range too long", base, base.AddDate(1, 1, 0), GranularityDay, true},
		{"end before start", base, base.Add(-time.Hour), GranularityDay, true},
		{"unknown granularity", base, base.Add(time.Hour), TimeSeriesGranularity("fortnight"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTimeSeriesRange(tt.start, tt.end, tt.granularity)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestTruncateToBucket(t *testing.T) {
	// 2026-03-18 is a Wednesday.
	wed := time.Date(2026, 3, 18, 14, 23, 45, 0, time.UTC)

	tests := []struct {
		name        string
		granularity TimeSeriesGranularity
		want        time.Time
	}{
		{"minute", GranularityMinute, time.Date(2026, 3, 18, 14, 23, 0, 0, time.UTC)},
		{"hour", GranularityHour, time.Date(2026, 3, 18, 14, 0, 0, 0, time.UTC)},
		{"day", GranularityDay, time.Date(2026, 3, 18, 0, 0, 0, 0, time.UTC)},
		{"week (Wednesday backs up to Monday)", GranularityWeek, time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)},
		{"month", GranularityMonth, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, truncateToBucket(wed, tt.granularity))
		})
	}
}

func TestTruncateToBucket_WeekOnMondayAndSunday(t *testing.T) {
	monday := time.Date(2026, 3, 16, 9, 0, 0, 0, time.UTC)
	sunday := time.Date(2026, 3, 22, 9, 0, 0, 0, time.UTC)
	wantMonday := time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)

	assert.Equal(t, wantMonday, truncateToBucket(monday, GranularityWeek), "a Monday must truncate to itself")
	assert.Equal(t, wantMonday, truncateToBucket(sunday, GranularityWeek), "a Sunday must truncate to the preceding Monday")
}

func TestNextBucket(t *testing.T) {
	start := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		granularity TimeSeriesGranularity
		want        time.Time
	}{
		{"minute", GranularityMinute, start.Add(time.Minute)},
		{"hour", GranularityHour, start.Add(time.Hour)},
		{"day", GranularityDay, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)},
		{"week", GranularityWeek, time.Date(2026, 2, 7, 0, 0, 0, 0, time.UTC)},
		{"month (Jan 31 -> Mar 3, Go's AddDate month-overflow semantics)", GranularityMonth, time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, nextBucket(start, tt.granularity))
		})
	}
}

func TestGetTimeSeries_BucketsAndBackfillsAndCountsDistinctly(t *testing.T) {
	SetupTestDB(t.Name())
	repo := &TrafficMetricRepositoryDB{DB: GetConnection()}

	day := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)

	seed := []TrafficMetric{
		// Hour 08: two requests, same fingerprint -> 1 unique fingerprint;
		// one authenticated (user "alice"), one anonymous.
		{
			Timestamp: day.Add(8 * time.Hour), HttpStatus: 200, ResponseTimeNs: 100_000_000,
			UserID:     "alice",
			ClientInfo: ClientInfo{Fingerprint: "fp-1"},
		},
		{
			Timestamp: day.Add(8*time.Hour + 30*time.Minute), HttpStatus: 200, ResponseTimeNs: 200_000_000,
			ClientInfo: ClientInfo{Fingerprint: "fp-1"},
		},
		// Hour 09: one error request, distinct fingerprint, no user.
		{
			Timestamp: day.Add(9 * time.Hour), HttpStatus: 500, ResponseTimeNs: 300_000_000,
			ClientInfo: ClientInfo{Fingerprint: "fp-2"},
		},
		// Hour 10: request with no fingerprint at all -> must not count as
		// a unique fingerprint (empty string, not a real value).
		{
			Timestamp: day.Add(10 * time.Hour), HttpStatus: 200, ResponseTimeNs: 400_000_000,
		},
		// Hour 12 left with zero requests entirely, to verify backfill.
	}
	for i := range seed {
		seed[i].HttpMethod = "GET"
		seed[i].Path = "/test"
		require.NoError(t, repo.DB.Create(&seed[i]).Error)
	}

	points, err := repo.GetTimeSeries(day, day.Add(13*time.Hour), GranularityHour)
	require.NoError(t, err)

	byHour := make(map[int]TimeSeriesPoint, len(points))
	for _, p := range points {
		byHour[p.Timestamp.Hour()] = p
	}

	// Backfill: every hour from 0 to 13 must be present, even ones with no data.
	assert.Len(t, points, 14, "expected one point per hour from 00:00 to 13:00 inclusive")
	assert.Equal(t, 0, byHour[12].RequestCount, "an hour with no requests must still appear, zero-valued")
	assert.Equal(t, 0.0, byHour[12].AverageResponseTimeMs)

	hour8 := byHour[8]
	assert.Equal(t, 2, hour8.RequestCount)
	assert.Equal(t, 1, hour8.UniqueFingerprints, "both requests shared the same fingerprint")
	assert.Equal(t, 1, hour8.UniqueUsers, "only one of the two requests was authenticated")
	assert.Equal(t, 0, hour8.ErrorCount)
	assert.InDelta(t, 150.0, hour8.AverageResponseTimeMs, 0.01, "average of 100ms and 200ms")

	hour9 := byHour[9]
	assert.Equal(t, 1, hour9.RequestCount)
	assert.Equal(t, 1, hour9.ErrorCount, "HTTP 500 counts as an error")
	assert.Equal(t, 0, hour9.UniqueUsers)

	hour10 := byHour[10]
	assert.Equal(t, 1, hour10.RequestCount)
	assert.Equal(t, 0, hour10.UniqueFingerprints, "an empty fingerprint must not count as a unique value")
}
