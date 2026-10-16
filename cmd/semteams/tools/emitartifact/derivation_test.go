package emitartifact

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/types"

	"github.com/c360studio/semteams/cmd/semteams/research"
)

type fakeKVStore struct {
	entries map[string][]byte
}

func (f *fakeKVStore) Get(_ context.Context, key string) ([]byte, error) {
	if f == nil || f.entries == nil {
		return nil, fmt.Errorf("key %q not found", key)
	}
	v, ok := f.entries[key]
	if !ok {
		return nil, fmt.Errorf("key %q not found", key)
	}
	return v, nil
}

func (f *fakeKVStore) Keys(_ context.Context) ([]string, error) {
	if f == nil || len(f.entries) == 0 {
		return nil, nil
	}
	keys := make([]string, 0, len(f.entries))
	for k := range f.entries {
		keys = append(keys, k)
	}
	return keys, nil
}

type fakeObjectStore struct {
	blobs map[string][]byte
}

func (f *fakeObjectStore) GetBytes(_ context.Context, key string) ([]byte, error) {
	if f == nil || f.blobs == nil {
		return nil, fmt.Errorf("object %q not found", key)
	}
	v, ok := f.blobs[key]
	if !ok {
		return nil, fmt.Errorf("object %q not found", key)
	}
	return v, nil
}

type fakeEntityTripleReader struct {
	entities map[string]map[string]any
}

func (f *fakeEntityTripleReader) ReadEntity(_ context.Context, entityID string) (map[string]any, error) {
	if f == nil || f.entities == nil {
		return nil, nil
	}
	return f.entities[entityID], nil
}

func marshalOutcome(t *testing.T, execID, callID, loopID, toolName, content string) []byte {
	t.Helper()
	rec := completedOutcomeRecord{
		Version:     "v1",
		ExecutionID: execID,
		RequestID:   "req-1",
		CallID:      callID,
		CallOrdinal: 1,
		Result: agentic.ToolResult{
			CallID:      callID,
			ExecutionID: execID,
			LoopID:      loopID,
			Name:        toolName,
			Content:     content,
		},
	}
	b, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshalOutcome: %v", err)
	}
	return b
}

func TestExecute_DerivationCheck_MultiChannelHappyPath(t *testing.T) {
	exec, tp, pub, _ := newExecutor(t)
	platform := types.PlatformMeta{Org: "c360", Platform: "semteams"}

	runEntityID := "c360.semteams.agent.chain.execution.run-100"
	gatherLoopEntityID := "c360.semteams.agentic-loop.agent.execution.loop-gather-1"
	synthLoopEntityID := "c360.semteams.agentic-loop.agent.execution.loop-synth-1"

	githubPayload, err := json.Marshal(map[string]any{
		"channel": "github",
		"records": []research.EvidenceRecord{
			{
				Channel:     research.ChannelGitHub,
				Locator:     "https://github.com/C360Studio/semteams/issues/271",
				OriginID:    "github://c360studio/semteams/issues/271",
				RetrievedAt: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC),
				SourceClass: research.SourceClassPrimary,
				RawBody:     "#271 Evidence contract & mechanical derivation check: enforce citations across github, web, and semsource.",
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal github payload: %v", err)
	}

	outcomes := &fakeKVStore{
		entries: map[string][]byte{
			outcomeKeyForExecutionID("exec-gh-1"): marshalOutcome(
				t,
				"exec-gh-1",
				"call_gather_gh_1",
				"loop-gather-1",
				"github_read",
				string(githubPayload),
			),
			outcomeKeyForExecutionID("exec-web-1"): marshalOutcome(
				t,
				"exec-web-1",
				"call_gather_web_1",
				"loop-gather-1",
				"web_search",
				"[1] NATS Documentation — Comparison with Other Systems\n    https://docs.nats.io/compare\n    NATS vs MQTT, Kafka, and other messaging systems for IoT and edge.\n",
			),
			outcomeKeyForExecutionID("exec-ss-1"): marshalOutcome(
				t,
				"exec-ss-1",
				"call_gather_ss_1",
				"loop-gather-1",
				"graph_query",
				`{"entity_id":"c360.semteams.semsource.git.repo.semstreams","summary":"Frozen SemStreams dependency supplies graph-ingest and rule-processor."}`,
			),
		},
	}
	entities := &fakeEntityTripleReader{
		entities: map[string]map[string]any{
			gatherLoopEntityID: {predicateRunEntityID: runEntityID, predicateRunID: "run-100"},
			synthLoopEntityID:  {predicateRunEntityID: runEntityID, predicateRunID: "run-100"},
		},
	}

	resolver := &NATSToolResultResolver{
		entityReader: entities,
		platform:     platform,
		outcomesKV:   outcomes,
	}
	exec.SetToolResultResolver(resolver)

	args := map[string]any{
		"revision": 1,
		"title":    "Multi-Channel Evidence Verification",
		"actors": []any{
			map[string]any{
				"name": "SemTeams Evidence Gate",
				"role": "Enforces mechanical citation verification",
				"evidence": []any{
					map[string]any{
						"channel":         "github",
						"locator":         "https://github.com/C360Studio/semteams/issues/271",
						"origin_id":       "github://c360studio/semteams/issues/271",
						"retrieved_at":    "2026-10-16T12:00:00Z",
						"source_class":    "primary",
						"quote":           "enforce citations across github, web, and semsource",
						"tool_result_ref": "call_gather_gh_1",
						"revision":        1,
					},
				},
			},
			map[string]any{
				"name": "SemStreams Runtime",
				"role": "Provides graph-ingest and rule-processor primitives",
				"evidence": []any{
					map[string]any{
						"channel":         "semsource",
						"locator":         "semsource://c360.semteams.semsource.git.repo.semstreams",
						"origin_id":       "semsource://c360.semteams.semsource.git.repo.semstreams",
						"retrieved_at":    "2026-10-16T12:01:00Z",
						"source_class":    "primary",
						"quote":           "Frozen SemStreams dependency supplies graph-ingest and rule-processor.",
						"tool_result_ref": "call_gather_ss_1",
						"revision":        1,
					},
				},
			},
		},
		"integration_points": []any{
			map[string]any{
				"from":      "SemTeams Evidence Gate",
				"to":        "SemStreams Runtime",
				"data":      "validated artifact triples",
				"direction": "write",
				"evidence": []any{
					map[string]any{
						"channel":         "web",
						"locator":         "https://docs.nats.io/compare",
						"origin_id":       "https://docs.nats.io/compare",
						"retrieved_at":    "2026-10-16T12:02:00Z",
						"source_class":    "official",
						"quote":           "NATS vs MQTT, Kafka, and other messaging systems for IoT and edge.",
						"tool_result_ref": "call_gather_web_1",
						"revision":        1,
					},
				},
			},
		},
		"tasks":        []any{"verify derivation in emit_research_artifact"},
		"test_harness": "meshtasticd-2.x",
	}

	call := agentic.ToolCall{
		ID:        "call-emit-1",
		Name:      ToolName,
		Arguments: args,
		LoopID:    "loop-synth-1",
		Metadata: map[string]any{
			agentic.MetadataKeyRunID:       "run-100",
			agentic.MetadataKeyRunEntityID: runEntityID,
		},
	}

	res, err := exec.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty", res.Error)
	}
	if len(tp.snapshot()) == 0 {
		t.Errorf("expected triples to be published on happy path")
	}
	_, _, pubCalls := pub.snapshot()
	if pubCalls != 1 {
		t.Errorf("expected 1 payload publish call, got %d", pubCalls)
	}
}

func TestExecute_DerivationCheck_RejectsUnknownToolResultRef(t *testing.T) {
	exec, tp, pub, _ := newExecutor(t)
	exec.SetToolResultResolver(&NATSToolResultResolver{
		platform:   types.PlatformMeta{Org: "c360", Platform: "semteams"},
		outcomesKV: &fakeKVStore{entries: map[string][]byte{}},
	})

	res, err := exec.Execute(context.Background(), defaultCall(defaultArtifactArgs()))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
	if !strings.Contains(res.Error, "not found") {
		t.Errorf("Result.Error = %q, want 'not found'", res.Error)
	}
	if len(tp.snapshot()) != 0 {
		t.Errorf("expected 0 triples published on derivation failure, got %d", len(tp.snapshot()))
	}
	_, _, pubCalls := pub.snapshot()
	if pubCalls != 0 {
		t.Errorf("expected 0 payload publishes on derivation failure, got %d", pubCalls)
	}
}

func TestExecute_DerivationCheck_RejectsCrossRunToolResultRef(t *testing.T) {
	exec, tp, pub, _ := newExecutor(t)
	platform := types.PlatformMeta{Org: "c360", Platform: "semteams"}

	outcomes := &fakeKVStore{
		entries: map[string][]byte{
			outcomeKeyForExecutionID("exec-foreign-1"): marshalOutcome(
				t,
				"exec-foreign-1",
				"call_gather_web_1",
				"loop-other-run",
				"web_search",
				"https://docs.nats.io/compare NATS vs MQTT, Kafka, and other messaging systems for IoT and edge.",
			),
		},
	}
	entities := &fakeEntityTripleReader{
		entities: map[string]map[string]any{
			"c360.semteams.agentic-loop.agent.execution.loop-other-run": {
				predicateRunEntityID: "c360.semteams.agent.chain.execution.run-other",
				predicateRunID:       "run-other",
			},
			"c360.semteams.agentic-loop.agent.execution.loop-abc": {
				predicateRunEntityID: "c360.semteams.agent.chain.execution.run-current",
				predicateRunID:       "run-current",
			},
		},
	}
	exec.SetToolResultResolver(&NATSToolResultResolver{
		entityReader: entities,
		platform:     platform,
		outcomesKV:   outcomes,
	})

	call := defaultCall(defaultArtifactArgs())
	call.Metadata = map[string]any{
		agentic.MetadataKeyRunID:       "run-current",
		agentic.MetadataKeyRunEntityID: "c360.semteams.agent.chain.execution.run-current",
	}

	res, err := exec.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
	if !strings.Contains(res.Error, "does not belong to current research run") {
		t.Errorf("Result.Error = %q, want cross-run error", res.Error)
	}
	if len(tp.snapshot()) != 0 || pub.calls != 0 {
		t.Errorf("expected no triples or payload published on cross-run failure")
	}
}

func TestExecute_DerivationCheck_DisambiguatesDuplicateCallIDAcrossRuns(t *testing.T) {
	exec, _, pub, _ := newExecutor(t)
	platform := types.PlatformMeta{Org: "c360", Platform: "semteams"}

	outcomes := &fakeKVStore{
		entries: map[string][]byte{
			outcomeKeyForExecutionID("exec-run1-web"): marshalOutcome(
				t,
				"exec-run1-web",
				"call_gather_web_1",
				"loop-run1-gather",
				"web_search",
				"https://example.com/old Old run content that does not match.",
			),
			outcomeKeyForExecutionID("exec-run2-web"): marshalOutcome(
				t,
				"exec-run2-web",
				"call_gather_web_1",
				"loop-run2-gather",
				"web_search",
				"https://docs.nats.io/compare NATS vs MQTT, Kafka, and other messaging systems for IoT and edge.",
			),
		},
	}
	entities := &fakeEntityTripleReader{
		entities: map[string]map[string]any{
			"c360.semteams.agentic-loop.agent.execution.loop-run1-gather": {
				predicateRunEntityID: "c360.semteams.agent.chain.execution.run-1",
				predicateRunID:       "run-1",
			},
			"c360.semteams.agentic-loop.agent.execution.loop-run2-gather": {
				predicateRunEntityID: "c360.semteams.agent.chain.execution.run-2",
				predicateRunID:       "run-2",
			},
		},
	}
	exec.SetToolResultResolver(&NATSToolResultResolver{
		entityReader: entities,
		platform:     platform,
		outcomesKV:   outcomes,
	})

	call := defaultCall(defaultArtifactArgs())
	call.LoopID = "loop-run2-synth"
	call.Metadata = map[string]any{
		agentic.MetadataKeyRunID:       "run-2",
		agentic.MetadataKeyRunEntityID: "c360.semteams.agent.chain.execution.run-2",
	}

	res, err := exec.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("expected multi-run disambiguation to pick run-2 outcome, got error: %s", res.Error)
	}
	if pub.calls != 1 {
		t.Errorf("expected 1 payload publish, got %d", pub.calls)
	}
}

func TestExecute_DerivationCheck_RejectsInventedLocatorAndFabricatedQuote(t *testing.T) {
	platform := types.PlatformMeta{Org: "c360", Platform: "semteams"}
	outcomes := &fakeKVStore{
		entries: map[string][]byte{
			outcomeKeyForExecutionID("exec-web-1"): marshalOutcome(
				t,
				"exec-web-1",
				"call_gather_web_1",
				"loop-abc",
				"web_search",
				"[1] NATS Documentation\n    https://docs.nats.io/compare\n    NATS vs MQTT, Kafka, and other messaging systems for IoT and edge.\n",
			),
		},
	}

	t.Run("invented locator not in tool output", func(t *testing.T) {
		exec, tp, pub, _ := newExecutor(t)
		exec.SetToolResultResolver(&NATSToolResultResolver{
			platform:   platform,
			outcomesKV: outcomes,
		})
		args := defaultArtifactArgs()
		args["actors"] = []any{
			map[string]any{
				"name": "OSH driver framework",
				"role": "host",
				"evidence": []any{
					map[string]any{
						"channel":         "web",
						"locator":         "https://invented.example.com/not-in-tool-output",
						"origin_id":       "https://invented.example.com/not-in-tool-output",
						"retrieved_at":    "2026-10-16T12:00:00Z",
						"source_class":    "secondary",
						"quote":           "NATS vs MQTT, Kafka, and other messaging systems for IoT and edge.",
						"tool_result_ref": "call_gather_web_1",
						"revision":        1,
					},
				},
			},
		}
		res, _ := exec.Execute(context.Background(), defaultCall(args))
		if res.ErrorKind != agentic.ToolErrorInvalidArgs {
			t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
		}
		if !strings.Contains(res.Error, "does not reference locator") {
			t.Errorf("Result.Error = %q, want locator mismatch", res.Error)
		}
		if len(tp.snapshot()) != 0 || pub.calls != 0 {
			t.Errorf("expected no side effects on locator mismatch")
		}
	})

	t.Run("fabricated quote not in tool output", func(t *testing.T) {
		exec, tp, pub, _ := newExecutor(t)
		exec.SetToolResultResolver(&NATSToolResultResolver{
			platform:   platform,
			outcomesKV: outcomes,
		})
		args := defaultArtifactArgs()
		args["actors"] = []any{
			map[string]any{
				"name": "OSH driver framework",
				"role": "host",
				"evidence": []any{
					map[string]any{
						"channel":         "web",
						"locator":         "https://docs.nats.io/compare",
						"origin_id":       "https://docs.nats.io/compare",
						"retrieved_at":    "2026-10-16T12:00:00Z",
						"source_class":    "official",
						"quote":           "This sentence was completely fabricated by the model.",
						"tool_result_ref": "call_gather_web_1",
						"revision":        1,
					},
				},
			},
		}
		res, _ := exec.Execute(context.Background(), defaultCall(args))
		if res.ErrorKind != agentic.ToolErrorInvalidArgs {
			t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
		}
		if !strings.Contains(res.Error, "not a verbatim substring") {
			t.Errorf("Result.Error = %q, want verbatim substring failure", res.Error)
		}
		if len(tp.snapshot()) != 0 || pub.calls != 0 {
			t.Errorf("expected no side effects on fabricated quote")
		}
	})
}

func TestExecute_DerivationCheck_Revision2PreservesRevision1AndAddsRevision2Evidence(t *testing.T) {
	exec, _, pub, _ := newExecutor(t)
	platform := types.PlatformMeta{Org: "c360", Platform: "semteams"}
	runEntityID := "c360.semteams.agent.chain.execution.run-rev2"

	outcomes := &fakeKVStore{
		entries: map[string][]byte{
			outcomeKeyForExecutionID("exec-rev1"): marshalOutcome(
				t,
				"exec-rev1",
				"call_gather_rev1",
				"loop-gather-rev1",
				"web_search",
				"https://docs.nats.io/compare NATS vs MQTT, Kafka, and other messaging systems for IoT and edge.",
			),
			outcomeKeyForExecutionID("exec-rev2"): marshalOutcome(
				t,
				"exec-rev2",
				"call_gather_rev2",
				"loop-gather-rev2",
				"web_search",
				"https://mqtt.org/specification MQTT v5.0 protocol specification for lightweight pub/sub messaging.",
			),
		},
	}
	entities := &fakeEntityTripleReader{
		entities: map[string]map[string]any{
			"c360.semteams.agentic-loop.agent.execution.loop-gather-rev1": {predicateRunEntityID: runEntityID},
			"c360.semteams.agentic-loop.agent.execution.loop-gather-rev2": {predicateRunEntityID: runEntityID},
			"c360.semteams.agentic-loop.agent.execution.loop-synth-rev2":  {predicateRunEntityID: runEntityID},
		},
	}
	exec.SetToolResultResolver(&NATSToolResultResolver{
		entityReader: entities,
		platform:     platform,
		outcomesKV:   outcomes,
	})

	args := map[string]any{
		"revision": 2,
		"actors": []any{
			map[string]any{
				"name": "NATS Broker",
				"role": "Unchanged claim from revision 1",
				"evidence": []any{
					map[string]any{
						"channel":         "web",
						"locator":         "https://docs.nats.io/compare",
						"origin_id":       "https://docs.nats.io/compare",
						"retrieved_at":    "2026-10-16T12:00:00Z",
						"source_class":    "official",
						"quote":           "NATS vs MQTT, Kafka, and other messaging systems for IoT and edge.",
						"tool_result_ref": "call_gather_rev1",
						"revision":        1,
					},
				},
			},
			map[string]any{
				"name": "MQTT Broker",
				"role": "New claim added in revision 2",
				"evidence": []any{
					map[string]any{
						"channel":         "web",
						"locator":         "https://mqtt.org/specification",
						"origin_id":       "https://mqtt.org/specification",
						"retrieved_at":    "2026-10-16T12:05:00Z",
						"source_class":    "official",
						"quote":           "MQTT v5.0 protocol specification for lightweight pub/sub messaging.",
						"tool_result_ref": "call_gather_rev2",
						"revision":        2,
					},
				},
			},
		},
		"integration_points": []any{
			map[string]any{
				"from":      "MQTT Broker",
				"to":        "NATS Broker",
				"data":      "telemetry frames",
				"direction": "write",
				"evidence": []any{
					map[string]any{
						"channel":         "web",
						"locator":         "https://mqtt.org/specification",
						"origin_id":       "https://mqtt.org/specification",
						"retrieved_at":    "2026-10-16T12:05:00Z",
						"source_class":    "official",
						"quote":           "lightweight pub/sub messaging",
						"tool_result_ref": "call_gather_rev2",
						"revision":        2,
					},
				},
			},
		},
		"tasks":          []any{"bridge MQTT v5.0 telemetry to NATS"},
		"addressed_gaps": []any{"added MQTT v5.0 specification citation"},
		"open_gaps":      []any{"needs_test_harness: not applicable — pure protocol comparison"},
	}

	call := agentic.ToolCall{
		ID:        "call-emit-rev2",
		Name:      ToolName,
		Arguments: args,
		LoopID:    "loop-synth-rev2",
		Metadata: map[string]any{
			agentic.MetadataKeyRunEntityID: runEntityID,
		},
	}

	res, err := exec.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty", res.Error)
	}
	if pub.calls != 1 {
		t.Errorf("expected 1 payload publish on revision 2, got %d", pub.calls)
	}
}

func TestNATSToolResultResolver_TrajectoryFallbackAndDispatchArgs(t *testing.T) {
	platform := types.PlatformMeta{Org: "c360", Platform: "semteams"}
	loopID := "loop-traj-1"
	callID := "call_http_1"
	blobKey := "trajectory-evidence/v1/sha256/abc123"

	bodyJSON, err := json.Marshal(trajectoryToolCompletionBody{
		Result: agentic.ToolResult{
			CallID:      callID,
			ExecutionID: "exec-http-1",
			LoopID:      loopID,
			Name:        "http_request",
			Content:     "HTTP 200 OK\n\nSemEngine durable execution contract handles attempt journals.",
		},
		DispatchArguments: map[string]any{
			"url": "https://docs.c360.io/semengine/durability",
		},
	})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	envJSON, err := json.Marshal(agentic.TrajectoryEvidenceV1{
		SchemaVersion: "v1",
		Kind:          agentic.TrajectoryKindToolCompleted,
		Body:          bodyJSON,
	})
	if err != nil {
		t.Fatalf("marshal env: %v", err)
	}
	factJSON, err := json.Marshal(agentic.TrajectoryFactV1{
		SchemaVersion:     "v1",
		LoopDigest:        agentic.TrajectoryLoopDigest(loopID),
		Kind:              agentic.TrajectoryKindToolCompleted,
		SourceCorrelation: callID,
		ToolPreview:       "http_request",
		Evidence: &message.StorageReference{
			StorageInstance: agentContentBucket,
			Key:             blobKey,
		},
	})
	if err != nil {
		t.Fatalf("marshal fact: %v", err)
	}

	resolver := &NATSToolResultResolver{
		platform: platform,
		trajectoriesKV: &fakeKVStore{
			entries: map[string][]byte{
				agentic.TrajectoryFactPrefix(loopID) + "fact1": factJSON,
			},
		},
		contentStore: &fakeObjectStore{
			blobs: map[string][]byte{
				blobKey: envJSON,
			},
		},
	}

	exec, _, pub, _ := newExecutor(t)
	exec.SetToolResultResolver(resolver)

	args := defaultArtifactArgs()
	// Locator is in DispatchArguments["url"], while quote is in Content!
	ev := []any{
		map[string]any{
			"channel":         "web",
			"locator":         "https://docs.c360.io/semengine/durability",
			"origin_id":       "https://docs.c360.io/semengine/durability",
			"retrieved_at":    "2026-10-16T12:00:00Z",
			"source_class":    "official",
			"quote":           "SemEngine durable execution contract handles attempt journals.",
			"tool_result_ref": loopID + "/" + callID,
			"revision":        1,
		},
	}
	args["actors"] = []any{
		map[string]any{"name": "SemEngine", "role": "substrate", "evidence": ev},
	}
	args["integration_points"] = []any{
		map[string]any{"from": "SemTeams", "to": "SemEngine", "data": "attempts", "direction": "write", "evidence": ev},
	}

	call := defaultCall(args)
	call.LoopID = loopID
	res, err := exec.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty", res.Error)
	}
	if pub.calls != 1 {
		t.Errorf("expected 1 payload publish, got %d", pub.calls)
	}
}
