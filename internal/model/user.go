package model

// User represents an account in the global users.db.
type User struct {
	ID           string  `json:"id"`
	Email        string  `json:"email"`
	PasswordHash string  `json:"-"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
	LastSyncAt   *string `json:"last_sync_at,omitempty"`
	StorageBytes int64   `json:"storage_bytes"`
	IsActive     bool    `json:"is_active"`
}

// RefreshToken represents a stored refresh token in users.db.
type RefreshToken struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	TokenHash string `json:"-"`
	ExpiresAt string `json:"expires_at"`
	CreatedAt string `json:"created_at"`
}
