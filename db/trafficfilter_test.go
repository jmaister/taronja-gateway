package db

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedFilterFixture(t *testing.T) (*TrafficMetricRepositoryDB, time.Time) {
	t.Helper()
	SetupTestDB(t.Name())
	repo := &TrafficMetricRepositoryDB{DB: GetConnection()}
	alice := User{Username: "alice", Email: "alice@example.com"}
	require.NoError(t, repo.DB.Create(&alice).Error)

	day := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	rows := []TrafficMetric{
		{Timestamp: day.Add(1 * time.Hour), HttpStatus: 200, UserID: alice.ID, ClientInfo: ClientInfo{Country: "ES", DeviceFamily: "Mac", OSFamily: "macOS", BrowserFamily: "Safari", Fingerprint: "fp-a", FingerprintType: "stable"}},
		{Timestamp: day.Add(2 * time.Hour), HttpStatus: 404, ClientInfo: ClientInfo{Country: "ES", DeviceFamily: "Mac", OSFamily: "macOS", BrowserFamily: "Chrome", Fingerprint: "fp-b", FingerprintType: "ja4h"}},
		{Timestamp: day.Add(3 * time.Hour), HttpStatus: 200, IsStaticAsset: true, ClientInfo: ClientInfo{Country: "FR", DeviceFamily: "iPhone", OSFamily: "iOS", BrowserFamily: "Safari", Fingerprint: "fp-b", FingerprintType: "ja4h"}},
		{Timestamp: day.Add(30 * time.Hour), HttpStatus: 500, ClientInfo: ClientInfo{DeviceFamily: "Other"}},
	}
	for i := range rows {
		rows[i].HttpMethod = "GET"
		rows[i].Path = "/p"
		require.NoError(t, repo.DB.Create(&rows[i]).Error)
	}
	return repo, day
}

func TestTrafficFilter_AppliesEveryField(t *testing.T) {
	repo, day := seedFilterFixture(t)
	yes, no := true, false
	notFound := 404
	end := day.Add(24 * time.Hour)
	tz := time.FixedZone("UTC+5", 5*3600)
	startLocal := day.Add(90 * time.Minute).In(tz)

	tests := []struct {
		name   string
		filter TrafficFilter
		want   int64
	}{
		{"no filter", TrafficFilter{}, 4},
		{"end bound", TrafficFilter{End: &end}, 3},
		{"start bound in a non-UTC zone", TrafficFilter{Start: &startLocal, End: &end}, 2},
		{"static only", TrafficFilter{IsStatic: &yes}, 1},
		{"non-static only", TrafficFilter{IsStatic: &no}, 3},
		{"status", TrafficFilter{StatusCode: &notFound}, 1},
		{"country", TrafficFilter{Country: "ES"}, 2},
		{"device", TrafficFilter{DeviceType: "iPhone"}, 1},
		{"platform", TrafficFilter{Platform: "macOS"}, 2},
		{"browser", TrafficFilter{Browser: "Safari"}, 2},
		{"fingerprint", TrafficFilter{Fingerprint: "fp-b"}, 2},
		{"fingerprint type", TrafficFilter{FingerprintType: "stable"}, 1},
		{"user", TrafficFilter{Username: "alice"}, 1},
		{"guest", TrafficFilter{Username: GuestUsername}, 3},
		{"unknown user", TrafficFilter{Username: "nobody"}, 0},
		{"combined", TrafficFilter{Country: "ES", Browser: "Chrome", StatusCode: &notFound}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repo.CountRequests(tt.filter)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCountBy_GroupsAndHonorsFilter(t *testing.T) {
	repo, _ := seedFilterFixture(t)

	byUser, err := repo.CountBy(DimensionUser, TrafficFilter{})
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"alice": 1, GuestUsername: 3}, byUser)

	byStatus, err := repo.CountBy(DimensionStatus, TrafficFilter{Country: "ES"})
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"200": 1, "404": 1}, byStatus)

	byBrowser, err := repo.CountBy(DimensionBrowser, TrafficFilter{Platform: "iOS"})
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"Safari": 1}, byBrowser)

	_, err = repo.CountBy(Dimension("password; DROP TABLE users"), TrafficFilter{})
	assert.Error(t, err)
}

func TestAveragesAreZeroWhenNothingMatches(t *testing.T) {
	repo, _ := seedFilterFixture(t)
	avg, err := repo.AverageResponseTimeNs(TrafficFilter{Country: "ZZ"})
	require.NoError(t, err)
	assert.Zero(t, avg)
}

func TestGetTimeSeries_AppliesFilter(t *testing.T) {
	repo, day := seedFilterFixture(t)
	end := day.Add(6 * time.Hour)

	points, err := repo.GetTimeSeries(TrafficFilter{Start: &day, End: &end, Country: "ES"}, GranularityHour)
	require.NoError(t, err)

	total, errors := 0, 0
	for _, p := range points {
		total += p.RequestCount
		errors += p.ErrorCount
		assert.LessOrEqual(t, p.NewVisitors, p.UniqueFingerprints)
	}
	assert.Equal(t, 2, total)
	assert.Equal(t, 1, errors)

	_, err = repo.GetTimeSeries(TrafficFilter{}, GranularityHour)
	assert.Error(t, err, "a series without a range must be rejected")
}
