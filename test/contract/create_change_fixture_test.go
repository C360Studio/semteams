package contract

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/types"
	"gopkg.in/yaml.v3"

	"github.com/c360studio/semteams/cmd/semteams/tools/emitchange"
)

// TestCreateChangeFixtureEmitChangeValid runs the create-change mock-LLM
// fixture's emit_change payload through the REAL executor at `go test`
// time. The mock serves this payload verbatim during the Playwright
// journey, and the real emit_change executor (NOT the mock) validates +
// stamps it — so a payload that violates §D3 (SHALL + WHEN/THEN scenario)
// or §D6 (task goal/target_files/test_command/assumptions/non_goals), or
// whose requirement_ref dangles, fails the author loop at journey runtime
// (an expensive Docker round-trip to discover). This test catches it here
// instead, and guards against fixture drift.
//
// Per [[feedback-mock-llm-journey-precompute-signatures]] +
// [[structural-contract-tests-insufficient-for-wiring]]: the part of a
// mock-LLM journey the mock CAN'T validate (a real tool executor running
// on a hand-authored payload) gets a structural pre-check.
func TestCreateChangeFixtureEmitChangeValid(t *testing.T) {
	args := loadFixtureToolCallArgs(t, "../fixtures/journeys/create-change.yaml", emitchange.ToolName)

	call := agentic.ToolCall{
		ID:        "preflight-1",
		Name:      emitchange.ToolName,
		Arguments: args,
		Metadata: map[string]any{
			// Rule 01 pins this key; emit_change resolves the run entity from
			// it. The exact key is pinned by createChangeRunLoopKey in
			// create_change_pack_test.go.
			agentic.MetadataKeyRelatedLoops: map[string]any{
				createChangeRunLoopKey: "preflight-run-loop",
			},
		},
	}

	pub := &recordingPublisher{}
	ex := emitchange.NewExecutor(pub, types.PlatformMeta{Org: "c360", Platform: "ops"}, nil)
	res, err := ex.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("emit_change Execute returned error: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("emit_change rejected the fixture payload: %s (kind=%s)\nThe fixture's emit_change arguments must satisfy the §D3/§D6 validator — fix the fixture, not this test.", res.Error, res.ErrorKind)
	}

	// The stamp must include the add-mfa change content the journey asserts on.
	var sawProposalIntent, sawDeltaOp bool
	for _, tr := range pub.triples {
		if strings.HasPrefix(tr.Predicate, "change.add-mfa.proposal.intent") {
			sawProposalIntent = true
		}
		if strings.Contains(tr.Predicate, "change.add-mfa.delta.auth.") && strings.HasSuffix(tr.Predicate, ".op") {
			sawDeltaOp = true
		}
	}
	if !sawProposalIntent {
		t.Error("emit_change did not stamp change.add-mfa.proposal.intent — the journey's change.* assertion would fail")
	}
	if !sawDeltaOp {
		t.Error("emit_change did not stamp a change.add-mfa.delta.auth.<rid>.op — the requirement delta was not recorded")
	}
}

// recordingPublisher is a minimal agentictools.TriplePublisher capturing
// the stamped triples for assertion.
type recordingPublisher struct {
	triples []message.Triple
}

func (p *recordingPublisher) AddTriple(_ context.Context, tr message.Triple) error {
	p.triples = append(p.triples, tr)
	return nil
}

func (p *recordingPublisher) AddTriplesBatch(_ context.Context, ts []message.Triple) error {
	p.triples = append(p.triples, ts...)
	return nil
}

// loadFixtureToolCallArgs reads a journey fixture and returns the parsed
// arguments of the first tool_call with the given name.
//
// TODO: if a second fixture contract test needs targeted single-tool
// extraction (e.g. a future P3 emit_* journey), move this to a shared
// test/contract helper rather than copying it.
func loadFixtureToolCallArgs(t *testing.T, path, toolName string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	var fx struct {
		Responses []struct {
			ToolCall *struct {
				Name          string `yaml:"name"`
				ArgumentsJSON string `yaml:"arguments_json"`
			} `yaml:"tool_call"`
		} `yaml:"responses"`
	}
	if err := yaml.Unmarshal(data, &fx); err != nil {
		t.Fatalf("unmarshal fixture %s: %v", path, err)
	}
	for _, r := range fx.Responses {
		if r.ToolCall == nil || r.ToolCall.Name != toolName {
			continue
		}
		var args map[string]any
		if err := json.Unmarshal([]byte(r.ToolCall.ArgumentsJSON), &args); err != nil {
			t.Fatalf("parse %s arguments_json: %v", toolName, err)
		}
		return args
	}
	t.Fatalf("fixture %s has no tool_call named %q", path, toolName)
	return nil
}
