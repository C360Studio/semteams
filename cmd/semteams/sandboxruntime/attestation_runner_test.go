package sandboxruntime

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/processor/agentic-tools/runner"
	"github.com/c360studio/semstreams/types"

	"github.com/c360studio/semteams/cmd/semteams/sandboxmanager"
)

// fakeRunner records every Exec call so tests can assert what the
// AttestationRunner forwarded. Returns a stable result.
type fakeRunner struct {
	calls []fakeRunnerCall
	res   *runner.ExecResult
	err   error
}

type fakeRunnerCall struct {
	TaskID    string
	Command   string
	TimeoutMs int
}

func (f *fakeRunner) Exec(_ context.Context, taskID, command string, timeoutMs int) (*runner.ExecResult, error) {
	f.calls = append(f.calls, fakeRunnerCall{TaskID: taskID, Command: command, TimeoutMs: timeoutMs})
	if f.err != nil {
		return nil, f.err
	}
	if f.res != nil {
		return f.res, nil
	}
	return &runner.ExecResult{Stdout: "ok", ExitCode: 0}, nil
}

// fakeEntityReader scripts ReadEntity returns. nil triples + nil err =
// "entity found but empty"; non-nil err = "entity not found / network
// failure." Records calls so tests can assert short-circuit behavior
// (taskID-empty must NOT trigger a read).
type fakeEntityReader struct {
	triples map[string]any
	err     error
	calls   []string
}

func (f *fakeEntityReader) ReadEntity(_ context.Context, entityID string) (map[string]any, error) {
	f.calls = append(f.calls, entityID)
	if f.err != nil {
		return nil, f.err
	}
	return f.triples, nil
}

func testPlatform() types.PlatformMeta {
	return types.PlatformMeta{Org: "c360", Platform: "semteams-test"}
}

func newRunner(t *testing.T, triples map[string]any, readErr error) (*AttestationRunner, *fakeRunner, *fakeEntityReader) {
	t.Helper()
	defaultRunner := &fakeRunner{}
	entities := &fakeEntityReader{triples: triples, err: readErr}
	ar := NewAttestationRunner(defaultRunner, entities, testPlatform(), nil)
	return ar, defaultRunner, entities
}

// attestationReady builds a triples map that mimics a successful
// devcontainer up — ready=true with a populated host workspace folder.
func attestationReady(wsf string) map[string]any {
	return map[string]any{
		sandboxmanager.PredicateAttestationReady:               true,
		sandboxmanager.PredicateAttestationHostWorkspaceFolder: wsf,
	}
}

func TestAttestationRunner_PassthroughWhenNoChainEntity(t *testing.T) {
	// Single-loop chains (the bare dispatch coordinator) don't have a
	// chain entity in the graph. The reader returns nil-with-err; the
	// runner falls back to passthrough.
	ar, defRunner, _ := newRunner(t, nil, errors.New("entity not found"))

	_, err := ar.Exec(context.Background(), "loop_abc", "echo hello", 5000)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if len(defRunner.calls) != 1 {
		t.Fatalf("default runner expected 1 call, got %d", len(defRunner.calls))
	}
	if defRunner.calls[0].Command != "echo hello" {
		t.Fatalf("command was wrapped despite no attestation: %q", defRunner.calls[0].Command)
	}
	if defRunner.calls[0].TaskID != "loop_abc" {
		t.Fatalf("task_id changed: %q", defRunner.calls[0].TaskID)
	}
	if defRunner.calls[0].TimeoutMs != 5000 {
		t.Fatalf("timeout dropped: %d", defRunner.calls[0].TimeoutMs)
	}
}

func TestAttestationRunner_PassthroughWhenAttestationNotReady(t *testing.T) {
	// Chain entity exists but attestation is degraded or terminal.
	// Don't route — there's no guarantee the per-tenant container can
	// honor the work's capabilities.
	triples := map[string]any{
		sandboxmanager.PredicateAttestationReady:               false,
		sandboxmanager.PredicateAttestationHostWorkspaceFolder: "/var/lib/semteams-tenants/abc/workspace",
	}
	ar, defRunner, _ := newRunner(t, triples, nil)

	_, err := ar.Exec(context.Background(), "chain123", "go test ./...", 60000)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if defRunner.calls[0].Command != "go test ./..." {
		t.Fatalf("command wrapped despite ready=false: %q", defRunner.calls[0].Command)
	}
}

func TestAttestationRunner_PassthroughWhenWorkspaceFolderEmpty(t *testing.T) {
	// Admission-failure paths land here per Phase A's omit-when-empty.
	// Even if some future bug stamps ready=true without the wsf,
	// passthrough is the safe behavior (wrapping with empty wsf would
	// reproduce the smoke-#13 "Dev container config not found" class).
	triples := map[string]any{
		sandboxmanager.PredicateAttestationReady: true,
		// host_workspace_folder absent
	}
	ar, defRunner, _ := newRunner(t, triples, nil)

	_, err := ar.Exec(context.Background(), "chain123", "echo hello", 1000)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if defRunner.calls[0].Command != "echo hello" {
		t.Fatalf("command wrapped despite empty wsf: %q", defRunner.calls[0].Command)
	}
}

func TestAttestationRunner_PassthroughWhenTaskIDEmpty(t *testing.T) {
	// Defensive: bash calls outside any chain context (no loop_id at
	// all, no chainbash rewrite). Without a task_id we have no chain
	// entity ID to construct. The runner MUST short-circuit before
	// the entity read (no entity ID is constructible; the call would
	// be wasted at best, misleading at worst).
	ar, defRunner, entities := newRunner(t, attestationReady("/wsf"), nil)

	_, err := ar.Exec(context.Background(), "", "echo hello", 1000)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if defRunner.calls[0].Command != "echo hello" {
		t.Fatalf("command wrapped despite empty task_id: %q", defRunner.calls[0].Command)
	}
	if len(entities.calls) != 0 {
		t.Fatalf("ReadEntity fired despite empty task_id (%d calls); short-circuit broken", len(entities.calls))
	}
}

func TestAttestationRunner_PassthroughOnNilTriples(t *testing.T) {
	// Distinct case from "read error": ReadEntity returns nil with no
	// error (entity exists but has zero triples). Passthrough is the
	// only sane behavior — there's nothing to route on. Covers the
	// explicit `if triples == nil { return "" }` branch.
	ar, defRunner, _ := newRunner(t, nil, nil)

	_, err := ar.Exec(context.Background(), "chain123", "echo hello", 1000)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if defRunner.calls[0].Command != "echo hello" {
		t.Fatalf("command wrapped despite nil triples: %q", defRunner.calls[0].Command)
	}
}

func TestAttestationRunner_PassthroughOnWrongTypeReady(t *testing.T) {
	// Defensive: a future producer drift could stamp ready as a
	// string "true" instead of native bool. The bool cast fails-
	// closed (returns zero value), so wrong-type → false → passthrough.
	// Locking this behavior prevents a "helpful" future refactor from
	// changing it to ParseBool — which would silently activate routing
	// for entities whose attestation state we don't actually
	// understand.
	triples := map[string]any{
		sandboxmanager.PredicateAttestationReady:               "true", // wrong type, on purpose
		sandboxmanager.PredicateAttestationHostWorkspaceFolder: "/wsf",
	}
	ar, defRunner, _ := newRunner(t, triples, nil)

	_, err := ar.Exec(context.Background(), "chain123", "echo hello", 1000)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if defRunner.calls[0].Command != "echo hello" {
		t.Fatalf("command wrapped despite wrong-type ready: %q", defRunner.calls[0].Command)
	}
}

func TestAttestationRunner_PassthroughOnCancelledContextNoLog(t *testing.T) {
	// When ctx is cancelled mid-call, the entity read fails — but the
	// caller will surface the same ctx error from the default runner.
	// Logging the read failure here would just duplicate noise.
	// Verify (a) passthrough still fires and (b) no warning is emitted
	// to a captured logger.
	defRunner := &fakeRunner{}
	entities := &fakeEntityReader{err: context.Canceled}
	var buf strings.Builder
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ar := NewAttestationRunner(defRunner, entities, testPlatform(), logger)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _ = ar.Exec(ctx, "chain123", "echo hello", 1000)

	if defRunner.calls[0].Command != "echo hello" {
		t.Fatalf("command wrapped despite cancelled ctx: %q", defRunner.calls[0].Command)
	}
	if strings.Contains(buf.String(), "chain entity read failed") {
		t.Fatalf("logged a read-failure warning on a cancelled ctx (duplicates the default-runner's ctx.Err surface): %q", buf.String())
	}
}

func TestAttestationRunner_PassthroughOnReadError_LogsWarn(t *testing.T) {
	// Distinguishes from the cancelled-ctx case: a NATS-down /
	// malformed-entity-ID error IS signal. Operators investigating
	// "why isn't my per-tenant sandbox being used?" should see this
	// at default log level (Warn), not have to crank to Debug.
	defRunner := &fakeRunner{}
	entities := &fakeEntityReader{err: errors.New("nats: jetstream not ready")}
	var buf strings.Builder
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	ar := NewAttestationRunner(defRunner, entities, testPlatform(), logger)

	_, _ = ar.Exec(context.Background(), "chain123", "echo hello", 1000)

	if !strings.Contains(buf.String(), "chain entity read failed") {
		t.Fatalf("expected Warn log on genuine read error; got: %q", buf.String())
	}
	if !strings.Contains(buf.String(), "nats: jetstream not ready") {
		t.Fatalf("log missing the underlying error detail: %q", buf.String())
	}
}

func TestAttestationRunner_WrapsCommandWhenReadyAndWsfPresent(t *testing.T) {
	// The happy path: attestation is ready with a non-empty wsf. The
	// command gets wrapped in `devcontainer exec --workspace-folder
	// '<wsf>' bash -lc '<cmd>'` and forwarded with the same task_id
	// and timeout.
	const wsf = "/var/lib/semteams-tenants/abc/workspace"
	ar, defRunner, _ := newRunner(t, attestationReady(wsf), nil)

	_, err := ar.Exec(context.Background(), "chain123", "go version", 30000)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if len(defRunner.calls) != 1 {
		t.Fatalf("default runner expected 1 call, got %d", len(defRunner.calls))
	}
	got := defRunner.calls[0]
	if got.TaskID != "chain123" {
		t.Fatalf("task_id rewritten: %q", got.TaskID)
	}
	if got.TimeoutMs != 30000 {
		t.Fatalf("timeout dropped: %d", got.TimeoutMs)
	}
	// Lock the exact wrapping shape. Drift here breaks the routing
	// silently — the wrapped command might still "run" (in the always-
	// warm sandbox) but not inside the per-tenant container.
	want := "'devcontainer' 'exec' '--workspace-folder' '" + wsf + "' 'bash' '-lc' 'go version'"
	if got.Command != want {
		t.Fatalf("wrapped command shape drift:\n got %q\nwant %q", got.Command, want)
	}
}

func TestAttestationRunner_QuotesCommandSafelyOnInjectionAttempt(t *testing.T) {
	// LLM-generated commands may contain single quotes, dollars,
	// backticks, semicolons, &&, ||, redirection. The wrapping must
	// shell-escape so an attacker (or a confused LLM) can't break out
	// of the inner `bash -lc 'cmd'` quoting to run unintended
	// commands. The escape pattern is the canonical
	// 'closing-quote backslash-quote opening-quote' rule.
	const wsf = "/wsf"
	ar, defRunner, _ := newRunner(t, attestationReady(wsf), nil)

	cases := []struct {
		name string
		cmd  string
	}{
		{"single-quote", `echo "it's fine"`},
		{"dollar", `echo $HOME`},
		{"backtick", "echo `whoami`"},
		{"semicolon", "echo a; rm -rf /"},
		{"and", "echo a && rm -rf /"},
		{"redirect", "cat /etc/passwd > /tmp/leak"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defRunner.calls = nil
			if _, err := ar.Exec(context.Background(), "chain1", tc.cmd, 1000); err != nil {
				t.Fatalf("Exec: %v", err)
			}
			got := defRunner.calls[0].Command
			// The wrapped command is `... bash -lc '<escaped cmd>'`.
			// Verify the original command's body is single-quote
			// enclosed at the end of the line (the inner -lc arg).
			//
			// Specifically: the LAST single-quoted token in the
			// wrapped string must, when single-quote-decoded,
			// equal tc.cmd. Anything else means injection.
			if !strings.HasSuffix(got, "'"+shellQuoteRoundtripable(tc.cmd)+"'") {
				t.Fatalf("command not safely enclosed:\n cmd=%q\nwrap=%q", tc.cmd, got)
			}
		})
	}
}

// shellQuoteRoundtripable returns the inner-content of the
// single-quote-escaped form (without the surrounding single quotes)
// so tests can assert the wrapping suffix without re-implementing the
// escape rule.
func shellQuoteRoundtripable(s string) string {
	return strings.ReplaceAll(s, "'", `'\''`)
}

func TestAttestationRunner_PropagatesRunnerError(t *testing.T) {
	// Errors from the underlying runner propagate unchanged. We don't
	// retry, don't wrap, don't reclassify — the caller (upstream
	// BashExecutor) is responsible for whatever error-shape it
	// presents to the LLM.
	defRunner := &fakeRunner{err: errors.New("network: i/o timeout")}
	entities := &fakeEntityReader{triples: attestationReady("/wsf")}
	ar := NewAttestationRunner(defRunner, entities, testPlatform(), nil)

	_, err := ar.Exec(context.Background(), "chain1", "echo hi", 1000)
	if err == nil {
		t.Fatalf("expected runner error to propagate")
	}
	if !strings.Contains(err.Error(), "network: i/o timeout") {
		t.Fatalf("error not propagated unchanged: %v", err)
	}
}

func TestAttestationRunner_PropagatesRunnerExecResult(t *testing.T) {
	// The wrapper is transparent on results: stdout/stderr/exit code
	// from the default runner flow back to the caller. Even when
	// wrapping is in effect, the wrapped command's exit code is the
	// devcontainer-exec'd command's exit code (devcontainer-cli
	// propagates the inner shell's exit).
	defRunner := &fakeRunner{res: &runner.ExecResult{
		Stdout: "go version go1.25.9", Stderr: "", ExitCode: 0,
	}}
	entities := &fakeEntityReader{triples: attestationReady("/wsf")}
	ar := NewAttestationRunner(defRunner, entities, testPlatform(), nil)

	res, err := ar.Exec(context.Background(), "chain1", "go version", 1000)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res == nil || res.Stdout != "go version go1.25.9" {
		t.Fatalf("result not propagated unchanged: %+v", res)
	}
}

func TestAttestationRunner_NewPanicsOnMissingDeps(t *testing.T) {
	// Boot-required wiring: misconfig should crash at startup, not
	// produce confusing nil-pointer surprises mid-call.
	cases := []struct {
		name    string
		factory func() *AttestationRunner
	}{
		{"nil-runner", func() *AttestationRunner {
			return NewAttestationRunner(nil, &fakeEntityReader{}, testPlatform(), nil)
		}},
		{"nil-entities", func() *AttestationRunner {
			return NewAttestationRunner(&fakeRunner{}, nil, testPlatform(), nil)
		}},
		{"empty-org", func() *AttestationRunner {
			return NewAttestationRunner(&fakeRunner{}, &fakeEntityReader{}, types.PlatformMeta{Org: "", Platform: "p"}, nil)
		}},
		{"empty-platform", func() *AttestationRunner {
			return NewAttestationRunner(&fakeRunner{}, &fakeEntityReader{}, types.PlatformMeta{Org: "o", Platform: ""}, nil)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Fatalf("expected panic for %s", tc.name)
				}
			}()
			_ = tc.factory()
		})
	}
}

func TestShellQuote_EscapesSingleQuote(t *testing.T) {
	// Pinned because changing the escape rule changes the security
	// boundary between LLM-generated content and the inner shell.
	cases := []struct {
		in, want string
	}{
		{"hello", "'hello'"},
		{"", "''"},
		{"it's", `'it'\''s'`},
		{`a'b'c`, `'a'\''b'\''c'`},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := shellQuote(tc.in); got != tc.want {
				t.Fatalf("shellQuote(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestWrapDevcontainerExec_Shape(t *testing.T) {
	// Lock the canonical wrapped-command shape. Phase D wires this
	// into upstream's BashExecutor; if the shape drifts (e.g., a
	// future refactor swaps to `--` separator), downstream consumers
	// expecting the current shape would break silently.
	got := wrapDevcontainerExec("/var/lib/x", "go version")
	want := "'devcontainer' 'exec' '--workspace-folder' '/var/lib/x' 'bash' '-lc' 'go version'"
	if got != want {
		t.Fatalf("shape drift:\n got %q\nwant %q", got, want)
	}
}
