package config

import (
	"os"
	"regexp"
)

// envVarRe matches ${VAR:-default}, ${VAR}, and $VAR patterns
var envVarRe = regexp.MustCompile(`\$\{([^}:]+)(:-([^}]*))?\}|\$([a-zA-Z_][a-zA-Z0-9_]*)`)

// ExpandEnvWithDefaults expands environment variables in a string,
// supporting ${VAR:-default} syntax for default values.
//
// Patterns:
//   - ${VAR} - expands to value of VAR, or empty if unset
//   - ${VAR:-default} - expands to value of VAR, or "default" if unset
//   - $VAR - expands to value of VAR, or empty if unset
func ExpandEnvWithDefaults(s string) string {
	return envVarRe.ReplaceAllStringFunc(s, func(match string) string {
		submatches := envVarRe.FindStringSubmatch(match)

		// $VAR form (group 4)
		if submatches[4] != "" {
			return os.Getenv(submatches[4])
		}

		// ${VAR} or ${VAR:-default} form
		varName := submatches[1]
		value := os.Getenv(varName)

		// If value is set, use it
		if value != "" {
			return value
		}

		// If unset, check for default (group 3)
		if submatches[2] != "" {
			return submatches[3] // The default value (may be empty)
		}

		return ""
	})
}
