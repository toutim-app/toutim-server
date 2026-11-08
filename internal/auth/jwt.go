package auth

import (
	"fmt"
	"time"

	"github.com/toutim-app/toutim-server/internal/config"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Claims represents the JWT claims for Toutim tokens.
type Claims struct {
	jwt.RegisteredClaims
	Email string `json:"email,omitempty"`
	Type  string `json:"type"` // "access" or "refresh"
}

// JWTManager handles JWT token generation and validation.
type JWTManager struct {
	secret          []byte
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
}

// NewJWTManager creates a new JWTManager from auth config.
func NewJWTManager(cfg config.AuthConfig) *JWTManager {
	return &JWTManager{
		secret:          []byte(cfg.JWTSecret),
		accessTokenTTL:  cfg.AccessTokenTTL,
		refreshTokenTTL: cfg.RefreshTokenTTL,
	}
}

// GenerateAccessToken creates a short-lived access token.
func (m *JWTManager) GenerateAccessToken(userID, email string) (string, error) {
	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.accessTokenTTL)),
		},
		Email: email,
		Type:  "access",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

// GenerateRefreshToken creates a long-lived refresh token with a unique JTI.
// Returns the signed token string and the JTI (for storage).
func (m *JWTManager) GenerateRefreshToken(userID string) (string, string, error) {
	now := time.Now()
	jti := uuid.New().String()

	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.refreshTokenTTL)),
		},
		Type: "refresh",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.secret)
	if err != nil {
		return "", "", err
	}

	return signed, jti, nil
}

// ValidateAccessToken parses and validates an access token.
func (m *JWTManager) ValidateAccessToken(tokenString string) (*Claims, error) {
	claims, err := m.parseToken(tokenString)
	if err != nil {
		return nil, err
	}
	if claims.Type != "access" {
		return nil, fmt.Errorf("invalid token type: expected access, got %s", claims.Type)
	}
	return claims, nil
}

// ValidateRefreshToken parses and validates a refresh token.
func (m *JWTManager) ValidateRefreshToken(tokenString string) (*Claims, error) {
	claims, err := m.parseToken(tokenString)
	if err != nil {
		return nil, err
	}
	if claims.Type != "refresh" {
		return nil, fmt.Errorf("invalid token type: expected refresh, got %s", claims.Type)
	}
	return claims, nil
}

func (m *JWTManager) parseToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return m.secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	return claims, nil
}
