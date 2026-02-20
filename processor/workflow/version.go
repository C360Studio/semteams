package workflow

import (
	"github.com/c360studio/semstreams/config"
)

// IsNewerVersion returns true if newVersion > oldVersion.
// Uses config.CompareVersions for semver comparison.
//   - Empty newVersion always returns false (preserves existing).
//   - Empty oldVersion returns true if newVersion is valid (new entry).
//   - Invalid versions return false (preserves existing on comparison error).
func IsNewerVersion(newVersion, oldVersion string) bool {
	// Empty new version is never newer
	if newVersion == "" {
		return false
	}

	// Empty old version means new version is always newer (if valid)
	if oldVersion == "" {
		// Validate that newVersion is a valid semver
		_, err := config.CompareVersions(newVersion, "0.0.0")
		return err == nil
	}

	// Compare versions - if comparison fails, preserve existing (return false)
	cmp, err := config.CompareVersions(newVersion, oldVersion)
	if err != nil {
		return false
	}

	return cmp > 0
}
