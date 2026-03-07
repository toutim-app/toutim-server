package sync

import (
	"encoding/json"
	"time"
)

// ConflictResult describes a conflict detected during push.
type ConflictResult struct {
	Entity     string          `json:"entity"`
	ID         string          `json:"id"`
	ClientData json.RawMessage `json:"client_data"`
	ServerData json.RawMessage `json:"server_data"`
	Resolution string          `json:"resolution"` // "client_wins" or "server_wins"
}

// resolveConflict compares client and server timestamps using LWW.
// Returns "client_wins" if the client's update is more recent.
func resolveConflict(clientUpdatedAt, serverUpdatedAt string) string {
	clientTime, err1 := time.Parse(time.RFC3339, clientUpdatedAt)
	serverTime, err2 := time.Parse(time.RFC3339, serverUpdatedAt)

	// On parse errors, server wins (conservative)
	if err1 != nil || err2 != nil {
		return "server_wins"
	}

	if clientTime.After(serverTime) {
		return "client_wins"
	}
	return "server_wins"
}
