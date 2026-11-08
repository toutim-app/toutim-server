package export

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/toutim-app/toutim-server/internal/auth"
	"github.com/toutim-app/toutim-server/internal/database"
	"github.com/toutim-app/toutim-server/internal/util"
)

// Handler handles export endpoints.
type Handler struct {
	dbMgr  *database.DatabaseManager
	logger *slog.Logger
}

// NewHandler creates a new export handler.
func NewHandler(dbMgr *database.DatabaseManager, logger *slog.Logger) *Handler {
	return &Handler{
		dbMgr:  dbMgr,
		logger: logger,
	}
}

// SQLite handles GET /api/export/sqlite
func (h *Handler) SQLite(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	db, err := h.dbMgr.UserDB(userID)
	if err != nil {
		h.logger.Error("failed to get user db", "error", err, "user_id", userID)
		util.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Checkpoint WAL to ensure the main file is consistent
	if _, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		h.logger.Error("failed to checkpoint WAL", "error", err, "user_id", userID)
		util.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Construct file path
	dbPath := filepath.Join(h.dbMgr.DataDir(), "user_data", userID+".db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		util.RespondError(w, http.StatusNotFound, "database not found")
		return
	}

	w.Header().Set("Content-Type", "application/x-sqlite3")
	w.Header().Set("Content-Disposition", `attachment; filename="toutim-export.db"`)

	http.ServeFile(w, r, dbPath)

	h.logger.Info("sqlite export", "user_id", userID)
}

// JSON handles GET /api/export/json
func (h *Handler) JSON(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	db, err := h.dbMgr.UserDB(userID)
	if err != nil {
		h.logger.Error("failed to get user db", "error", err, "user_id", userID)
		util.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}

	export := map[string]any{
		"exported_at": util.NowRFC3339(),
	}

	// Export each table
	tables := []string{"collections", "field_definitions", "items", "item_field_values"}
	for _, table := range tables {
		rows, err := queryAllRows(db, table)
		if err != nil {
			h.logger.Error("failed to export table", "table", table, "error", err, "user_id", userID)
			util.RespondError(w, http.StatusInternalServerError, "internal error")
			return
		}
		export[table] = rows
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="toutim-export.json"`)

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(export); err != nil {
		h.logger.Error("failed to encode export", "error", err, "user_id", userID)
	}

	h.logger.Info("json export", "user_id", userID)
}

// queryAllRows fetches all rows from a table as []map[string]any.
func queryAllRows(db *sql.DB, table string) ([]map[string]any, error) {
	query := fmt.Sprintf("SELECT * FROM %s", table)
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var result []map[string]any
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}

		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}

		row := make(map[string]any, len(cols))
		for i, col := range cols {
			row[col] = values[i]
		}
		result = append(result, row)
	}

	if result == nil {
		result = []map[string]any{}
	}

	return result, rows.Err()
}
