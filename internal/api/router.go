package api

import (
	"log/slog"
	"net/http"

	"github.com/fabienbmusic/toutim-server/internal/auth"
	"github.com/fabienbmusic/toutim-server/internal/util"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

const version = "0.1.0"

// NewRouter creates and configures the chi router with all routes.
func NewRouter(logger *slog.Logger, authHandler *auth.Handler, jwtMgr *auth.JWTManager) chi.Router {
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
			r.Use(ContentTypeJSON)
			r.Post("/register", authHandler.Register)
			r.Post("/login", authHandler.Login)
			r.Post("/refresh", authHandler.Refresh)

			// Auth routes (protected)
			r.Group(func(r chi.Router) {
				r.Use(auth.Middleware(jwtMgr))
				r.Post("/logout", authHandler.Logout)
				r.Delete("/account", authHandler.DeleteAccount)
			})
		})

		// Protected routes (sync, export, files) — to be added in later phases
		r.Group(func(r chi.Router) {
			r.Use(auth.Middleware(jwtMgr))
			// Phase 4: sync routes
			// Phase 5: file routes
			// Phase 6: export routes
		})
	})

	return r
}
