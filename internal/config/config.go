package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/BurntSushi/toml"
)

// Config holds all server configuration.
type Config struct {
	Server   ServerConfig   `toml:"server"`
	Data     DataConfig     `toml:"data"`
	Database DatabaseConfig `toml:"database"`
	Auth     AuthConfig     `toml:"auth"`
	Sync     SyncConfig     `toml:"sync"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port int    `toml:"port"`
	Host string `toml:"host"`
}

// DataConfig holds data storage settings.
type DataConfig struct {
	Dir            string `toml:"dir"`
	MaxUserStorage string `toml:"max_user_storage"`
}

// DatabaseConfig holds SQLite connection pool settings.
type DatabaseConfig struct {
	PoolSize    int           `toml:"pool_size"`
	IdleTimeout time.Duration `toml:"idle_timeout"`
	BusyTimeout time.Duration `toml:"busy_timeout"`
}

// AuthConfig holds authentication settings.
type AuthConfig struct {
	JWTSecret       string        `toml:"jwt_secret"`
	AccessTokenTTL  time.Duration `toml:"access_token_ttl"`
	RefreshTokenTTL time.Duration `toml:"refresh_token_ttl"`
}

// SyncConfig holds sync engine settings.
type SyncConfig struct {
	MaxChangesPerPush int    `toml:"max_changes_per_push"`
	MaxFileSize       string `toml:"max_file_size"`
}

// defaults returns a Config with sensible default values.
func defaults() Config {
	return Config{
		Server: ServerConfig{
			Port: 8080,
			Host: "0.0.0.0",
		},
		Data: DataConfig{
			Dir:            "./data",
			MaxUserStorage: "500MB",
		},
		Database: DatabaseConfig{
			PoolSize:    100,
			IdleTimeout: 5 * time.Minute,
			BusyTimeout: 5 * time.Second,
		},
		Auth: AuthConfig{
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: 30 * 24 * time.Hour, // 30 days
		},
		Sync: SyncConfig{
			MaxChangesPerPush: 1000,
			MaxFileSize:       "10MB",
		},
	}
}

// Load reads configuration from config.toml (if present) and overlays
// TOUTIM_* environment variables. Returns defaults for any unset values.
func Load() (Config, error) {
	cfg := defaults()

	// Try to load config.toml
	if _, err := os.Stat("config.toml"); err == nil {
		if _, err := toml.DecodeFile("config.toml", &cfg); err != nil {
			return cfg, fmt.Errorf("parse config.toml: %w", err)
		}
	}

	// Overlay environment variables
	if v := os.Getenv("TOUTIM_PORT"); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil {
			return cfg, fmt.Errorf("invalid TOUTIM_PORT: %w", err)
		}
		cfg.Server.Port = port
	}
	if v := os.Getenv("TOUTIM_HOST"); v != "" {
		cfg.Server.Host = v
	}
	if v := os.Getenv("TOUTIM_DATA_DIR"); v != "" {
		cfg.Data.Dir = v
	}
	if v := os.Getenv("TOUTIM_JWT_SECRET"); v != "" {
		cfg.Auth.JWTSecret = v
	}
	if v := os.Getenv("TOUTIM_POOL_SIZE"); v != "" {
		size, err := strconv.Atoi(v)
		if err != nil {
			return cfg, fmt.Errorf("invalid TOUTIM_POOL_SIZE: %w", err)
		}
		cfg.Database.PoolSize = size
	}

	return cfg, nil
}

// Validate checks that required configuration values are set.
func (c Config) Validate() error {
	if c.Auth.JWTSecret == "" {
		return fmt.Errorf("jwt_secret is required (set TOUTIM_JWT_SECRET or auth.jwt_secret in config.toml)")
	}
	if len(c.Auth.JWTSecret) < 32 {
		return fmt.Errorf("jwt_secret must be at least 32 characters")
	}
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid port: %d", c.Server.Port)
	}
	return nil
}
