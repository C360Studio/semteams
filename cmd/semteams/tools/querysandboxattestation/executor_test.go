package querysandboxattestation

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/types"

	"github.com/c360studio/semteams/cmd/semteams/runanchor"
	"github.com/c360studio/semteams/cmd/semteams/sandboxmanager"
)

type fakeReader struct {
	triples map[string]any
	lastID  string
	err     error
}

func (f *fakeReader) ReadEntity(_ context.Context, entityID string) (map[string]any, error) {
	f.lastID = entityID
	if f.err != nil {
		return nil, f.err
	}
	return f.triples, nil
}

func newCatalog(t *testing.T) *sandboxmanager.Catalog {
	t.Helper()
	cat, err := sandboxmanager.BuiltinCatalog("/repo")
	if err != nil {
		t.Fatalf("BuiltinCatalog: %v", err)
	}
	return cat
}

func newExecutor(t *testing.T, triples map[string]any) (*Executor, *fakeReader) {
	t.Helper()
	reader := &fakeReader{triples: triples}
	fixedNow := func() time.Time { return time.Date(2026, 5, 31, 14, 0, 0, 0, time.UTC) }
	exec := NewExecutor(newCatalog(t), reader, types.PlatformMeta{Org: "c360", Platform: "ops"}, fixedNow, nil)
	return exec, reader
}

func makeCall(args map[string]any) agentic.ToolCall {
	return agentic.ToolCall{
		ID:        "c1",
		Name:      ToolName,
		Arguments: args,
		Metadata: map[string]any{
			agentic.MetadataKeyRelatedLoops: map[string]any{
				runanchor.ChainEntityRoleKey: "c360.ops.agent.chain.execution.c1",
			},
		},
	}
}

func freshAttestationTriples(profileID, signature string, attestedAt time.Time, ttl time.Duration) map[string]any {
	return map[string]any{
		sandboxmanager.PredicateAttestationProfile:               profileID,
		sandboxmanager.PredicateAttestationReady:                 true,
		sandboxmanager.PredicateAttestationDegraded:              false,
		sandboxmanager.PredicateAttestationTerminal:              false,
		sandboxmanager.PredicateAttestationAdmission:             "admitted",
		sandboxmanager.PredicateAttestationSignature:             signature,
		sandboxmanager.PredicateAttestationRequirementsHash:      "rh",
		sandboxmanager.PredicateAttestationAttestedAt:            attestedAt.Format(time.RFC3339Nano),
		sandboxmanager.PredicateAttestationTTL:                   int64(ttl.Seconds()),
		sandboxmanager.PredicateAttestationVerifiedPrefix + "go": "go version go1.25.4",
	}
}

func TestExecute_HitFresh(t *testing.T) {
	req := sandboxmanager.SandboxRequirements{Languages: []string{"go"}}
	prof, _ := sandboxmanager.MatchProfile(req, newCatalog(t))
	sig := sandboxmanager.SignatureFor(prof, req)

	triples := freshAttestationTriples(prof.ID(), sig,
		time.Date(2026, 5, 31, 13, 30, 0, 0, time.UTC), 24*time.Hour)
	exec, _ := newExecutor(t, triples)
	res, err := exec.Execute(context.Background(), makeCall(map[string]any{
		"languages": []any{"go"},
	}))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("error: %s", res.Error)
	}

	var resp lookupResponse
	if err := json.Unmarshal([]byte(res.Content), &resp); err != nil {
		t.Fatalf("parse content: %v", err)
	}
	if !resp.Found {
		t.Fatalf("expected found=true; got %+v", resp)
	}
	if !resp.Fresh {
		t.Fatalf("expected fresh=true (30min into a 24h TTL); got staleness=%d", resp.StalenessSeconds)
	}
	if resp.ProfileMatch != prof.ID() {
		t.Fatalf("profile match: %q", resp.ProfileMatch)
	}
	if resp.Attestation == nil {
		t.Fatalf("attestation body missing")
	}
	if resp.Attestation.Verified["go"] == "" {
		t.Fatalf("verified.go reassembly missing: %+v", resp.Attestation.Verified)
	}
}

func TestExecute_HitStale(t *testing.T) {
	req := sandboxmanager.SandboxRequirements{Languages: []string{"go"}}
	prof, _ := sandboxmanager.MatchProfile(req, newCatalog(t))
	sig := sandboxmanager.SignatureFor(prof, req)

	// Attested 48h before fixedNow; TTL 24h → stale
	stale := time.Date(2026, 5, 29, 14, 0, 0, 0, time.UTC)
	triples := freshAttestationTriples(prof.ID(), sig, stale, 24*time.Hour)
	exec, _ := newExecutor(t, triples)
	res, _ := exec.Execute(context.Background(), makeCall(map[string]any{
		"languages": []any{"go"},
	}))

	var resp lookupResponse
	json.Unmarshal([]byte(res.Content), &resp)
	if !resp.Found {
		t.Fatalf("expected found=true even when stale")
	}
	if resp.Fresh {
		t.Fatalf("expected fresh=false (48h > 24h TTL)")
	}
	if resp.StalenessSeconds < 24*60*60 {
		t.Fatalf("expected staleness > 24h; got %ds", resp.StalenessSeconds)
	}
}

func TestExecute_MissDifferentSignature(t *testing.T) {
	// Chain has an attestation for a DIFFERENT signature
	otherSig := "deadbeef"
	triples := freshAttestationTriples("svelte-ui@v1", otherSig, time.Now().UTC(), 24*time.Hour)
	exec, _ := newExecutor(t, triples)
	res, _ := exec.Execute(context.Background(), makeCall(map[string]any{
		"languages": []any{"go"},
	}))
	var resp lookupResponse
	json.Unmarshal([]byte(res.Content), &resp)
	if resp.Found {
		t.Fatalf("expected found=false; chain has different-signature attestation")
	}
}

func TestExecute_MissEmptyEntity(t *testing.T) {
	exec, _ := newExecutor(t, map[string]any{})
	res, _ := exec.Execute(context.Background(), makeCall(map[string]any{
		"languages": []any{"go"},
	}))
	var resp lookupResponse
	json.Unmarshal([]byte(res.Content), &resp)
	if resp.Found {
		t.Fatalf("expected found=false on empty entity")
	}
}

func TestExecute_FrontDoorLoopIDFallback_ReadsExistingLoopEntity(t *testing.T) {
	exec, reader := newExecutor(t, map[string]any{})
	call := agentic.ToolCall{
		ID:        "c1",
		Name:      ToolName,
		LoopID:    "loop-1",
		Arguments: map[string]any{"languages": []any{"go"}},
		Metadata:  map[string]any{},
	}
	res, _ := exec.Execute(context.Background(), call)
	if res.Error != "" {
		t.Fatalf("expected no error, got %q", res.Error)
	}
	want := "c360.ops.agent.agentic-loop.execution.loop-1"
	if reader.lastID != want {
		t.Fatalf("read entity ID wrong: got %q want %q", reader.lastID, want)
	}
}

func TestExecute_NoMatch_StructuredResponse(t *testing.T) {
	exec, _ := newExecutor(t, map[string]any{})
	res, _ := exec.Execute(context.Background(), makeCall(map[string]any{
		"languages": []any{"rust"},
	}))
	var resp noMatchResponse
	if err := json.Unmarshal([]byte(res.Content), &resp); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !resp.NoMatch {
		t.Fatalf("expected no_match=true")
	}
	if !strings.Contains(resp.Reason, "rust") {
		t.Fatalf("reason should mention unmet capability: %s", resp.Reason)
	}
	if got := res.Metadata["no_match"]; got != true {
		t.Fatalf("metadata no_match wrong: %v", got)
	}
}

func TestExecute_BadArgs(t *testing.T) {
	exec, _ := newExecutor(t, nil)
	res, _ := exec.Execute(context.Background(), makeCall(map[string]any{
		"languages": []any{42},
	}))
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Fatalf("expected invalid_args, got %s", res.ErrorKind)
	}
}

func TestExecute_NoChainEntity(t *testing.T) {
	exec, _ := newExecutor(t, nil)
	call := agentic.ToolCall{
		ID:        "c1",
		Name:      ToolName,
		Arguments: map[string]any{"languages": []any{"go"}},
		Metadata:  map[string]any{},
	}
	res, _ := exec.Execute(context.Background(), call)
	if res.ErrorKind != agentic.ToolErrorInternal {
		t.Fatalf("expected internal error, got %s; err=%q", res.ErrorKind, res.Error)
	}
}

func TestAttestationFromTriples_RoundtripsVerified(t *testing.T) {
	triples := map[string]any{
		sandboxmanager.PredicateAttestationSignature:               "sig",
		sandboxmanager.PredicateAttestationProfile:                 "go-backend@v1",
		sandboxmanager.PredicateAttestationReady:                   true,
		sandboxmanager.PredicateAttestationVerifiedPrefix + "go":   "go version go1.25",
		sandboxmanager.PredicateAttestationVerifiedPrefix + "task": "task v3",
	}
	att, ok := attestationFromTriples(triples, "sig")
	if !ok {
		t.Fatalf("expected match on signature")
	}
	if att.Verified["go"] == "" {
		t.Fatalf("verified.go missing")
	}
	if att.Verified["task"] == "" {
		t.Fatalf("verified.task missing")
	}
}

func TestAttestationFromTriples_RoundTripsReasonSlices(t *testing.T) {
	// PR 4.2 finding M5: reasons stamp as []string. Verify that
	// the round-trip survives the NATS JSON encode/decode (Go
	// []string → JSON array → []any of strings on the read side).
	jsonRoundTrip := func(v any) any {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var out any
		if err := json.Unmarshal(b, &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		return out
	}

	triples := map[string]any{
		sandboxmanager.PredicateAttestationSignature:        "sig",
		sandboxmanager.PredicateAttestationDegradedReasons:  jsonRoundTrip([]string{"go: exit 1", "task: not found"}),
		sandboxmanager.PredicateAttestationFailedProbes:     jsonRoundTrip([]string{"go", "task"}),
		sandboxmanager.PredicateAttestationAdmissionReasons: jsonRoundTrip([]string{"network:public requires AllowPublicNetwork operator policy"}),
	}
	att, ok := attestationFromTriples(triples, "sig")
	if !ok {
		t.Fatalf("expected match")
	}
	if len(att.DegradedReasons) != 2 {
		t.Fatalf("DegradedReasons round-trip lost entries: %v", att.DegradedReasons)
	}
	if len(att.FailedProbes) != 2 {
		t.Fatalf("FailedProbes round-trip lost entries: %v", att.FailedProbes)
	}
	if len(att.AdmissionReasons) != 1 {
		t.Fatalf("AdmissionReasons round-trip lost entries: %v", att.AdmissionReasons)
	}
}

func TestAttestationFromTriples_ReasonsWithSeparator_Survive(t *testing.T) {
	// PR 4.2 finding M5 regression: a reason fragment that contains
	// the legacy "; " separator must NOT split into two reasons.
	triples := map[string]any{
		sandboxmanager.PredicateAttestationSignature:       "sig",
		sandboxmanager.PredicateAttestationDegradedReasons: []any{"path; with semicolon inside"},
	}
	att, ok := attestationFromTriples(triples, "sig")
	if !ok {
		t.Fatalf("expected match")
	}
	if len(att.DegradedReasons) != 1 {
		t.Fatalf("reason with embedded separator split: %v", att.DegradedReasons)
	}
}

func TestAttestationFromTriples_TTLAcceptsMultipleTypes(t *testing.T) {
	cases := []any{int64(86400), float64(86400), int(86400)}
	for _, v := range cases {
		triples := map[string]any{
			sandboxmanager.PredicateAttestationSignature: "sig",
			sandboxmanager.PredicateAttestationTTL:       v,
		}
		att, ok := attestationFromTriples(triples, "sig")
		if !ok {
			t.Fatalf("expected match (%T)", v)
		}
		if att.TTL != 24*time.Hour {
			t.Fatalf("TTL %T not parsed: %v", v, att.TTL)
		}
	}
}

func TestNew_PanicsOnNilCatalog(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic")
		}
	}()
	NewExecutor(nil, &fakeReader{}, types.PlatformMeta{Org: "c360", Platform: "ops"}, nil, nil)
}

func TestNew_PanicsOnNilReader(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic")
		}
	}()
	cat, _ := sandboxmanager.BuiltinCatalog("/repo")
	NewExecutor(cat, nil, types.PlatformMeta{Org: "c360", Platform: "ops"}, nil, nil)
}
