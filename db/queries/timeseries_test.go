package queries

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
			assert.Equal(t, tt.want, TruncateToBucket(wed, tt.granularity))
		})
	}
}

func TestTruncateToBucket_WeekOnMondayAndSunday(t *testing.T) {
	monday := time.Date(2026, 3, 16, 9, 0, 0, 0, time.UTC)
	sunday := time.Date(2026, 3, 22, 9, 0, 0, 0, time.UTC)
	wantMonday := time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)

	assert.Equal(t, wantMonday, TruncateToBucket(monday, GranularityWeek), "a Monday must truncate to itself")
	assert.Equal(t, wantMonday, TruncateToBucket(sunday, GranularityWeek), "a Sunday must truncate to the preceding Monday")
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
			assert.Equal(t, tt.want, NextBucket(start, tt.granularity))
		})
	}
}
