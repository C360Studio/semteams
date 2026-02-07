package workflow

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testLogger returns a no-op logger for tests
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestLoadWorkflowDefinitionsFromFiles_SingleFile(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Create a valid workflow file
	workflow := `{
		"id": "test-workflow",
		"name": "Test Workflow",
		"enabled": true,
		"trigger": {"subject": "test.trigger"},
		"steps": [
			{
				"name": "step1",
				"action": {"type": "publish", "subject": "test.action"}
			}
		]
	}`

	filePath := filepath.Join(tmpDir, "workflow.json")
	err := os.WriteFile(filePath, []byte(workflow), 0644)
	require.NoError(t, err)

	// Create component with config
	c := &Component{
		config: Config{
			WorkflowFiles: []string{filePath},
		},
		logger: testLogger(),
	}

	// Load definitions
	definitions, err := c.loadWorkflowDefinitionsFromFiles()
	require.NoError(t, err)
	require.Len(t, definitions, 1)
	assert.Equal(t, "test-workflow", definitions[0].ID)
	assert.Equal(t, "Test Workflow", definitions[0].Name)
}

func TestLoadWorkflowDefinitionsFromFiles_ArrayOfWorkflows(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file with array of workflows
	workflows := `[
		{
			"id": "workflow-1",
			"name": "Workflow One",
			"enabled": true,
			"trigger": {"subject": "trigger.one"},
			"steps": [{"name": "step1", "action": {"type": "publish", "subject": "action.one"}}]
		},
		{
			"id": "workflow-2",
			"name": "Workflow Two",
			"enabled": true,
			"trigger": {"subject": "trigger.two"},
			"steps": [{"name": "step1", "action": {"type": "publish", "subject": "action.two"}}]
		}
	]`

	filePath := filepath.Join(tmpDir, "workflows.json")
	err := os.WriteFile(filePath, []byte(workflows), 0644)
	require.NoError(t, err)

	c := &Component{
		config: Config{
			WorkflowFiles: []string{filePath},
		},
		logger: testLogger(),
	}

	definitions, err := c.loadWorkflowDefinitionsFromFiles()
	require.NoError(t, err)
	require.Len(t, definitions, 2)
	assert.Equal(t, "workflow-1", definitions[0].ID)
	assert.Equal(t, "workflow-2", definitions[1].ID)
}

func TestLoadWorkflowDefinitionsFromFiles_GlobPattern(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple workflow files
	for i, id := range []string{"workflow-a", "workflow-b", "workflow-c"} {
		workflow := `{
			"id": "` + id + `",
			"name": "Workflow ` + string(rune('A'+i)) + `",
			"enabled": true,
			"trigger": {"subject": "trigger.` + id + `"},
			"steps": [{"name": "step1", "action": {"type": "publish", "subject": "action.` + id + `"}}]
		}`
		filePath := filepath.Join(tmpDir, id+".json")
		err := os.WriteFile(filePath, []byte(workflow), 0644)
		require.NoError(t, err)
	}

	c := &Component{
		config: Config{
			WorkflowFiles: []string{filepath.Join(tmpDir, "*.json")},
		},
		logger: testLogger(),
	}

	definitions, err := c.loadWorkflowDefinitionsFromFiles()
	require.NoError(t, err)
	require.Len(t, definitions, 3)

	ids := make(map[string]bool)
	for _, def := range definitions {
		ids[def.ID] = true
	}
	assert.True(t, ids["workflow-a"])
	assert.True(t, ids["workflow-b"])
	assert.True(t, ids["workflow-c"])
}

func TestLoadWorkflowDefinitionsFromFiles_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()

	filePath := filepath.Join(tmpDir, "invalid.json")
	err := os.WriteFile(filePath, []byte("not valid json"), 0644)
	require.NoError(t, err)

	c := &Component{
		config: Config{
			WorkflowFiles: []string{filePath},
		},
		logger: testLogger(),
	}

	_, err = c.loadWorkflowDefinitionsFromFiles()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse workflow file")
}

func TestLoadWorkflowDefinitionsFromFiles_ValidationFailure(t *testing.T) {
	tmpDir := t.TempDir()

	// Create workflow missing required fields
	workflow := `{
		"id": "",
		"name": "Invalid Workflow",
		"enabled": true,
		"trigger": {"subject": "test"},
		"steps": []
	}`

	filePath := filepath.Join(tmpDir, "invalid.json")
	err := os.WriteFile(filePath, []byte(workflow), 0644)
	require.NoError(t, err)

	c := &Component{
		config: Config{
			WorkflowFiles: []string{filePath},
		},
		logger: testLogger(),
	}

	_, err = c.loadWorkflowDefinitionsFromFiles()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "validation failed")
}

func TestLoadWorkflowDefinitionsFromFiles_NonExistentFile(t *testing.T) {
	c := &Component{
		config: Config{
			WorkflowFiles: []string{"/nonexistent/path/workflow.json"},
		},
		logger: testLogger(),
	}

	_, err := c.loadWorkflowDefinitionsFromFiles()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to stat workflow file")
}

func TestLoadWorkflowDefinitionsFromFiles_EmptyPatterns(t *testing.T) {
	c := &Component{
		config: Config{
			WorkflowFiles: []string{},
		},
		logger: testLogger(),
	}

	definitions, err := c.loadWorkflowDefinitionsFromFiles()
	require.NoError(t, err)
	assert.Len(t, definitions, 0)
}

func TestLoadWorkflowDefinitionsFromFiles_GlobNoMatches(t *testing.T) {
	tmpDir := t.TempDir()

	c := &Component{
		config: Config{
			WorkflowFiles: []string{filepath.Join(tmpDir, "*.json")},
		},
		logger: testLogger(),
	}

	// Glob with no matches returns empty slice, not error
	definitions, err := c.loadWorkflowDefinitionsFromFiles()
	require.NoError(t, err)
	assert.Len(t, definitions, 0)
}

func TestHasGlobChars(t *testing.T) {
	tests := []struct {
		pattern  string
		expected bool
	}{
		{"/path/to/file.json", false},
		{"/path/to/*.json", true},
		{"/path/to/file?.json", true},
		{"/path/[ab]/file.json", true},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			assert.Equal(t, tt.expected, hasGlobChars(tt.pattern))
		})
	}
}

func TestLoadWorkflowDefinitionsFromFiles_EmptyArray(t *testing.T) {
	tmpDir := t.TempDir()

	filePath := filepath.Join(tmpDir, "empty.json")
	err := os.WriteFile(filePath, []byte("[]"), 0644)
	require.NoError(t, err)

	c := &Component{
		config: Config{
			WorkflowFiles: []string{filePath},
		},
		logger: testLogger(),
	}

	definitions, err := c.loadWorkflowDefinitionsFromFiles()
	require.NoError(t, err)
	assert.Len(t, definitions, 0)
}

func TestLoadWorkflowDefinitionsFromFiles_InvalidGlobPattern(t *testing.T) {
	c := &Component{
		config: Config{
			WorkflowFiles: []string{"[invalid"},
		},
		logger: testLogger(),
	}

	_, err := c.loadWorkflowDefinitionsFromFiles()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid glob pattern")
}

func TestLoadWorkflowDefinitionsFromFiles_WrongJSONStructure(t *testing.T) {
	tmpDir := t.TempDir()

	// Valid JSON but wrong structure (not a workflow definition)
	filePath := filepath.Join(tmpDir, "wrong.json")
	err := os.WriteFile(filePath, []byte(`{"foo": "bar", "baz": 123}`), 0644)
	require.NoError(t, err)

	c := &Component{
		config: Config{
			WorkflowFiles: []string{filePath},
		},
		logger: testLogger(),
	}

	// This should fail validation because required fields are missing
	_, err = c.loadWorkflowDefinitionsFromFiles()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "validation failed")
}

func TestLoadWorkflowDefinitionsFromFiles_DuplicateWorkflowIDs(t *testing.T) {
	tmpDir := t.TempDir()

	// Create two files with the same workflow ID
	workflow1 := `{
		"id": "duplicate-id",
		"name": "Workflow One",
		"enabled": true,
		"trigger": {"subject": "trigger.one"},
		"steps": [{"name": "step1", "action": {"type": "publish", "subject": "action.one"}}]
	}`
	workflow2 := `{
		"id": "duplicate-id",
		"name": "Workflow Two",
		"enabled": true,
		"trigger": {"subject": "trigger.two"},
		"steps": [{"name": "step1", "action": {"type": "publish", "subject": "action.two"}}]
	}`

	err := os.WriteFile(filepath.Join(tmpDir, "workflow1.json"), []byte(workflow1), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, "workflow2.json"), []byte(workflow2), 0644)
	require.NoError(t, err)

	c := &Component{
		config: Config{
			WorkflowFiles: []string{filepath.Join(tmpDir, "*.json")},
		},
		logger: testLogger(),
	}

	_, err = c.loadWorkflowDefinitionsFromFiles()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate workflow ID")
}

func TestLoadWorkflowDefinitionsFromFiles_DuplicateIDsInSameFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Array with duplicate IDs in same file
	workflows := `[
		{
			"id": "same-id",
			"name": "Workflow One",
			"enabled": true,
			"trigger": {"subject": "trigger.one"},
			"steps": [{"name": "step1", "action": {"type": "publish", "subject": "action.one"}}]
		},
		{
			"id": "same-id",
			"name": "Workflow Two",
			"enabled": true,
			"trigger": {"subject": "trigger.two"},
			"steps": [{"name": "step1", "action": {"type": "publish", "subject": "action.two"}}]
		}
	]`

	filePath := filepath.Join(tmpDir, "duplicates.json")
	err := os.WriteFile(filePath, []byte(workflows), 0644)
	require.NoError(t, err)

	c := &Component{
		config: Config{
			WorkflowFiles: []string{filePath},
		},
		logger: testLogger(),
	}

	_, err = c.loadWorkflowDefinitionsFromFiles()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate workflow ID")
}

func TestLoadWorkflowDefinitionsFromFiles_FileTooLarge(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file larger than maxWorkflowFileSize (1MB)
	largeContent := make([]byte, maxWorkflowFileSize+1)
	for i := range largeContent {
		largeContent[i] = 'x'
	}

	filePath := filepath.Join(tmpDir, "large.json")
	err := os.WriteFile(filePath, largeContent, 0644)
	require.NoError(t, err)

	c := &Component{
		config: Config{
			WorkflowFiles: []string{filePath},
		},
		logger: testLogger(),
	}

	_, err = c.loadWorkflowDefinitionsFromFiles()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum size")
}

func TestConfigValidate_InvalidGlobPattern(t *testing.T) {
	cfg := Config{
		DefinitionsBucket:    "WORKFLOW_DEFINITIONS",
		ExecutionsBucket:     "WORKFLOW_EXECUTIONS",
		StreamName:           "WORKFLOW",
		DefaultTimeout:       "10m",
		DefaultMaxIterations: 10,
		WorkflowFiles:        []string{"[invalid"},
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid workflow_files pattern")
}
