package api

import (
	"log/slog"
	"net/http"

	"github.com/fabienbmusic/toutim-server/internal/auth"
	"github.com/fabienbmusic/toutim-server/internal/database"
	"github.com/fabienbmusic/toutim-server/internal/export"
	"github.com/fabienbmusic/toutim-server/internal/files"
	"github.com/fabienbmusic/toutim-server/internal/sync"
	"github.com/fabienbmusic/toutim-server/internal/util"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

const version = "0.1.0"

// NewRouter creates and configures the chi router with all routes.
func NewRouter(
	logger *slog.Logger,
	dbMgr *database.DatabaseManager,
	authHandler *auth.Handler,
	jwtMgr *auth.JWTManager,
	syncHandler *sync.Handler,
	fileHandler *files.Handler,
	exportHandler *export.Handler,
) chi.Router {
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(RequestLogger(logger))
	r.Use(middleware.Recoverer)
	r.Use(CORSMiddleware())

	// Health check (no auth required)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		util.RespondJSON(w, http.StatusOK, map[string]string{
			"status":  "ok",
			"version": version,
		})
	})

	// API routes
	r.Route("/api", func(r chi.Router) {
		// Auth routes (public)
		r.Route("/auth", func(r chi.Router) {
			// GET endpoint — opened from browser via email link
			r.Get("/verify-email", authHandler.VerifyEmail)

			r.Group(func(r chi.Router) {
				r.Use(ContentTypeJSON)
				r.Post("/register", authHandler.Register)
				r.Post("/login", authHandler.Login)
				r.Post("/refresh", authHandler.Refresh)
				r.Post("/resend-verification", authHandler.ResendVerification)
				r.Post("/forgot-password", authHandler.ForgotPassword)
				r.Post("/reset-password", authHandler.ResetPassword)
			})

			// Auth routes (protected)
			r.Group(func(r chi.Router) {
				r.Use(auth.Middleware(jwtMgr))
				r.Post("/logout", authHandler.Logout)
				r.Delete("/account", authHandler.DeleteAccount)
			})
		})

		// Protected routes
		r.Group(func(r chi.Router) {
			r.Use(auth.Middleware(jwtMgr))

			// Sync routes (require verified email)
			r.Route("/sync", func(r chi.Router) {
				r.Use(ContentTypeJSON)
				r.Use(RequireEmailVerified(dbMgr))
				r.Post("/push", syncHandler.Push)
				r.Post("/pull", syncHandler.Pull)
				r.Get("/status", syncHandler.Status)
			})

			// File routes
			r.Route("/files", func(r chi.Router) {
				r.Post("/upload", fileHandler.Upload)
				r.Get("/{fileID}", fileHandler.Download)
				r.Delete("/{fileID}", fileHandler.Delete)
			})

			// Export routes
			r.Route("/export", func(r chi.Router) {
				r.Get("/sqlite", exportHandler.SQLite)
				r.Get("/json", exportHandler.JSON)
			})
		})
	})

	return r
}

// RequireEmailVerified returns middleware that blocks requests from users
// whose email is not verified.
func RequireEmailVerified(dbMgr *database.DatabaseManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := auth.UserIDFromContext(r.Context())
			if userID == "" {
				util.RespondError(w, http.StatusUnauthorized, "unauthorized")
				return
			}

			user, err := database.GetUserByID(dbMgr.UsersDB(), userID)
			if err != nil || user == nil {
				util.RespondError(w, http.StatusUnauthorized, "user not found")
				return
			}

			if !user.EmailVerified {
				util.RespondError(w, http.StatusForbidden, "email not verified")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
