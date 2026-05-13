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

	"github.com/c360studio/semteams/cmd/semteams/chain"
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

func (f *fakeTriplePublisher) AddTriplesBatch(ctx context.Context, triples []message.Triple) error {
	for _, t := range triples {
		if err := f.AddTriple(ctx, t); err != nil {
			return err
		}
	}
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

	// Per-check schema: evidence MUST be in the check's `required`
	// list. Smoke #8 run-8 fix — the check-level schema and
	// Validate must agree, otherwise the LLM sees the schema
	// say "optional" and the tool reject "required" (contradictory
	// signals trigger Goodhart-y retry loops).
	checksProp, ok := props["checks"].(map[string]any)
	if !ok {
		t.Fatalf("schema missing checks property")
	}
	checkItem, ok := checksProp["items"].(map[string]any)
	if !ok {
		t.Fatalf("checks.items missing or wrong type")
	}
	checkRequired, _ := checkItem["required"].([]string)
	checkRequiredSet := map[string]bool{}
	for _, r := range checkRequired {
		checkRequiredSet[r] = true
	}
	if !checkRequiredSet["evidence"] {
		t.Errorf("check.evidence must be in per-check `required` (have %v)", checkRequired)
	}
	// Same per-check assertion for the rest of the always-required fields.
	for _, want := range []string{"target", "runtime", "ref"} {
		if !checkRequiredSet[want] {
			t.Errorf("check.%s must be in per-check `required` (have %v)", want, checkRequired)
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
		"planner_loop":  "loop-planner-001",
		"reviewer_loop": "loop-reviewer-001",
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

// Slug derivation is exercised at the helper level in
// cmd/semteams/slug/slug_test.go (the prior local TestDeriveSlug
// tests retired here when the helper was extracted in the PR C
// cleanup). The integration tests below — TestExecute_DegenerateTitle_Rejected
// and the various happy-path tests — keep covering the
// emitspecartifact-specific HasSuffix("-") rejection contract.

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
			"evidence": []any{
				map[string]any{"kind": "test_file_exists", "args": map[string]any{"path": "cmd/semteams/tools/x/executor_test.go"}},
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
// into test code (configs/personas/fragments/builder/
// 15-commitment-driven-authoring.md).
func TestRenderMarkdown_ResolvedTestHarnessFieldsRendered(t *testing.T) {
	tmpDir := t.TempDir()
	tp := &fakeTriplePublisher{}
	pub := &fakePublisher{}
	resolver := func(_ context.Context, name string) (*testharness.TestHarness, error) {
		if name == "meshtasticd-2.x" {
			return &testharness.TestHarness{
				Name:                "meshtasticd-2.x",
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
			"test_harness": "meshtasticd-2.x",
			"test_runtime": "java-junit-testcontainers",
			"ref": map[string]any{
				"type": "filepath",
				"path": "src/test/java/com/example/FooIT.java",
			},
			"evidence": []any{
				map[string]any{"kind": "test_file_exists", "args": map[string]any{"path": "src/test/java/com/example/FooIT.java"}},
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

	// Find the rendered .md file (sidecar .checks.json is also written).
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("read tmpDir: %v", err)
	}
	var mdPath string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") {
			mdPath = filepath.Join(tmpDir, e.Name())
			break
		}
	}
	if mdPath == "" {
		t.Fatalf("no .md file found among %d entries in tmpDir", len(entries))
	}
	body, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("read rendered: %v", err)
	}
	got := string(body)

	for _, want := range []string{
		"**Test harness**: `meshtasticd-2.x`",
		"**Image**: `meshtastic/meshtasticd:3.5.0`",
		"**TCP exposes**: port 4403 (`meshtastic-protobuf`)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered SPEC.md missing %q\nfull body:\n%s", want, got)
		}
	}
}

// TestRenderMarkdown_NilResolver_NameOnly pins the fallback shape:
// when no resolver is wired (nil), the rendered SPEC carries the
// architect's test_harness NAME without resolved Image / TCP fields,
// and no sidecar is written. The builder's contract reads the name-only
// form as a chain gap and surfaces needs_clarification rather than
// fabricating a harness image.
//
// Note: when a resolver IS wired and fails, the tool now returns
// ToolErrorInvalidArgs (ADR-036 Phase 1 fatal contract). The name-only
// fallback applies only when the resolver is nil (no test-harness
// manager wired by the operator).
func TestRenderMarkdown_NilResolver_NameOnly(t *testing.T) {
	tmpDir := t.TempDir()
	tp := &fakeTriplePublisher{}
	pub := &fakePublisher{}
	// nil resolver — no manager wired; test_harness name ships without projection.
	exec := NewExecutor(tp, pub, types.PlatformMeta{Org: "c360", Platform: "semteams"}, nil, tmpDir, nil)

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
			"evidence": []any{
				map[string]any{"kind": "test_file_exists", "args": map[string]any{"path": "x_test.go"}},
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

	// Sidecar is written with checks but no harnesses (nil resolver → empty
	// harnesses map → omitted from JSON by omitempty).
	entries, _ := os.ReadDir(tmpDir)
	var mdPath string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") {
			mdPath = filepath.Join(tmpDir, e.Name())
			break
		}
	}
	if mdPath == "" {
		t.Fatalf("no .md file found")
	}
	body, _ := os.ReadFile(mdPath)
	got := string(body)
	if !strings.Contains(got, "**Test harness**: `ghost-harness`") {
		t.Errorf("expected test_harness name to render on nil resolver; body:\n%s", got)
	}
	if strings.Contains(got, "**Image**:") {
		t.Errorf("expected NO **Image** line with nil resolver; body:\n%s", got)
	}
	if strings.Contains(got, "**TCP exposes**:") {
		t.Errorf("expected NO **TCP exposes** line with nil resolver; body:\n%s", got)
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
		// Per smoke #8 run-8, every check now carries ≥1 evidence
		// rule (Validate enforces). C2's evidence kind appears in
		// the rendered markdown for the same reason C1's does.
		"`test_file_exists`",
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

// ---------------------------------------------------------------------
// ADR-036 Phase 1 — sidecar (<slug>.checks.json) tests
// ---------------------------------------------------------------------

// newResolverFor builds a TestHarnessResolver that returns the supplied
// harness for its name and an error for any other name.
func newResolverFor(harnesses ...*testharness.TestHarness) TestHarnessResolver {
	m := make(map[string]*testharness.TestHarness, len(harnesses))
	for _, h := range harnesses {
		m[h.Name] = h
	}
	return func(_ context.Context, name string) (*testharness.TestHarness, error) {
		if h, ok := m[name]; ok {
			return h, nil
		}
		return nil, errors.New("not in catalog")
	}
}

// fixtureHarness returns a minimal well-formed TestHarness for testing.
func fixtureHarness(name, image string, port int) *testharness.TestHarness {
	return &testharness.TestHarness{
		Name:                name,
		Image:               image,
		SmokeContractSchema: name + ".smoke.v1",
		DomainDescription:   "test fixture for " + name,
		Exposes: testharness.Exposes{
			TCP: []testharness.PortExpose{{Port: port, Protocol: "tcp-" + name}},
		},
	}
}

// readSidecar reads and unmarshals the <slug>.checks.json sidecar from dir.
func readSidecar(t *testing.T, dir, slug string) SidecarPayload {
	t.Helper()
	path := filepath.Join(dir, slug+".checks.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read sidecar %s: %v", path, err)
	}
	var s SidecarPayload
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("unmarshal sidecar: %v", err)
	}
	return s
}

// getSlugFromResult extracts "slug" from the tool result Content JSON.
func getSlugFromResult(t *testing.T, res agentic.ToolResult) string {
	t.Helper()
	var c struct {
		Slug string `json:"slug"`
	}
	if err := json.Unmarshal([]byte(res.Content), &c); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	return c.Slug
}

// TestSidecar_EmptyChecks_NoSidecarWritten pins that no .checks.json
// is written when the artifact carries no checks. Preserves
// existing test expectations (dir has exactly one .md file).
func TestSidecar_EmptyChecks_NoSidecarWritten(t *testing.T) {
	exec, _, _, tmpDir := newExecutorWithDir(t)
	res, err := exec.Execute(context.Background(), defaultCall(defaultArtifactArgs()))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q", res.Error)
	}
	entries, _ := os.ReadDir(tmpDir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".checks.json") {
			t.Errorf("expected no .checks.json with empty checks; found %s", e.Name())
		}
	}
}

// TestSidecar_WithChecksOneHarnessRef verifies the sidecar is written
// with checks[] and a harnesses map with the single resolved entry.
func TestSidecar_WithChecksOneHarnessRef(t *testing.T) {
	tmpDir := t.TempDir()
	tp := &fakeTriplePublisher{}
	pub := &fakePublisher{}
	h := fixtureHarness("meshtasticd-2.x", "meshtastic/meshtasticd:3.5.0", 4403)
	resolver := newResolverFor(h)
	exec := NewExecutor(tp, pub, types.PlatformMeta{Org: "c360", Platform: "semteams"}, nil, tmpDir, resolver)

	args := argsWithOneTestcontainerCheck("meshtasticd-2.x")
	res, err := exec.Execute(context.Background(), defaultCall(args))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q", res.Error)
	}

	slug := getSlugFromResult(t, res)
	sidecar := readSidecar(t, tmpDir, slug)

	if len(sidecar.Checks) != 1 {
		t.Errorf("sidecar.Checks len = %d, want 1", len(sidecar.Checks))
	}
	if len(sidecar.Harnesses) != 1 {
		t.Errorf("sidecar.Harnesses len = %d, want 1", len(sidecar.Harnesses))
	}
	resolved, ok := sidecar.Harnesses["meshtasticd-2.x"]
	if !ok {
		t.Fatalf("sidecar.Harnesses missing meshtasticd-2.x; keys=%v", harnesseKeys(sidecar.Harnesses))
	}
	if resolved.Image != "meshtastic/meshtasticd:3.5.0" {
		t.Errorf("resolved.Image = %q, want meshtastic/meshtasticd:3.5.0", resolved.Image)
	}
	if resolved.ID != "meshtasticd-2.x" {
		t.Errorf("resolved.ID = %q, want meshtasticd-2.x", resolved.ID)
	}
	if len(resolved.Ports) != 1 {
		t.Errorf("resolved.Ports len = %d, want 1", len(resolved.Ports))
	}
}

// TestSidecar_TwoDistinctHarnessRefs verifies the harnesses map has two
// entries when two distinct test_harness IDs appear across the checks.
func TestSidecar_TwoDistinctHarnessRefs(t *testing.T) {
	tmpDir := t.TempDir()
	tp := &fakeTriplePublisher{}
	pub := &fakePublisher{}
	h1 := fixtureHarness("harness-a", "img-a:1.0", 1234)
	h2 := fixtureHarness("harness-b", "img-b:2.0", 5678)
	resolver := newResolverFor(h1, h2)
	exec := NewExecutor(tp, pub, types.PlatformMeta{Org: "c360", Platform: "semteams"}, nil, tmpDir, resolver)

	args := argsWithTwoTestcontainerChecks("harness-a", "harness-b")
	res, err := exec.Execute(context.Background(), defaultCall(args))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q", res.Error)
	}

	slug := getSlugFromResult(t, res)
	sidecar := readSidecar(t, tmpDir, slug)

	if len(sidecar.Harnesses) != 2 {
		t.Errorf("sidecar.Harnesses len = %d, want 2", len(sidecar.Harnesses))
	}
	if _, ok := sidecar.Harnesses["harness-a"]; !ok {
		t.Errorf("sidecar.Harnesses missing harness-a")
	}
	if _, ok := sidecar.Harnesses["harness-b"]; !ok {
		t.Errorf("sidecar.Harnesses missing harness-b")
	}
}

// TestSidecar_DuplicateHarnessRefs verifies duplicate test_harness refs
// across checks are deduplicated to a single harnesses map entry.
func TestSidecar_DuplicateHarnessRefs(t *testing.T) {
	tmpDir := t.TempDir()
	tp := &fakeTriplePublisher{}
	pub := &fakePublisher{}
	h := fixtureHarness("same-harness", "img:1.0", 9000)
	resolver := newResolverFor(h)
	exec := NewExecutor(tp, pub, types.PlatformMeta{Org: "c360", Platform: "semteams"}, nil, tmpDir, resolver)

	// Two checks referencing the same test_harness.
	args := defaultArtifactArgs()
	args["checks"] = []any{
		testcontainerCheck("same-harness", "target A"),
		testcontainerCheck("same-harness", "target B"),
	}
	res, err := exec.Execute(context.Background(), defaultCall(args))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q", res.Error)
	}

	slug := getSlugFromResult(t, res)
	sidecar := readSidecar(t, tmpDir, slug)

	if len(sidecar.Checks) != 2 {
		t.Errorf("sidecar.Checks len = %d, want 2", len(sidecar.Checks))
	}
	if len(sidecar.Harnesses) != 1 {
		t.Errorf("sidecar.Harnesses len = %d, want 1 (deduped)", len(sidecar.Harnesses))
	}
}

// TestSidecar_InProcessUnitCheck_NoHarnesEntry verifies that a check with
// runtime=in-process-unit (no test_harness) produces a sidecar with
// checks but an empty/nil harnesses map.
func TestSidecar_InProcessUnitCheck_NoHarnessEntry(t *testing.T) {
	tmpDir := t.TempDir()
	tp := &fakeTriplePublisher{}
	pub := &fakePublisher{}
	// Resolver present but never called (in-process-unit has no test_harness).
	resolver := newResolverFor()
	exec := NewExecutor(tp, pub, types.PlatformMeta{Org: "c360", Platform: "semteams"}, nil, tmpDir, resolver)

	args := defaultArtifactArgs()
	args["checks"] = []any{
		map[string]any{
			"target":  "unit logic correctness",
			"runtime": "in-process-unit",
			"ref":     map[string]any{"type": "filepath", "path": "pkg/foo_test.go"},
			"evidence": []any{
				map[string]any{"kind": "test_file_exists", "args": map[string]any{"path": "pkg/foo_test.go"}},
			},
		},
	}
	res, err := exec.Execute(context.Background(), defaultCall(args))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q", res.Error)
	}

	slug := getSlugFromResult(t, res)
	sidecar := readSidecar(t, tmpDir, slug)

	if len(sidecar.Checks) != 1 {
		t.Errorf("sidecar.Checks len = %d, want 1", len(sidecar.Checks))
	}
	if len(sidecar.Harnesses) != 0 {
		t.Errorf("sidecar.Harnesses = %v, want empty (in-process-unit has no test_harness)", sidecar.Harnesses)
	}
}

// TestSidecar_UnknownCatalogID_ToolErrorInvalidArgs verifies that a
// resolver error (unknown catalog ID) aborts execution before any triples
// or files are written, returning ToolErrorInvalidArgs with the catalog ID.
func TestSidecar_UnknownCatalogID_ToolErrorInvalidArgs(t *testing.T) {
	tmpDir := t.TempDir()
	tp := &fakeTriplePublisher{}
	pub := &fakePublisher{}
	// Resolver that always returns an error.
	resolver := newResolverFor() // empty — any lookup fails
	exec := NewExecutor(tp, pub, types.PlatformMeta{Org: "c360", Platform: "semteams"}, nil, tmpDir, resolver)

	args := argsWithOneTestcontainerCheck("unknown-harness-id")
	res, err := exec.Execute(context.Background(), defaultCall(args))
	if err != nil {
		t.Fatalf("Execute err = %v, want nil", err)
	}

	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want ToolErrorInvalidArgs", res.ErrorKind)
	}
	if !strings.Contains(res.Error, "unknown-harness-id") {
		t.Errorf("error should name the failing catalog ID; got %q", res.Error)
	}

	// No triples published (short-circuited before triple mint).
	if got := tp.snapshot(); len(got) != 0 {
		t.Errorf("expected 0 triples on catalog-miss, got %d", len(got))
	}
	// No payload published.
	if pub.calls != 0 {
		t.Errorf("expected 0 payload publish calls, got %d", pub.calls)
	}

	// No .checks.json written (sidecar fails before write).
	entries, _ := os.ReadDir(tmpDir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".checks.json") {
			t.Errorf("expected no .checks.json on catalog-miss; found %s", e.Name())
		}
	}
}

// TestSidecar_IsHumanDiffable verifies the sidecar is indented JSON
// (json.MarshalIndent output, not compact).
func TestSidecar_IsHumanDiffable(t *testing.T) {
	tmpDir := t.TempDir()
	tp := &fakeTriplePublisher{}
	pub := &fakePublisher{}
	h := fixtureHarness("neat-harness", "img:latest", 8080)
	resolver := newResolverFor(h)
	exec := NewExecutor(tp, pub, types.PlatformMeta{Org: "c360", Platform: "semteams"}, nil, tmpDir, resolver)

	args := argsWithOneTestcontainerCheck("neat-harness")
	res, err := exec.Execute(context.Background(), defaultCall(args))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q", res.Error)
	}

	slug := getSlugFromResult(t, res)
	path := filepath.Join(tmpDir, slug+".checks.json")
	raw, _ := os.ReadFile(path)

	// Indented JSON has newlines; compact JSON does not.
	if !strings.Contains(string(raw), "\n") {
		t.Errorf("sidecar should be indented (human-diffable); got compact: %s", raw)
	}
}

// TestSidecar_IdempotentOverwrite verifies re-running with the same slug
// overwrites the sidecar with identical content.
func TestSidecar_IdempotentOverwrite(t *testing.T) {
	tmpDir := t.TempDir()
	tp := &fakeTriplePublisher{}
	pub := &fakePublisher{}
	h := fixtureHarness("idem-harness", "img:1.0", 7777)
	resolver := newResolverFor(h)
	exec := NewExecutor(tp, pub, types.PlatformMeta{Org: "c360", Platform: "semteams"}, nil, tmpDir, resolver)

	args := argsWithOneTestcontainerCheck("idem-harness")

	for i := range 2 {
		res, err := exec.Execute(context.Background(), defaultCall(args))
		if err != nil {
			t.Fatalf("run %d Execute err: %v", i, err)
		}
		if res.Error != "" {
			t.Fatalf("run %d Result.Error = %q", i, res.Error)
		}
	}

	// Confirm only one .checks.json in the dir (no accumulation).
	entries, _ := os.ReadDir(tmpDir)
	var sidecarCount int
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".checks.json") {
			sidecarCount++
		}
	}
	if sidecarCount != 1 {
		t.Errorf("expected exactly 1 .checks.json after 2 runs, got %d", sidecarCount)
	}
}

// ---------------------------------------------------------------------
// Sidecar test helpers
// ---------------------------------------------------------------------

func testcontainerCheck(harnessName, target string) map[string]any {
	return map[string]any{
		"target":       target,
		"runtime":      "process-local-testcontainer",
		"test_harness": harnessName,
		"test_runtime": "go-testing-net",
		"ref":          map[string]any{"type": "filepath", "path": "x_test.go"},
		// Evidence is required per the smoke #8 run-8 fix —
		// verification.Check.Validate rejects checks with no rules
		// because qa-reviewer (R3.7.2.j′) cannot render a verdict
		// without one.
		"evidence": []any{
			map[string]any{"kind": "test_uses_build_tag", "args": map[string]any{"tag": "integration"}},
		},
	}
}

func argsWithOneTestcontainerCheck(harnessName string) map[string]any {
	args := defaultArtifactArgs()
	args["checks"] = []any{testcontainerCheck(harnessName, "integration surface verified")}
	return args
}

func argsWithTwoTestcontainerChecks(h1, h2 string) map[string]any {
	args := defaultArtifactArgs()
	args["checks"] = []any{
		testcontainerCheck(h1, "check for "+h1),
		testcontainerCheck(h2, "check for "+h2),
	}
	return args
}

func harnesseKeys(m map[string]testharness.ResolvedManifest) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// ---------------------------------------------------------------------
// Chain entity triple emission (ADR-038 PR B Phase 4)
// ---------------------------------------------------------------------

// fakeChainResolver implements ChainResolver for tests. Returns the
// configured entity ID (or err if non-nil) for any loopID.
type fakeChainResolver struct {
	entityID string
	err      error
	calls    int
	lastArg  string
}

func (f *fakeChainResolver) ChainEntityID(_ context.Context, loopID string) (string, error) {
	f.calls++
	f.lastArg = loopID
	if f.err != nil {
		return "", f.err
	}
	return f.entityID, nil
}

// chainTriplesBySubject filters a triple slice to those subject-matching
// the given chain entity ID, returning a predicate→object map for
// assertion brevity.
func chainTriplesBySubject(triples []message.Triple, chainEntityID string) map[string]any {
	out := map[string]any{}
	for _, t := range triples {
		if t.Subject == chainEntityID {
			out[t.Predicate] = t.Object
		}
	}
	return out
}

func TestExecute_ChainTriples_NoResolver_LoopEntityOnly(t *testing.T) {
	// Default executor leaves chainResolver unset → chain triples are
	// skipped, loop-entity triples land as before. Backward-compat
	// guarantee for any caller that doesn't opt in.
	exec, tp, _, _ := newExecutorWithDir(t)
	_, err := exec.Execute(context.Background(), defaultCall(defaultArtifactArgs()))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	for _, tr := range tp.snapshot() {
		if tr.Predicate == chainPredicateSpecArtifactLoop ||
			tr.Predicate == chainPredicateSpecArtifactPath ||
			tr.Predicate == chainPredicateSpecArtifactCheckCount {
			t.Errorf("chain triple %q must not be written without chainResolver", tr.Predicate)
		}
	}
}

func TestExecute_ChainTriples_HappyPath(t *testing.T) {
	exec, tp, _, _ := newExecutorWithDir(t)
	const wantChainEntityID = "c360.semteams.agent.chain.execution.dispatch_xyz"
	resolver := &fakeChainResolver{entityID: wantChainEntityID}
	exec.SetChainResolver(resolver)

	args := argsWithOneTestcontainerCheck("meshtasticd-2.x")
	_, err := exec.Execute(context.Background(), defaultCall(args))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}

	if resolver.calls != 1 {
		t.Errorf("ChainEntityID calls = %d, want 1", resolver.calls)
	}
	if resolver.lastArg != "loop-research-001" {
		t.Errorf("ChainEntityID called with %q, want loop-research-001 (provenance.research_artifact_loop)", resolver.lastArg)
	}

	chainTriples := chainTriplesBySubject(tp.snapshot(), wantChainEntityID)
	if got := chainTriples[chainPredicateSpecArtifactLoop]; got != "loop-architect-abc" {
		t.Errorf("%s = %v, want loop-architect-abc (architect's loop_id)", chainPredicateSpecArtifactLoop, got)
	}
	if path, ok := chainTriples[chainPredicateSpecArtifactPath].(string); !ok || path == "" {
		t.Errorf("%s missing or empty: %v", chainPredicateSpecArtifactPath, chainTriples[chainPredicateSpecArtifactPath])
	}
	if got := chainTriples[chainPredicateSpecArtifactCheckCount]; got != 1 {
		t.Errorf("%s = %v, want 1 (one testcontainer check)", chainPredicateSpecArtifactCheckCount, got)
	}

	// Loop-entity triples must still land on the architect's loop entity —
	// drift-safe phasing per ADR-038 D5; PR D removes them later.
	wantLoopEntityID := "c360.semteams.agent.agentic-loop.execution.loop-architect-abc"
	loopTriples := chainTriplesBySubject(tp.snapshot(), wantLoopEntityID)
	if loopTriples[predicatePath] == nil {
		t.Errorf("loop-entity triple %s missing — drift-safe phasing requires both subject sets to land", predicatePath)
	}
}

func TestExecute_ChainTriples_ResolverError_LoopEntityStillLands(t *testing.T) {
	exec, tp, _, _ := newExecutorWithDir(t)
	resolver := &fakeChainResolver{err: errors.New("graph KV unavailable")}
	exec.SetChainResolver(resolver)

	res, err := exec.Execute(context.Background(), defaultCall(defaultArtifactArgs()))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Errorf("Tool result error = %q; chain failure must be fail-soft, not surface to LLM", res.Error)
	}

	for _, tr := range tp.snapshot() {
		if tr.Predicate == chainPredicateSpecArtifactLoop {
			t.Errorf("chain triple %q must not be written when resolver errors", tr.Predicate)
		}
	}
	wantLoopEntityID := "c360.semteams.agent.agentic-loop.execution.loop-architect-abc"
	loopTriples := chainTriplesBySubject(tp.snapshot(), wantLoopEntityID)
	if loopTriples[predicatePath] == nil {
		t.Errorf("loop-entity triple %s must still land when chain resolver fails", predicatePath)
	}
}

func TestExecute_ChainTriples_EmptyResearchRootLoop_Skipped(t *testing.T) {
	exec, _, _, _ := newExecutorWithDir(t)
	resolver := &fakeChainResolver{entityID: "c360.semteams.agent.chain.execution.x"}
	exec.SetChainResolver(resolver)

	// Emit an artifact with provenance.research_artifact_loop empty.
	// The architect persona is supposed to populate this, but a contract
	// violation should be fail-soft on the chain side. Loop-entity
	// triples still land (the existing dev_via_spec.artifact.research_root_loop
	// triple will be empty, which downstream review picks up).
	args := defaultArtifactArgs()
	args["provenance"] = map[string]any{
		"research_artifact_loop": "",
		"planner_loop":           "loop-planner-001",
	}
	_, err := exec.Execute(context.Background(), defaultCall(args))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if resolver.calls != 0 {
		t.Errorf("resolver should not be called when research_artifact_loop is empty; got %d calls", resolver.calls)
	}
}

// ---------------------------------------------------------------------
// Chain-canonical lineage + slug overrides (smoke #8 run-5 D1 + D2 fix)
// ---------------------------------------------------------------------

// stubChainReader implements ChainReader for tests. Returns the
// configured (entityID, triples) pair to every ReadChainFor call;
// errors land via err. Mirrors the same shape as emitplan / emitconsensus.
type stubChainReader struct {
	entityID string
	triples  map[string]any
	err      error
	calls    int
	mu       sync.Mutex
}

func (s *stubChainReader) ReadChainFor(_ context.Context, _ string) (string, map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.err != nil {
		return s.entityID, nil, s.err
	}
	return s.entityID, s.triples, nil
}

// findTripleOnLoopEntity returns the object value for the named
// predicate stamped on the architect's loop entity (the
// dev_via_spec.artifact.* cluster). Used to read back the actual
// rendered slug since the renderer writes it to disk + the predicateSlug
// triple.
func findTripleOnLoopEntity(triples []message.Triple, loopEntityID, predicate string) (any, bool) {
	for _, t := range triples {
		if t.Subject == loopEntityID && t.Predicate == predicate {
			return t.Object, true
		}
	}
	return nil, false
}

// spyChainReader records the loop ID it was invoked with — used to
// pin the smoke #8 run-6 hotfix that the chain ancestry walk anchors
// on a completed-ancestor metadata key, not the running call.LoopID.
type spyChainReader struct {
	entityID   string
	triples    map[string]any
	lastLoopID string
	mu         sync.Mutex
}

func (s *spyChainReader) ReadChainFor(_ context.Context, loopID string) (string, map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastLoopID = loopID
	return s.entityID, s.triples, nil
}

// TestExecute_ChainAnchorsOnPriorLoopMetadata pins the smoke #8
// run-6 hotfix at the architect seam: emit-tools MUST anchor the
// chain walk on a completed ancestor's loop_id from task-property
// metadata, not the running loop's own LoopID. The architect spawn
// rule (rules/dev-via-spec/05-challenger-accept-to-architect.json)
// already sets `related_loops: { "researcher": ... }` — the
// researcher loop is a completed-ancestor with stamped
// agent.loop.parent triples.
func TestExecute_ChainAnchorsOnRelatedLoopsResearcher(t *testing.T) {
	exec, _, _, _ := newExecutorWithDir(t)
	stub := &spyChainReader{
		entityID: "c360.semteams.agent.chain.execution.dispatch_root",
		triples:  map[string]any{},
	}
	exec.SetChainReader(stub)

	call := defaultCall(defaultArtifactArgs())
	call.Metadata = map[string]any{
		agentic.MetadataKeyRelatedLoops: map[string]any{
			"researcher": "researcher-completed",
		},
	}
	if _, err := exec.Execute(context.Background(), call); err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if stub.lastLoopID != "researcher-completed" {
		t.Errorf("ReadChainFor lastLoopID = %q, want metadata anchor 'researcher-completed' (running LoopID %q must NOT be used when related_loops.researcher is set)", stub.lastLoopID, call.LoopID)
	}
}

// TestExecute_ChainAnchorFallsBackToCallLoopID pins fallback when no
// metadata anchor is set — preserves pre-hotfix behaviour for tests
// and deployments without the spawn convention.
func TestExecute_ChainAnchorFallsBackToCallLoopID(t *testing.T) {
	exec, _, _, _ := newExecutorWithDir(t)
	stub := &spyChainReader{
		entityID: "c360.semteams.agent.chain.execution.dispatch_root",
		triples:  map[string]any{},
	}
	exec.SetChainReader(stub)

	call := defaultCall(defaultArtifactArgs())
	if _, err := exec.Execute(context.Background(), call); err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if stub.lastLoopID != call.LoopID {
		t.Errorf("ReadChainFor lastLoopID = %q, want call.LoopID %q (fallback)", stub.lastLoopID, call.LoopID)
	}
}

// TestExecute_ChainSlugStemOverridesTitleSlug pins the smoke #8 run-5
// D2 fix: when chain.slug.stem is present, the rendered file uses
// "<stem>-implementation" instead of the title-derived slug. Closes
// the cross-arc slug drift the architect inherited from the
// challenger.
func TestExecute_ChainSlugStemOverridesTitleSlug(t *testing.T) {
	exec, tp, _, dir := newExecutorWithDir(t)
	exec.SetChainReader(&stubChainReader{
		entityID: "c360.semteams.agent.chain.execution.dispatch_root",
		triples: map[string]any{
			chain.PredicateSlugStem: "2026-05-09-osh-meshtastic-driver",
		},
	})

	args := defaultArtifactArgs()
	// Architect picked up the challenger's drifted title.
	args["title"] = "OSH Meshtastic IDriver Implementation"

	res, err := exec.Execute(context.Background(), defaultCall(args))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty", res.Error)
	}

	// Look for the rendered file directly on disk — its name carries
	// the slug.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var foundMD string
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".md" {
			foundMD = e.Name()
			break
		}
	}
	const wantName = "2026-05-09-osh-meshtastic-driver-implementation.md"
	if foundMD != wantName {
		t.Errorf("rendered .md filename = %q, want %q (chain.slug.stem must override drifting title)", foundMD, wantName)
	}

	// Also confirm the slug triple on the loop entity reflects the
	// chain-derived value (downstream consumers — bootstrapworkspace,
	// evidence preprocessor — read this too).
	loopEntityID := agentic.LoopExecutionEntityID("c360", "semteams", "loop-architect-abc")
	gotSlug, ok := findTripleOnLoopEntity(tp.snapshot(), loopEntityID, predicateSlug)
	if !ok {
		t.Fatal("predicateSlug triple missing on loop entity")
	}
	if gotSlug != "2026-05-09-osh-meshtastic-driver-implementation" {
		t.Errorf("predicateSlug triple = %v, want chain-derived stem-implementation", gotSlug)
	}
}

// TestExecute_ChainProvenanceOverridesLLMValues pins the smoke #8 run-5
// D1 fix: chain.{research_artifact_loop, plan_loop, plan_reviewer_loop}
// must override LLM-supplied provenance.* loop_ids. Smoke #8 run-5
// evidence: architect cited a downstream loop as the "approved planner
// loop"; chain entity has the right loop IDs distinct, and the override
// propagates them into the rendered spec.
func TestExecute_ChainProvenanceOverridesLLMValues(t *testing.T) {
	exec, tp, _, dir := newExecutorWithDir(t)
	exec.SetChainReader(&stubChainReader{
		entityID: "c360.semteams.agent.chain.execution.dispatch_root",
		triples: map[string]any{
			chain.PredicateResearchArtifactLoop: "researcher_canonical",
			chain.PredicatePlanLoop:             "planner_canonical",
			chain.PredicatePlanReviewerLoop:     "reviewer_canonical",
		},
	})

	args := defaultArtifactArgs()
	// Architect's local guess — typically wrong (smoke #8 run-5 had a
	// downstream loop ID in the planner slot etc.).
	args["provenance"] = map[string]any{
		"research_artifact_loop": "wrong_research",
		"planner_loop":           "wrong_planner",
		"reviewer_loop":          "wrong_reviewer",
	}

	res, err := exec.Execute(context.Background(), defaultCall(args))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty", res.Error)
	}

	// Find the rendered .md and read its body.
	entries, _ := os.ReadDir(dir)
	var fullPath string
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".md" {
			fullPath = filepath.Join(dir, e.Name())
			break
		}
	}
	body, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("read rendered markdown: %v", err)
	}
	for _, want := range []string{"researcher_canonical", "planner_canonical", "reviewer_canonical"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("rendered body missing chain-canonical %q; got body:\n%s", want, body)
		}
	}
	for _, wrong := range []string{"wrong_research", "wrong_planner", "wrong_reviewer"} {
		if strings.Contains(string(body), wrong) {
			t.Errorf("rendered body still contains LLM-supplied %q (chain override should have replaced it); got body:\n%s", wrong, body)
		}
	}

	// research_root_loop triple on loop entity also reflects the
	// chain-canonical value (drives chain.spec_artifact.* triples
	// downstream).
	loopEntityID := agentic.LoopExecutionEntityID("c360", "semteams", "loop-architect-abc")
	gotResearchRoot, ok := findTripleOnLoopEntity(tp.snapshot(), loopEntityID, predicateResearchRootLoop)
	if !ok {
		t.Fatal("predicateResearchRootLoop triple missing on loop entity")
	}
	if gotResearchRoot != "researcher_canonical" {
		t.Errorf("predicateResearchRootLoop triple = %v, want chain-canonical researcher_canonical", gotResearchRoot)
	}
}

// TestExecute_ChainReadFailureFallsBackToLLMValues pins fail-soft:
// chain read errors fall back to LLM-supplied provenance + the
// title-derived slug, logged at Warn. Same shape as emitplan /
// emitconsensus equivalents.
func TestExecute_ChainReadFailureFallsBackToLLMValues(t *testing.T) {
	exec, tp, _, dir := newExecutorWithDir(t)
	stub := &stubChainReader{
		entityID: "c360.semteams.agent.chain.execution.dispatch_root",
		err:      errors.New("graph timeout"),
	}
	exec.SetChainReader(stub)

	res, err := exec.Execute(context.Background(), defaultCall(defaultArtifactArgs()))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q (chain read failure must not surface to caller)", res.Error)
	}
	if stub.calls != 1 {
		t.Errorf("stub ReadChainFor calls = %d, want 1", stub.calls)
	}

	entries, _ := os.ReadDir(dir)
	var foundMD string
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".md" {
			foundMD = e.Name()
			break
		}
	}
	if !strings.HasPrefix(foundMD, time.Now().UTC().Format("2006-01-02")+"-osh-meshtastic-driver") {
		t.Errorf("rendered file = %q, want title-derived slug fallback", foundMD)
	}

	loopEntityID := agentic.LoopExecutionEntityID("c360", "semteams", "loop-architect-abc")
	gotResearchRoot, ok := findTripleOnLoopEntity(tp.snapshot(), loopEntityID, predicateResearchRootLoop)
	if !ok {
		t.Fatal("predicateResearchRootLoop triple missing on loop entity")
	}
	if gotResearchRoot != "loop-research-001" {
		t.Errorf("predicateResearchRootLoop = %v, want LLM-supplied loop-research-001 (chain-read failed)", gotResearchRoot)
	}
}
