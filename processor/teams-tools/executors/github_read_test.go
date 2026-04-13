package executors

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/c360studio/semstreams/agentic"
)

// --- ListTools ---

func TestGitHubReadExecutor_ListTools(t *testing.T) {
	e := NewGitHubReadExecutor(&mockGitHubClient{})
	tools := e.ListTools()

	if len(tools) != 5 {
		t.Fatalf("expected 5 tools, got %d", len(tools))
	}

	expected := map[string]bool{
		"github_get_issue":     true,
		"github_list_issues":   true,
		"github_search_issues": true,
		"github_get_pr":        true,
		"github_get_file":      true,
	}
	for _, tool := range tools {
		if !expected[tool.Name] {
			t.Errorf("unexpected tool: %s", tool.Name)
		}
		delete(expected, tool.Name)
		if tool.Description == "" {
			t.Errorf("tool %s has empty description", tool.Name)
		}
		if tool.Parameters == nil {
			t.Errorf("tool %s has nil parameters", tool.Name)
		}
	}
	for name := range expected {
		t.Errorf("missing expected tool: %s", name)
	}
}

// --- UnknownTool ---

func TestGitHubReadExecutor_UnknownTool(t *testing.T) {
	e := NewGitHubReadExecutor(&mockGitHubClient{})
	result, err := e.Execute(context.Background(), agentic.ToolCall{
		ID:        "c1",
		Name:      "does_not_exist",
		Arguments: map[string]any{},
	})
	if err == nil {
		t.Error("expected Go error for unknown tool")
	}
	if result.Error == "" {
		t.Error("expected ToolResult.Error for unknown tool")
	}
}

// --- github_get_issue ---

func TestGitHubReadExecutor_GetIssue_Success(t *testing.T) {
	issue := sampleIssue()
	mock := &mockGitHubClient{
		getIssueFunc: func(_ context.Context, owner, repo string, number int) (*GitHubIssue, error) {
			if owner != "acme" || repo != "ops" || number != 42 {
				t.Errorf("unexpected args: %s/%s#%d", owner, repo, number)
			}
			return issue, nil
		},
	}

	e := NewGitHubReadExecutor(mock)
	result, err := e.Execute(context.Background(), agentic.ToolCall{
		ID:   "c1",
		Name: "github_get_issue",
		Arguments: map[string]any{
			"owner":  "acme",
			"repo":   "ops",
			"number": float64(42),
		},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected ToolResult.Error: %s", result.Error)
	}
	if result.CallID != "c1" {
		t.Errorf("expected CallID c1, got %s", result.CallID)
	}

	var parsed GitHubIssue
	if err := json.Unmarshal([]byte(result.Content), &parsed); err != nil {
		t.Fatalf("content is not valid JSON: %v", err)
	}
	if parsed.Number != 42 {
		t.Errorf("expected number 42, got %d", parsed.Number)
	}

	if result.Metadata["number"] != 42 {
		t.Errorf("unexpected metadata number: %v", result.Metadata["number"])
	}
}

func TestGitHubReadExecutor_GetIssue_MissingOwner(t *testing.T) {
	e := NewGitHubReadExecutor(&mockGitHubClient{})
	result, err := e.Execute(context.Background(), agentic.ToolCall{
		ID:   "c2",
		Name: "github_get_issue",
		Arguments: map[string]any{
			"repo":   "ops",
			"number": float64(1),
		},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.Error == "" {
		t.Error("expected ToolResult.Error for missing owner")
	}
}

func TestGitHubReadExecutor_GetIssue_MissingNumber(t *testing.T) {
	e := NewGitHubReadExecutor(&mockGitHubClient{})
	result, err := e.Execute(context.Background(), agentic.ToolCall{
		ID:   "c3",
		Name: "github_get_issue",
		Arguments: map[string]any{
			"owner": "acme",
			"repo":  "ops",
		},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.Error == "" {
		t.Error("expected ToolResult.Error for missing number")
	}
}

func TestGitHubReadExecutor_GetIssue_ClientError(t *testing.T) {
	mock := &mockGitHubClient{
		getIssueFunc: func(_ context.Context, _, _ string, _ int) (*GitHubIssue, error) {
			return nil, errors.New("API rate limit exceeded")
		},
	}
	e := NewGitHubReadExecutor(mock)
	result, err := e.Execute(context.Background(), agentic.ToolCall{
		ID:   "c4",
		Name: "github_get_issue",
		Arguments: map[string]any{
			"owner":  "acme",
			"repo":   "ops",
			"number": float64(1),
		},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.Error == "" {
		t.Error("expected ToolResult.Error for client failure")
	}
}

// --- github_list_issues ---

func TestGitHubReadExecutor_ListIssues_Success(t *testing.T) {
	mock := &mockGitHubClient{
		listIssuesFunc: func(_ context.Context, owner, repo string, opts ListIssuesOpts) ([]GitHubIssue, error) {
			if owner != "acme" || repo != "ops" {
				t.Errorf("unexpected repo: %s/%s", owner, repo)
			}
			if opts.State != "open" {
				t.Errorf("expected state 'open', got %q", opts.State)
			}
			return []GitHubIssue{*sampleIssue()}, nil
		},
	}

	e := NewGitHubReadExecutor(mock)
	result, err := e.Execute(context.Background(), agentic.ToolCall{
		ID:   "c5",
		Name: "github_list_issues",
		Arguments: map[string]any{
			"owner": "acme",
			"repo":  "ops",
			"state": "open",
		},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected ToolResult.Error: %s", result.Error)
	}

	var issues []GitHubIssue
	if err := json.Unmarshal([]byte(result.Content), &issues); err != nil {
		t.Fatalf("content is not valid JSON: %v", err)
	}
	if len(issues) != 1 {
		t.Errorf("expected 1 issue, got %d", len(issues))
	}
}

func TestGitHubReadExecutor_ListIssues_DefaultState(t *testing.T) {
	mock := &mockGitHubClient{
		listIssuesFunc: func(_ context.Context, _, _ string, opts ListIssuesOpts) ([]GitHubIssue, error) {
			if opts.State != "open" {
				t.Errorf("expected default state 'open', got %q", opts.State)
			}
			return nil, nil
		},
	}

	e := NewGitHubReadExecutor(mock)
	result, err := e.Execute(context.Background(), agentic.ToolCall{
		ID:   "c6",
		Name: "github_list_issues",
		Arguments: map[string]any{
			"owner": "acme",
			"repo":  "ops",
		},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected ToolResult.Error: %s", result.Error)
	}
}

func TestGitHubReadExecutor_ListIssues_WithLabels(t *testing.T) {
	mock := &mockGitHubClient{
		listIssuesFunc: func(_ context.Context, _, _ string, opts ListIssuesOpts) ([]GitHubIssue, error) {
			if len(opts.Labels) != 2 || opts.Labels[0] != "bug" {
				t.Errorf("unexpected labels: %v", opts.Labels)
			}
			return nil, nil
		},
	}

	e := NewGitHubReadExecutor(mock)
	_, err := e.Execute(context.Background(), agentic.ToolCall{
		ID:   "c7",
		Name: "github_list_issues",
		Arguments: map[string]any{
			"owner":  "acme",
			"repo":   "ops",
			"labels": []interface{}{"bug", "triage"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
}

func TestGitHubReadExecutor_ListIssues_MissingRepo(t *testing.T) {
	e := NewGitHubReadExecutor(&mockGitHubClient{})
	result, err := e.Execute(context.Background(), agentic.ToolCall{
		ID:        "c8",
		Name:      "github_list_issues",
		Arguments: map[string]any{"owner": "acme"},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.Error == "" {
		t.Error("expected ToolResult.Error for missing repo")
	}
}

func TestGitHubReadExecutor_ListIssues_ClientError(t *testing.T) {
	mock := &mockGitHubClient{
		listIssuesFunc: func(_ context.Context, _, _ string, _ ListIssuesOpts) ([]GitHubIssue, error) {
			return nil, errors.New("network error")
		},
	}
	e := NewGitHubReadExecutor(mock)
	result, err := e.Execute(context.Background(), agentic.ToolCall{
		ID:   "c9",
		Name: "github_list_issues",
		Arguments: map[string]any{
			"owner": "acme",
			"repo":  "ops",
		},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.Error == "" {
		t.Error("expected ToolResult.Error for client failure")
	}
}

// --- github_search_issues ---

func TestGitHubReadExecutor_SearchIssues_Success(t *testing.T) {
	mock := &mockGitHubClient{
		searchIssuesFunc: func(_ context.Context, query string) ([]GitHubIssue, error) {
			if query != "repo:acme/ops is:open label:bug" {
				t.Errorf("unexpected query: %q", query)
			}
			return []GitHubIssue{*sampleIssue()}, nil
		},
	}

	e := NewGitHubReadExecutor(mock)
	result, err := e.Execute(context.Background(), agentic.ToolCall{
		ID:   "c10",
		Name: "github_search_issues",
		Arguments: map[string]any{
			"query": "repo:acme/ops is:open label:bug",
		},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected ToolResult.Error: %s", result.Error)
	}
	if result.Metadata["count"] != 1 {
		t.Errorf("expected count 1 in metadata, got %v", result.Metadata["count"])
	}
}

func TestGitHubReadExecutor_SearchIssues_MissingQuery(t *testing.T) {
	e := NewGitHubReadExecutor(&mockGitHubClient{})
	result, err := e.Execute(context.Background(), agentic.ToolCall{
		ID:        "c11",
		Name:      "github_search_issues",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.Error == "" {
		t.Error("expected ToolResult.Error for missing query")
	}
}

func TestGitHubReadExecutor_SearchIssues_ClientError(t *testing.T) {
	mock := &mockGitHubClient{
		searchIssuesFunc: func(_ context.Context, _ string) ([]GitHubIssue, error) {
			return nil, errors.New("search unavailable")
		},
	}
	e := NewGitHubReadExecutor(mock)
	result, err := e.Execute(context.Background(), agentic.ToolCall{
		ID:        "c12",
		Name:      "github_search_issues",
		Arguments: map[string]any{"query": "foo"},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.Error == "" {
		t.Error("expected ToolResult.Error for client failure")
	}
}

// --- github_get_pr ---

func TestGitHubReadExecutor_GetPR_Success(t *testing.T) {
	pr := samplePR()
	mock := &mockGitHubClient{
		getPRFunc: func(_ context.Context, owner, repo string, number int) (*GitHubPR, error) {
			if owner != "acme" || repo != "ops" || number != 10 {
				t.Errorf("unexpected args: %s/%s#%d", owner, repo, number)
			}
			return pr, nil
		},
	}

	e := NewGitHubReadExecutor(mock)
	result, err := e.Execute(context.Background(), agentic.ToolCall{
		ID:   "c13",
		Name: "github_get_pr",
		Arguments: map[string]any{
			"owner":  "acme",
			"repo":   "ops",
			"number": float64(10),
		},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected ToolResult.Error: %s", result.Error)
	}

	var parsed GitHubPR
	if err := json.Unmarshal([]byte(result.Content), &parsed); err != nil {
		t.Fatalf("content is not valid JSON: %v", err)
	}
	if parsed.Number != 10 {
		t.Errorf("expected number 10, got %d", parsed.Number)
	}
	if parsed.Head != "feat/feature-branch" {
		t.Errorf("expected head 'feat/feature-branch', got %q", parsed.Head)
	}
}

func TestGitHubReadExecutor_GetPR_MissingNumber(t *testing.T) {
	e := NewGitHubReadExecutor(&mockGitHubClient{})
	result, err := e.Execute(context.Background(), agentic.ToolCall{
		ID:   "c14",
		Name: "github_get_pr",
		Arguments: map[string]any{
			"owner": "acme",
			"repo":  "ops",
		},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.Error == "" {
		t.Error("expected ToolResult.Error for missing number")
	}
}

func TestGitHubReadExecutor_GetPR_ClientError(t *testing.T) {
	mock := &mockGitHubClient{
		getPRFunc: func(_ context.Context, _, _ string, _ int) (*GitHubPR, error) {
			return nil, errors.New("PR not found")
		},
	}
	e := NewGitHubReadExecutor(mock)
	result, err := e.Execute(context.Background(), agentic.ToolCall{
		ID:   "c15",
		Name: "github_get_pr",
		Arguments: map[string]any{
			"owner":  "acme",
			"repo":   "ops",
			"number": float64(99),
		},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.Error == "" {
		t.Error("expected ToolResult.Error for client failure")
	}
}

// --- github_get_file ---

func TestGitHubReadExecutor_GetFile_Success(t *testing.T) {
	fileContent := "package main\n\nfunc main() {}\n"
	mock := &mockGitHubClient{
		getFileFunc: func(_ context.Context, owner, repo, path, ref string) (string, error) {
			if owner != "acme" || repo != "ops" {
				t.Errorf("unexpected repo: %s/%s", owner, repo)
			}
			if path != "cmd/main.go" {
				t.Errorf("unexpected path: %s", path)
			}
			if ref != "feat/branch" {
				t.Errorf("unexpected ref: %s", ref)
			}
			return fileContent, nil
		},
	}

	e := NewGitHubReadExecutor(mock)
	result, err := e.Execute(context.Background(), agentic.ToolCall{
		ID:   "c16",
		Name: "github_get_file",
		Arguments: map[string]any{
			"owner": "acme",
			"repo":  "ops",
			"path":  "cmd/main.go",
			"ref":   "feat/branch",
		},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected ToolResult.Error: %s", result.Error)
	}
	if result.Content != fileContent {
		t.Errorf("unexpected content: %q", result.Content)
	}
	if result.Metadata["path"] != "cmd/main.go" {
		t.Errorf("unexpected path in metadata: %v", result.Metadata["path"])
	}
	if result.Metadata["ref"] != "feat/branch" {
		t.Errorf("unexpected ref in metadata: %v", result.Metadata["ref"])
	}
}

func TestGitHubReadExecutor_GetFile_DefaultRef(t *testing.T) {
	mock := &mockGitHubClient{
		getFileFunc: func(_ context.Context, _, _, _, ref string) (string, error) {
			if ref != "main" {
				t.Errorf("expected default ref 'main', got %q", ref)
			}
			return "content", nil
		},
	}

	e := NewGitHubReadExecutor(mock)
	result, err := e.Execute(context.Background(), agentic.ToolCall{
		ID:   "c17",
		Name: "github_get_file",
		Arguments: map[string]any{
			"owner": "acme",
			"repo":  "ops",
			"path":  "README.md",
		},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected ToolResult.Error: %s", result.Error)
	}
}

func TestGitHubReadExecutor_GetFile_MissingPath(t *testing.T) {
	e := NewGitHubReadExecutor(&mockGitHubClient{})
	result, err := e.Execute(context.Background(), agentic.ToolCall{
		ID:   "c18",
		Name: "github_get_file",
		Arguments: map[string]any{
			"owner": "acme",
			"repo":  "ops",
		},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.Error == "" {
		t.Error("expected ToolResult.Error for missing path")
	}
}

func TestGitHubReadExecutor_GetFile_ClientError(t *testing.T) {
	mock := &mockGitHubClient{
		getFileFunc: func(_ context.Context, _, _, _, _ string) (string, error) {
			return "", errors.New("file not found")
		},
	}
	e := NewGitHubReadExecutor(mock)
	result, err := e.Execute(context.Background(), agentic.ToolCall{
		ID:   "c19",
		Name: "github_get_file",
		Arguments: map[string]any{
			"owner": "acme",
			"repo":  "ops",
			"path":  "missing.go",
		},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.Error == "" {
		t.Error("expected ToolResult.Error for client failure")
	}
}
