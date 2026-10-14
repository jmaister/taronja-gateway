package repositories

import (
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/db/queries"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTimeSeries_BucketsAndBackfillsAndCountsDistinctly(t *testing.T) {
	db.SetupTestDB(t.Name())
	repo := &TrafficMetricRepositoryDB{DB: db.GetConnection()}

	day := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)

	seed := []db.TrafficMetric{
		// Hour 08: two requests, same fingerprint -> 1 unique fingerprint;
		// one authenticated (user "alice"), one anonymous.
		{
			Timestamp: day.Add(8 * time.Hour), HttpStatus: 200, ResponseTimeNs: 100_000_000,
			UserID:     "alice",
			ClientInfo: db.ClientInfo{Fingerprint: "fp-1"},
		},
		{
			Timestamp: day.Add(8*time.Hour + 30*time.Minute), HttpStatus: 200, ResponseTimeNs: 200_000_000,
			ClientInfo: db.ClientInfo{Fingerprint: "fp-1"},
		},
		// Hour 09: one error request, distinct fingerprint, no user.
		{
			Timestamp: day.Add(9 * time.Hour), HttpStatus: 500, ResponseTimeNs: 300_000_000,
			ClientInfo: db.ClientInfo{Fingerprint: "fp-2"},
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

	points, err := repo.GetTimeSeries(rangeFilter(day, day.Add(13*time.Hour)), queries.GranularityHour)
	require.NoError(t, err)

	byHour := make(map[int]queries.TimeSeriesPoint, len(points))
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

func TestGetTimeSeries_NewVsReturningVisitors(t *testing.T) {
	db.SetupTestDB(t.Name())
	repo := &TrafficMetricRepositoryDB{DB: db.GetConnection()}

	day := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	queryStart := day.Add(8 * time.Hour)

	seed := []db.TrafficMetric{
		// fp-veteran: first ever seen well BEFORE the query window even
		// starts. Its only appearance *inside* the window (hour 11) must
		// count as returning, not new — and must not appear in any
		// bucket's NewVisitors at all, since its true first-seen bucket
		// is outside the requested range entirely.
		{Timestamp: day.Add(-30 * 24 * time.Hour), HttpStatus: 200, ClientInfo: db.ClientInfo{Fingerprint: "fp-veteran"}},
		{Timestamp: day.Add(11 * time.Hour), HttpStatus: 200, ClientInfo: db.ClientInfo{Fingerprint: "fp-veteran"}},

		// fp-new-then-back: first ever seen at hour 8, *inside* the
		// window, then seen again at hour 10. Hour 8 must count it as
		// new; hour 10 must count it as returning.
		{Timestamp: day.Add(8 * time.Hour), HttpStatus: 200, ClientInfo: db.ClientInfo{Fingerprint: "fp-new-then-back"}},
		{Timestamp: day.Add(10 * time.Hour), HttpStatus: 200, ClientInfo: db.ClientInfo{Fingerprint: "fp-new-then-back"}},

		// fp-brand-new: only ever appears once, at hour 9, with no prior
		// history at all. Purely new, no return visit in range.
		{Timestamp: day.Add(9 * time.Hour), HttpStatus: 200, ClientInfo: db.ClientInfo{Fingerprint: "fp-brand-new"}},
	}
	for i := range seed {
		seed[i].HttpMethod = "GET"
		seed[i].Path = "/test"
		require.NoError(t, repo.DB.Create(&seed[i]).Error)
	}

	points, err := repo.GetTimeSeries(rangeFilter(queryStart, queryStart.Add(4*time.Hour)), queries.GranularityHour)
	require.NoError(t, err)

	byHour := make(map[int]queries.TimeSeriesPoint, len(points))
	for _, p := range points {
		byHour[p.Timestamp.Hour()] = p
	}

	hour8 := byHour[8]
	assert.Equal(t, 1, hour8.UniqueFingerprints)
	assert.Equal(t, 1, hour8.NewVisitors, "fp-new-then-back's first-ever appearance is this bucket")
	assert.Equal(t, 0, hour8.ReturningVisitors)

	hour9 := byHour[9]
	assert.Equal(t, 1, hour9.UniqueFingerprints)
	assert.Equal(t, 1, hour9.NewVisitors, "fp-brand-new has no prior history at all")
	assert.Equal(t, 0, hour9.ReturningVisitors)

	hour10 := byHour[10]
	assert.Equal(t, 1, hour10.UniqueFingerprints)
	assert.Equal(t, 0, hour10.NewVisitors, "fp-new-then-back was already seen at hour 8")
	assert.Equal(t, 1, hour10.ReturningVisitors)

	hour11 := byHour[11]
	assert.Equal(t, 1, hour11.UniqueFingerprints)
	assert.Equal(t, 0, hour11.NewVisitors, "fp-veteran's true first-ever appearance is 30 days before this query range")
	assert.Equal(t, 1, hour11.ReturningVisitors)
}

func rangeFilter(start, end time.Time) queries.TrafficFilter {
	return queries.TrafficFilter{Start: &start, End: &end}
}

func TestFingerprintFirstSeen_MaintainedOnCreateAndBatch(t *testing.T) {
	db.SetupTestDB(t.Name())
	repo := &TrafficMetricRepositoryDB{DB: db.GetConnection()}
	day := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	mk := func(fp string, at time.Time) *db.TrafficMetric {
		return &db.TrafficMetric{HttpMethod: "GET", Path: "/x", HttpStatus: 200, Timestamp: at, ClientInfo: db.ClientInfo{Fingerprint: fp}}
	}
	require.NoError(t, repo.Create(mk("a", day.Add(5*time.Hour))))
	require.NoError(t, repo.CreateBatch([]*db.TrafficMetric{
		mk("a", day.Add(2*time.Hour)), // earlier row arriving later lowers first_timestamp
		mk("b", day.Add(3*time.Hour)),
		mk("", day.Add(1*time.Hour)),
	}))

	var rows []struct {
		Fingerprint    string
		FirstTimestamp time.Time
	}
	require.NoError(t, db.GetConnection().Raw("SELECT fingerprint, first_timestamp FROM fingerprint_first_seen ORDER BY fingerprint").Scan(&rows).Error)
	require.Len(t, rows, 2, "empty fingerprints are not tracked")
	assert.True(t, rows[0].FirstTimestamp.Equal(day.Add(2*time.Hour)))
	assert.True(t, rows[1].FirstTimestamp.Equal(day.Add(3*time.Hour)))
}

func TestGetTimeSeries_NewVisitorsHonorsFilter(t *testing.T) {
	db.SetupTestDB(t.Name())
	repo := &TrafficMetricRepositoryDB{DB: db.GetConnection()}
	day := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	seed := []db.TrafficMetric{
		{Timestamp: day.Add(1 * time.Hour), ClientInfo: db.ClientInfo{Fingerprint: "es-user", Country: "ES"}},
		{Timestamp: day.Add(1 * time.Hour), ClientInfo: db.ClientInfo{Fingerprint: "us-user", Country: "US"}},
	}
	for i := range seed {
		seed[i].HttpMethod, seed[i].Path, seed[i].HttpStatus = "GET", "/x", 200
		require.NoError(t, repo.DB.Create(&seed[i]).Error)
	}

	f := rangeFilter(day, day.Add(2*time.Hour))
	points, err := repo.GetTimeSeries(f, queries.GranularityHour)
	require.NoError(t, err)
	assert.Equal(t, 2, points[1].NewVisitors)

	f.Country = "ES"
	points, err = repo.GetTimeSeries(f, queries.GranularityHour)
	require.NoError(t, err)
	assert.Equal(t, 1, points[1].UniqueFingerprints)
	assert.Equal(t, 1, points[1].NewVisitors)
	assert.Equal(t, 0, points[1].ReturningVisitors)
}
