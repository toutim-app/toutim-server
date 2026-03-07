package model

// Collection represents a user's collection in their personal database.
type Collection struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	Icon        *string `json:"icon,omitempty"`
	Color       string  `json:"color"`
	TemplateID  *string `json:"template_id,omitempty"`
	ViewMode    string  `json:"view_mode"`
	SyncStatus  string  `json:"sync_status"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}
