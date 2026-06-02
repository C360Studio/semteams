package addsource

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/natsclient"
)

// fakeRequester records the most recent RequestWithRetry call and
// returns a canned response. Each test sets respBytes / respErr; the
// fake never blocks, so happy-path tests don't need timeouts. Capturing
// the retry config lets us assert defaults match natsclient.DefaultRetryConfig().
type fakeRequester struct {
	gotSubject string
	gotPayload []byte
	gotTimeout time.Duration
	gotRetry   natsclient.RetryConfig
	respBytes  []byte
	respErr    error
}

func (f *fakeRequester) RequestWithRetry(_ context.Context, subject string, data []byte, timeout time.Duration, retry natsclient.RetryConfig) ([]byte, error) {
	f.gotSubject = subject
	f.gotPayload = data
	f.gotTimeout = timeout
	f.gotRetry = retry
	return f.respBytes, f.respErr
}

func okReply(t *testing.T) []byte {
	t.Helper()
	body, err := json.Marshal(AddReply{
		Components: []AddedComponent{{
			InstanceName: "git-source-osh-osh-core-main",
			FactoryName:  "git",
			SourceType:   "git",
			Created:      true,
		}},
		StatusSubject: "graph.ingest.status",
		ReadyWhen:     "source_status.phase in ['watching', 'idle']",
		Timestamp:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("marshal okReply: %v", err)
	}
	return body
}

func newExecutorWithCfg(t *testing.T, cfg Config, respBytes []byte, respErr error) (*RepoExecutor, *fakeRequester) {
	t.Helper()
	req := &fakeRequester{respBytes: respBytes, respErr: respErr}
	return NewRepoExecutor(req, cfg, nil), req
}

func defaultCfg() Config {
	return Config{
		AllowedNamespaces: []string{"research", "ops"},
		DefaultNamespace:  "research",
	}
}

func defaultCall(args map[string]any) agentic.ToolCall {
	return agentic.ToolCall{
		ID:         "call-001",
		Name:       RepoToolName,
		Arguments:  args,
		ApprovedBy: "alice@example.com",
		TraceID:    "trace-xyz",
	}
}

// =====================================================================
// ListTools — schema sanity
// =====================================================================

func TestListTools_SchemaShape(t *testing.T) {
	exec, _ := newExecutorWithCfg(t, defaultCfg(), nil, nil)
	defs := exec.ListTools()
	if len(defs) != 1 {
		t.Fatalf("ListTools length = %d, want 1", len(defs))
	}
	def := defs[0]
	if def.Name != RepoToolName {
		t.Errorf("tool name = %q, want %q", def.Name, RepoToolName)
	}
	props, ok := def.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("tool schema missing properties block: %v", def.Parameters)
	}
	for _, key := range []string{"url", "branch", "namespace"} {
		if _, ok := props[key]; !ok {
			t.Errorf("missing property %q in tool schema", key)
		}
	}
	required, _ := def.Parameters["required"].([]string)
	if len(required) != 1 || required[0] != "url" {
		t.Errorf("required = %v, want [url]", required)
	}
}

// =====================================================================
// Argument validation
// =====================================================================

func TestExecute_MissingURL(t *testing.T) {
	exec, req := newExecutorWithCfg(t, defaultCfg(), okReply(t), nil)

	res, err := exec.Execute(context.Background(), defaultCall(map[string]any{}))
	if err != nil {
		t.Fatalf("Execute returned err = %v, want nil (LLM-facing errors via Result.Error)", err)
	}
	if !strings.Contains(res.Error, "url is required") {
		t.Errorf("Result.Error = %q, want contains 'url is required'", res.Error)
	}
	if req.gotSubject != "" {
		t.Errorf("did not expect NATS publish on missing url; got subject %q", req.gotSubject)
	}
}

func TestExecute_InvalidURLScheme(t *testing.T) {
	exec, _ := newExecutorWithCfg(t, defaultCfg(), okReply(t), nil)

	res, err := exec.Execute(context.Background(), defaultCall(map[string]any{
		"url": "ftp://example.com/repo.git",
	}))
	if err != nil {
		t.Fatalf("Execute returned err: %v", err)
	}
	if !strings.Contains(res.Error, "unsupported scheme") {
		t.Errorf("Result.Error = %q, want contains 'unsupported scheme'", res.Error)
	}
}

func TestExecute_AcceptsSSHForm(t *testing.T) {
	exec, _ := newExecutorWithCfg(t, defaultCfg(), okReply(t), nil)
	res, err := exec.Execute(context.Background(), defaultCall(map[string]any{
		"url": "git@github.com:example/repo.git",
	}))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Errorf("ssh form rejected: %s", res.Error)
	}
}

func TestExecute_RejectsURLWithoutHost(t *testing.T) {
	exec, _ := newExecutorWithCfg(t, defaultCfg(), okReply(t), nil)
	res, _ := exec.Execute(context.Background(), defaultCall(map[string]any{
		"url": "https:///path",
	}))
	if !strings.Contains(res.Error, "missing host") {
		t.Errorf("Result.Error = %q, want contains 'missing host'", res.Error)
	}
}

// =====================================================================
// Namespace resolution
// =====================================================================

func TestExecute_EmptyAllowlistDisablesTool(t *testing.T) {
	exec, _ := newExecutorWithCfg(t, Config{}, okReply(t), nil)
	res, _ := exec.Execute(context.Background(), defaultCall(map[string]any{
		"url": "https://github.com/example/repo",
	}))
	if !strings.Contains(res.Error, "namespace allowlist") {
		t.Errorf("Result.Error = %q, want contains 'namespace allowlist'", res.Error)
	}
}

func TestExecute_NamespaceNotInAllowlist(t *testing.T) {
	exec, _ := newExecutorWithCfg(t, defaultCfg(), okReply(t), nil)
	res, _ := exec.Execute(context.Background(), defaultCall(map[string]any{
		"url":       "https://github.com/example/repo",
		"namespace": "production",
	}))
	if !strings.Contains(res.Error, `"production"`) || !strings.Contains(res.Error, "not in") {
		t.Errorf("Result.Error = %q, want contains denied-namespace message", res.Error)
	}
}

func TestExecute_DefaultNamespaceFallback(t *testing.T) {
	exec, req := newExecutorWithCfg(t, defaultCfg(), okReply(t), nil)
	res, err := exec.Execute(context.Background(), defaultCall(map[string]any{
		"url": "https://github.com/example/repo",
	}))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty", res.Error)
	}
	if req.gotSubject != "graph.ingest.add.research" {
		t.Errorf("subject = %q, want graph.ingest.add.research (default ns)", req.gotSubject)
	}
}

func TestExecute_NoDefaultAndNoCallerNamespaceErrors(t *testing.T) {
	cfg := defaultCfg()
	cfg.DefaultNamespace = ""
	exec, _ := newExecutorWithCfg(t, cfg, okReply(t), nil)
	res, _ := exec.Execute(context.Background(), defaultCall(map[string]any{
		"url": "https://github.com/example/repo",
	}))
	if !strings.Contains(res.Error, "namespace is required") {
		t.Errorf("Result.Error = %q, want contains 'namespace is required'", res.Error)
	}
}

func TestExecute_DefaultNamespaceMisconfigured(t *testing.T) {
	cfg := Config{
		AllowedNamespaces: []string{"research"},
		DefaultNamespace:  "ops", // not in allowlist
	}
	exec, _ := newExecutorWithCfg(t, cfg, okReply(t), nil)
	res, _ := exec.Execute(context.Background(), defaultCall(map[string]any{
		"url": "https://github.com/example/repo",
	}))
	if !strings.Contains(res.Error, "deployment misconfiguration") {
		t.Errorf("Result.Error = %q, want contains 'deployment misconfiguration'", res.Error)
	}
}

// =====================================================================
// Happy path + payload verification
// =====================================================================

func TestExecute_HappyPath_PayloadShape(t *testing.T) {
	exec, req := newExecutorWithCfg(t, defaultCfg(), okReply(t), nil)
	res, err := exec.Execute(context.Background(), defaultCall(map[string]any{
		"url":       "https://github.com/sensorhub-tools/osh-core",
		"branch":    "develop",
		"namespace": "research",
	}))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty", res.Error)
	}

	if req.gotSubject != "graph.ingest.add.research" {
		t.Errorf("subject = %q, want graph.ingest.add.research", req.gotSubject)
	}
	if req.gotTimeout != defaultRequestTimeout {
		t.Errorf("timeout = %v, want %v", req.gotTimeout, defaultRequestTimeout)
	}

	var sent AddRequest
	if err := json.Unmarshal(req.gotPayload, &sent); err != nil {
		t.Fatalf("decode sent payload: %v", err)
	}
	if sent.Source.Type != SourceTypeGit {
		t.Errorf("source.type = %q, want %q", sent.Source.Type, SourceTypeGit)
	}
	if sent.Source.URL != "https://github.com/sensorhub-tools/osh-core" {
		t.Errorf("source.url = %q", sent.Source.URL)
	}
	if sent.Source.Branch != "develop" {
		t.Errorf("source.branch = %q, want develop", sent.Source.Branch)
	}
	if !sent.Source.Watch {
		t.Errorf("source.watch should be true (continuous tracking)")
	}
}

func TestExecute_DefaultBranchAppliedWhenOmitted(t *testing.T) {
	exec, req := newExecutorWithCfg(t, defaultCfg(), okReply(t), nil)
	_, err := exec.Execute(context.Background(), defaultCall(map[string]any{
		"url": "https://github.com/example/repo",
	}))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	var sent AddRequest
	_ = json.Unmarshal(req.gotPayload, &sent)
	if sent.Source.Branch != defaultBranch {
		t.Errorf("default branch = %q, want %q", sent.Source.Branch, defaultBranch)
	}
}

func TestExecute_ProvenancePropagation(t *testing.T) {
	cfg := defaultCfg()
	cfg.Actor = "semteams.coordinator"
	exec, req := newExecutorWithCfg(t, cfg, okReply(t), nil)

	call := defaultCall(map[string]any{
		"url": "https://github.com/example/repo",
	})
	_, err := exec.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}

	var sent AddRequest
	_ = json.Unmarshal(req.gotPayload, &sent)
	if sent.Provenance.Actor != "semteams.coordinator" {
		t.Errorf("provenance.actor = %q, want semteams.coordinator", sent.Provenance.Actor)
	}
	if sent.Provenance.OnBehalfOf != "alice@example.com" {
		t.Errorf("provenance.on_behalf_of = %q, want alice@example.com (from ApprovedBy)", sent.Provenance.OnBehalfOf)
	}
	if sent.Provenance.TraceID != "trace-xyz" {
		t.Errorf("provenance.trace_id = %q", sent.Provenance.TraceID)
	}
}

func TestExecute_DefaultActorWhenConfigEmpty(t *testing.T) {
	exec, req := newExecutorWithCfg(t, defaultCfg(), okReply(t), nil)
	_, err := exec.Execute(context.Background(), defaultCall(map[string]any{
		"url": "https://github.com/example/repo",
	}))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	var sent AddRequest
	_ = json.Unmarshal(req.gotPayload, &sent)
	// Assert non-empty + semteams namespace prefix rather than the
	// exact string. The default name is implementation detail; the
	// "originator is a semteams agent" claim is the contract.
	if sent.Provenance.Actor == "" {
		t.Error("default actor empty; want semteams.* fallback")
	}
	if !strings.HasPrefix(sent.Provenance.Actor, "semteams.") {
		t.Errorf("default actor = %q, want semteams.* prefix", sent.Provenance.Actor)
	}
}

// TestExecute_NamespaceInAllowlistAccepted explicitly exercises the
// allowlisted-but-not-default path. The happy-path test transitively
// covers the default ("research"); this case uses "ops" which is in
// the allowlist but is not the default.
func TestExecute_NamespaceInAllowlistAccepted(t *testing.T) {
	exec, req := newExecutorWithCfg(t, defaultCfg(), okReply(t), nil)
	res, err := exec.Execute(context.Background(), defaultCall(map[string]any{
		"url":       "https://github.com/example/repo",
		"namespace": "ops",
	}))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty", res.Error)
	}
	if req.gotSubject != "graph.ingest.add.ops" {
		t.Errorf("subject = %q, want graph.ingest.add.ops", req.gotSubject)
	}
}

// =====================================================================
// Reply paths
// =====================================================================

func TestExecute_SyncErrorInReply(t *testing.T) {
	errReply, err := json.Marshal(AddReply{
		Error: &IngestError{
			Code:    CodeValidationFailed,
			Message: "url field empty",
		},
		Timestamp: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("marshal err reply: %v", err)
	}
	exec, _ := newExecutorWithCfg(t, defaultCfg(), errReply, nil)

	res, _ := exec.Execute(context.Background(), defaultCall(map[string]any{
		"url": "https://github.com/example/repo",
	}))
	if !strings.Contains(res.Error, string(CodeValidationFailed)) {
		t.Errorf("Result.Error = %q, want contains %q", res.Error, CodeValidationFailed)
	}
	if !strings.Contains(res.Error, "url field empty") {
		t.Errorf("Result.Error = %q, want contains the error message body", res.Error)
	}
}

func TestExecute_NATSRequestFails(t *testing.T) {
	exec, _ := newExecutorWithCfg(t, defaultCfg(), nil, errors.New("no responders"))
	res, _ := exec.Execute(context.Background(), defaultCall(map[string]any{
		"url": "https://github.com/example/repo",
	}))
	if !strings.Contains(res.Error, "no responders") {
		t.Errorf("Result.Error = %q, want contains 'no responders'", res.Error)
	}
	if !strings.Contains(res.Error, "graph.ingest.add.research") {
		t.Errorf("Result.Error = %q, want subject in error message", res.Error)
	}
}

func TestExecute_MalformedReply(t *testing.T) {
	// Valid JSON but wrong shape — exercises the post-json.Valid
	// Unmarshal failure path. The pre-Valid guard added 2026-06-02
	// (Footgun fix mirroring the sibling-site pattern) doesn't fire
	// here because `42` IS valid JSON; Unmarshal then fails at the
	// type level (cannot unmarshal number into AddReply struct).
	exec, _ := newExecutorWithCfg(t, defaultCfg(), []byte("42"), nil)
	res, _ := exec.Execute(context.Background(), defaultCall(map[string]any{
		"url": "https://github.com/example/repo",
	}))
	if !strings.Contains(res.Error, "decode AddReply") {
		t.Errorf("Result.Error = %q, want contains 'decode AddReply'", res.Error)
	}
}

// TestExecute_LegacyHandlerErrorBody_SurfacedAsToolError verifies the
// natsclient RequestWithRetry Footgun guard: when the responder returns
// a Go error, SubscribeForRequests wire-encodes it as a legacy
// `error: <msg>` text body with nil err. RequestWithRetry has no
// ClassifyReply variant upstream beta.92, so the body flows through to
// our caller. Pre-fix, json.Unmarshal would silently corrupt with
// "invalid character 'e' looking for beginning of value" — the same
// bug class chain.NATSEntityReader (bae5706) + chainpause.NATSPauseDataReader
// (10f9d29) closed via RequestClassified. This site uses a json.Valid
// pre-decode guard as the local workaround until upstream lands the
// Classified variant for RequestWithRetry.
func TestExecute_LegacyHandlerErrorBody_SurfacedAsToolError(t *testing.T) {
	// Mirrors the exact shape SubscribeForRequests emits when a
	// handler returns a non-classified Go error.
	body := []byte("error: not found: graph.ingest.add.research")
	exec, _ := newExecutorWithCfg(t, defaultCfg(), body, nil)
	res, _ := exec.Execute(context.Background(), defaultCall(map[string]any{
		"url": "https://github.com/example/repo",
	}))
	if res.Error == "" {
		t.Fatalf("expected tool error on legacy handler-error body, got empty")
	}
	if strings.Contains(res.Error, "invalid character 'e'") {
		t.Errorf("Result.Error leaked the silent-corruption decode failure (pre-fix shape): %q", res.Error)
	}
	if !strings.Contains(res.Error, "non-JSON response") {
		t.Errorf("Result.Error doesn't name the wire-shape class: %q", res.Error)
	}
	if !strings.Contains(res.Error, "graph.ingest.add.research") {
		t.Errorf("Result.Error should surface the upstream body for diagnosis: %q", res.Error)
	}
}

func TestExecute_HappyContent_Decodable(t *testing.T) {
	exec, _ := newExecutorWithCfg(t, defaultCfg(), okReply(t), nil)
	res, err := exec.Execute(context.Background(), defaultCall(map[string]any{
		"url": "https://github.com/example/repo",
	}))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	var content struct {
		Namespace     string           `json:"namespace"`
		Components    []AddedComponent `json:"components"`
		StatusSubject string           `json:"status_subject"`
		ReadyWhen     string           `json:"ready_when"`
	}
	if err := json.Unmarshal([]byte(res.Content), &content); err != nil {
		t.Fatalf("decode tool result content: %v\nraw=%s", err, res.Content)
	}
	if content.Namespace != "research" {
		t.Errorf("content.namespace = %q, want research", content.Namespace)
	}
	if len(content.Components) != 1 {
		t.Errorf("content.components length = %d, want 1", len(content.Components))
	}
	if content.StatusSubject != "graph.ingest.status" {
		t.Errorf("content.status_subject = %q", content.StatusSubject)
	}
}

// =====================================================================
// validateRepoURL — narrower coverage of the URL guard
// =====================================================================

func TestValidateRepoURL_AcceptedShapes(t *testing.T) {
	cases := []string{
		"https://github.com/example/repo",
		"https://github.com/example/repo.git",
		"http://gitea.local/example/repo",
		"git@github.com:example/repo.git",
		"git@gitlab.com:group/subgroup/repo.git",
	}
	for _, c := range cases {
		if err := validateRepoURL(c); err != nil {
			t.Errorf("validateRepoURL(%q) = %v, want nil", c, err)
		}
	}
}

func TestValidateRepoURL_RejectedShapes(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"ftp://example.com/repo", "unsupported scheme"},
		{"file:///tmp/repo", "unsupported scheme"},
		{"https:///path", "missing host"},
		{"://broken", "invalid url"},
		{"", "unsupported scheme"},
		// SSH-form edge cases — tightened in slice-1 review (H2).
		// Bare "git@" with no host:path → invalid.
		{"git@", "invalid ssh form"},
		// Host but no path → invalid.
		{"git@github.com:", "invalid ssh form"},
		// Path but no host → invalid.
		{"git@:example/repo", "invalid ssh form"},
		// Empty host AND path → invalid.
		{"git@:", "invalid ssh form"},
	}
	for _, c := range cases {
		err := validateRepoURL(c.in)
		if err == nil {
			t.Errorf("validateRepoURL(%q) = nil, want error containing %q", c.in, c.want)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("validateRepoURL(%q) = %v, want contains %q", c.in, err, c.want)
		}
	}
}
