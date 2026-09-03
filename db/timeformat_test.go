package db

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseStoredTimestamp(t *testing.T) {
	testCases := []struct {
		name  string
		value string
		want  time.Time
	}{
		{
			name:  "RFC3339 with Z (driver-normalized UTC)",
			value: "2026-12-25T05:00:00Z",
			want:  time.Date(2026, 12, 25, 5, 0, 0, 0, time.UTC),
		},
		{
			name:  "RFC3339Nano with numeric offset (driver-normalized non-UTC)",
			value: "2026-06-01T08:00:00.123456789+01:00",
			want:  time.Date(2026, 6, 1, 8, 0, 0, 123456789, time.FixedZone("", 3600)),
		},
		{
			name:  "Go .String() format with a real (alphabetic) zone abbreviation",
			value: "2026-06-01 08:00:00.123456789 +0100 BST",
			want:  time.Date(2026, 6, 1, 8, 0, 0, 123456789, time.FixedZone("BST", 3600)),
		},
		{
			name:  "Go .String() format with no fractional seconds and UTC",
			value: "2026-06-01 08:00:00 +0000 UTC",
			want:  time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC),
		},
		{
			// The real case this test exists for: confirmed against an
			// actual pre-upgrade database. Token.ExpiresAt is API-caller-
			// supplied (a JSON RFC3339 timestamp with a bare numeric
			// offset, no named zone) — Go represents that as a FixedZone
			// whose name is the offset string itself, and this driver's
			// own decode falls back to raw passthrough text for it rather
			// than the usual RFC3339 normalization, since it isn't a
			// recognized named zone.
			name:  "raw passthrough with a numeric (unnamed) zone abbreviation",
			value: "2026-12-25 10:00:00 +0500 +0500",
			want:  time.Date(2026, 12, 25, 10, 0, 0, 0, time.FixedZone("", 5*60*60)),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseStoredTimestamp(tc.value)
			require.NoError(t, err)
			assert.True(t, tc.want.Equal(got), "want %v, got %v", tc.want, got)
			wantOffset := func() int { _, o := tc.want.Zone(); return o }()
			gotOffset := func() int { _, o := got.Zone(); return o }()
			assert.Equal(t, wantOffset, gotOffset, "offset mismatch: want %v, got %v", tc.want, got)
		})
	}
}

func TestParseStoredTimestamp_Unparseable(t *testing.T) {
	_, err := parseStoredTimestamp("not a timestamp at all")
	assert.Error(t, err)
}
