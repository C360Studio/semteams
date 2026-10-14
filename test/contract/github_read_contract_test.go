package contract

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
	agentictools "github.com/c360studio/semstreams/processor/agentic-tools"

	"github.com/c360studio/semteams/cmd/semteams/portfolio"
	"github.com/c360studio/semteams/cmd/semteams/tools/githubread"
)

type contractStubGitHubClient struct{}

func (contractStubGitHubClient) ListIssues(_ context.Context, owner, repo string, _ githubread.ListOpts) ([]githubread.Issue, error) {
	fullRepo := owner + "/" + repo
	ts := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	return []githubread.Issue{
		{
			Number:    273,
			Title:     "program-report: GitHub read adapter as a product-local tool",
			Body:      "Deliver the read-only GitHub adapter for Program Pulse.",
			State:     "open",
			HTMLURL:   "https://github.com/" + fullRepo + "/issues/273",
			CreatedAt: ts,
			UpdatedAt: ts,
		},
	}, nil
}

func (contractStubGitHubClient) ListPullRequests(_ context.Context, owner, repo string, _ githubread.ListOpts) ([]githubread.PullRequest, error) {
	fullRepo := owner + "/" + repo
	ts := time.Date(2026, 10, 14, 12, 5, 0, 0, time.UTC)
	return []githubread.PullRequest{
		{
			Number:    327,
			Title:     "feat(tools): add github_read product-local adapter",
			State:     "open",
			Draft:     true,
			HeadSHA:   "f1a6530bcdef1234",
			HTMLURL:   "https://github.com/" + fullRepo + "/pull/327",
			CreatedAt: ts,
			UpdatedAt: ts,
		},
	}, nil
}

func (contractStubGitHubClient) ListReleases(_ context.Context, owner, repo string, _ githubread.ListOpts) ([]githubread.Release, error) {
	fullRepo := owner + "/" + repo
	ts := time.Date(2026, 10, 14, 12, 6, 0, 0, time.UTC)
	return []githubread.Release{
		{
			ID:          501,
			TagName:     "v0.4.0-alpha.1",
			Name:        "Program Pulse Alpha",
			HTMLURL:     "https://github.com/" + fullRepo + "/releases/tag/v0.4.0-alpha.1",
			CreatedAt:   ts,
			PublishedAt: ts,
		},
	}, nil
}

func (contractStubGitHubClient) ListCommits(_ context.Context, owner, repo string, _ githubread.ListOpts) ([]githubread.Commit, error) {
	fullRepo := owner + "/" + repo
	ts := time.Date(2026, 10, 14, 12, 8, 0, 0, time.UTC)
	return []githubread.Commit{
		{
			SHA:         "f1a6530bcdef1234567890abcdef1234567890ab",
			Message:     "docs(openspec): propose github-read-adapter change for #273",
			Author:      "coby",
			HTMLURL:     "https://github.com/" + fullRepo + "/commit/f1a6530bcdef1234567890abcdef1234567890ab",
			CommittedAt: ts,
		},
	}, nil
}

func (contractStubGitHubClient) ListWorkflowRuns(_ context.Context, owner, repo string, _ githubread.ListOpts) ([]githubread.WorkflowRun, error) {
	fullRepo := owner + "/" + repo
	ts := time.Date(2026, 10, 14, 12, 10, 0, 0, time.UTC)
	return []githubread.WorkflowRun{
		{
			ID:         9001,
			Name:       "Repository CI",
			HeadBranch: "gemini/github-read-adapter",
			HeadSHA:    "f1a6530",
			Status:     "completed",
			Conclusion: "success",
			HTMLURL:    "https://github.com/" + fullRepo + "/actions/runs/9001",
			CreatedAt:  ts,
			UpdatedAt:  ts,
		},
	}, nil
}

func TestGitHubReadToolDefinitionAndCategoryContract(t *testing.T) {
	t.Parallel()

	var _ agentictools.ToolExecutor = (*githubread.Executor)(nil)

	samplePath := filepath.Join("..", "..", "configs", "portfolio", "portfolio.example.json")
	cfg, err := portfolio.LoadFile(samplePath)
	if err != nil {
		t.Fatalf("load sample portfolio: %v", err)
	}

	exec := githubread.NewExecutor(githubread.Config{
		Portfolio: cfg,
		Client:    contractStubGitHubClient{},
		Cursors:   githubread.NewMemoryCursorStore(),
	}, nil)

	defs := exec.ListTools()
	if len(defs) != 1 {
		t.Fatalf("ListTools length = %d, want 1", len(defs))
	}
	def := defs[0]
	if def.Name != githubread.ToolName {
		t.Errorf("tool name = %q, want %q", def.Name, githubread.ToolName)
	}
	if def.Name != "github_read" {
		t.Errorf("tool name string = %q, want %q", def.Name, "github_read")
	}

	props, ok := def.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema missing properties: %#v", def.Parameters)
	}
	for _, key := range []string{"program", "project", "repository", "kinds", "state", "limit", "include_unchanged"} {
		if _, exists := props[key]; !exists {
			t.Errorf("schema missing property %q", key)
		}
	}
	for _, forbiddenMutation := range []string{"create", "update", "close", "merge", "comment", "label", "branch"} {
		if _, exists := props[forbiddenMutation]; exists {
			t.Errorf("read-only github_read schema must not expose mutation field %q", forbiddenMutation)
		}
	}

	registry := agentictools.NewExecutorRegistry()
	if err := registry.RegisterTool(githubread.ToolName, exec); err != nil {
		t.Fatalf("RegisterTool(github_read): %v", err)
	}
	if got := agentictools.GetToolCategory(githubread.ToolName); got != githubread.ToolCategory {
		t.Errorf("GetToolCategory(github_read) = %q, want %q", got, githubread.ToolCategory)
	}
}

func TestGitHubReadResultsValidateAsEvidenceRecordsContract(t *testing.T) {
	t.Parallel()

	samplePath := filepath.Join("..", "..", "configs", "portfolio", "portfolio.example.json")
	cfg, err := portfolio.LoadFile(samplePath)
	if err != nil {
		t.Fatalf("load sample portfolio: %v", err)
	}

	exec := githubread.NewExecutor(githubread.Config{
		Portfolio: cfg,
		Client:    contractStubGitHubClient{},
		Cursors:   githubread.NewMemoryCursorStore(),
	}, nil)

	res, err := exec.Execute(context.Background(), agentic.ToolCall{
		ID:   "call-contract-evidence",
		Name: githubread.ToolName,
		Arguments: map[string]any{
			"repository": "c360studio/semteams",
			"kinds":      []any{"issues", "pull_requests", "releases", "commits", "ci_status"},
		},
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("unexpected ToolResult error (%s): %s", res.ErrorKind, res.Error)
	}

	var parsed githubread.Result
	if err := json.Unmarshal([]byte(res.Content), &parsed); err != nil {
		t.Fatalf("unmarshal github_read Result: %v", err)
	}
	if len(parsed.Records) != 5 {
		t.Fatalf("len(parsed.Records) = %d, want 5", len(parsed.Records))
	}
	for i, rec := range parsed.Records {
		if err := rec.ValidateToolResult(); err != nil {
			t.Errorf("record[%d] (%s) failed ValidateToolResult: %v", i, rec.Kind, err)
		}
	}
}

func TestGitHubReadSecondRunUnchangedRepositoriesYieldsNoNewActivityContract(t *testing.T) {
	t.Parallel()

	samplePath := filepath.Join("..", "..", "configs", "portfolio", "portfolio.example.json")
	cfg, err := portfolio.LoadFile(samplePath)
	if err != nil {
		t.Fatalf("load sample portfolio: %v", err)
	}

	cursorStore, err := githubread.NewFileCursorStore(filepath.Join(t.TempDir(), "cursors.json"))
	if err != nil {
		t.Fatalf("NewFileCursorStore: %v", err)
	}
	exec := githubread.NewExecutor(githubread.Config{
		Portfolio: cfg,
		Client:    contractStubGitHubClient{},
		Cursors:   cursorStore,
	}, nil)

	call := agentic.ToolCall{
		ID:   "call-run-1",
		Name: githubread.ToolName,
		Arguments: map[string]any{
			"repository": "c360studio/semteams",
			"kinds":      []any{"issues", "pull_requests", "releases", "commits", "ci_status"},
		},
	}

	first, err := exec.Execute(context.Background(), call)
	if err != nil || first.Error != "" {
		t.Fatalf("first run failed: err=%v toolErr=%s", err, first.Error)
	}
	var firstResult githubread.Result
	if err := json.Unmarshal([]byte(first.Content), &firstResult); err != nil {
		t.Fatalf("first unmarshal failed: %v", err)
	}
	if firstResult.NewCount != 5 || len(firstResult.Records) != 5 {
		t.Fatalf("first run NewCount=%d len(Records)=%d, want 5", firstResult.NewCount, len(firstResult.Records))
	}

	call.ID = "call-run-2"
	second, err := exec.Execute(context.Background(), call)
	if err != nil || second.Error != "" {
		t.Fatalf("second run failed: err=%v toolErr=%s", err, second.Error)
	}
	var secondResult githubread.Result
	if err := json.Unmarshal([]byte(second.Content), &secondResult); err != nil {
		t.Fatalf("second unmarshal failed: %v", err)
	}
	if secondResult.NewCount != 0 || len(secondResult.Records) != 0 {
		t.Fatalf("second run over unchanged repositories returned NewCount=%d len(Records)=%d, want 0", secondResult.NewCount, len(secondResult.Records))
	}
	if secondResult.UnchangedCount != 5 {
		t.Fatalf("second run UnchangedCount = %d, want 5", secondResult.UnchangedCount)
	}
}

func TestGitHubReadReadmeAndADR060AddendumContract(t *testing.T) {
	t.Parallel()

	readmeBytes, err := os.ReadFile(filepath.Join("..", "..", "cmd", "semteams", "tools", "README.md"))
	if err != nil {
		t.Fatalf("read tools README: %v", err)
	}
	readme := string(readmeBytes)
	if !strings.Contains(readme, "`github_read`") {
		t.Error("cmd/semteams/tools/README.md must list github_read in the live product-local tools table")
	}
	if !strings.Contains(readme, "ADR-060") {
		t.Error("cmd/semteams/tools/README.md must reference ADR-060 for github_read")
	}

	adrBytes, err := os.ReadFile(filepath.Join("..", "..", "docs", "adr", "060-program-manager-direction-and-research-design.md"))
	if err != nil {
		t.Fatalf("read ADR-060: %v", err)
	}
	adr := string(adrBytes)
	if !strings.Contains(adr, "Framework-alignment review for the GitHub read adapter (#273)") {
		t.Error("docs/adr/060-program-manager-direction-and-research-design.md must contain the #273 framework-alignment review addendum")
	}
}
