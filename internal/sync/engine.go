package sync

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/toutim-app/toutim-server/internal/database"
)

// Engine handles the sync logic for push and pull operations.
type Engine struct {
	dbMgr             *database.DatabaseManager
	logger            *slog.Logger
	maxChangesPerPush int
}

// NewEngine creates a new sync engine.
func NewEngine(dbMgr *database.DatabaseManager, logger *slog.Logger, maxChangesPerPush int) *Engine {
	return &Engine{
		dbMgr:             dbMgr,
		logger:            logger,
		maxChangesPerPush: maxChangesPerPush,
	}
}

// Push applies client changes to the user's database.
func (e *Engine) Push(userID string, req PushRequest) (*PushResponse, error) {
	if len(req.Changes) > e.maxChangesPerPush {
		return nil, fmt.Errorf("too many changes: %d (max %d)", len(req.Changes), e.maxChangesPerPush)
	}

	db, err := e.dbMgr.UserDB(userID)
	if err != nil {
		return nil, fmt.Errorf("get user db: %w", err)
	}

	// Sort changes by entity dependency order
	ordered := orderChanges(req.Changes)

	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var conflicts []ConflictResult
	applied := 0

	for _, change := range ordered {
		tableName, ok := tableForEntity[change.Entity]
		if !ok {
			e.logger.Warn("unknown entity type", "entity", change.Entity)
			continue
		}

		switch change.Action {
		case "create":
			if err := e.applyCreate(tx, tableName, change); err != nil {
				e.logger.Error("failed to apply create", "entity", change.Entity, "id", change.ID, "error", err)
				return nil, fmt.Errorf("apply create %s/%s: %w", change.Entity, change.ID, err)
			}
			applied++

		case "update":
			conflict, err := e.applyUpdate(tx, tableName, change)
			if err != nil {
				e.logger.Error("failed to apply update", "entity", change.Entity, "id", change.ID, "error", err)
				return nil, fmt.Errorf("apply update %s/%s: %w", change.Entity, change.ID, err)
			}
			if conflict != nil {
				conflicts = append(conflicts, *conflict)
			} else {
				applied++
			}

		case "delete":
			conflict, err := e.applyDelete(tx, tableName, change)
			if err != nil {
				e.logger.Error("failed to apply delete", "entity", change.Entity, "id", change.ID, "error", err)
				return nil, fmt.Errorf("apply delete %s/%s: %w", change.Entity, change.ID, err)
			}
			if conflict != nil {
				conflicts = append(conflicts, *conflict)
			} else {
				applied++
			}

		default:
			e.logger.Warn("unknown action", "action", change.Action)
		}

		// Log to sync_log
		if err := e.logChange(tx, change.Entity, change.ID, change.Action); err != nil {
			return nil, fmt.Errorf("log change: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	// Get current version after commit
	version, err := e.getCurrentVersion(db)
	if err != nil {
		return nil, fmt.Errorf("get version: %w", err)
	}

	// Update last_sync_at in users.db
	now := time.Now().UTC().Format(time.RFC3339)
	database.UpdateLastSync(e.dbMgr.UsersDB(), userID, now)

	return &PushResponse{
		Conflicts: conflicts,
		Version:   version,
		Applied:   applied,
	}, nil
}

// Pull returns changes since the given version.
func (e *Engine) Pull(userID string, req PullRequest) (*PullResponse, error) {
	db, err := e.dbMgr.UserDB(userID)
	if err != nil {
		return nil, fmt.Errorf("get user db: %w", err)
	}

	// Get all sync log entries since the requested version
	rows, err := db.Query(`
		SELECT entity, entity_id, action, version
		FROM sync_log
		WHERE version > ?
		ORDER BY version ASC`, req.SinceVersion)
	if err != nil {
		return nil, fmt.Errorf("query sync_log: %w", err)
	}
	defer rows.Close()

	var changes []Change
	for rows.Next() {
		var entity, entityID, action string
		var version int64
		if err := rows.Scan(&entity, &entityID, &action, &version); err != nil {
			return nil, fmt.Errorf("scan sync_log: %w", err)
		}

		change := Change{
			Entity: entity,
			ID:     entityID,
			Action: action,
		}

		// For non-delete actions, fetch the current entity data
		if action != "delete" {
			tableName, ok := tableForEntity[entity]
			if !ok {
				continue
			}
			data, err := e.fetchEntityData(db, tableName, entityID)
			if err != nil {
				// Entity may have been deleted since the log entry
				if err == sql.ErrNoRows {
					change.Action = "delete"
				} else {
					return nil, fmt.Errorf("fetch entity %s/%s: %w", entity, entityID, err)
				}
			} else {
				change.Data = data
			}
		}

		changes = append(changes, change)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sync_log: %w", err)
	}

	// Deduplicate: keep only the latest action for each entity
	changes = deduplicateChanges(changes)

	version, err := e.getCurrentVersion(db)
	if err != nil {
		return nil, fmt.Errorf("get version: %w", err)
	}

	if changes == nil {
		changes = []Change{}
	}

	return &PullResponse{
		Changes: changes,
		Version: version,
	}, nil
}

// Status returns the sync status for a user.
func (e *Engine) Status(userID string) (*SyncStatusResponse, error) {
	db, err := e.dbMgr.UserDB(userID)
	if err != nil {
		return nil, fmt.Errorf("get user db: %w", err)
	}

	version, err := e.getCurrentVersion(db)
	if err != nil {
		return nil, fmt.Errorf("get version: %w", err)
	}

	user, err := database.GetUserByID(e.dbMgr.UsersDB(), userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	var lastSyncAt *string
	var storageBytes int64
	if user != nil {
		lastSyncAt = user.LastSyncAt
		storageBytes = user.StorageBytes
	}

	return &SyncStatusResponse{
		LastSyncAt:     lastSyncAt,
		CurrentVersion: version,
		StorageBytes:   storageBytes,
	}, nil
}

// applyCreate inserts a new entity. If it already exists, treats it as an upsert.
func (e *Engine) applyCreate(tx *sql.Tx, tableName string, change Change) error {
	data, err := parseChangeData(change)
	if err != nil {
		return err
	}

	// Ensure ID is set
	data["id"] = change.ID

	cols, allowedCols := filterColumns(change.Entity, data)
	if len(cols) == 0 {
		return fmt.Errorf("no valid columns for entity %s", change.Entity)
	}

	placeholders := make([]string, len(cols))
	values := make([]any, len(cols))
	updateParts := make([]string, 0, len(cols))
	for i, col := range cols {
		placeholders[i] = "?"
		values[i] = data[col]
		if col != "id" {
			updateParts = append(updateParts, fmt.Sprintf("%s = excluded.%s", col, col))
		}
	}

	colNames := strings.Join(cols, ", ")
	phStr := strings.Join(placeholders, ", ")

	var query string
	if len(updateParts) > 0 {
		// INSERT OR REPLACE to handle upserts
		query = fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) ON CONFLICT(id) DO UPDATE SET %s",
			tableName, colNames, phStr, strings.Join(updateParts, ", "))
	} else {
		query = fmt.Sprintf("INSERT OR IGNORE INTO %s (%s) VALUES (%s)",
			tableName, colNames, phStr)
	}

	_, err = tx.Exec(query, values...)
	if err != nil {
		return fmt.Errorf("insert %s: %w (columns: %v, allowed: %v)", tableName, err, cols, allowedCols)
	}
	return nil
}

// applyUpdate updates an existing entity after checking for conflicts.
func (e *Engine) applyUpdate(tx *sql.Tx, tableName string, change Change) (*ConflictResult, error) {
	data, err := parseChangeData(change)
	if err != nil {
		return nil, err
	}

	// Check if entity exists and get its updated_at
	serverUpdatedAt, serverData, err := e.getEntityState(tx, tableName, change.ID)
	if err == sql.ErrNoRows {
		// Entity doesn't exist on server, treat as create
		return nil, e.applyCreate(tx, tableName, change)
	}
	if err != nil {
		return nil, fmt.Errorf("get entity state: %w", err)
	}

	// Conflict detection
	if serverUpdatedAt != "" && change.ClientUpdatedAt != "" {
		resolution := resolveConflict(change.ClientUpdatedAt, serverUpdatedAt)
		if resolution == "server_wins" {
			return &ConflictResult{
				Entity:     change.Entity,
				ID:         change.ID,
				ClientData: change.Data,
				ServerData: serverData,
				Resolution: resolution,
			}, nil
		}
	}

	// Client wins — apply the update
	cols, _ := filterColumns(change.Entity, data)
	if len(cols) == 0 {
		return nil, nil
	}

	setParts := make([]string, 0, len(cols))
	values := make([]any, 0, len(cols))
	for _, col := range cols {
		if col == "id" {
			continue
		}
		setParts = append(setParts, fmt.Sprintf("%s = ?", col))
		values = append(values, data[col])
	}

	if len(setParts) == 0 {
		return nil, nil
	}

	values = append(values, change.ID)
	query := fmt.Sprintf("UPDATE %s SET %s WHERE id = ?", tableName, strings.Join(setParts, ", "))
	_, err = tx.Exec(query, values...)
	if err != nil {
		return nil, fmt.Errorf("update %s: %w", tableName, err)
	}
	return nil, nil
}

// applyDelete deletes an entity after checking for conflicts.
func (e *Engine) applyDelete(tx *sql.Tx, tableName string, change Change) (*ConflictResult, error) {
	// Check if entity exists
	serverUpdatedAt, serverData, err := e.getEntityState(tx, tableName, change.ID)
	if err == sql.ErrNoRows {
		// Already deleted, no-op
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get entity state: %w", err)
	}

	// Conflict detection for deletes
	if serverUpdatedAt != "" && change.ClientUpdatedAt != "" {
		resolution := resolveConflict(change.ClientUpdatedAt, serverUpdatedAt)
		if resolution == "server_wins" {
			return &ConflictResult{
				Entity:     change.Entity,
				ID:         change.ID,
				ClientData: nil,
				ServerData: serverData,
				Resolution: resolution,
			}, nil
		}
	}

	query := fmt.Sprintf("DELETE FROM %s WHERE id = ?", tableName)
	_, err = tx.Exec(query, change.ID)
	if err != nil {
		return nil, fmt.Errorf("delete from %s: %w", tableName, err)
	}
	return nil, nil
}

// logChange records a change in the sync_log table.
func (e *Engine) logChange(tx *sql.Tx, entity, entityID, action string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Get next version
	var maxVersion int64
	err := tx.QueryRow("SELECT COALESCE(MAX(version), 0) FROM sync_log").Scan(&maxVersion)
	if err != nil {
		return err
	}
	nextVersion := maxVersion + 1

	_, err = tx.Exec(`
		INSERT INTO sync_log (entity, entity_id, action, timestamp, version)
		VALUES (?, ?, ?, ?, ?)`,
		entity, entityID, action, now, nextVersion)
	return err
}

// getCurrentVersion returns the current max version from sync_log.
func (e *Engine) getCurrentVersion(db *sql.DB) (int64, error) {
	var version int64
	err := db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM sync_log").Scan(&version)
	return version, err
}

// getEntityState fetches the updated_at and full data of an entity.
func (e *Engine) getEntityState(tx *sql.Tx, tableName, id string) (string, json.RawMessage, error) {
	data, err := e.fetchEntityDataTx(tx, tableName, id)
	if err != nil {
		return "", nil, err
	}

	// Extract updated_at from the data
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", data, nil
	}

	updatedAt, _ := parsed["updated_at"].(string)
	return updatedAt, data, nil
}

// fetchEntityData fetches all columns of an entity as JSON.
func (e *Engine) fetchEntityData(db *sql.DB, tableName, id string) (json.RawMessage, error) {
	query := fmt.Sprintf("SELECT * FROM %s WHERE id = ?", tableName)
	rows, err := db.Query(query, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanRowToJSON(rows)
}

// fetchEntityDataTx fetches all columns of an entity as JSON within a transaction.
func (e *Engine) fetchEntityDataTx(tx *sql.Tx, tableName, id string) (json.RawMessage, error) {
	query := fmt.Sprintf("SELECT * FROM %s WHERE id = ?", tableName)
	rows, err := tx.Query(query, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanRowToJSON(rows)
}

// scanRowToJSON scans a single row into a JSON object.
func scanRowToJSON(rows *sql.Rows) (json.RawMessage, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	if !rows.Next() {
		return nil, sql.ErrNoRows
	}

	values := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range values {
		ptrs[i] = &values[i]
	}

	if err := rows.Scan(ptrs...); err != nil {
		return nil, err
	}

	result := make(map[string]any, len(cols))
	for i, col := range cols {
		result[col] = values[i]
	}

	data, err := json.Marshal(result)
	return data, err
}

// parseChangeData parses the JSON data from a change into a map.
func parseChangeData(change Change) (map[string]any, error) {
	if len(change.Data) == 0 {
		return map[string]any{}, nil
	}

	var data map[string]any
	if err := json.Unmarshal(change.Data, &data); err != nil {
		return nil, fmt.Errorf("parse change data: %w", err)
	}
	return data, nil
}

// filterColumns returns only the columns that are allowed for the entity type.
func filterColumns(entity string, data map[string]any) ([]string, []string) {
	allowed, ok := columnsForEntity[entity]
	if !ok {
		return nil, nil
	}

	allowedSet := make(map[string]bool, len(allowed))
	for _, col := range allowed {
		allowedSet[col] = true
	}

	var filtered []string
	for key := range data {
		if allowedSet[key] {
			filtered = append(filtered, key)
		}
	}

	return filtered, allowed
}

// orderChanges sorts changes by entity dependency order.
// Creates go first (collections → field_defs → items → field_values).
// Deletes go last in reverse order.
func orderChanges(changes []Change) []Change {
	// Separate by action type
	var creates, updates, deletes []Change
	for _, c := range changes {
		switch c.Action {
		case "create":
			creates = append(creates, c)
		case "update":
			updates = append(updates, c)
		case "delete":
			deletes = append(deletes, c)
		default:
			updates = append(updates, c)
		}
	}

	// Sort creates by dependency order
	creates = sortByEntityOrder(creates, entityCreateOrder)
	// Sort deletes by reverse dependency order
	deletes = sortByEntityOrder(deletes, entityDeleteOrder)

	// Combine: creates first, then updates, then deletes
	result := make([]Change, 0, len(changes))
	result = append(result, creates...)
	result = append(result, updates...)
	result = append(result, deletes...)
	return result
}

// sortByEntityOrder sorts changes according to the given entity order.
func sortByEntityOrder(changes []Change, order []string) []Change {
	orderMap := make(map[string]int, len(order))
	for i, entity := range order {
		orderMap[entity] = i
	}

	// Bucket sort by entity order
	buckets := make([][]Change, len(order)+1)
	for i := range buckets {
		buckets[i] = []Change{}
	}

	for _, c := range changes {
		idx, ok := orderMap[c.Entity]
		if !ok {
			idx = len(order) // Unknown entities go last
		}
		buckets[idx] = append(buckets[idx], c)
	}

	var result []Change
	for _, bucket := range buckets {
		result = append(result, bucket...)
	}
	return result
}

// deduplicateChanges keeps only the latest change for each entity/id pair.
func deduplicateChanges(changes []Change) []Change {
	seen := make(map[string]int) // "entity:id" -> index in result
	var result []Change

	for _, c := range changes {
		key := c.Entity + ":" + c.ID
		if idx, ok := seen[key]; ok {
			// Replace with the later version
			result[idx] = c
		} else {
			seen[key] = len(result)
			result = append(result, c)
		}
	}

	return result
}
