package model

// Item represents an item within a collection.
type Item struct {
	ID           string `json:"id"`
	CollectionID string `json:"collection_id"`
	Favorite     bool   `json:"favorite"`
	SyncStatus   string `json:"sync_status"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}
