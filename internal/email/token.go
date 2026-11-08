package email

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/toutim-app/toutim-server/internal/util"
)

const tokenBytes = 32 // 256-bit tokens

// GenerateToken creates a cryptographically random URL-safe token.
// Returns (rawToken, sha256Hash, error).
// The raw token is sent in the email link; the hash is stored in the database.
func GenerateToken() (string, string, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generate token: %w", err)
	}
	raw := hex.EncodeToString(b)
	hash := util.HashToken(raw)
	return raw, hash, nil
}
