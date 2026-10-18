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

	// Run 1: all 5 kinds return 1 evidence record each (initial onboarding baseline).
	parsed1 := executeAndDecode(t, exec, "call-1")
	if gotAuth, _ := seenAuth.Load().(string); gotAuth != "Bearer ghp_test_token_123" {
		t.Errorf("Authorization header = %q, want Bearer ghp_test_token_123", gotAuth)
	}
	if !parsed1.Baseline || parsed1.NoChange {
		t.Errorf("call-1 Baseline=%t NoChange=%t, want true/false", parsed1.Baseline, parsed1.NoChange)
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
	run2Now := fixedNow.Add(5 * time.Minute)
	exec2 := githubread.NewExecutor(githubread.Config{
		Portfolio: testPortfolio(t),
		Token:     "ghp_test_token_123",
		APIBase:   srv.URL,
		Cursors:   reloadedStore,
		Clock:     func() time.Time { return run2Now },
	}, nil)

	parsed2 := executeAndDecode(t, exec2, "call-2")
	if parsed2.Baseline || !parsed2.NoChange {
		t.Errorf("call-2 Baseline=%t NoChange=%t, want false/true", parsed2.Baseline, parsed2.NoChange)
	}
	if parsed2.NewCount != 0 || len(parsed2.Records) != 0 || parsed2.UnchangedCount != 5 {
		t.Fatalf("call-2 over unchanged repository got NewCount=%d len=%d UnchangedCount=%d, want 0/0/5",
			parsed2.NewCount, len(parsed2.Records), parsed2.UnchangedCount)
	}
	issueCurAfterNoOp, ok := reloadedStore.Get("C360Studio/semteams", githubread.KindIssues)
	if !ok || !issueCurAfterNoOp.BaselineEstablished {
		t.Fatalf("expected committed issues cursor with BaselineEstablished=true, got %+v (ok=%t)", issueCurAfterNoOp, ok)
	}
	if !issueCurAfterNoOp.LastCheckedAt.Equal(run2Now) {
		t.Errorf("LastCheckedAt after no-op = %v, want %v", issueCurAfterNoOp.LastCheckedAt, run2Now)
	}
	if !issueCurAfterNoOp.LastPersistedAt.Equal(fixedNow) {
		t.Errorf("LastPersistedAt after no-op = %v, want unchanged %v", issueCurAfterNoOp.LastPersistedAt, fixedNow)
	}

	// Run 3: mutate only issue #273 and verify only that 1 record is emitted.
	run3Now := fixedNow.Add(10 * time.Minute)
	exec3 := githubread.NewExecutor(githubread.Config{
		Portfolio: testPortfolio(t),
		Token:     "ghp_test_token_123",
		APIBase:   srv.URL,
		Cursors:   reloadedStore,
		Clock:     func() time.Time { return run3Now },
	}, nil)
	issueTitle.Store("Implement github_read adapter (updated)")
	parsed3 := executeAndDecode(t, exec3, "call-3")
	if parsed3.Baseline || parsed3.NoChange {
		t.Errorf("call-3 Baseline=%t NoChange=%t, want false/false", parsed3.Baseline, parsed3.NoChange)
	}
	if parsed3.NewCount != 1 || len(parsed3.Records) != 1 {
		t.Fatalf("call-3 after 1 updated issue should emit 1 record, got %d", len(parsed3.Records))
	}
	if parsed3.Records[0].Locator != "github://C360Studio/semteams/issues/273" {
		t.Errorf("call-3 record locator = %q, want github://C360Studio/semteams/issues/273", parsed3.Records[0].Locator)
	}
	issueCurAfterUpdate, ok := reloadedStore.Get("C360Studio/semteams", githubread.KindIssues)
	if !ok || !issueCurAfterUpdate.LastPersistedAt.Equal(run3Now) || !issueCurAfterUpdate.LastCheckedAt.Equal(run3Now) {
		t.Errorf("issues cursor timestamps after update = %+v, want LastPersistedAt & LastCheckedAt = %v", issueCurAfterUpdate, run3Now)
	}
}

func TestExecutorRunScopedCursorStagingAndReplaySafety(t *testing.T) {
	t.Parallel()

	var issueTitle atomic.Value
	issueTitle.Store("Initial issue state")
	var seenAuth atomic.Value
	srv := newStubGitHubServer(t, &issueTitle, &seenAuth)
	defer srv.Close()

	cursorFile := filepath.Join(t.TempDir(), "cursors.json")
	store, err := githubread.NewFileCursorStore(cursorFile)
	if err != nil {
		t.Fatalf("NewFileCursorStore: %v", err)
	}

	now := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	currentNow := now
	exec := githubread.NewExecutor(githubread.Config{
		Portfolio: testPortfolio(t),
		APIBase:   srv.URL,
		Cursors:   store,
		Clock:     func() time.Time { return currentNow },
	}, nil)

	execInLoop := func(callID, loopID string) githubread.Result {
		t.Helper()
		res, execErr := exec.Execute(context.Background(), agentic.ToolCall{
			ID:        callID,
			Name:      githubread.ToolName,
			LoopID:    loopID,
			Arguments: map[string]any{"repository": "C360Studio/semteams", "kinds": []any{"issues"}},
		})
		if execErr != nil || res.Error != "" {
			t.Fatalf("Execute(%s, %s): err=%v res.Error=%q", callID, loopID, execErr, res.Error)
		}
		var parsed githubread.Result
		if err := json.Unmarshal([]byte(res.Content), &parsed); err != nil {
			t.Fatalf("unmarshal %s: %v", callID, err)
		}
		return parsed
	}

	// 1. Run "loop-aborted" executes github_read (stages cursor) but fails before emit_research_artifact.
	abortedRes := execInLoop("call-aborted-1", "loop-aborted")
	if !abortedRes.Baseline || abortedRes.NewCount != 1 {
		t.Fatalf("loop-aborted call-1 got Baseline=%t NewCount=%d, want true/1", abortedRes.Baseline, abortedRes.NewCount)
	}
	if _, committed := store.Get("C360Studio/semteams", githubread.KindIssues); committed {
		t.Fatal("uncommitted run must not advance committed cursor store")
	}

	// 2. Intra-run retry within "loop-aborted" reads from pre-run committed baseline and returns identical delta.
	retryRes := execInLoop("call-aborted-2", "loop-aborted")
	if !retryRes.Baseline || retryRes.NewCount != 1 {
		t.Fatalf("intra-run retry got Baseline=%t NewCount=%d, want true/1 (no self-suppression before CommitRun)",
			retryRes.Baseline, retryRes.NewCount)
	}

	// 3. New run "loop-commit-1" still sees initial baseline because "loop-aborted" never committed.
	run1Res := execInLoop("call-run1", "loop-commit-1")
	if !run1Res.Baseline || run1Res.NewCount != 1 {
		t.Fatalf("loop-commit-1 got Baseline=%t NewCount=%d, want true/1", run1Res.Baseline, run1Res.NewCount)
	}
	if err := store.CommitRun("loop-commit-1", now); err != nil {
		t.Fatalf("CommitRun(loop-commit-1): %v", err)
	}
	// Idempotent second CommitRun on the same run key must succeed cleanly without error.
	if err := store.CommitRun("loop-commit-1", now); err != nil {
		t.Fatalf("idempotent second CommitRun(loop-commit-1): %v", err)
	}

	cur1, ok := store.Get("C360Studio/semteams", githubread.KindIssues)
	if !ok || !cur1.BaselineEstablished || !cur1.LastPersistedAt.Equal(now) || !cur1.LastCheckedAt.Equal(now) {
		t.Fatalf("committed cursor after loop-commit-1 = %+v (ok=%t), want BaselineEstablished=true and timestamps=%v", cur1, ok, now)
	}

	// 4. Subsequent run "loop-noop-2" with no upstream changes stages and commits LastCheckedAt without advancing LastPersistedAt or Watermark.
	currentNow = now.Add(24 * time.Hour)
	run2Res := execInLoop("call-run2", "loop-noop-2")
	if run2Res.Baseline || !run2Res.NoChange || run2Res.NewCount != 0 || run2Res.UnchangedCount != 1 {
		t.Fatalf("loop-noop-2 got Baseline=%t NoChange=%t NewCount=%d UnchangedCount=%d, want false/true/0/1",
			run2Res.Baseline, run2Res.NoChange, run2Res.NewCount, run2Res.UnchangedCount)
	}
	if err := store.CommitRun("loop-noop-2", currentNow); err != nil {
		t.Fatalf("CommitRun(loop-noop-2): %v", err)
	}
	cur2, ok := store.Get("C360Studio/semteams", githubread.KindIssues)
	if !ok {
		t.Fatal("missing cursor after loop-noop-2 commit")
	}
	if !cur2.LastCheckedAt.Equal(currentNow) {
		t.Errorf("cur2.LastCheckedAt = %v, want %v", cur2.LastCheckedAt, currentNow)
	}
	if !cur2.LastPersistedAt.Equal(now) {
		t.Errorf("cur2.LastPersistedAt = %v, want preserved %v", cur2.LastPersistedAt, now)
	}
	if cur2.Watermark != cur1.Watermark {
		t.Errorf("cur2.Watermark = %q, want preserved %q", cur2.Watermark, cur1.Watermark)
	}
}

func TestExecutorPartialRepositoryFailureResilience(t *testing.T) {
	t.Parallel()

	pf, err := portfolio.Parse([]byte(`{
		"programs": [{
			"id": "c360",
			"name": "c360",
			"projects": [
				{
					"id": "semteams",
					"name": "SemTeams",
					"repositories": [{"owner": "C360Studio", "name": "semteams"}]
				},
				{
					"id": "semengine",
					"name": "SemEngine",
					"repositories": [{"owner": "C360Studio", "name": "semengine"}]
				}
			]
		}]
	}`))
	if err != nil {
		t.Fatalf("parse multi-repo portfolio: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/C360Studio/semteams/issues":
			_, _ = w.Write([]byte(`[{
				"number": 268,
				"title": "Make Program Pulse scheduled, incremental, and replay-safe",
				"body": "Incremental cursor and cron cadence.",
				"state": "open",
				"html_url": "https://github.com/C360Studio/semteams/issues/268",
				"user": {"login": "coby"},
				"created_at": "2026-10-18T08:00:00Z",
				"updated_at": "2026-10-18T09:00:00Z"
			}]`))
		case "/repos/C360Studio/semengine/issues":
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"message":"upstream GitHub 502 for semengine"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	store := githubread.NewMemoryCursorStore()
	exec := githubread.NewExecutor(githubread.Config{
		Portfolio: pf,
		APIBase:   srv.URL,
		Cursors:   store,
	}, nil)

	res, err := exec.Execute(context.Background(), agentic.ToolCall{
		ID:     "call-partial",
		Name:   githubread.ToolName,
		LoopID: "loop-partial-1",
		Arguments: map[string]any{
			"program": "c360",
			"kinds":   []any{"issues"},
		},
	})
	if err != nil {
		t.Fatalf("Execute(partial): unexpected Go error: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Execute(partial) should succeed with partial records, got tool error: %s", res.Error)
	}

	var parsed githubread.Result
	if err := json.Unmarshal([]byte(res.Content), &parsed); err != nil {
		t.Fatalf("unmarshal partial result: %v", err)
	}
	if parsed.NewCount != 1 || len(parsed.Records) != 1 {
		t.Fatalf("partial result NewCount=%d len(Records)=%d, want 1 healthy record", parsed.NewCount, len(parsed.Records))
	}
	if parsed.NoChange {
		t.Error("partial result with failure and new record must not report NoChange=true")
	}
	if len(parsed.Failures) != 1 {
		t.Fatalf("partial result len(Failures)=%d, want 1", len(parsed.Failures))
	}
	if parsed.Failures[0].Repository != "C360Studio/semengine" || parsed.Failures[0].Kind != githubread.KindIssues {
		t.Errorf("Failures[0] = %+v, want C360Studio/semengine issues", parsed.Failures[0])
	}

	if err := store.CommitRun("loop-partial-1", time.Date(2026, 10, 18, 9, 5, 0, 0, time.UTC)); err != nil {
		t.Fatalf("CommitRun(loop-partial-1): %v", err)
	}
	if _, ok := store.Get("C360Studio/semteams", githubread.KindIssues); !ok {
		t.Error("healthy repository C360Studio/semteams cursor was not committed")
	}
	if _, ok := store.Get("C360Studio/semengine", githubread.KindIssues); ok {
		t.Error("failed repository C360Studio/semengine cursor must not advance on partial failure")
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
