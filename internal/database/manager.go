package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/fabienbmusic/toutim-server/internal/config"
	"github.com/fabienbmusic/toutim-server/internal/util"
	_ "modernc.org/sqlite"
)

// DatabaseManager manages the global users.db and per-user SQLite databases.
type DatabaseManager struct {
	dataDir string
	usersDB *sql.DB
	pool    *ConnectionPool
	logger  *slog.Logger
}

// NewDatabaseManager creates a new DatabaseManager, initializes directories,
// opens users.db, and starts the connection pool cleanup goroutine.
func NewDatabaseManager(ctx context.Context, cfg config.Config, logger *slog.Logger) (*DatabaseManager, error) {
	dataDir := cfg.Data.Dir

	// Create directories
	userDataDir := filepath.Join(dataDir, "user_data")
	if err := os.MkdirAll(userDataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data directories: %w", err)
	}

	// Open users.db
	usersDBPath := filepath.Join(dataDir, "users.db")
	usersDB, err := openSQLite(usersDBPath, cfg.Database.BusyTimeout)
	if err != nil {
		return nil, fmt.Errorf("open users.db: %w", err)
	}

	// Apply migrations to users.db
	if err := migrateDB(usersDB, usersMigrations); err != nil {
		usersDB.Close()
		return nil, fmt.Errorf("migrate users.db: %w", err)
	}

	pool := NewConnectionPool(cfg.Database.PoolSize, cfg.Database.IdleTimeout, logger)
	pool.StartCleanup(ctx)

	logger.Info("database manager initialized",
		"data_dir", dataDir,
		"pool_size", cfg.Database.PoolSize,
	)

	return &DatabaseManager{
		dataDir: dataDir,
		usersDB: usersDB,
		pool:    pool,
		logger:  logger,
	}, nil
}

// UsersDB returns the global users database connection.
func (m *DatabaseManager) UsersDB() *sql.DB {
	return m.usersDB
}

// UserDB returns a connection to a user's personal database.
// Creates the database and applies migrations if it doesn't exist.
func (m *DatabaseManager) UserDB(userID string) (*sql.DB, error) {
	if !util.IsValidUUID(userID) {
		return nil, fmt.Errorf("invalid user ID format")
	}

	// Check pool first
	if db, ok := m.pool.Get(userID); ok {
		return db, nil
	}

	// Open or create the database
	dbPath := filepath.Join(m.dataDir, "user_data", userID+".db")
	db, err := openSQLite(dbPath, 5000)
	if err != nil {
		return nil, fmt.Errorf("open user db: %w", err)
	}

	// Apply migrations
	if err := migrateDB(db, userDataMigrations); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate user db: %w", err)
	}

	// Add to pool
	m.pool.Put(userID, db)

	return db, nil
}

// RemoveUserDB removes a user's database connection from the pool
// and deletes their database file and files directory.
func (m *DatabaseManager) RemoveUserDB(userID string) error {
	if !util.IsValidUUID(userID) {
		return fmt.Errorf("invalid user ID format")
	}

	// Remove from pool (closes connection)
	m.pool.Remove(userID)

	// Delete database file
	dbPath := filepath.Join(m.dataDir, "user_data", userID+".db")
	if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete user db: %w", err)
	}
	// Also remove WAL and SHM files
	os.Remove(dbPath + "-wal")
	os.Remove(dbPath + "-shm")

	// Delete user files directory
	filesDir := filepath.Join(m.dataDir, "user_data", userID+"_files")
	if err := os.RemoveAll(filesDir); err != nil {
		return fmt.Errorf("delete user files: %w", err)
	}

	return nil
}

// Close closes all database connections.
func (m *DatabaseManager) Close() {
	m.pool.Close()
	if err := m.usersDB.Close(); err != nil {
		m.logger.Error("failed to close users.db", "error", err)
	}
}

// openSQLite opens a SQLite database with WAL mode and foreign keys enabled.
func openSQLite(path string, busyTimeoutMs any) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=%v&_foreign_keys=on", path, busyTimeoutMs)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}

	// Verify connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}

	// Enable WAL mode explicitly (in case DSN param doesn't work)
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable WAL: %w", err)
	}

	// Enable foreign keys explicitly
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	return db, nil
}
