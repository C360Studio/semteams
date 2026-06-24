package analyzeproof

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/types"
)

func platform() types.PlatformMeta { return types.PlatformMeta{Org: "c360", Platform: "ops"} }

const testRunEntity = "c360.ops.agent.chain.execution.run-123"

var fixedNow = time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)

type fakeReader struct {
	id      string
	triples map[string]any
	err     error
}

func (f fakeReader) ReadEntity(_ context.Context, id string) (map[string]any, error) {
	if f.err != nil {
		return nil, f.err
	}
	if id != f.id {
		return map[string]any{}, nil
	}
	return f.triples, nil
}

type fakePub struct {
	mu      sync.Mutex
	triples []message.Triple
}

func (f *fakePub) AddTriple(_ context.Context, t message.Triple) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.triples = append(f.triples, t)
	return nil
}

func (f *fakePub) AddTriplesBatch(_ context.Context, ts []message.Triple) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.triples = append(f.triples, ts...)
	return nil
}

func (f *fakePub) byPredicate() map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	m := make(map[string]any, len(f.triples))
	for _, tr := range f.triples {
		m[tr.Predicate] = tr.Object
	}
	return m
}

func TestAnalyze_PassesFreshReadyDependency(t *testing.T) {
	a := Analyze(map[string]any{
		"proof.claim.mavlink.mission_upload.status":      "accepted",
		"proof.claim.mavlink.mission_upload.requires":    `["px4_sitl.boots"]`,
		"proof.dependency.px4_sitl.boots.status":         "ready",
		"proof.dependency.px4_sitl.boots.profile_ref":    "mavlink.px4-sitl@v1",
		"proof.readiness.smoke-001.profile_ref":          "mavlink.px4-sitl@v1",
		"proof.readiness.smoke-001.status":               "passed",
		"proof.readiness.smoke-001.smoke_status":         "passed",
		"proof.readiness.smoke-001.expires_at":           fixedNow.Add(time.Hour).Format(time.RFC3339Nano),
		"proof.readiness.smoke-001.failure_signature":    "",
		"proof.evidence.smoke-log.covers":                `["mavlink.mission_upload"]`,
		"proof.harness_profile.mavlink.px4-sitl@v1.team": "test-harness",
	}, fixedNow)

	if a.Status != statusPassed {
		t.Fatalf("status = %q, want %q: %#v", a.Status, statusPassed, a.Findings)
	}
	if len(a.Findings) != 0 {
		t.Fatalf("findings = %#v, want none", a.Findings)
	}
}

func TestAnalyze_MissingDependencyBlocksToHarness(t *testing.T) {
	a := Analyze(map[string]any{
		"proof.claim.mavlink.mission_upload.status":   "accepted",
		"proof.claim.mavlink.mission_upload.requires": `["px4_sitl.boots"]`,
	}, fixedNow)

	assertSingleFinding(t, a, statusFailed, "missing_proof_dependency", routeTestHarness)
	if got := a.Findings[0].Dependency; got != "px4_sitl.boots" {
		t.Fatalf("dependency = %q, want px4_sitl.boots", got)
	}
}

func TestAnalyze_ExpiredReadinessBlocksToHarness(t *testing.T) {
	a := Analyze(map[string]any{
		"proof.claim.mavlink.mission_upload.status":   "accepted",
		"proof.claim.mavlink.mission_upload.requires": `["px4_sitl.boots"]`,
		"proof.dependency.px4_sitl.boots.status":      "ready",
		"proof.dependency.px4_sitl.boots.profile_ref": "mavlink.px4-sitl@v1",
		"proof.readiness.smoke-001.profile_ref":       "mavlink.px4-sitl@v1",
		"proof.readiness.smoke-001.status":            "passed",
		"proof.readiness.smoke-001.expires_at":        fixedNow.Add(-time.Second).Format(time.RFC3339Nano),
	}, fixedNow)

	assertSingleFinding(t, a, statusFailed, "stale_readiness_record", routeTestHarness)
	if got := a.Findings[0].Readiness; got != "smoke-001" {
		t.Fatalf("readiness = %q, want smoke-001", got)
	}
}

func TestAnalyze_ActiveWaiverAllowsMissingDependency(t *testing.T) {
	a := Analyze(map[string]any{
		"proof.claim.mavlink.mission_upload.status":   "accepted",
		"proof.claim.mavlink.mission_upload.requires": `["px4_sitl.boots"]`,
		"proof.waiver.operator-001.status":            "active",
		"proof.waiver.operator-001.dependencies":      `["px4_sitl.boots"]`,
		"proof.waiver.operator-001.expires_at":        fixedNow.Add(time.Hour).Format(time.RFC3339Nano),
		"proof.waiver.operator-001.reason":            "PX4 unavailable in this environment",
	}, fixedNow)

	if a.Status != statusPassed {
		t.Fatalf("status = %q, want %q: %#v", a.Status, statusPassed, a.Findings)
	}
	if len(a.Findings) != 0 {
		t.Fatalf("findings = %#v, want none", a.Findings)
	}
}

func TestAnalyze_ExpiredWaiverBlocks(t *testing.T) {
	a := Analyze(map[string]any{
		"proof.claim.mavlink.mission_upload.status":   "accepted",
		"proof.claim.mavlink.mission_upload.requires": `["px4_sitl.boots"]`,
		"proof.waiver.operator-001.status":            "active",
		"proof.waiver.operator-001.dependencies":      `["px4_sitl.boots"]`,
		"proof.waiver.operator-001.expires_at":        fixedNow.Add(-time.Second).Format(time.RFC3339Nano),
	}, fixedNow)

	assertSingleFinding(t, a, statusFailed, "expired_waiver", routeCoordinator)
	if got := a.Findings[0].Waiver; got != "operator-001" {
		t.Fatalf("waiver = %q, want operator-001", got)
	}
}

func TestAnalyze_ExpiredClaimWaiverBlocksUnprovedClaim(t *testing.T) {
	a := Analyze(map[string]any{
		"proof.claim.mavlink.mission_upload.status": "accepted",
		"proof.waiver.operator-001.status":          "active",
		"proof.waiver.operator-001.claims":          `["mavlink.mission_upload"]`,
		"proof.waiver.operator-001.expires_at":      fixedNow.Add(-time.Second).Format(time.RFC3339Nano),
	}, fixedNow)

	assertSingleFinding(t, a, statusFailed, "expired_waiver", routeCoordinator)
	if got := a.Findings[0].Claim; got != "mavlink.mission_upload" {
		t.Fatalf("claim = %q, want mavlink.mission_upload", got)
	}
}

func TestAnalyze_NoAcceptedClaimsIsAmbiguous(t *testing.T) {
	a := Analyze(map[string]any{"proof.claim.draft.status": "draft"}, fixedNow)
	assertSingleFinding(t, a, statusAmbiguous, "no_accepted_claims", routeCoordinator)
}

func TestExecute_StampsFormalClaimTriples(t *testing.T) {
	pub := &fakePub{}
	ex := NewExecutor(
		fakeReader{id: testRunEntity, triples: map[string]any{
			"proof.claim.mavlink.mission_upload.status":   "accepted",
			"proof.claim.mavlink.mission_upload.requires": `["px4_sitl.boots"]`,
			"proof.dependency.px4_sitl.boots.status":      "missing",
			"proof.dependency.px4_sitl.boots.kind":        "service",
			"proof.dependency.px4_sitl.boots.description": "PX4 SITL boots headlessly",
			"proof.dependency.px4_sitl.boots.profile_ref": "mavlink.px4-sitl@v1",
			"proof.readiness.smoke-001.profile_ref":       "mavlink.px4-sitl@v1",
			"proof.readiness.smoke-001.status":            "stale",
			"proof.readiness.smoke-001.evidence":          `["smoke-log"]`,
			"proof.evidence.smoke-log.kind":               "log",
			"proof.evidence.smoke-log.created_at":         fixedNow.Add(-2 * time.Hour).Format(time.RFC3339Nano),
			"proof.evidence.smoke-log.covers":             `["mavlink.mission_upload"]`,
			"proof.waiver.operator-001.status":            "active",
			"proof.waiver.operator-001.claims":            `["unrelated.claim"]`,
			"proof.waiver.operator-001.reason":            "Different proof waiver",
			"proof.waiver.operator-001.expires_at":        fixedNow.Add(time.Hour).Format(time.RFC3339Nano),
		}},
		pub,
		platform(),
		nil,
	)
	ex.now = func() time.Time { return fixedNow }

	res, err := ex.Execute(context.Background(), callWith(nil))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("unexpected tool error: %s", res.Error)
	}
	var body struct {
		Status       string    `json:"status"`
		FindingCount int       `json:"finding_count"`
		Findings     []Finding `json:"findings"`
		ProofFacts   struct {
			Dependencies []map[string]any `json:"dependencies"`
			Readiness    []map[string]any `json:"readiness"`
			Evidence     []map[string]any `json:"evidence"`
			Waivers      []map[string]any `json:"waivers"`
		} `json:"proof_facts"`
	}
	if err := json.Unmarshal([]byte(res.Content), &body); err != nil {
		t.Fatalf("unmarshal content: %v\n%s", err, res.Content)
	}
	if body.Status != statusFailed || body.FindingCount != 1 {
		t.Fatalf("content = %+v, want failed with one finding", body)
	}
	if len(body.ProofFacts.Dependencies) != 1 ||
		body.ProofFacts.Dependencies[0]["id"] != "px4_sitl.boots" ||
		body.ProofFacts.Dependencies[0]["status"] != "missing" {
		t.Fatalf("dependency proof summary = %#v, want px4_sitl.boots missing", body.ProofFacts.Dependencies)
	}
	if len(body.ProofFacts.Readiness) != 1 ||
		body.ProofFacts.Readiness[0]["id"] != "smoke-001" ||
		body.ProofFacts.Readiness[0]["status"] != "stale" {
		t.Fatalf("readiness proof summary = %#v, want smoke-001 stale", body.ProofFacts.Readiness)
	}
	if len(body.ProofFacts.Evidence) != 1 ||
		body.ProofFacts.Evidence[0]["id"] != "smoke-log" {
		t.Fatalf("evidence proof summary = %#v, want smoke-log", body.ProofFacts.Evidence)
	}
	if len(body.ProofFacts.Waivers) != 1 ||
		body.ProofFacts.Waivers[0]["id"] != "operator-001" {
		t.Fatalf("waiver proof summary = %#v, want operator-001", body.ProofFacts.Waivers)
	}

	m := pub.byPredicate()
	want := map[string]string{
		"formal_claims.status":                  statusFailed,
		"formal_claims.analyzer.version":        analyzerVersion,
		"formal_claims.finding_count":           "1",
		"formal_claims.route.test_harness":      "present",
		"formal_claims.finding.f001.kind":       "missing_proof_dependency",
		"formal_claims.finding.f001.route":      routeTestHarness,
		"formal_claims.finding.f001.severity":   severityBlocker,
		"formal_claims.finding.f001.claim":      "mavlink.mission_upload",
		"formal_claims.finding.f001.dependency": "px4_sitl.boots",
	}
	for pred, wantValue := range want {
		if got, _ := m[pred].(string); got != wantValue {
			t.Errorf("%s = %q, want %q", pred, got, wantValue)
		}
	}
	if got, _ := m["formal_claims.analyzed_at"].(string); !strings.HasPrefix(got, "2026-06-24T12:00:00") {
		t.Errorf("formal_claims.analyzed_at = %q, want fixed timestamp", got)
	}
	for _, tr := range pub.triples {
		if tr.Subject != testRunEntity {
			t.Fatalf("triple %q subject = %q, want %q", tr.Predicate, tr.Subject, testRunEntity)
		}
	}
}

func TestExecute_RejectsUnknownArguments(t *testing.T) {
	pub := &fakePub{}
	ex := NewExecutor(fakeReader{id: testRunEntity, triples: map[string]any{}}, pub, platform(), nil)
	res, _ := ex.Execute(context.Background(), callWith(map[string]any{"mode": "creative"}))
	if res.Error == "" || !strings.Contains(res.Error, "unknown field") {
		t.Fatalf("expected unknown-field error, got %q", res.Error)
	}
}

func callWith(args map[string]any) agentic.ToolCall {
	return agentic.ToolCall{
		ID: "call-1", Name: ToolName, Arguments: args,
		Metadata: map[string]any{agentic.MetadataKeyRunEntityID: testRunEntity},
	}
}

func assertSingleFinding(t *testing.T, a Analysis, status, kind, route string) {
	t.Helper()
	if a.Status != status {
		t.Fatalf("status = %q, want %q: %#v", a.Status, status, a.Findings)
	}
	if len(a.Findings) != 1 {
		t.Fatalf("findings = %#v, want one", a.Findings)
	}
	if got := a.Findings[0].Kind; got != kind {
		t.Fatalf("kind = %q, want %q", got, kind)
	}
	if got := a.Findings[0].Route; got != route {
		t.Fatalf("route = %q, want %q", got, route)
	}
}
