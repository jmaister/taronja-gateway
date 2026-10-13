package dbmigrations

// Unexported pieces the external tests in this directory exercise directly.
var (
	NeedsLegacyBridge               = needsLegacyBridge
	ApplyLegacyDataMigrations       = applyLegacyDataMigrations
	MigrateTimestampsToUTC          = migrateTimestampsToUTC
	MigrateLegacyFingerprintColumns = migrateLegacyFingerprintColumns
	MigrateSessionTokensToHashed    = migrateSessionTokensToHashed
	UTCTimestampColumns             = utcTimestampColumns
	MigrationFiles                  = migrationFiles
)

const LegacyBridgeDoneVersion = legacyBridgeDoneVersion
