package repositories

import (
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/db/queries"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedFilterFixture(t *testing.T) (*TrafficMetricRepositoryDB, time.Time) {
	t.Helper()
	db.SetupTestDB(t.Name())
	repo := &TrafficMetricRepositoryDB{DB: db.GetConnection()}
	alice := db.User{Username: "alice", Email: "alice@example.com"}
	require.NoError(t, repo.DB.Create(&alice).Error)

	day := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	rows := []db.TrafficMetric{
		{Timestamp: day.Add(1 * time.Hour), HttpStatus: 200, UserID: alice.ID, ClientInfo: db.ClientInfo{Country: "ES", DeviceFamily: "Mac", OSFamily: "macOS", BrowserFamily: "Safari", Fingerprint: "fp-a", FingerprintType: "stable"}},
		{Timestamp: day.Add(2 * time.Hour), HttpStatus: 404, ClientInfo: db.ClientInfo{Country: "ES", DeviceFamily: "Mac", OSFamily: "macOS", BrowserFamily: "Chrome", Fingerprint: "fp-b", FingerprintType: "ja4h"}},
		{Timestamp: day.Add(3 * time.Hour), HttpStatus: 200, IsStaticAsset: true, ClientInfo: db.ClientInfo{Country: "FR", DeviceFamily: "iPhone", OSFamily: "iOS", BrowserFamily: "Safari", Fingerprint: "fp-b", FingerprintType: "ja4h"}},
		{Timestamp: day.Add(30 * time.Hour), HttpStatus: 500, ClientInfo: db.ClientInfo{DeviceFamily: "Other"}},
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
		filter queries.TrafficFilter
		want   int64
	}{
		{"no filter", queries.TrafficFilter{}, 4},
		{"end bound", queries.TrafficFilter{End: &end}, 3},
		{"start bound in a non-UTC zone", queries.TrafficFilter{Start: &startLocal, End: &end}, 2},
		{"static only", queries.TrafficFilter{IsStatic: &yes}, 1},
		{"non-static only", queries.TrafficFilter{IsStatic: &no}, 3},
		{"status", queries.TrafficFilter{StatusCode: &notFound}, 1},
		{"country", queries.TrafficFilter{Country: "ES"}, 2},
		{"device", queries.TrafficFilter{DeviceType: "iPhone"}, 1},
		{"platform", queries.TrafficFilter{Platform: "macOS"}, 2},
		{"browser", queries.TrafficFilter{Browser: "Safari"}, 2},
		{"fingerprint", queries.TrafficFilter{Fingerprint: "fp-b"}, 2},
		{"fingerprint type", queries.TrafficFilter{FingerprintType: "stable"}, 1},
		{"user", queries.TrafficFilter{Username: "alice"}, 1},
		{"guest", queries.TrafficFilter{Username: queries.GuestUsername}, 3},
		{"unknown user", queries.TrafficFilter{Username: "nobody"}, 0},
		{"combined", queries.TrafficFilter{Country: "ES", Browser: "Chrome", StatusCode: &notFound}, 1},
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

	byUser, err := repo.CountBy(queries.DimensionUser, queries.TrafficFilter{})
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"alice": 1, queries.GuestUsername: 3}, byUser)

	byStatus, err := repo.CountBy(queries.DimensionStatus, queries.TrafficFilter{Country: "ES"})
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"200": 1, "404": 1}, byStatus)

	byBrowser, err := repo.CountBy(queries.DimensionBrowser, queries.TrafficFilter{Platform: "iOS"})
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"Safari": 1}, byBrowser)

	_, err = repo.CountBy(queries.Dimension("password; DROP TABLE users"), queries.TrafficFilter{})
	assert.Error(t, err)
}

func TestAveragesAreZeroWhenNothingMatches(t *testing.T) {
	repo, _ := seedFilterFixture(t)
	avg, err := repo.AverageResponseTimeNs(queries.TrafficFilter{Country: "ZZ"})
	require.NoError(t, err)
	assert.Zero(t, avg)
}

func TestGetTimeSeries_AppliesFilter(t *testing.T) {
	repo, day := seedFilterFixture(t)
	end := day.Add(6 * time.Hour)

	points, err := repo.GetTimeSeries(queries.TrafficFilter{Start: &day, End: &end, Country: "ES"}, queries.GranularityHour)
	require.NoError(t, err)

	total, errors := 0, 0
	for _, p := range points {
		total += p.RequestCount
		errors += p.ErrorCount
		assert.LessOrEqual(t, p.NewVisitors, p.UniqueFingerprints)
	}
	assert.Equal(t, 2, total)
	assert.Equal(t, 1, errors)

	_, err = repo.GetTimeSeries(queries.TrafficFilter{}, queries.GranularityHour)
	assert.Error(t, err, "a series without a range must be rejected")
}
