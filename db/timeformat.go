package db

import (
	"fmt"
	"regexp"
	"time"
)

// storedTimestampLayouts are every text format a timestamp column in this
// database might contain, tried in order until one succeeds. RFC3339(Nano)
// covers what this driver (modernc.org/sqlite) normalizes a successfully-
// decoded DATETIME-affinity column to when scanned into a Go string (see
// db/migrations.go's doc comment on migrateTimestampsToUTC for how that was
// confirmed); the remaining entries cover Go's time.Time.String() format
// (what actually gets written on INSERT) — the "-0700 MST" form for a
// non-UTC zone or "-0700 UTC"/no-zone variants — for the rare case a value
// reaches this parser without having gone through that scan-time
// normalization first. Go's fractional-seconds parsing is flexible about
// digit count regardless of how many are in the layout itself (see
// time.Parse's doc comment), so one layout per format is enough; there's no
// need for separate 6-digit/9-digit variants.
//
// None of these match every value this driver can actually produce, though
// — see storedTimestampOffsetPrefix below for the case they miss, confirmed
// against a real database rather than assumed.
var storedTimestampLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02 15:04:05.999999 -0700 MST",
	"2006-01-02 15:04:05.999999-07:00",
	"2006-01-02 15:04:05-07:00",
	"2006-01-02 15:04:05",
}

// storedTimestampOffsetPrefix matches the "date time[.fraction] ±HHMM"
// prefix of Go's time.Time.String() format — deliberately not whatever
// trailing text follows it, which storedTimestampLayouts' "MST"-style verb
// only matches when that text is exactly one plain alphabetic zone name and
// nothing else. Two real, unrelated shapes break that assumption, both
// found against real pre-upgrade databases rather than reasoned out in
// advance: a time.Time with a FixedZone that has no name at all — what Go
// constructs for a JSON/RFC3339 timestamp with a bare numeric offset
// (Token.ExpiresAt, API-caller-supplied, stored as
// "2026-12-25 10:00:00 +0500 +0500" — the offset repeated verbatim as the
// "zone name" too) — and a time.Time that still carries its monotonic
// clock reading, appended after the zone name as " m=±<seconds>" (any
// value built from a bare time.Now() and never subjected to an operation
// that strips it — e.g. a genuinely old Session.CreatedAt from before this
// project normalized these — stored as
// "2026-09-03 22:30:04.74361174 +0100 BST m=+13.399250358"). The trailing
// text never changes the actual instant, which the numeric offset alone
// already fully determines, so parseStoredTimestamp's fallback below
// matches only the offset prefix and discards everything after it —
// zone name, monotonic reading, both, or neither — rather than trying to
// enumerate every shape that trailing text might take.
var storedTimestampOffsetPrefix = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}(\.\d+)? [+-]\d{4}`)

// parseStoredTimestamp parses a timestamp column's text value using
// whichever of storedTimestampLayouts matches, falling back to
// storedTimestampOffsetPrefix if none do, and returning the last layout's
// error if even that fails. Shared by CountersRepositoryDB.
// toUserCounterSummary (parsing a raw SQL query's string result) and
// migrateTimestampsToUTC (parsing existing rows to normalize them to UTC)
// — both are reading the same kind of driver-produced text, so duplicating
// this parsing logic in two places would just be two chances to let them
// drift apart.
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
