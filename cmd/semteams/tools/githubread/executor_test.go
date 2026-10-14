package githubread_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
	agentictools "github.com/c360studio/semstreams/processor/agentic-tools"

	"github.com/c360studio/semteams/cmd/semteams/portfolio"
	"github.com/c360studio/semteams/cmd/semteams/research"
	"github.com/c360studio/semteams/cmd/semteams/tools/githubread"
)

func testPortfolio(t *testing.T) *portfolio.Portfolio {
	t.Helper()
	p, err := portfolio.Parse([]byte(`{
		"programs": [{
			"id": "c360",
			"name": "c360",
			"projects": [{
				"id": "semteams",
				"name": "SemTeams",
				"repositories": [
					{"owner": "C360Studio", "name": "semteams"}
				]
			}]
		}]
	}`))
	if err != nil {
		t.Fatalf("parse test portfolio: %v", err)
	}
	return p
}

func newStubGitHubServer(t *testing.T, issueTitle *atomic.Value, seenAuth *atomic.Value) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method %s", r.Method)
		}
		seenAuth.Store(r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/repos/C360Studio/semteams/issues":
			title := issueTitle.Load().(string)
			_, _ = w.Write([]byte(`[
				{
					"number": 273,
					"title": "` + title + `",
					"body": "Every result is an evidence record with channel: github.",
					"state": "open",
					"html_url": "https://github.com/C360Studio/semteams/issues/273",
					"user": {"login": "coby"},
					"labels": [{"name": "area:packs"}],
					"assignees": [{"login": "coby"}],
					"milestone": {"title": "v0.2.0"},
					"created_at": "2026-10-14T10:00:00Z",
					"updated_at": "2026-10-14T12:00:00Z"
				},
				{
					"number": 327,
					"title": "PR mixed into /issues",
					"state": "open",
					"pull_request": {"url": "https://api.github.com/repos/C360Studio/semteams/pulls/327"},
					"created_at": "2026-10-14T11:00:00Z",
					"updated_at": "2026-10-14T12:00:00Z"
				}
			]`))
		case "/repos/C360Studio/semteams/pulls":
			_, _ = w.Write([]byte(`[{
				"number": 327,
				"title": "feat(tools): add github_read",
				"body": "Closes #273",
				"state": "open",
				"draft": true,
				"html_url": "https://github.com/C360Studio/semteams/pull/327",
				"user": {"login": "coby"},
				"head": {"ref": "gemini/github-read-adapter", "sha": "abc1234"},
				"base": {"ref": "main"},
				"labels": [{"name": "type:feature"}],
				"created_at": "2026-10-14T11:00:00Z",
				"updated_at": "2026-10-14T12:05:00Z"
			}]`))
		case "/repos/C360Studio/semteams/releases":
			_, _ = w.Write([]byte(`[{
				"id": 501,
				"tag_name": "v0.1.0",
				"name": "v0.1.0 Initial",
				"body": "Baseline release notes",
				"draft": false,
				"prerelease": false,
				"html_url": "https://github.com/C360Studio/semteams/releases/tag/v0.1.0",
				"author": {"login": "coby"},
				"created_at": "2026-10-01T00:00:00Z",
				"published_at": "2026-10-01T01:00:00Z"
			}]`))
		case "/repos/C360Studio/semteams/commits":
			_, _ = w.Write([]byte(`[{
				"sha": "34d83c14abcdef",
				"html_url": "https://github.com/C360Studio/semteams/commit/34d83c14abcdef",
				"commit": {
					"message": "feat(work): hydrate GitHub Projects v2",
					"author": {"name": "Coby", "date": "2026-10-14T13:00:00Z"},
					"committer": {"date": "2026-10-14T13:01:00Z"}
				},
				"author": {"login": "coby"}
			}]`))
		case "/repos/C360Studio/semteams/actions/runs":
			_, _ = w.Write([]byte(`{
				"workflow_runs": [{
					"id": 9001,
					"name": "Repository CI",
					"head_branch": "main",
					"head_sha": "34d83c14abcdef",
					"event": "push",
					"status": "completed",
					"conclusion": "success",
					"html_url": "https://github.com/C360Studio/semteams/actions/runs/9001",
					"created_at": "2026-10-14T13:02:00Z",
					"updated_at": "2026-10-14T13:05:00Z"
				}]
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func executeAndDecode(t *testing.T, exec *githubread.Executor, callID string) githubread.Result {
	t.Helper()
	res, err := exec.Execute(context.Background(), agentic.ToolCall{
		ID:        callID,
		Name:      githubread.ToolName,
		Arguments: map[string]any{"repository": "C360Studio/semteams"},
	})
	if err != nil {
		t.Fatalf("Execute %s: %v", callID, err)
	}
	if res.Error != "" {
		t.Fatalf("Execute %s returned tool error: %s", callID, res.Error)
	}
	var parsed githubread.Result
	if err := json.Unmarshal([]byte(res.Content), &parsed); err != nil {
		t.Fatalf("unmarshal %s result: %v", callID, err)
	}
	return parsed
}

func TestExecutorHTTPClientAndCursorSuppression(t *testing.T) {
	t.Parallel()

	var issueTitle atomic.Value
	issueTitle.Store("Implement github_read adapter")

	var seenAuth atomic.Value
	srv := newStubGitHubServer(t, &issueTitle, &seenAuth)
	defer srv.Close()

	cursorFile := filepath.Join(t.TempDir(), "cursors.json")
	store, err := githubread.NewFileCursorStore(cursorFile)
	if err != nil {
		t.Fatalf("NewFileCursorStore: %v", err)
	}

	fixedNow := time.Date(2026, 10, 14, 14, 30, 0, 0, time.UTC)
	exec := githubread.NewExecutor(githubread.Config{
		Portfolio: testPortfolio(t),
		Token:     "ghp_test_token_123",
		APIBase:   srv.URL,
		Cursors:   store,
		Clock:     func() time.Time { return fixedNow },
	}, nil)

	if gotCat := agentictools.GetToolCategory(githubread.ToolName); gotCat != githubread.ToolCategory {
		t.Errorf("GetToolCategory(%q) = %q, want %q", githubread.ToolName, gotCat, githubread.ToolCategory)
	}

	// Run 1: all 5 kinds return 1 evidence record each.
	parsed1 := executeAndDecode(t, exec, "call-1")
	if gotAuth, _ := seenAuth.Load().(string); gotAuth != "Bearer ghp_test_token_123" {
		t.Errorf("Authorization header = %q, want Bearer ghp_test_token_123", gotAuth)
	}
	if parsed1.NewCount != 5 || len(parsed1.Records) != 5 {
		t.Fatalf("call-1 NewCount=%d len(Records)=%d, want 5", parsed1.NewCount, len(parsed1.Records))
	}
	for i, rec := range parsed1.Records {
		if err := rec.ValidateToolResult(); err != nil {
			t.Errorf("Records[%d] failed ValidateToolResult: %v", i, err)
		}
		if rec.Channel != research.ChannelGitHub || rec.SourceClass != research.SourceClassPrimary {
			t.Errorf("Records[%d] channel/source_class = %q/%q, want github/primary", i, rec.Channel, rec.SourceClass)
		}
	}

	// Run 2 over unchanged repository: reloads cursor store from disk and yields 0 new records.
	reloadedStore, err := githubread.NewFileCursorStore(cursorFile)
	if err != nil {
		t.Fatalf("reload FileCursorStore: %v", err)
	}
	exec2 := githubread.NewExecutor(githubread.Config{
		Portfolio: testPortfolio(t),
		Token:     "ghp_test_token_123",
		APIBase:   srv.URL,
		Cursors:   reloadedStore,
		Clock:     func() time.Time { return fixedNow.Add(5 * time.Minute) },
	}, nil)

	parsed2 := executeAndDecode(t, exec2, "call-2")
	if parsed2.NewCount != 0 || len(parsed2.Records) != 0 || parsed2.UnchangedCount != 5 {
		t.Fatalf("call-2 over unchanged repository got NewCount=%d len=%d UnchangedCount=%d, want 0/0/5",
			parsed2.NewCount, len(parsed2.Records), parsed2.UnchangedCount)
	}

	// Run 3: mutate only issue #273 and verify only that 1 record is emitted.
	issueTitle.Store("Implement github_read adapter (updated)")
	parsed3 := executeAndDecode(t, exec2, "call-3")
	if parsed3.NewCount != 1 || len(parsed3.Records) != 1 {
		t.Fatalf("call-3 after 1 updated issue should emit 1 record, got %d", len(parsed3.Records))
	}
	if parsed3.Records[0].Locator != "github://C360Studio/semteams/issues/273" {
		t.Errorf("call-3 record locator = %q, want github://C360Studio/semteams/issues/273", parsed3.Records[0].Locator)
	}
}

func TestExecutorRejectsUnconfiguredRepository(t *testing.T) {
	t.Parallel()

	exec := githubread.NewExecutor(githubread.Config{
		Portfolio: testPortfolio(t),
	}, nil)

	res, err := exec.Execute(context.Background(), agentic.ToolCall{
		ID:        "call-unconfigured",
		Name:      githubread.ToolName,
		Arguments: map[string]any{"repository": "other/unconfigured"},
	})
	if err != nil {
		t.Fatalf("Execute returned unexpected Go error: %v", err)
	}
	if res.Error == "" {
		t.Fatal("expected tool error for unconfigured repository, got empty error")
	}
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %q, want %q", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
}
