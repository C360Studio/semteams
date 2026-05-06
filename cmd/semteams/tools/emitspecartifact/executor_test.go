package emitspecartifact

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/types"

	"github.com/c360studio/semteams/cmd/semteams/testharness"
)

// ---------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------

// fakeTriplePublisher records every AddTriple call. By default returns
// nil; tests exercising the failure path set err.
type fakeTriplePublisher struct {
	mu      sync.Mutex
	triples []message.Triple
	err     error
}

func (f *fakeTriplePublisher) AddTriple(_ context.Context, triple message.Triple) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.triples = append(f.triples, triple)
	return nil
}

func (f *fakeTriplePublisher) snapshot() []message.Triple {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]message.Triple, len(f.triples))
	copy(out, f.triples)
	return out
}

// fakePublisher records every Publish call. Mirrors fakeTriplePublisher's
// concurrency posture (sync.Mutex + snapshot accessor).
type fakePublisher struct {
	mu      sync.Mutex
	subject string
	data    []byte
	err     error
	calls   int
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

func (f *fakePublisher) snapshot() (subject string, data []byte, calls int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.subject, append([]byte(nil), f.data...), f.calls
}

// ---------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------

// newExecutorWithDir constructs an Executor writing to a temp directory.
// The temp dir is automatically cleaned up at test end.
func newExecutorWithDir(t *testing.T) (*Executor, *fakeTriplePublisher, *fakePublisher, string) {
	t.Helper()
	tmpDir := t.TempDir()
	tp := &fakeTriplePublisher{}
	pub := &fakePublisher{}
	exec := NewExecutor(tp, pub, types.PlatformMeta{Org: "c360", Platform: "semteams"}, nil, tmpDir, nil)
	return exec, tp, pub, tmpDir
}

func defaultCall(args map[string]any) agentic.ToolCall {
	return agentic.ToolCall{
		ID:        "call-001",
		Name:      ToolName,
		Arguments: args,
		LoopID:    "loop-architect-abc",
		TraceID:   "trace-xyz",
	}
}

func defaultArtifactArgs() map[string]any {
	return map[string]any{
		"title":   "OSH Meshtastic Driver",
		"goal":    "Implement an OSH IDriver backed by Meshtastic radio events.",
		"context": "The OSH platform exposes observation endpoints; Meshtastic provides LoRa mesh transport.",
		"actors": []any{
			map[string]any{"name": "OSH driver framework", "role": "host of the IDriver interface"},
			map[string]any{"name": "Meshtastic radio", "role": "LoRa mesh transport"},
		},
		"integration_points": []any{
			map[string]any{"from": "Meshtastic radio", "to": "OSH driver framework", "data": "MeshPacket payloads", "direction": "read"},
		},
		"tasks": []any{
			map[string]any{
				"title":          "Implement IDriver",
				"scope":          "backend",
				"grounds_actors": []any{"OSH driver framework"},
			},
			map[string]any{
				"title": "Expose OGC CS endpoints",
				"scope": "backend",
			},
		},
		"provenance": map[string]any{
			"research_artifact_loop": "loop-research-001",
			"planner_loop":           "loop-planner-001",
			"reviewer_loop":          "loop-reviewer-001",
			"challenger_loop":        "loop-challenger-001",
		},
	}
}

// ---------------------------------------------------------------------
// ListTools — schema sanity
// ---------------------------------------------------------------------

func TestListTools_SchemaShape(t *testing.T) {
	exec, _, _, _ := newExecutorWithDir(t)
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
	for _, key := range []string{"title", "goal", "context", "actors", "integration_points", "tasks", "checks", "provenance"} {
		if _, ok := props[key]; !ok {
			t.Errorf("missing property %q in tool schema", key)
		}
	}
	// checks is optional at the wire level (the architect-persona contract
	// for required emission lands in R3.7.2.f); confirm it's NOT in `required`.
	requiredList, _ := def.Parameters["required"].([]string)
	for _, r := range requiredList {
		if r == "checks" {
			t.Errorf("checks must not be in required (R3.7.2.f extends the contract): %v", requiredList)
		}
	}
	// Server-supplied fields must not appear in the LLM-facing schema.
	for _, forbidden := range []string{"generated_at", "slug"} {
		if _, ok := props[forbidden]; ok {
			t.Errorf("schema must not advertise %q (server-supplied)", forbidden)
		}
	}
	required, _ := def.Parameters["required"].([]string)
	wantRequired := map[string]bool{"title": true, "goal": true, "context": true, "actors": true, "tasks": true, "provenance": true}
	got := map[string]bool{}
	for _, r := range required {
		got[r] = true
	}
	for k := range wantRequired {
		if !got[k] {
			t.Errorf("required field missing: %q (have %v)", k, required)
		}
	}
}

// ---------------------------------------------------------------------
// Routing / preconditions
// ---------------------------------------------------------------------

func TestExecute_WrongToolName(t *testing.T) {
	exec, _, _, _ := newExecutorWithDir(t)
	call := defaultCall(defaultArtifactArgs())
	call.Name = "something-else"
	res, err := exec.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("Execute err = %v, want nil", err)
	}
	if res.ErrorKind != agentic.ToolErrorNotFound {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorNotFound)
	}
}

func TestExecute_MissingLoopID(t *testing.T) {
	exec, _, _, _ := newExecutorWithDir(t)
	call := defaultCall(defaultArtifactArgs())
	call.LoopID = ""
	res, err := exec.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("Execute err = %v, want nil", err)
	}
	if res.ErrorKind != agentic.ToolErrorInternal {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInternal)
	}
	if !strings.Contains(res.Error, "loop_id") {
		t.Errorf("Result.Error = %q, want contains 'loop_id'", res.Error)
	}
}

// ---------------------------------------------------------------------
// Argument validation failures
// ---------------------------------------------------------------------

func TestExecute_MissingTitle_FailsValidation(t *testing.T) {
	exec, _, _, _ := newExecutorWithDir(t)
	args := defaultArtifactArgs()
	delete(args, "title")
	res, _ := exec.Execute(context.Background(), defaultCall(args))
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
	if !strings.Contains(res.Error, "title") {
		t.Errorf("Result.Error = %q, want contains 'title'", res.Error)
	}
}

func TestExecute_MissingGoal_FailsValidation(t *testing.T) {
	exec, _, _, _ := newExecutorWithDir(t)
	args := defaultArtifactArgs()
	delete(args, "goal")
	res, _ := exec.Execute(context.Background(), defaultCall(args))
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
}

func TestExecute_NoActors_FailsValidation(t *testing.T) {
	exec, _, _, _ := newExecutorWithDir(t)
	args := defaultArtifactArgs()
	args["actors"] = []any{}
	res, _ := exec.Execute(context.Background(), defaultCall(args))
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
	if !strings.Contains(res.Error, "actors") {
		t.Errorf("Result.Error = %q, want contains 'actors'", res.Error)
	}
}

func TestExecute_NoTasks_FailsValidation(t *testing.T) {
	exec, _, _, _ := newExecutorWithDir(t)
	args := defaultArtifactArgs()
	args["tasks"] = []any{}
	res, _ := exec.Execute(context.Background(), defaultCall(args))
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
	if !strings.Contains(res.Error, "tasks") {
		t.Errorf("Result.Error = %q, want contains 'tasks'", res.Error)
	}
}

func TestExecute_BadIntegrationPointDirection_FailsValidation(t *testing.T) {
	exec, _, _, _ := newExecutorWithDir(t)
	args := defaultArtifactArgs()
	args["integration_points"] = []any{
		map[string]any{"from": "A", "to": "B", "data": "x", "direction": "sideways"},
	}
	res, _ := exec.Execute(context.Background(), defaultCall(args))
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
	if !strings.Contains(res.Error, "direction") {
		t.Errorf("Result.Error = %q, want contains 'direction'", res.Error)
	}
}

func TestExecute_MissingProvenanceResearchLoop_FailsValidation(t *testing.T) {
	exec, _, _, _ := newExecutorWithDir(t)
	args := defaultArtifactArgs()
	args["provenance"] = map[string]any{
		"planner_loop":    "loop-planner-001",
		"reviewer_loop":   "loop-reviewer-001",
		"challenger_loop": "loop-challenger-001",
		// research_artifact_loop intentionally omitted
	}
	res, _ := exec.Execute(context.Background(), defaultCall(args))
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
	if !strings.Contains(res.Error, "research_artifact_loop") {
		t.Errorf("Result.Error = %q, want contains 'research_artifact_loop'", res.Error)
	}
}

// Titles with no ASCII alphanumeric content produce a slug ending in "-"
// and must be rejected before any file write or triple/payload publish.
func TestExecute_DegenerateTitle_Rejected(t *testing.T) {
	exec, tp, pub, tmpDir := newExecutorWithDir(t)
	args := defaultArtifactArgs()
	args["title"] = "!!!"

	res, err := exec.Execute(context.Background(), defaultCall(args))
	if err != nil {
		t.Fatalf("Execute err = %v, want nil", err)
	}
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want ToolErrorInvalidArgs", res.ErrorKind)
	}
	if !strings.Contains(res.Error, "title") {
		t.Errorf("Result.Error = %q, want 'title' in message", res.Error)
	}
	// No file must have been written.
	entries, _ := os.ReadDir(tmpDir)
	if len(entries) != 0 {
		t.Errorf("expected no files in tmpDir after degenerate-title rejection, got %d", len(entries))
	}
	// No triples or payload published.
	if got := tp.snapshot(); len(got) != 0 {
		t.Errorf("expected 0 triples published, got %d", len(got))
	}
	if pub.calls != 0 {
		t.Errorf("expected 0 payload publish calls, got %d", pub.calls)
	}
}

// LLM-supplied generated_at and slug are stripped — the tool always
// derives them server-side.
func TestExecute_LLMSuppliedGeneratedAtAndSlugIgnored(t *testing.T) {
	exec, _, pub, _ := newExecutorWithDir(t)
	args := defaultArtifactArgs()
	args["generated_at"] = "1970-01-01T00:00:00Z"
	args["slug"] = "some-llm-slug"

	beforeCall := time.Now().UTC().Add(-1 * time.Second)
	res, err := exec.Execute(context.Background(), defaultCall(args))
	if err != nil {
		t.Fatalf("Execute err = %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty", res.Error)
	}

	subject, data, _ := pub.snapshot()
	if !strings.HasPrefix(subject, payloadSubjectPrefix+".loop-architect-abc") {
		t.Errorf("subject = %q, want prefix %q", subject, payloadSubjectPrefix+".loop-architect-abc")
	}

	var sent map[string]any
	if err := json.Unmarshal(data, &sent); err != nil {
		t.Fatalf("decode published payload: %v", err)
	}

	// generated_at should be ~now, not 1970.
	genAtStr, _ := sent["generated_at"].(string)
	genAt, err := time.Parse(time.RFC3339Nano, genAtStr)
	if err != nil {
		t.Fatalf("parse generated_at %q: %v", genAtStr, err)
	}
	if genAt.Before(beforeCall) {
		t.Errorf("generated_at = %v, must be >= wall-clock start of test (%v) — LLM-supplied 1970 leaked through", genAt, beforeCall)
	}

	// slug must NOT be the LLM-supplied value.
	slug, _ := sent["slug"].(string)
	if slug == "some-llm-slug" {
		t.Errorf("slug = %q, should have been overridden by server-side derivation", slug)
	}
	if slug == "" {
		t.Errorf("slug is empty — server-side derivation failed")
	}
}

// When the LLM supplies provenance as a non-map type (string, array, null),
// architect_loop must still be set from the ToolCall. The missing required
// provenance fields (research_artifact_loop etc.) should surface via Validate,
// not as a JSON-unmarshal error or panic.
func TestExecute_ProvenanceNotAMap_StillSetsArchitectLoop(t *testing.T) {
	exec, _, _, _ := newExecutorWithDir(t)
	args := defaultArtifactArgs()
	args["provenance"] = "garbage-string"

	res, err := exec.Execute(context.Background(), defaultCall(args))
	if err != nil {
		t.Fatalf("Execute err = %v, want nil", err)
	}
	// Validate must catch the missing required fields — not a panic or
	// marshal-unmarshal error.
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want ToolErrorInvalidArgs", res.ErrorKind)
	}
	// The error should mention a provenance field (Validate's complaint),
	// not a generic unmarshal failure.
	if strings.Contains(res.Error, "unmarshal") {
		t.Errorf("Result.Error = %q — should be Validate's complaint, not unmarshal error", res.Error)
	}
}

// ArchitectLoop in provenance must come from the ToolCall, not LLM args.
func TestExecute_LLMSuppliedArchitectLoopIgnored(t *testing.T) {
	exec, _, pub, _ := newExecutorWithDir(t)
	args := defaultArtifactArgs()
	// Inject architect_loop into the provenance map — executor must strip it.
	prov := args["provenance"].(map[string]any)
	prov["architect_loop"] = "loop-spoof-architect"

	res, err := exec.Execute(context.Background(), defaultCall(args))
	if err != nil {
		t.Fatalf("Execute err = %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty", res.Error)
	}

	_, data, _ := pub.snapshot()
	var sent map[string]any
	if err := json.Unmarshal(data, &sent); err != nil {
		t.Fatalf("decode published payload: %v", err)
	}
	provSent, _ := sent["provenance"].(map[string]any)
	if al, _ := provSent["architect_loop"].(string); al != "loop-architect-abc" {
		t.Errorf("architect_loop = %q, want loop-architect-abc (call.LoopID, not args)", al)
	}
}

// ---------------------------------------------------------------------
// Happy path — triples + payload + result
// ---------------------------------------------------------------------

func TestExecute_HappyPath_TripleSet(t *testing.T) {
	exec, tp, _, _ := newExecutorWithDir(t)
	res, err := exec.Execute(context.Background(), defaultCall(defaultArtifactArgs()))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty", res.Error)
	}

	triples := tp.snapshot()
	wantLoopEntityID := "c360.semteams.agent.agentic-loop.execution.loop-architect-abc"

	// Triple count is load-bearing for downstream rule wiring (R3.7.2.h
	// fires on the count triple). Drift in either direction (drop a
	// triple, accidentally duplicate one) should fail loudly here.
	const wantTripleCount = 8 // path, slug, generated_at, actor_count, integration_point_count, task_count, check_count, research_root_loop
	if len(triples) != wantTripleCount {
		t.Errorf("triple count = %d, want %d", len(triples), wantTripleCount)
	}

	gotPredicates := map[string]any{}
	for _, tr := range triples {
		if tr.Subject != wantLoopEntityID {
			t.Errorf("triple subject = %q, want %q", tr.Subject, wantLoopEntityID)
		}
		if tr.Source != toolSource {
			t.Errorf("triple source = %q, want %q", tr.Source, toolSource)
		}
		gotPredicates[tr.Predicate] = tr.Object
	}

	// Verify count predicates.
	if got := gotPredicates[predicateActorCount]; got != 2 {
		t.Errorf("triple %q = %v, want 2", predicateActorCount, got)
	}
	if got := gotPredicates[predicateIntegrationPointCount]; got != 1 {
		t.Errorf("triple %q = %v, want 1", predicateIntegrationPointCount, got)
	}
	if got := gotPredicates[predicateTaskCount]; got != 2 {
		t.Errorf("triple %q = %v, want 2", predicateTaskCount, got)
	}
	// check_count is emitted unconditionally (zero is meaningful);
	// defaultArtifactArgs() emits no checks for back-compat.
	if got := gotPredicates[predicateCheckCount]; got != 0 {
		t.Errorf("triple %q = %v, want 0 (default args have no checks)", predicateCheckCount, got)
	}
	if got := gotPredicates[predicateResearchRootLoop]; got != "loop-research-001" {
		t.Errorf("triple %q = %v, want loop-research-001", predicateResearchRootLoop, got)
	}

	// Slug and path must be non-empty strings.
	if slug, ok := gotPredicates[predicateSlug].(string); !ok || slug == "" {
		t.Errorf("triple %q missing or empty: %v", predicateSlug, gotPredicates[predicateSlug])
	}
	if path, ok := gotPredicates[predicatePath].(string); !ok || path == "" {
		t.Errorf("triple %q missing or empty: %v", predicatePath, gotPredicates[predicatePath])
	}
	if genAt, ok := gotPredicates[predicateGeneratedAt].(string); !ok || genAt == "" {
		t.Errorf("triple %q missing or empty: %v", predicateGeneratedAt, gotPredicates[predicateGeneratedAt])
	}
}

func TestExecute_HappyPath_PayloadPublishedToStableSubject(t *testing.T) {
	exec, _, pub, _ := newExecutorWithDir(t)
	_, err := exec.Execute(context.Background(), defaultCall(defaultArtifactArgs()))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	subject, data, calls := pub.snapshot()
	if calls != 1 {
		t.Errorf("payload publish calls = %d, want 1", calls)
	}
	if subject != "dev_via_spec.artifact.loop-architect-abc" {
		t.Errorf("subject = %q, want dev_via_spec.artifact.loop-architect-abc", subject)
	}
	var roundTrip map[string]any
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if got := roundTrip["title"]; got != "OSH Meshtastic Driver" {
		t.Errorf("payload.title = %v, want OSH Meshtastic Driver", got)
	}
}

// The predicateGeneratedAt triple object and the payload's generated_at field
// must be byte-identical strings (same now, same RFC3339Nano format).
func TestExecute_TimestampsCorrelate(t *testing.T) {
	exec, tp, pub, _ := newExecutorWithDir(t)
	res, err := exec.Execute(context.Background(), defaultCall(defaultArtifactArgs()))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty", res.Error)
	}

	// Extract generated_at from the published payload.
	_, data, _ := pub.snapshot()
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	payloadGenAt, _ := payload["generated_at"].(string)
	if payloadGenAt == "" {
		t.Fatalf("payload.generated_at is empty")
	}

	// Extract the predicateGeneratedAt triple object.
	triples := tp.snapshot()
	var tripleGenAt string
	for _, tr := range triples {
		if tr.Predicate == predicateGeneratedAt {
			tripleGenAt, _ = tr.Object.(string)
			break
		}
	}
	if tripleGenAt == "" {
		t.Fatalf("predicateGeneratedAt triple not found or empty")
	}

	if payloadGenAt != tripleGenAt {
		t.Errorf("timestamp mismatch: payload.generated_at = %q, triple object = %q — must be byte-identical", payloadGenAt, tripleGenAt)
	}
}

func TestExecute_ResultContent_ShapeAndCounts(t *testing.T) {
	exec, _, _, _ := newExecutorWithDir(t)
	res, err := exec.Execute(context.Background(), defaultCall(defaultArtifactArgs()))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty", res.Error)
	}

	var content struct {
		Slug                  string `json:"slug"`
		Path                  string `json:"path"`
		GeneratedAt           string `json:"generated_at"`
		ActorCount            int    `json:"actor_count"`
		IntegrationPointCount int    `json:"integration_point_count"`
		TaskCount             int    `json:"task_count"`
		ResearchRootLoop      string `json:"research_root_loop"`
		PayloadSubject        string `json:"payload_subject"`
		LoopEntityID          string `json:"loop_entity_id"`
	}
	if err := json.Unmarshal([]byte(res.Content), &content); err != nil {
		t.Fatalf("decode tool result: %v\nraw=%s", err, res.Content)
	}
	if content.Slug == "" {
		t.Errorf("slug is empty")
	}
	if content.Path == "" {
		t.Errorf("path is empty")
	}
	if content.ActorCount != 2 {
		t.Errorf("actor_count = %d, want 2", content.ActorCount)
	}
	if content.IntegrationPointCount != 1 {
		t.Errorf("integration_point_count = %d, want 1", content.IntegrationPointCount)
	}
	if content.TaskCount != 2 {
		t.Errorf("task_count = %d, want 2", content.TaskCount)
	}
	if content.ResearchRootLoop != "loop-research-001" {
		t.Errorf("research_root_loop = %q, want loop-research-001", content.ResearchRootLoop)
	}
	if content.PayloadSubject != "dev_via_spec.artifact.loop-architect-abc" {
		t.Errorf("payload_subject = %q", content.PayloadSubject)
	}
	if content.LoopEntityID != "c360.semteams.agent.agentic-loop.execution.loop-architect-abc" {
		t.Errorf("loop_entity_id = %q", content.LoopEntityID)
	}
}

// ---------------------------------------------------------------------
// Markdown file written to disk
// ---------------------------------------------------------------------

func TestExecute_HappyPath_MarkdownFileWritten(t *testing.T) {
	exec, _, _, tmpDir := newExecutorWithDir(t)
	res, err := exec.Execute(context.Background(), defaultCall(defaultArtifactArgs()))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty", res.Error)
	}

	var content struct {
		Path string `json:"path"`
		Slug string `json:"slug"`
	}
	if err := json.Unmarshal([]byte(res.Content), &content); err != nil {
		t.Fatalf("decode result: %v", err)
	}

	// File must exist at the reported path.
	data, err := os.ReadFile(content.Path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", content.Path, err)
	}
	md := string(data)

	// Key sections presence (snapshot-style, not byte-exact).
	for _, section := range []string{
		"# OSH Meshtastic Driver",
		"## Goal",
		"## Context",
		"## Actors",
		"## Integration Points",
		"## Tasks",
		"### T1 — Implement IDriver",
		"### T2 — Expose OGC CS endpoints",
		"## Provenance",
		"loop-research-001",
		"loop-planner-001",
		"loop-reviewer-001",
		"loop-challenger-001",
		"loop-architect-abc",
	} {
		if !strings.Contains(md, section) {
			t.Errorf("markdown missing %q\n---\n%s", section, md)
		}
	}

	// SR2 has no grounds_actors — the template must emit the missing-grounding flag.
	if !strings.Contains(md, "_flagged: missing grounding_") {
		t.Errorf("markdown missing '_flagged: missing grounding_' for SR2 (no grounds_actors)\n---\n%s", md)
	}

	// Slug must match path basename.
	expectedFilename := content.Slug + ".md"
	if filepath.Base(content.Path) != expectedFilename {
		t.Errorf("path basename = %q, want %q", filepath.Base(content.Path), expectedFilename)
	}

	// File must be inside the executor's output dir (tmpDir).
	if !strings.HasPrefix(content.Path, tmpDir) {
		t.Errorf("path %q is not under tmpDir %q", content.Path, tmpDir)
	}
}

// Idempotent overwrite: two runs with the same title produce the same slug
// (on the same day) and the second run overwrites the first file without error.
func TestExecute_IdempotentOverwrite(t *testing.T) {
	exec, _, _, tmpDir := newExecutorWithDir(t)

	run := func(goal string) string {
		t.Helper()
		args := defaultArtifactArgs()
		args["goal"] = goal
		res, err := exec.Execute(context.Background(), defaultCall(args))
		if err != nil {
			t.Fatalf("Execute err: %v", err)
		}
		if res.Error != "" {
			t.Fatalf("Result.Error = %q", res.Error)
		}
		var content struct {
			Path string `json:"path"`
		}
		_ = json.Unmarshal([]byte(res.Content), &content)
		return content.Path
	}

	path1 := run("First run goal.")
	path2 := run("Second run goal.")

	if path1 != path2 {
		t.Errorf("paths differ on same title same day: %q vs %q", path1, path2)
	}

	data, _ := os.ReadFile(path2)
	if !strings.Contains(string(data), "Second run goal.") {
		t.Errorf("second run did not overwrite: file does not contain updated goal")
	}

	// Confirm only one file in the output dir.
	entries, _ := os.ReadDir(tmpDir)
	if len(entries) != 1 {
		t.Errorf("expected 1 file in output dir, got %d", len(entries))
	}
}

// Output directory is created automatically if it doesn't exist.
func TestExecute_OutputDirAutoCreated(t *testing.T) {
	tp := &fakeTriplePublisher{}
	pub := &fakePublisher{}
	tmpBase := t.TempDir()
	nestedDir := filepath.Join(tmpBase, "deep", "nested", "specs")
	exec := NewExecutor(tp, pub, types.PlatformMeta{Org: "c360", Platform: "semteams"}, nil, nestedDir, nil)

	res, err := exec.Execute(context.Background(), defaultCall(defaultArtifactArgs()))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty", res.Error)
	}

	var content struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal([]byte(res.Content), &content)
	if _, err := os.Stat(content.Path); err != nil {
		t.Errorf("output file not found at %q: %v", content.Path, err)
	}
}

// Output directory from environment variable override.
func TestExecute_OutputDirEnvOverride(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv(envOutputDir, tmpDir)
	tp := &fakeTriplePublisher{}
	pub := &fakePublisher{}
	// Pass empty outputDir so the constructor picks up the env var.
	exec := NewExecutor(tp, pub, types.PlatformMeta{Org: "c360", Platform: "semteams"}, nil, "", nil)

	res, err := exec.Execute(context.Background(), defaultCall(defaultArtifactArgs()))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty", res.Error)
	}

	var content struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal([]byte(res.Content), &content)
	if !strings.HasPrefix(content.Path, tmpDir) {
		t.Errorf("path %q is not under env-override dir %q", content.Path, tmpDir)
	}
}

// Constructor arg beats env var: when outputDir is non-empty, the env var
// must be ignored even if set.
func TestExecute_OutputDirCallerBeatsEnv(t *testing.T) {
	envDir := t.TempDir()
	constructorDir := t.TempDir()
	t.Setenv(envOutputDir, envDir)

	tp := &fakeTriplePublisher{}
	pub := &fakePublisher{}
	exec := NewExecutor(tp, pub, types.PlatformMeta{Org: "c360", Platform: "semteams"}, nil, constructorDir, nil)

	res, err := exec.Execute(context.Background(), defaultCall(defaultArtifactArgs()))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty", res.Error)
	}

	var content struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal([]byte(res.Content), &content)
	if !strings.HasPrefix(content.Path, constructorDir) {
		t.Errorf("path %q is not under constructorDir %q — env var leaked through", content.Path, constructorDir)
	}
	if strings.HasPrefix(content.Path, envDir) {
		t.Errorf("path %q is under envDir %q — constructor arg should win", content.Path, envDir)
	}
}

// ---------------------------------------------------------------------
// Failure paths
// ---------------------------------------------------------------------

func TestExecute_TriplePublisherFails_NoPayloadPublished(t *testing.T) {
	exec, tp, pub, tmpDir := newExecutorWithDir(t)
	tp.err = errors.New("graph-ingest unreachable")

	res, err := exec.Execute(context.Background(), defaultCall(defaultArtifactArgs()))
	if err != nil {
		t.Fatalf("Execute err = %v, want nil", err)
	}
	if res.ErrorKind != agentic.ToolErrorNetwork {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorNetwork)
	}
	if !strings.Contains(res.Error, "graph-ingest unreachable") {
		t.Errorf("Result.Error = %q, want contains 'graph-ingest unreachable'", res.Error)
	}
	if pub.calls != 0 {
		t.Errorf("payload publish calls = %d, want 0 (must short-circuit on triple failure)", pub.calls)
	}
	// The markdown file is written before triples; it persists on triple failure.
	// A subsequent LLM retry will overwrite it (idempotent on slug) — this is the
	// documented contract. Asserting here so a future write-after-triples refactor
	// surfaces in the diff.
	entries, _ := os.ReadDir(tmpDir)
	if len(entries) != 1 {
		t.Errorf("expected 1 markdown file to persist after triple failure, got %d — file must persist for idempotent retry", len(entries))
	}
}

func TestExecute_PayloadPublishFails_ReportsNetworkError(t *testing.T) {
	exec, _, pub, _ := newExecutorWithDir(t)
	pub.err = errors.New("nats not connected")

	res, err := exec.Execute(context.Background(), defaultCall(defaultArtifactArgs()))
	if err != nil {
		t.Fatalf("Execute err = %v, want nil", err)
	}
	if res.ErrorKind != agentic.ToolErrorNetwork {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorNetwork)
	}
	if !strings.Contains(res.Error, "nats not connected") {
		t.Errorf("Result.Error = %q, want contains 'nats not connected'", res.Error)
	}
	if !strings.Contains(res.Error, "dev_via_spec.artifact.loop-architect-abc") {
		t.Errorf("Result.Error = %q, want subject in error message", res.Error)
	}
}

// ---------------------------------------------------------------------
// deriveSlug unit tests
// ---------------------------------------------------------------------

func TestDeriveSlug(t *testing.T) {
	t.Parallel()
	ref := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		title string
		want  string
	}{
		{"OSH Meshtastic Driver", "2026-04-30-osh-meshtastic-driver"},
		{"  Spaces  Around  ", "2026-04-30-spaces-around"},
		{"Special!@#$Characters", "2026-04-30-special-characters"},
		{"Multi   Spaces", "2026-04-30-multi-spaces"},
		{"Already-Kebab", "2026-04-30-already-kebab"},
		// Non-ASCII title: all chars collapse to a single "-" leaving an empty
		// title segment. Execute rejects this via the HasSuffix("-") check.
		// This row documents the current shape so a Unicode-normalise fix
		// surfaces in the diff.
		{"日本語", "2026-04-30-"},
	}
	for _, tc := range cases {
		t.Run(tc.title, func(t *testing.T) {
			t.Parallel()
			got := deriveSlug(tc.title, ref)
			if got != tc.want {
				t.Errorf("deriveSlug(%q) = %q, want %q", tc.title, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------
// R3.7.2.b — checks wiring
// ---------------------------------------------------------------------

// argsWithChecks returns defaultArtifactArgs() plus a representative
// pair of checks — one testcontainer (real-stack), one in-process unit.
// Used across the R3.7.2.b tests.
func argsWithChecks() map[string]any {
	args := defaultArtifactArgs()
	args["checks"] = []any{
		map[string]any{
			"target":       "executor publishes graph.mutation.entity.add on success",
			"runtime":      "process-local-testcontainer",
			"test_harness": "nats-jetstream",
			"test_runtime": "go-testing-net",
			"ref": map[string]any{
				"type": "filepath",
				"path": "cmd/semteams/sandbox/integration_test.go",
			},
			"evidence": []any{
				map[string]any{"kind": "test_uses_build_tag", "args": map[string]any{"tag": "integration"}},
			},
		},
		map[string]any{
			"target":  "executor returns ToolErrorInvalidArgs on malformed input",
			"runtime": "in-process-unit",
			"ref": map[string]any{
				"type": "filepath",
				"path": "cmd/semteams/tools/x/executor_test.go",
			},
		},
	}
	return args
}

func TestExecute_HappyPath_WithChecks_TripleSet(t *testing.T) {
	exec, tp, _, _ := newExecutorWithDir(t)
	res, err := exec.Execute(context.Background(), defaultCall(argsWithChecks()))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty", res.Error)
	}

	gotPredicates := map[string]any{}
	for _, tr := range tp.snapshot() {
		gotPredicates[tr.Predicate] = tr.Object
	}
	if got := gotPredicates[predicateCheckCount]; got != 2 {
		t.Errorf("triple %q = %v, want 2", predicateCheckCount, got)
	}
}

func TestExecute_WithChecks_PayloadRoundTrips(t *testing.T) {
	exec, _, pub, _ := newExecutorWithDir(t)
	_, err := exec.Execute(context.Background(), defaultCall(argsWithChecks()))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	_, data, _ := pub.snapshot()
	var roundTrip map[string]any
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	checks, ok := roundTrip["checks"].([]any)
	if !ok {
		t.Fatalf("payload missing checks array; payload=%s", data)
	}
	if len(checks) != 2 {
		t.Errorf("checks len: got %d, want 2", len(checks))
	}
	first, ok := checks[0].(map[string]any)
	if !ok {
		t.Fatalf("checks[0] not a map: %T", checks[0])
	}
	if first["runtime"] != "process-local-testcontainer" {
		t.Errorf("checks[0].runtime: got %v", first["runtime"])
	}
	if first["test_harness"] != "nats-jetstream" {
		t.Errorf("checks[0].test_harness: got %v", first["test_harness"])
	}
}

func TestExecute_WithoutChecks_PayloadOmitsField(t *testing.T) {
	exec, _, pub, _ := newExecutorWithDir(t)
	_, err := exec.Execute(context.Background(), defaultCall(defaultArtifactArgs()))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	_, data, _ := pub.snapshot()
	if got := string(data); strings.Contains(got, `"checks"`) {
		t.Errorf("expected checks to be omitted from payload; payload=%s", got)
	}
}

// ---------------------------------------------------------------------
// R3.7.2.h′ — harness resolver projects Image + TCP exposes into SPEC.md
// ---------------------------------------------------------------------

// TestRenderMarkdown_ResolvedTestHarnessFieldsRendered pins the
// trust-boundary fix the reviewer caught: without resolved fields in
// SPEC.md the builder has no in-sandbox path to look up the catalog.
// With the resolver wired, the rendered markdown carries `**Image**`
// and `**TCP exposes**` lines next to `**Test harness**` for every C
// whose test_harness resolves; the builder transcribes them verbatim
// into test code (configs/personas/fragments/dev-via-spec-builder/
// 15-commitment-driven-authoring.md).
func TestRenderMarkdown_ResolvedTestHarnessFieldsRendered(t *testing.T) {
	tmpDir := t.TempDir()
	tp := &fakeTriplePublisher{}
	pub := &fakePublisher{}
	resolver := func(_ context.Context, name string) (*testharness.TestHarness, error) {
		if name == "meshtasticd-3.x" {
			return &testharness.TestHarness{
				Name:                "meshtasticd-3.x",
				Image:               "meshtastic/meshtasticd:3.5.0",
				SmokeContractSchema: "meshtasticd.smoke_contract.v1",
				DomainDescription:   "test fixture",
				Exposes: testharness.Exposes{
					TCP: []testharness.PortExpose{{Port: 4403, Protocol: "meshtastic-protobuf"}},
				},
			}, nil
		}
		return nil, errors.New("not in catalog")
	}
	exec := NewExecutor(tp, pub, types.PlatformMeta{Org: "c360", Platform: "semteams"}, nil, tmpDir, resolver)

	args := defaultArtifactArgs()
	args["checks"] = []any{
		map[string]any{
			"target":       "driver emits CS API observation when meshtasticd publishes POSITION_APP",
			"runtime":      "process-local-testcontainer",
			"test_harness": "meshtasticd-3.x",
			"test_runtime": "java-junit-testcontainers",
			"ref": map[string]any{
				"type": "filepath",
				"path": "src/test/java/com/example/FooIT.java",
			},
		},
	}

	res, err := exec.Execute(context.Background(), defaultCall(args))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty", res.Error)
	}

	// Find the rendered file.
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("read tmpDir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly one rendered file, got %d", len(entries))
	}
	body, err := os.ReadFile(filepath.Join(tmpDir, entries[0].Name()))
	if err != nil {
		t.Fatalf("read rendered: %v", err)
	}
	got := string(body)

	for _, want := range []string{
		"**Test harness**: `meshtasticd-3.x`",
		"**Image**: `meshtastic/meshtasticd:3.5.0`",
		"**TCP exposes**: port 4403 (`meshtastic-protobuf`)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered SPEC.md missing %q\nfull body:\n%s", want, got)
		}
	}
}

// TestRenderMarkdown_ResolverMiss_NameOnly pins the fallback shape:
// when the resolver returns an error, the rendered SPEC carries the
// architect's test_harness NAME without the resolved Image / TCP fields.
// The builder's contract reads this as a chain gap and surfaces
// needs_clarification rather than fabricating; here we just verify
// the renderer doesn't blow up and the name still ships.
func TestRenderMarkdown_ResolverMiss_NameOnly(t *testing.T) {
	tmpDir := t.TempDir()
	tp := &fakeTriplePublisher{}
	pub := &fakePublisher{}
	resolver := func(_ context.Context, _ string) (*testharness.TestHarness, error) {
		return nil, errors.New("simulated catalog miss")
	}
	exec := NewExecutor(tp, pub, types.PlatformMeta{Org: "c360", Platform: "semteams"}, nil, tmpDir, resolver)

	args := defaultArtifactArgs()
	args["checks"] = []any{
		map[string]any{
			"target":       "x",
			"runtime":      "process-local-testcontainer",
			"test_harness": "ghost-harness",
			"test_runtime": "go-testing-net",
			"ref": map[string]any{
				"type": "filepath",
				"path": "x_test.go",
			},
		},
	}
	res, err := exec.Execute(context.Background(), defaultCall(args))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty", res.Error)
	}

	entries, _ := os.ReadDir(tmpDir)
	body, _ := os.ReadFile(filepath.Join(tmpDir, entries[0].Name()))
	got := string(body)
	if !strings.Contains(got, "**Test harness**: `ghost-harness`") {
		t.Errorf("expected test_harness name to render even on resolver miss; body:\n%s", got)
	}
	if strings.Contains(got, "**Image**:") {
		t.Errorf("expected NO **Image** line on resolver miss; body:\n%s", got)
	}
	if strings.Contains(got, "**TCP exposes**:") {
		t.Errorf("expected NO **TCP exposes** line on resolver miss; body:\n%s", got)
	}
}

func TestExecute_BadCheck_FailsValidation(t *testing.T) {
	exec, _, _, _ := newExecutorWithDir(t)
	args := defaultArtifactArgs()
	// testcontainer runtime without test_harness → Validate cascade rejects
	args["checks"] = []any{
		map[string]any{
			"target":       "x",
			"runtime":      "process-local-testcontainer",
			"test_runtime": "go-testing-net",
			"ref": map[string]any{
				"type": "filepath",
				"path": "x_test.go",
			},
		},
	}
	res, err := exec.Execute(context.Background(), defaultCall(args))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error == "" {
		t.Fatal("expected validation error from invalid check, got nil")
	}
	if !strings.Contains(res.Error, "checks[0]") {
		t.Errorf("error should name the offending check index, got %q", res.Error)
	}
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind: got %v, want ToolErrorInvalidArgs", res.ErrorKind)
	}
}

func TestExecute_WithChecks_MarkdownContainsSection(t *testing.T) {
	exec, _, _, dir := newExecutorWithDir(t)
	res, err := exec.Execute(context.Background(), defaultCall(argsWithChecks()))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty", res.Error)
	}
	// Find the rendered file (slug is dynamic; one .md file in dir).
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var mdPath string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") {
			mdPath = filepath.Join(dir, e.Name())
			break
		}
	}
	if mdPath == "" {
		t.Fatal("no rendered markdown file found")
	}
	body, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(body)
	for _, want := range []string{
		"## Verification Checks",
		"C1 — executor publishes graph.mutation.entity.add",
		"`process-local-testcontainer`",
		"`nats-jetstream`",
		"`go-testing-net`",
		"`test_uses_build_tag`",
		"C2 — executor returns ToolErrorInvalidArgs",
		"`in-process-unit`",
		"_none — reviewer may flag as under-specified_",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("markdown missing %q\n--- got ---\n%s", want, rendered)
		}
	}
}

func TestExecute_WithoutChecks_MarkdownShowsReviewerHint(t *testing.T) {
	exec, _, _, dir := newExecutorWithDir(t)
	_, err := exec.Execute(context.Background(), defaultCall(defaultArtifactArgs()))
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var mdPath string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") {
			mdPath = filepath.Join(dir, e.Name())
			break
		}
	}
	body, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(body)
	if !strings.Contains(rendered, "## Verification Checks") {
		t.Errorf("markdown missing section header")
	}
	if !strings.Contains(rendered, "No verification checks emitted") {
		t.Errorf("markdown should call out missing checks for reviewer attention")
	}
}
