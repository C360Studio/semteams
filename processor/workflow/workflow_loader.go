package workflow

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	wfschema "github.com/c360studio/semstreams/processor/workflow/schema"
)

// maxWorkflowFileSize is the maximum allowed size for a workflow definition file (1MB)
const maxWorkflowFileSize = 1 * 1024 * 1024

// loadWorkflowDefinitionsFromFiles loads workflow definitions from JSON files.
// Supports glob patterns in file paths. Supports both single definition and
// array of definitions per file.
func (c *Component) loadWorkflowDefinitionsFromFiles() ([]wfschema.Definition, error) {
	var allDefinitions []wfschema.Definition
	seenIDs := make(map[string]string) // workflow_id -> source_path

	for _, pattern := range c.config.WorkflowFiles {
		// Expand glob pattern
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid glob pattern %q: %w", pattern, err)
		}

		// If no matches and pattern has no glob chars, treat as literal path
		if len(matches) == 0 {
			if hasGlobChars(pattern) {
				c.logger.Warn("Glob pattern matched no files", slog.String("pattern", pattern))
				continue
			}
			// Treat non-glob pattern as literal path (will fail on ReadFile if missing)
			matches = []string{pattern}
		}

		for _, path := range matches {
			c.logger.Debug("Loading workflow from file", slog.String("path", path))

			// Check file size before reading
			fi, err := os.Stat(path)
			if err != nil {
				return nil, fmt.Errorf("failed to stat workflow file %s: %w", path, err)
			}
			if fi.Size() > maxWorkflowFileSize {
				return nil, fmt.Errorf("workflow file %s exceeds maximum size (%d > %d bytes)",
					path, fi.Size(), maxWorkflowFileSize)
			}

			data, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("failed to read workflow file %s: %w", path, err)
			}

			// Parse JSON - support both single definition and array
			var definitions []wfschema.Definition
			if err := json.Unmarshal(data, &definitions); err != nil {
				// Try parsing as single definition
				var singleDef wfschema.Definition
				if err2 := json.Unmarshal(data, &singleDef); err2 != nil {
					return nil, fmt.Errorf("failed to parse workflow file %s (tried array and object): array error: %v, object error: %w",
						path, err, err2)
				}
				definitions = []wfschema.Definition{singleDef}
			}

			// Validate each definition and check for duplicates
			for i := range definitions {
				if err := definitions[i].Validate(); err != nil {
					return nil, fmt.Errorf("validation failed for workflow in %s: %w", path, err)
				}

				// Check for duplicate workflow IDs
				if existingPath, exists := seenIDs[definitions[i].ID]; exists {
					return nil, fmt.Errorf("duplicate workflow ID %q: found in both %s and %s",
						definitions[i].ID, existingPath, path)
				}
				seenIDs[definitions[i].ID] = path
			}

			c.logger.Info("Loaded workflow definitions from file",
				slog.String("path", path),
				slog.Int("count", len(definitions)))
			allDefinitions = append(allDefinitions, definitions...)
		}
	}

	return allDefinitions, nil
}

// hasGlobChars returns true if the pattern contains glob special characters
func hasGlobChars(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[")
}
