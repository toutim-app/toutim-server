package database

import (
	"database/sql"
	_ "embed"
	"fmt"
)

//go:embed schema/users_schema.sql
var usersSchemaSQL string

//go:embed schema/users_schema_v2.sql
var usersSchemaV2SQL string

//go:embed schema/user_data_schema.sql
var userDataSchemaSQL string

//go:embed schema/user_data_schema_v2.sql
var userDataSchemaV2SQL string

// Migration represents a versioned database migration.
type Migration struct {
	Version int
	SQL     string
}

// Users database migrations.
var usersMigrations = []Migration{
	{Version: 1, SQL: usersSchemaSQL},
	{Version: 2, SQL: usersSchemaV2SQL},
}

// Per-user database migrations.
var userDataMigrations = []Migration{
	{Version: 1, SQL: userDataSchemaSQL},
	{Version: 2, SQL: userDataSchemaV2SQL},
}

// migrateDB applies pending migrations to a database.
func migrateDB(db *sql.DB, migrations []Migration) error {
	currentVersion, err := getCurrentVersion(db)
	if err != nil {
		return fmt.Errorf("get current version: %w", err)
	}

	for _, m := range migrations {
		if m.Version > currentVersion {
			if _, err := db.Exec(m.SQL); err != nil {
				return fmt.Errorf("migration v%d: %w", m.Version, err)
			}
		}
	}

	if len(migrations) > 0 {
		latest := migrations[len(migrations)-1].Version
		if latest > currentVersion {
			if err := setCurrentVersion(db, latest); err != nil {
				return fmt.Errorf("set version: %w", err)
			}
		}
	}

	return nil
}

// getCurrentVersion returns the current schema version from PRAGMA user_version.
func getCurrentVersion(db *sql.DB) (int, error) {
	var version int
	err := db.QueryRow("PRAGMA user_version").Scan(&version)
	return version, err
}

// setCurrentVersion sets the schema version via PRAGMA user_version.
func setCurrentVersion(db *sql.DB, version int) error {
	_, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", version))
	return err
}
