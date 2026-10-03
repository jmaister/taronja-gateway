package db

import (
	"fmt"
	"regexp"
	"time"
)

// storedTimestampLayouts are the text formats a timestamp column may hold, tried in
// order: RFC3339(Nano), which the driver normalizes DATETIME columns to when
// scanned into a string, then Go's time.Time.String() forms as written on INSERT.
// See storedTimestampOffsetPrefix for what these miss.
var storedTimestampLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02 15:04:05.999999 -0700 MST",
	"2006-01-02 15:04:05.999999-07:00",
	"2006-01-02 15:04:05-07:00",
	"2006-01-02 15:04:05",
}

// storedTimestampOffsetPrefix matches the "date time[.fraction] ±HHMM" prefix of
// time.Time.String() output. It is the fallback for values the layouts miss: a
// FixedZone without a name ("+0500 +0500") and a trailing monotonic reading
// (" m=+13.39"). The numeric offset alone determines the instant.
var storedTimestampOffsetPrefix = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}(\.\d+)? [+-]\d{4}`)

// parseStoredTimestamp parses a timestamp column's text using storedTimestampLayouts,
// then storedTimestampOffsetPrefix, returning an error if neither matches. Shared
// by toUserCounterSummary and migrateTimestampsToUTC.
func parseStoredTimestamp(value string) (time.Time, error) {
	var lastErr error
	for _, layout := range storedTimestampLayouts {
		if t, err := time.Parse(layout, value); err == nil {
			return t, nil
		} else {
			lastErr = err
		}
	}
	if prefix := storedTimestampOffsetPrefix.FindString(value); prefix != "" {
		if t, err := time.Parse("2006-01-02 15:04:05.999999999 -0700", prefix); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("value %q matched none of the recognized timestamp formats: %w", value, lastErr)
}
