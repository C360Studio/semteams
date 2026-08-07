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
	// All create-change fixtures' emit_change payloads — the recovery fixture
	// carries TWO (the initial draft + the tightened re-draft), so validate every
	// emit_change call across these files.
	fixtures := []string{
		"../fixtures/journeys/create-change.yaml",
		"../fixtures/journeys/create-change-recovery.yaml",
		"../fixtures/journeys/mavlink-hard-spec.yaml",
	}
	for _, path := range fixtures {
		payloads := allFixtureToolCallArgs(t, path, emitchange.ToolName)
		if len(payloads) == 0 {
			t.Errorf("%s has no emit_change tool_call", path)
			continue
		}
		for i, args := range payloads {
			slugValue, _ := args["slug"].(string)
			if slugValue == "" {
				t.Fatalf("%s emit_change[%d] has no slug", path, i)
			}
			call := agentic.ToolCall{
				ID:        "preflight",
				Name:      emitchange.ToolName,
				Arguments: args,
				// Rule 01 pins this key; emit_change resolves the run entity from
				// it. The exact key is pinned by createChangeRunLoopKey in
				// create_change_pack_test.go.
				Metadata: map[string]any{
					agentic.MetadataKeyRelatedLoops: map[string]any{
						createChangeRunLoopKey: "preflight-run-loop",
					},
				},
			}
			pub := &recordingPublisher{}
			ex := emitchange.NewExecutor(pub, types.PlatformMeta{Org: "c360", Platform: "ops"}, nil)
			res, err := ex.Execute(context.Background(), call)
			if err != nil {
				t.Fatalf("%s emit_change[%d] Execute returned error: %v", path, i, err)
			}
			if res.Error != "" {
				t.Fatalf("%s emit_change[%d] rejected by the validator: %s (kind=%s)\nThe fixture's emit_change arguments must satisfy the §D3/§D6 validator — fix the fixture, not this test.", path, i, res.Error, res.ErrorKind)
			}
			// The stamp must include the change content the journeys assert on.
			var sawProposalIntent, sawDeltaOp bool
			for _, tr := range pub.triples {
				if strings.HasPrefix(tr.Predicate, "change."+slugValue+".proposal.intent") {
					sawProposalIntent = true
				}
				if strings.Contains(tr.Predicate, "change."+slugValue+".delta.") && strings.HasSuffix(tr.Predicate, ".op") {
					sawDeltaOp = true
				}
			}
			if !sawProposalIntent {
				t.Errorf("%s emit_change[%d] did not stamp change.%s.proposal.intent", path, i, slugValue)
			}
			if !sawDeltaOp {
				t.Errorf("%s emit_change[%d] did not stamp a change.%s.delta.<capability>.<rid>.op", path, i, slugValue)
			}
		}
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

// allFixtureToolCallArgs reads a journey fixture and returns the parsed
// arguments of EVERY tool_call with the given name, in fixture order.
//
// TODO: if a second fixture contract test needs this extraction (e.g. a future
// P3 emit_* journey), move it to a shared test/contract helper rather than
// copying it.
func allFixtureToolCallArgs(t *testing.T, path, toolName string) []map[string]any {
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
	var out []map[string]any
	for _, r := range fx.Responses {
		if r.ToolCall == nil || r.ToolCall.Name != toolName {
			continue
		}
		var args map[string]any
		if err := json.Unmarshal([]byte(r.ToolCall.ArgumentsJSON), &args); err != nil {
			t.Fatalf("parse %s arguments_json in %s: %v", toolName, path, err)
		}
		out = append(out, args)
	}
	return out
}

// CreateEntityWithTriples satisfies beta.159's widened TriplePublisher;
// the fake delegates to AddTriplesBatch so recording semantics are identical.
func (p *recordingPublisher) CreateEntityWithTriples(ctx context.Context, _ string, _ message.Type, triples []message.Triple) error {
	return p.AddTriplesBatch(ctx, triples)
}
