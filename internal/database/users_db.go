package database

import (
	"database/sql"
	"fmt"

	"github.com/fabienbmusic/toutim-server/internal/model"
)

// CreateUser inserts a new user into users.db.
func CreateUser(db *sql.DB, user model.User) error {
	_, err := db.Exec(`
		INSERT INTO users (id, email, password_hash, created_at, updated_at, is_active)
		VALUES (?, ?, ?, ?, ?, ?)`,
		user.ID, user.Email, user.PasswordHash,
		user.CreatedAt, user.UpdatedAt, boolToInt(user.IsActive),
	)
	if err != nil {
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

// GetUserByEmail retrieves a user by email. Returns nil if not found.
func GetUserByEmail(db *sql.DB, email string) (*model.User, error) {
	return scanUser(db.QueryRow(`
		SELECT id, email, password_hash, created_at, updated_at, last_sync_at, storage_bytes, is_active
		FROM users WHERE email = ?`, email))
}

// GetUserByID retrieves a user by ID. Returns nil if not found.
func GetUserByID(db *sql.DB, id string) (*model.User, error) {
	return scanUser(db.QueryRow(`
		SELECT id, email, password_hash, created_at, updated_at, last_sync_at, storage_bytes, is_active
		FROM users WHERE id = ?`, id))
}

// UpdateLastSync updates the last_sync_at timestamp for a user.
func UpdateLastSync(db *sql.DB, userID string, timestamp string) error {
	_, err := db.Exec(`UPDATE users SET last_sync_at = ? WHERE id = ?`, timestamp, userID)
	return err
}

// DeleteUser deletes a user from users.db.
func DeleteUser(db *sql.DB, userID string) error {
	_, err := db.Exec(`DELETE FROM users WHERE id = ?`, userID)
	return err
}

// StoreRefreshToken inserts a refresh token into users.db.
func StoreRefreshToken(db *sql.DB, token model.RefreshToken) error {
	_, err := db.Exec(`
		INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		token.ID, token.UserID, token.TokenHash, token.ExpiresAt, token.CreatedAt,
	)
	return err
}

// GetRefreshToken retrieves a refresh token by its hash. Returns nil if not found.
func GetRefreshToken(db *sql.DB, tokenHash string) (*model.RefreshToken, error) {
	row := db.QueryRow(`
		SELECT id, user_id, token_hash, expires_at, created_at
		FROM refresh_tokens WHERE token_hash = ?`, tokenHash)

	var t model.RefreshToken
	err := row.Scan(&t.ID, &t.UserID, &t.TokenHash, &t.ExpiresAt, &t.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// DeleteRefreshToken removes a refresh token by its hash.
func DeleteRefreshToken(db *sql.DB, tokenHash string) error {
	_, err := db.Exec(`DELETE FROM refresh_tokens WHERE token_hash = ?`, tokenHash)
	return err
}

// DeleteRefreshTokensByUser removes all refresh tokens for a user.
func DeleteRefreshTokensByUser(db *sql.DB, userID string) error {
	_, err := db.Exec(`DELETE FROM refresh_tokens WHERE user_id = ?`, userID)
	return err
}

// scanUser scans a user row into a model.User. Returns nil on sql.ErrNoRows.
func scanUser(row *sql.Row) (*model.User, error) {
	var u model.User
	var isActive int
	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt,
		&u.LastSyncAt, &u.StorageBytes, &isActive)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	u.IsActive = isActive == 1
	return &u, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
