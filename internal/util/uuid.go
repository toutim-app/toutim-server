package util

import "regexp"

// uuidRegex matches UUID v4 format (lowercase hex).
var uuidRegex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// IsValidUUID checks if a string is a valid UUID format.
// Used for path traversal protection when constructing file paths from user IDs.
func IsValidUUID(s string) bool {
	return uuidRegex.MatchString(s)
}
