package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/toutim-app/toutim-server/internal/util"
)

type contextKey string

const (
	contextKeyUserID contextKey = "userID"
	contextKeyEmail  contextKey = "email"
)

// Middleware returns an HTTP middleware that validates JWT access tokens.
func Middleware(jwtMgr *JWTManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				util.RespondError(w, http.StatusUnauthorized, "missing authorization header")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || parts[0] != "Bearer" {
				util.RespondError(w, http.StatusUnauthorized, "invalid authorization format")
				return
			}

			claims, err := jwtMgr.ValidateAccessToken(parts[1])
			if err != nil {
				util.RespondError(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}

			// Store user info in context
			ctx := context.WithValue(r.Context(), contextKeyUserID, claims.Subject)
			ctx = context.WithValue(ctx, contextKeyEmail, claims.Email)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserIDFromContext extracts the user ID from the request context.
func UserIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(contextKeyUserID).(string)
	return v
}

// EmailFromContext extracts the email from the request context.
func EmailFromContext(ctx context.Context) string {
	v, _ := ctx.Value(contextKeyEmail).(string)
	return v
}
