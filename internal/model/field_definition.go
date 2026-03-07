package model

import "encoding/json"

// FieldDefinition describes a custom field schema within a collection.
type FieldDefinition struct {
	ID           string          `json:"id"`
	CollectionID string          `json:"collection_id"`
	Name         string          `json:"name"`
	Type         string          `json:"type"`
	SortOrder    int             `json:"sort_order"`
	IsRequired   bool            `json:"is_required"`
	ShowInList   bool            `json:"show_in_list"`
	ShowInCard   bool            `json:"show_in_card"`
	Searchable   bool            `json:"searchable"`
	Sortable     bool            `json:"sortable"`
	DefaultValue json.RawMessage `json:"default_value,omitempty"`
	Options      json.RawMessage `json:"options"`
	SyncStatus   string          `json:"sync_status"`
}
