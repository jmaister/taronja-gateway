package dbmigrations_test

import (
	"io/fs"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFingerprintFirstSeenMigration_BackfillsExistingRows(t *testing.T) {
	db.SetupTestDB(t.Name())
	gdb := db.GetConnection()
	day := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, gdb.Exec("DROP TRIGGER trg_traffic_metrics_first_seen").Error)
	require.NoError(t, gdb.Exec("DELETE FROM fingerprint_first_seen").Error)
	for i, at := range []time.Time{day.Add(4 * time.Hour), day.Add(1 * time.Hour)} {
		m := &db.TrafficMetric{HttpMethod: "GET", Path: "/x", HttpStatus: 200, Timestamp: at, ClientInfo: db.ClientInfo{Fingerprint: "fp"}}
		require.NoError(t, gdb.Create(m).Error, i)
	}

	sqlText, err := fs.ReadFile(db.MigrationSchema().Migrations, "0002_fingerprint_first_seen.up.sql")
	require.NoError(t, err)
	require.NoError(t, gdb.Exec(string(sqlText)).Error)

	var first time.Time
	require.NoError(t, gdb.Raw("SELECT first_timestamp FROM fingerprint_first_seen WHERE fingerprint = 'fp'").Row().Scan(&first))
	assert.True(t, first.Equal(day.Add(1*time.Hour)))
}
