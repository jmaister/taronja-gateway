package db

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// oldTrafficMetric mirrors TrafficMetric exactly as it looked before
// IsStaticAsset was added — every other field unchanged, including the
// embedded ClientInfo — so AutoMigrate-ing it first, through GORM itself
// (not hand-written DDL, which GORM's SQLite migrator doesn't necessarily
// treat the same as a table it created), then AutoMigrate-ing the real,
// current TrafficMetric against the same database reproduces exactly the
// "add a NOT NULL column to an already-populated table" situation an
// upgrading deployment hits for real.
type oldTrafficMetric struct {
	gorm.Model
	HttpMethod     string    `gorm:"type:varchar(10);not null"`
	Path           string    `gorm:"type:varchar(500);not null"`
	HttpStatus     int       `gorm:"not null"`
	ResponseTimeNs int64     `gorm:"not null"`
	Timestamp      time.Time `gorm:"not null"`
	ResponseSize   int64     `gorm:"default:0"`
	Error          string    `gorm:"type:text"`
	UserID         string    `gorm:"type:varchar(255)"`
	SessionID      string    `gorm:"type:varchar(255)"`
	ClientInfo
}

func (oldTrafficMetric) TableName() string { return "traffic_metrics" }

// TestAutoMigrate_AddingNotNullColumnToExistingRows_NeedsADefault is a
// regression test for a real startup panic, found by actually running an
// old build of this gateway (from before TrafficMetric.IsStaticAsset
// existed) against a real SQLite database, seeding a row through it, and
// then running the current build against that same database: SQLite's
// `ALTER TABLE ADD COLUMN` refuses a NOT NULL column with no DEFAULT the
// moment the table already has at least one row ("Cannot add a NOT NULL
// column with default value NULL") — AutoMigrate doesn't catch or work
// around this, it just surfaces the SQL error, and db.Init() panics on any
// AutoMigrate error, so this took down the whole gateway on startup for
// anyone upgrading a database with existing traffic_metrics rows.
//
// The fix (db/schema.go's IsStaticAsset gorm tag) is
// `not null;default:false`, not just `not null` — the `default:false` is
// what lets SQLite's ALTER TABLE supply a value for pre-existing rows
// instead of refusing outright. Any future NOT NULL column added to an
// already-shipped table needs the same `default:...` tag, or it
// reproduces this exact panic for anyone with existing data — this test
// guards specifically against regressing that fix, not against every
// possible future instance of the mistake.
func TestAutoMigrate_AddingNotNullColumnToExistingRows_NeedsADefault(t *testing.T) {
	gdb, err := gorm.Open(sqlite.Dialector{
		DriverName: "sqlite",
		DSN:        "file::memory:?_" + t.Name(),
	}, &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, gdb.AutoMigrate(&oldTrafficMetric{}))
	require.NoError(t, gdb.Create(&oldTrafficMetric{
		HttpMethod: "GET", Path: "/old-row", HttpStatus: 200, ResponseTimeNs: 1000, Timestamp: time.Now(),
	}).Error)

	// This is the actual regression check: before the default:false fix,
	// this AutoMigrate call fails with "Cannot add a NOT NULL column with
	// default value NULL" and db.Init() would panic on it for real.
	require.NoError(t, gdb.AutoMigrate(&TrafficMetric{}))

	var isStaticAsset bool
	require.NoError(t, gdb.Raw("SELECT is_static_asset FROM traffic_metrics WHERE path = ?", "/old-row").Row().Scan(&isStaticAsset))
	require.False(t, isStaticAsset, "pre-existing row should default to false, not fail the migration or come back NULL/true")
}
