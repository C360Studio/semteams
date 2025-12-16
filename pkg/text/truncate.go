// Package text provides text manipulation utilities.
package text

import "strings"

// TruncateAtWord truncates text to maxChars characters (runes, not bytes),
// breaking at the last word boundary before the limit.
// Returns original text unchanged if shorter than maxChars.
// Appends "..." if truncation occurred.
func TruncateAtWord(s string, maxChars int) string {
	runes := []rune(s)
	if len(runes) <= maxChars {
		return s
	}

	// Leave room for "..."
	limit := maxChars - 3
	if limit <= 0 {
		return "..."
	}

	// Find last space before limit
	truncated := string(runes[:limit])
	lastSpace := strings.LastIndex(truncated, " ")
	if lastSpace <= 0 {
		// No space found, hard truncate
		return truncated + "..."
	}

	return truncated[:lastSpace] + "..."
}
