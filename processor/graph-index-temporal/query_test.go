package graphindextemporal

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGenerateTemporalPrefixes_ShortRange verifies day-level prefixes for ranges ≤ 30 days.
func TestGenerateTemporalPrefixes_ShortRange(t *testing.T) {
	tests := []struct {
		name     string
		start    string
		end      string
		expected []string
	}{
		{
			name:     "single hour within one day",
			start:    "2026-03-02T10:00:00Z",
			end:      "2026-03-02T11:59:59Z",
			expected: []string{"2026.03.02.>"},
		},
		{
			name:  "two consecutive days",
			start: "2026-03-02T00:00:00Z",
			end:   "2026-03-03T23:59:59Z",
			expected: []string{
				"2026.03.02.>",
				"2026.03.03.>",
			},
		},
		{
			name:  "spans month boundary (Feb to Mar)",
			start: "2026-02-28T00:00:00Z",
			end:   "2026-03-01T23:59:59Z",
			expected: []string{
				"2026.02.28.>",
				"2026.03.01.>",
			},
		},
		{
			name:  "exactly 30 days uses day-level prefixes",
			start: "2026-01-01T00:00:00Z",
			end:   "2026-01-30T23:59:59Z",
			// 30 days: day prefixes for Jan 01–30
			expected: func() []string {
				var p []string
				for d := 1; d <= 30; d++ {
					p = append(p, time.Date(2026, 1, d, 0, 0, 0, 0, time.UTC).Format("2006.01.02")+".>")
				}
				return p
			}(),
		},
		{
			name:  "single day spans year boundary",
			start: "2025-12-31T23:00:00Z",
			end:   "2026-01-01T00:59:59Z",
			expected: []string{
				"2025.12.31.>",
				"2026.01.01.>",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, err := time.Parse(time.RFC3339, tt.start)
			require.NoError(t, err)
			end, err := time.Parse(time.RFC3339, tt.end)
			require.NoError(t, err)

			got := generateTemporalPrefixes(start, end)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestGenerateTemporalPrefixes_LongRange verifies month-level prefixes for ranges > 30 days.
func TestGenerateTemporalPrefixes_LongRange(t *testing.T) {
	tests := []struct {
		name     string
		start    string
		end      string
		expected []string
	}{
		{
			name:  "31 days falls into month-level prefixes",
			start: "2026-01-01T00:00:00Z",
			end:   "2026-02-01T00:00:00Z", // exactly 31 days
			expected: []string{
				"2026.01.>",
				"2026.02.>",
			},
		},
		{
			name:  "three months",
			start: "2026-01-15T00:00:00Z",
			end:   "2026-03-15T23:59:59Z",
			expected: []string{
				"2026.01.>",
				"2026.02.>",
				"2026.03.>",
			},
		},
		{
			name:  "spans year boundary across multiple months",
			start: "2025-11-01T00:00:00Z",
			end:   "2026-02-28T23:59:59Z",
			expected: []string{
				"2025.11.>",
				"2025.12.>",
				"2026.01.>",
				"2026.02.>",
			},
		},
		{
			name:  "full year",
			start: "2026-01-01T00:00:00Z",
			end:   "2026-12-31T23:59:59Z",
			expected: []string{
				"2026.01.>",
				"2026.02.>",
				"2026.03.>",
				"2026.04.>",
				"2026.05.>",
				"2026.06.>",
				"2026.07.>",
				"2026.08.>",
				"2026.09.>",
				"2026.10.>",
				"2026.11.>",
				"2026.12.>",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, err := time.Parse(time.RFC3339, tt.start)
			require.NoError(t, err)
			end, err := time.Parse(time.RFC3339, tt.end)
			require.NoError(t, err)

			got := generateTemporalPrefixes(start, end)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestGenerateTemporalPrefixes_PrefixFormat checks that each prefix ends with ".>"
// and has the right number of dot-separated components.
func TestGenerateTemporalPrefixes_PrefixFormat(t *testing.T) {
	start, _ := time.Parse(time.RFC3339, "2026-03-02T00:00:00Z")
	end, _ := time.Parse(time.RFC3339, "2026-03-04T23:59:59Z")

	prefixes := generateTemporalPrefixes(start, end)
	require.NotEmpty(t, prefixes)

	for _, p := range prefixes {
		assert.True(t, len(p) > 0, "prefix must not be empty")
		assert.Equal(t, byte('>'), p[len(p)-1], "prefix must end with '>'")
		// Day-level: YYYY.MM.DD.> has 4 components split by '.'
		// The ">" is the last component.
		var count int
		for i := 0; i < len(p); i++ {
			if p[i] == '.' {
				count++
			}
		}
		assert.Equal(t, 3, count, "day-level prefix must have 3 dots: %q", p)
	}
}
