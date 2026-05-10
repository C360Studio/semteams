package emitcuratorartifact

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/types"
)

// fakeTriplePublisher records triples passed to AddTriple.
type fakeTriplePublisher struct {
	mu      sync.Mutex
	triples []message.Triple
	err     error
}

func (f *fakeTriplePublisher) AddTriple(_ context.Context, t message.Triple) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.triples = append(f.triples, t)
	return nil
}

func (f *fakeTriplePublisher) snapshot() []message.Triple {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]message.Triple, len(f.triples))
	copy(out, f.triples)
	return out
}

// fakePublisher records the last Publish call.
type fakePublisher struct {
	mu      sync.Mutex
	subject string
	data    []byte
	calls   int
	err     error
}

func (f *fakePublisher) Publish(_ context.Context, subject string, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return f.err
	}
	f.subject = subject
	f.data = append([]byte(nil), data...)
	return nil
}

func (f *fakePublisher) snapshot() (string, []byte, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.subject, append([]byte(nil), f.data...), f.calls
}

func newExecutor(t *testing.T) (*Executor, *fakeTriplePublisher, *fakePublisher) {
	t.Helper()
	tp := &fakeTriplePublisher{}
	pub := &fakePublisher{}
	exec := NewExecutor(tp, pub, types.PlatformMeta{Org: "c360", Platform: "test"}, nil)
	return exec, tp, pub
}

func defaultCall(args map[string]any) agentic.ToolCall {
	return agentic.ToolCall{
		ID:        "call-curator-001",
		Name:      ToolName,
		Arguments: args,
		LoopID:    "loop-curator-abc",
		TraceID:   "trace-y",
	}
}

func defaultArgs() map[string]any {
	return map[string]any{
		"added_sources": []any{
			map[string]any{
				"url":       "https://github.com/sensorhub-tools/osh-core",
				"namespace": "osh-core",
				"covers":    "OSH IDriver interface, IPersistentDriver, BaseSensorModule",
			},
		},
		"verified_entity_ids": []any{
			"osh-core.org.sensorhub.api.module.IModule",
			"osh-core.org.sensorhub.api.module.IModuleProvider",
		},
	}
}

func defaultArgsWithSourceDirs() map[string]any {
	args := defaultArgs()
	args["source_dirs"] = []any{
		map[string]any{
			"namespace":  "osh-core",
			"mount_path": "/sources/osh-core",
			"covers":     "OSH module/driver interfaces, gradle build wiring",
		},
	}
	return args
}

// triplesByPredicate indexes triples by predicate name for assertion.
func triplesByPredicate(triples []message.Triple) map[string]any {
	out := make(map[string]any, len(triples))
	for _, tr := range triples {
		out[tr.Predicate] = tr.Object
	}
	return out
}

// ---- ListTools tests ----

func TestListTools_SchemaShape(t *testing.T) {
	exec, _, _ := newExecutor(t)
	defs := exec.ListTools()
	if len(defs) != 1 {
		t.Fatalf("ListTools length = %d, want 1", len(defs))
	}
	def := defs[0]
	if def.Name != ToolName {
		t.Errorf("tool name = %q, want %q", def.Name, ToolName)
	}
	props, ok := def.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema missing properties: %#v", def.Parameters)
	}
	for _, key := range []string{"added_sources", "verified_entity_ids", "source_dirs"} {
		if _, ok := props[key]; !ok {
			t.Errorf("missing property %q in tool schema", key)
		}
	}
	for _, forbidden := range []string{"loop_id", "produced_at"} {
		if _, ok := props[forbidden]; ok {
			t.Errorf("schema must not advertise %q (server-supplied)", forbidden)
		}
	}
	required, _ := def.Parameters["required"].([]string)
	wantRequired := map[string]bool{"added_sources": true, "verified_entity_ids": true}
	got := map[string]bool{}
	for _, r := range required {
		got[r] = true
	}
	for k := range wantRequired {
		if !got[k] {
			t.Errorf("required field missing: %q (have %v)", k, required)
		}
	}
	// source_dirs must NOT be required (it's optional per ADR-040 addendum).
	if got["source_dirs"] {
		t.Error("source_dirs must not be in required array (optional field)")
	}
}

// ---- Execute routing tests ----

func TestExecute_WrongToolName(t *testing.T) {
	exec, _, _ := newExecutor(t)
	call := defaultCall(defaultArgs())
	call.Name = "something-else"
	res, _ := exec.Execute(context.Background(), call)
	if res.ErrorKind != agentic.ToolErrorNotFound {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorNotFound)
	}
}

func TestExecute_MissingLoopID(t *testing.T) {
	exec, _, _ := newExecutor(t)
	call := defaultCall(defaultArgs())
	call.LoopID = ""
	res, _ := exec.Execute(context.Background(), call)
	if res.ErrorKind != agentic.ToolErrorInternal {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInternal)
	}
}

// ---- Validation failure tests ----

func TestExecute_EmptyAddedSources_FailsValidation(t *testing.T) {
	exec, _, _ := newExecutor(t)
	args := defaultArgs()
	args["added_sources"] = []any{}
	res, _ := exec.Execute(context.Background(), defaultCall(args))
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
	if !strings.Contains(res.Error, "added_sources") {
		t.Errorf("Result.Error = %q, want contains 'added_sources'", res.Error)
	}
}

func TestExecute_AddedSourceMissingURL_FailsValidation(t *testing.T) {
	exec, _, _ := newExecutor(t)
	args := defaultArgs()
	args["added_sources"] = []any{
		map[string]any{"url": "", "namespace": "osh-core", "covers": "stuff"},
	}
	res, _ := exec.Execute(context.Background(), defaultCall(args))
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
}

func TestExecute_AddedSourceMissingNamespace_FailsValidation(t *testing.T) {
	exec, _, _ := newExecutor(t)
	args := defaultArgs()
	args["added_sources"] = []any{
		map[string]any{"url": "https://example.com/repo", "namespace": "", "covers": "stuff"},
	}
	res, _ := exec.Execute(context.Background(), defaultCall(args))
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
}

func TestExecute_AddedSourceMissingCovers_FailsValidation(t *testing.T) {
	exec, _, _ := newExecutor(t)
	args := defaultArgs()
	args["added_sources"] = []any{
		map[string]any{"url": "https://example.com/repo", "namespace": "osh-core", "covers": ""},
	}
	res, _ := exec.Execute(context.Background(), defaultCall(args))
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
}

func TestExecute_EmptyVerifiedEntityIDs_FailsValidation(t *testing.T) {
	exec, _, _ := newExecutor(t)
	args := defaultArgs()
	args["verified_entity_ids"] = []any{}
	res, _ := exec.Execute(context.Background(), defaultCall(args))
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
	if !strings.Contains(res.Error, "verified_entity_ids") {
		t.Errorf("Result.Error = %q, want contains 'verified_entity_ids'", res.Error)
	}
}

func TestExecute_SourceDirsMissingNamespace_FailsValidation(t *testing.T) {
	exec, _, _ := newExecutor(t)
	args := defaultArgs()
	args["source_dirs"] = []any{
		map[string]any{"namespace": "", "mount_path": "/sources/osh-core", "covers": "stuff"},
	}
	res, _ := exec.Execute(context.Background(), defaultCall(args))
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
}

// ---- Happy-path tests ----

func TestExecute_HappyPath_WithoutSourceDirs(t *testing.T) {
	exec, tp, pub := newExecutor(t)
	res, err := exec.Execute(context.Background(), defaultCall(defaultArgs()))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty", res.Error)
	}

	// Payload published.
	subject, data, calls := pub.snapshot()
	if calls != 1 {
		t.Errorf("publish calls = %d, want 1", calls)
	}
	if subject != "curator.artifact.loop-curator-abc" {
		t.Errorf("subject = %q, want curator.artifact.loop-curator-abc", subject)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["loop_id"] != "loop-curator-abc" {
		t.Errorf("payload loop_id = %v, want loop-curator-abc", payload["loop_id"])
	}
	// source_dirs must be absent when not supplied.
	if _, ok := payload["source_dirs"]; ok {
		t.Error("source_dirs must be absent from payload when not supplied (omitempty)")
	}

	// Five triples with correct predicates.
	triples := tp.snapshot()
	const wantTripleCount = 5
	if len(triples) != wantTripleCount {
		t.Errorf("triple count = %d, want %d", len(triples), wantTripleCount)
	}
	wantEntityID := "c360.test.agent.agentic-loop.execution.loop-curator-abc"
	got := triplesByPredicate(triples)
	for _, tr := range triples {
		if tr.Subject != wantEntityID {
			t.Errorf("triple subject = %q, want %q", tr.Subject, wantEntityID)
		}
		if tr.Source != toolSource {
			t.Errorf("triple source = %q, want %q", tr.Source, toolSource)
		}
	}
	if got[predicateAddedSourcesCount] != "1" {
		t.Errorf("added_sources_count = %v, want '1'", got[predicateAddedSourcesCount])
	}
	if got[predicateVerifiedEntityIDsCount] != "2" {
		t.Errorf("verified_entity_ids_count = %v, want '2'", got[predicateVerifiedEntityIDsCount])
	}
	if got[predicateSourceDirsCount] != "0" {
		t.Errorf("source_dirs_count = %v, want '0'", got[predicateSourceDirsCount])
	}
	if got[predicateNamespaces] != "osh-core" {
		t.Errorf("namespaces = %v, want 'osh-core'", got[predicateNamespaces])
	}
	genAt, _ := got[predicateGeneratedAt].(string)
	if genAt == "" {
		t.Error("generated_at must be non-empty RFC3339 string")
	}

	// Content is the marshaled payload (what the next researcher reads via
	// read_loop_result).
	if res.Content == "" {
		t.Error("Result.Content must be the marshaled curator artifact JSON")
	}
	var contentParsed map[string]any
	if err := json.Unmarshal([]byte(res.Content), &contentParsed); err != nil {
		t.Fatalf("parse Result.Content as JSON: %v", err)
	}
	if contentParsed["loop_id"] != "loop-curator-abc" {
		t.Errorf("Content loop_id = %v, want loop-curator-abc", contentParsed["loop_id"])
	}
}

func TestExecute_HappyPath_WithSourceDirs(t *testing.T) {
	exec, tp, pub := newExecutor(t)
	res, err := exec.Execute(context.Background(), defaultCall(defaultArgsWithSourceDirs()))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty", res.Error)
	}

	// source_dirs count.
	triples := tp.snapshot()
	got := triplesByPredicate(triples)
	if got[predicateSourceDirsCount] != "1" {
		t.Errorf("source_dirs_count = %v, want '1'", got[predicateSourceDirsCount])
	}

	// Payload includes source_dirs.
	_, data, _ := pub.snapshot()
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	dirs, ok := payload["source_dirs"].([]any)
	if !ok || len(dirs) != 1 {
		t.Errorf("payload source_dirs = %v, want 1-entry array", payload["source_dirs"])
	}
}

func TestExecute_HappyPath_NamespacesDedupAndSort(t *testing.T) {
	// Two sources sharing one namespace, plus a second namespace from
	// source_dirs. Result: deduplicated, sorted.
	exec, tp, _ := newExecutor(t)
	args := map[string]any{
		"added_sources": []any{
			map[string]any{"url": "https://example.com/b-repo", "namespace": "b-ns", "covers": "B"},
			map[string]any{"url": "https://example.com/a-repo", "namespace": "a-ns", "covers": "A"},
			// duplicate namespace for b-ns — should appear once in output.
			map[string]any{"url": "https://example.com/b-repo2", "namespace": "b-ns", "covers": "B2"},
		},
		"verified_entity_ids": []any{"a-ns.something.Foo"},
		"source_dirs": []any{
			// c-ns from source_dirs, not present in added_sources.
			map[string]any{"namespace": "c-ns", "mount_path": "/sources/c-ns", "covers": "C"},
		},
	}
	res, err := exec.Execute(context.Background(), defaultCall(args))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty", res.Error)
	}

	got := triplesByPredicate(tp.snapshot())
	// Expected: a-ns, b-ns, c-ns (deduplicated b-ns duplicate, sorted)
	if got[predicateNamespaces] != "a-ns,b-ns,c-ns" {
		t.Errorf("namespaces = %v, want 'a-ns,b-ns,c-ns'", got[predicateNamespaces])
	}
	if got[predicateAddedSourcesCount] != "3" {
		t.Errorf("added_sources_count = %v, want '3'", got[predicateAddedSourcesCount])
	}
}

func TestExecute_PayloadPublishFails(t *testing.T) {
	exec, tp, pub := newExecutor(t)
	pub.err = errors.New("nats: connection closed")

	res, _ := exec.Execute(context.Background(), defaultCall(defaultArgs()))
	if res.ErrorKind != agentic.ToolErrorNetwork {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorNetwork)
	}
	// No triples should be written if publish fails.
	if len(tp.snapshot()) != 0 {
		t.Errorf("triple count = %d, want 0 after payload publish failure", len(tp.snapshot()))
	}
}

func TestExecute_TriplePublishFails(t *testing.T) {
	exec, tp, pub := newExecutor(t)
	tp.err = errors.New("graph-ingest unreachable")

	res, _ := exec.Execute(context.Background(), defaultCall(defaultArgs()))
	if res.ErrorKind != agentic.ToolErrorNetwork {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorNetwork)
	}
	// Payload was published (publish happens before triples).
	if _, _, calls := pub.snapshot(); calls != 1 {
		t.Errorf("payload publish calls = %d, want 1 (publish fires before triples)", calls)
	}
}

func TestExecute_SubjectFormat(t *testing.T) {
	exec, _, pub := newExecutor(t)
	call := defaultCall(defaultArgs())
	call.LoopID = "loop-xyz-9999"

	res, _ := exec.Execute(context.Background(), call)
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty", res.Error)
	}

	subject, _, _ := pub.snapshot()
	want := "curator.artifact.loop-xyz-9999"
	if subject != want {
		t.Errorf("subject = %q, want %q", subject, want)
	}
}

func TestExecute_ContentMatchesPublishedPayload(t *testing.T) {
	exec, _, pub := newExecutor(t)
	res, _ := exec.Execute(context.Background(), defaultCall(defaultArgs()))
	if res.Error != "" {
		t.Fatalf("Result.Error = %q", res.Error)
	}

	_, data, _ := pub.snapshot()
	if res.Content != string(data) {
		t.Errorf("Result.Content does not match published payload bytes")
	}
}

func TestExecute_ServerFieldsNotOverriddenByLLM(t *testing.T) {
	// LLM-claimed loop_id and produced_at must be stripped and
	// replaced with server-supplied values.
	exec, _, pub := newExecutor(t)
	args := defaultArgs()
	args["loop_id"] = "llm-injected-loop-id"
	args["produced_at"] = "2000-01-01T00:00:00Z"

	call := defaultCall(args)
	call.LoopID = "server-loop-id"

	res, _ := exec.Execute(context.Background(), call)
	if res.Error != "" {
		t.Fatalf("Result.Error = %q", res.Error)
	}

	_, data, _ := pub.snapshot()
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["loop_id"] != "server-loop-id" {
		t.Errorf("payload loop_id = %v, want 'server-loop-id' (LLM-injected value must be stripped)", payload["loop_id"])
	}
	if payload["produced_at"] == "2000-01-01T00:00:00Z" {
		t.Errorf("payload produced_at was left as LLM-injected value; must be server-supplied")
	}
}
