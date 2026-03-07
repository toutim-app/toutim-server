package sync

import "encoding/json"

// Change represents a single entity modification from the client.
type Change struct {
	Entity          string          `json:"entity"`           // "collection", "item", "field_definition", "field_value"
	ID              string          `json:"id"`               // UUID of the entity
	Action          string          `json:"action"`           // "create", "update", "delete"
	Data            json.RawMessage `json:"data,omitempty"`   // Entity data as JSON
	ClientUpdatedAt string          `json:"client_updated_at"` // RFC 3339
}

// PushRequest is the body of POST /api/sync/push.
type PushRequest struct {
	Changes     []Change `json:"changes"`
	LastVersion int64    `json:"last_version"`
}

// PushResponse is the response to a push.
type PushResponse struct {
	Conflicts []ConflictResult `json:"conflicts"`
	Version   int64            `json:"version"`
	Applied   int              `json:"applied"`
}

// PullRequest is the body of POST /api/sync/pull.
type PullRequest struct {
	SinceVersion int64 `json:"since_version"`
}

// PullResponse is the response to a pull.
type PullResponse struct {
	Changes []Change `json:"changes"`
	Version int64    `json:"version"`
}

// SyncStatusResponse is the response to GET /api/sync/status.
type SyncStatusResponse struct {
	LastSyncAt     *string `json:"last_sync_at"`
	CurrentVersion int64   `json:"current_version"`
	StorageBytes   int64   `json:"storage_bytes"`
}

// entityOrder defines the dependency order for applying changes.
// Collections must exist before field_definitions and items,
// and items/field_definitions must exist before field_values.
var entityCreateOrder = []string{"collection", "field_definition", "item", "field_value"}
var entityDeleteOrder = []string{"field_value", "item", "field_definition", "collection"}

// tableForEntity maps entity names to their SQL table names.
var tableForEntity = map[string]string{
	"collection":       "collections",
	"item":             "items",
	"field_definition": "field_definitions",
	"field_value":      "item_field_values",
}

// columnsForEntity defines the allowed columns for each entity type.
// Only these columns can be inserted/updated via sync.
var columnsForEntity = map[string][]string{
	"collection": {
		"id", "name", "description", "icon", "color", "template_id",
		"view_mode", "sync_status", "created_at", "updated_at",
	},
	"item": {
		"id", "collection_id", "favorite", "sync_status", "created_at", "updated_at",
	},
	"field_definition": {
		"id", "collection_id", "name", "type", "sort_order",
		"is_required", "show_in_list", "show_in_card", "searchable", "sortable",
		"default_value", "options", "sync_status",
	},
	"field_value": {
		"id", "item_id", "field_definition_id", "value", "updated_at",
	},
}
