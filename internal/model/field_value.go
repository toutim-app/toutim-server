package model

import "encoding/json"

// FieldValue holds the value of a field for a specific item.
type FieldValue struct {
	ID                string          `json:"id"`
	ItemID            string          `json:"item_id"`
	FieldDefinitionID string          `json:"field_definition_id"`
	Value             json.RawMessage `json:"value,omitempty"`
	UpdatedAt         string          `json:"updated_at"`
}
