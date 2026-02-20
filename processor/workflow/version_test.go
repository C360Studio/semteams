package workflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsNewerVersion(t *testing.T) {
	tests := []struct {
		name       string
		newVersion string
		oldVersion string
		expected   bool
	}{
		// Valid semver comparisons
		{"newer major", "2.0.0", "1.0.0", true},
		{"newer minor", "1.2.0", "1.1.0", true},
		{"newer patch", "1.0.2", "1.0.1", true},
		{"same version", "1.0.0", "1.0.0", false},
		{"older major", "1.0.0", "2.0.0", false},
		{"older minor", "1.1.0", "1.2.0", false},
		{"older patch", "1.0.1", "1.0.2", false},

		// Empty version handling
		{"new vs empty old", "1.0.0", "", true},
		{"empty new vs old", "", "1.0.0", false},
		{"both empty", "", "", false},
		{"0.0.1 vs empty", "0.0.1", "", true},

		// With v prefix (supported by config.CompareVersions)
		{"v prefix newer", "v2.0.0", "v1.0.0", true},
		{"v prefix same", "v1.0.0", "v1.0.0", false},
		{"v prefix vs no prefix", "v2.0.0", "1.0.0", true},

		// Invalid versions - should return false (preserve existing)
		{"invalid new version", "invalid", "1.0.0", false},
		{"invalid old version", "2.0.0", "invalid", false},
		{"partial version (invalid)", "1.2", "1.0.0", false},
		{"single number (invalid)", "2", "1.0.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsNewerVersion(tt.newVersion, tt.oldVersion)
			assert.Equal(t, tt.expected, result, "IsNewerVersion(%q, %q)", tt.newVersion, tt.oldVersion)
		})
	}
}
