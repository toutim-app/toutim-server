package sync

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/fabienbmusic/toutim-server/internal/auth"
	"github.com/fabienbmusic/toutim-server/internal/util"
)

// Handler handles sync HTTP endpoints.
type Handler struct {
	engine *Engine
	logger *slog.Logger
}

// NewHandler creates a new sync handler.
func NewHandler(engine *Engine, logger *slog.Logger) *Handler {
	return &Handler{
		engine: engine,
		logger: logger,
	}
}

// Push handles POST /api/sync/push
func (h *Handler) Push(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	var req PushRequest
	if !util.DecodeJSON(w, r, &req) {
		return
	}

	resp, err := h.engine.Push(userID, req)
	if err != nil {
		h.logger.Error("push failed", "user_id", userID, "error", err)
		// Check for "too many changes" error
		if strings.HasPrefix(err.Error(), "too many changes") {
			util.RespondError(w, http.StatusBadRequest, err.Error())
			return
		}
		util.RespondError(w, http.StatusInternalServerError, "sync push failed")
		return
	}

	h.logger.Info("push completed",
		"user_id", userID,
		"applied", resp.Applied,
		"conflicts", len(resp.Conflicts),
		"version", resp.Version,
	)

	util.RespondJSON(w, http.StatusOK, resp)
}

// Pull handles POST /api/sync/pull
func (h *Handler) Pull(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	var req PullRequest
	if !util.DecodeJSON(w, r, &req) {
		return
	}

	resp, err := h.engine.Pull(userID, req)
	if err != nil {
		h.logger.Error("pull failed", "user_id", userID, "error", err)
		util.RespondError(w, http.StatusInternalServerError, "sync pull failed")
		return
	}

	h.logger.Info("pull completed",
		"user_id", userID,
		"changes", len(resp.Changes),
		"version", resp.Version,
	)

	util.RespondJSON(w, http.StatusOK, resp)
}

// Status handles GET /api/sync/status
func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	resp, err := h.engine.Status(userID)
	if err != nil {
		h.logger.Error("status failed", "user_id", userID, "error", err)
		util.RespondError(w, http.StatusInternalServerError, "sync status failed")
		return
	}

	util.RespondJSON(w, http.StatusOK, resp)
}
