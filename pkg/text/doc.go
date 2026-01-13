// Package text provides text manipulation utilities.
//
// # Overview
//
// The text package provides common text manipulation functions used throughout
// the codebase. It focuses on Unicode-safe operations that work correctly with
// multi-byte characters.
//
// # Functions
//
// TruncateAtWord:
//
// Truncates text to a maximum character count, breaking at word boundaries
// when possible. This is useful for generating summaries, preview text, or
// any UI that needs to display text within a fixed width.
//
//	text.TruncateAtWord("Hello world, this is a long sentence", 20)
//	// Returns: "Hello world, this..."
//
// Key behaviors:
//   - Works with runes (Unicode code points), not bytes
//   - Breaks at last space before limit when possible
//   - Falls back to hard truncation if no space found
//   - Appends "..." suffix when truncation occurs
//   - Returns original text unchanged if already short enough
//
// # Unicode Safety
//
// All functions operate on runes rather than bytes, ensuring correct behavior
// with multi-byte UTF-8 characters:
//
//	// Japanese text - each character is 3 bytes
//	text.TruncateAtWord("こんにちは世界", 5)
//	// Returns: "こん..." (correct character count, not byte count)
//
// # Usage Example
//
//	// Generate preview text for a list of items
//	for _, item := range items {
//	    preview := text.TruncateAtWord(item.Description, 100)
//	    fmt.Printf("- %s: %s\n", item.Title, preview)
//	}
//
//	// Community summary truncation
//	summary := text.TruncateAtWord(community.Summary, 500)
//
// # Thread Safety
//
// All functions are pure (no shared state) and safe for concurrent use.
//
// # See Also
//
// Related packages:
//   - [strings]: Go standard library string operations
//   - [unicode/utf8]: UTF-8 encoding utilities
package text
