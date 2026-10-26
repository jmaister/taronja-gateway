package dbmigrations_test

import (
	"io/fs"
	"testing"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A database that finished the legacy bridge before FeatureFlag existed only
// gets the table from 0003, so the SQL must produce a table GORM can use.
func TestFeatureFlagsMigration_CreatesUsableTable(t *testing.T) {
	db.SetupTestDB(t.Name())
	gdb := db.GetConnection()
	require.NoError(t, gdb.Exec("DROP TABLE feature_flags").Error)

	sqlText, err := fs.ReadFile(db.MigrationSchema().Migrations, "0003_feature_flags.up.sql")
	require.NoError(t, err)
	require.NoError(t, gdb.Exec(string(sqlText)).Error)

	require.NoError(t, gdb.Exec("INSERT INTO feature_flags (`key`) VALUES ('defaults')").Error)
	var flag db.FeatureFlag
	require.NoError(t, gdb.First(&flag, "`key` = ?", "defaults").Error)
	assert.Equal(t, db.FeatureFlagTypeBoolean, flag.Type)
	assert.False(t, flag.Enabled)

	require.NoError(t, gdb.Create(&db.FeatureFlag{Key: "on", Type: db.FeatureFlagTypeBoolean, Enabled: true}).Error)
	var on db.FeatureFlag
	require.NoError(t, gdb.First(&on, "`key` = ?", "on").Error)
	assert.True(t, on.Enabled)
}
