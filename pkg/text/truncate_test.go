package text

import "testing"

func TestTruncateAtWord(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxChars int
		want     string
	}{
		{
			name:     "shorter than limit",
			input:    "hello world",
			maxChars: 100,
			want:     "hello world",
		},
		{
			name:     "exact limit",
			input:    "hello",
			maxChars: 5,
			want:     "hello",
		},
		{
			name:     "truncate at word boundary",
			input:    "hello world foo bar",
			maxChars: 15,
			want:     "hello world...",
		},
		{
			name:     "no spaces - hard truncate",
			input:    "helloworld",
			maxChars: 8,
			want:     "hello...",
		},
		{
			name:     "very short limit",
			input:    "hello world",
			maxChars: 3,
			want:     "...",
		},
		{
			name:     "limit of 4",
			input:    "hello world",
			maxChars: 4,
			want:     "h...",
		},
		{
			name:     "empty string",
			input:    "",
			maxChars: 10,
			want:     "",
		},
		{
			name:     "single word longer than limit",
			input:    "supercalifragilisticexpialidocious",
			maxChars: 15,
			want:     "supercalifra...", // 12 chars + "..." = 15
		},
		{
			name:     "multiple spaces preserved until truncation",
			input:    "hello  world  foo  bar",
			maxChars: 16,
			want:     "hello  world...",
		},
		{
			name:     "unicode CJK characters",
			input:    "Hello 世界 test more text",
			maxChars: 12,
			want:     "Hello 世界...",
		},
		{
			name:     "unicode emoji",
			input:    "Hello 🚀 world test",
			maxChars: 12,
			want:     "Hello 🚀...",
		},
		{
			name:     "unicode only - no spaces",
			input:    "世界你好朋友们",
			maxChars: 6,
			want:     "世界你...",
		},
		{
			name:     "truncate at 250 chars (default)",
			input:    "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate.",
			maxChars: 250,
			want:     "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateAtWord(tt.input, tt.maxChars)
			if got != tt.want {
				t.Errorf("TruncateAtWord(%q, %d) = %q, want %q", tt.input, tt.maxChars, got, tt.want)
			}
			// Verify result doesn't exceed maxChars (in runes, not bytes)
			gotRunes := len([]rune(got))
			if gotRunes > tt.maxChars {
				t.Errorf("TruncateAtWord(%q, %d) returned %d runes, exceeds max %d", tt.input, tt.maxChars, gotRunes, tt.maxChars)
			}
		})
	}
}
