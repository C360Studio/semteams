package emitartifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/types"

	"github.com/c360studio/semteams/cmd/semteams/research"
)

type fakeTrajectoryKVStore struct {
	entries       map[string][]byte
	getErr        error
	filterErr     error
	recordedCalls []string
}

func (f *fakeTrajectoryKVStore) Get(_ context.Context, key string) ([]byte, error) {
	if f != nil && f.getErr != nil {
		return nil, f.getErr
	}
	if f == nil || f.entries == nil {
		return nil, fmt.Errorf("key %q not found", key)
	}
	v, ok := f.entries[key]
	if !ok {
		return nil, fmt.Errorf("key %q not found", key)
	}
	return v, nil
}

func (f *fakeTrajectoryKVStore) FilteredKeys(_ context.Context, prefixFilter string) ([]string, error) {
	if f == nil {
		return nil, nil
	}
	f.recordedCalls = append(f.recordedCalls, prefixFilter)
	if f.filterErr != nil {
		return nil, f.filterErr
	}
	prefix := strings.TrimSuffix(prefixFilter, ">")
	var keys []string
	for k := range f.entries {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

type fakeObjectStore struct {
	blobs  map[string][]byte
	getErr error
}

func (f *fakeObjectStore) GetBytes(_ context.Context, key string) ([]byte, error) {
	if f != nil && f.getErr != nil {
		return nil, f.getErr
	}
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
	entities    map[string]map[string]any
	allEntities map[string]map[string][]any
	err         error
}

func (f *fakeEntityTripleReader) ReadEntity(_ context.Context, entityID string) (map[string]any, error) {
	if f != nil && f.err != nil {
		return nil, f.err
	}
	if f == nil || f.entities == nil {
		return nil, nil
	}
	return f.entities[entityID], nil
}

func (f *fakeEntityTripleReader) ReadEntityAll(_ context.Context, entityID string) (map[string][]any, error) {
	if f != nil && f.err != nil {
		return nil, f.err
	}
	if f != nil && f.allEntities != nil {
		if m, ok := f.allEntities[entityID]; ok {
			return m, nil
		}
	}
	if f == nil || f.entities == nil {
		return nil, nil
	}
	single := f.entities[entityID]
	if len(single) == 0 {
		return nil, nil
	}
	out := make(map[string][]any, len(single))
	for k, v := range single {
		switch s := v.(type) {
		case []any:
			out[k] = s
		case []string:
			items := make([]any, 0, len(s))
			for _, str := range s {
				items = append(items, str)
			}
			out[k] = items
		default:
			out[k] = []any{v}
		}
	}
	return out, nil
}

type trajectoryEntrySpec struct {
	LoopID            string
	CallID            string
	ExecutionID       string
	ToolName          string
	Status            agentic.TrajectoryStatus
	Error             string
	ErrorKind         agentic.ToolErrorKind
	Content           string
	DispatchArguments map[string]any
	ObservedAt        time.Time
	StorageInstance   string
	DigestOverride    string
}

func addTrajectoryEntry(
	t *testing.T,
	kv *fakeTrajectoryKVStore,
	obj *fakeObjectStore,
	spec trajectoryEntrySpec,
) {
	t.Helper()
	if spec.Status == "" {
		spec.Status = agentic.TrajectoryStatusCompleted
	}
	if spec.ObservedAt.IsZero() {
		spec.ObservedAt = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	}
	if spec.StorageInstance == "" {
		spec.StorageInstance = defaultAgentContentBucket
	}

	bodyJSON, err := json.Marshal(trajectoryToolCompletionBody{
		Result: agentic.ToolResult{
			CallID:      spec.CallID,
			ExecutionID: spec.ExecutionID,
			LoopID:      spec.LoopID,
			Name:        spec.ToolName,
			Content:     spec.Content,
			Error:       spec.Error,
			ErrorKind:   spec.ErrorKind,
		},
		DispatchArguments: spec.DispatchArguments,
	})
	if err != nil {
		t.Fatalf("marshal trajectory body: %v", err)
	}
	envJSON, err := json.Marshal(agentic.TrajectoryEvidenceV1{
		SchemaVersion: "v1",
		Kind:          agentic.TrajectoryKindToolCompleted,
		Body:          bodyJSON,
	})
	if err != nil {
		t.Fatalf("marshal trajectory evidence envelope: %v", err)
	}

	sum := sha256.Sum256(envJSON)
	digest := hex.EncodeToString(sum[:])
	blobKey := "trajectory-evidence/v1/sha256/" + digest
	if obj.blobs == nil {
		obj.blobs = make(map[string][]byte)
	}
	obj.blobs[blobKey] = envJSON

	factDigest := digest
	if spec.DigestOverride != "" {
		factDigest = spec.DigestOverride
	}

	factJSON, err := json.Marshal(agentic.TrajectoryFactV1{
		SchemaVersion:     "v1",
		LoopDigest:        agentic.TrajectoryLoopDigest(spec.LoopID),
		Kind:              agentic.TrajectoryKindToolCompleted,
		Status:            spec.Status,
		ObservedAt:        spec.ObservedAt,
		SourceCorrelation: spec.CallID,
		ToolPreview:       spec.ToolName,
		EvidenceDigest:    factDigest,
		Evidence: &message.StorageReference{
			StorageInstance: spec.StorageInstance,
			Key:             blobKey,
		},
	})
	if err != nil {
		t.Fatalf("marshal trajectory fact: %v", err)
	}

	if kv.entries == nil {
		kv.entries = make(map[string][]byte)
	}
	factKey := fmt.Sprintf("%s%s.%s", agentic.TrajectoryFactPrefix(spec.LoopID), spec.CallID, spec.ExecutionID)
	kv.entries[factKey] = factJSON
}

func TestExecute_DerivationCheck_MultiChannelHappyPath(t *testing.T) {
	exec, tp, pub, _ := newExecutor(t)
	platform := types.PlatformMeta{Org: "c360", Platform: "semteams"}

	runEntityID := agentic.ChainExecutionEntityID("c360", "semteams", "run-100")
	planLoopEntityID := agentic.LoopExecutionEntityID("c360", "semteams", "loop-plan-1")
	gatherLoopEntityID := agentic.LoopExecutionEntityID("c360", "semteams", "loop-gather-1")
	synthLoopEntityID := agentic.LoopExecutionEntityID("c360", "semteams", "loop-synth-1")

	observedAt := time.Date(2026, 10, 16, 12, 34, 56, 0, time.UTC)
	githubPayload, err := json.Marshal(map[string]any{
		"channel": "github",
		"records": []research.EvidenceRecord{
			{
				Channel:     research.ChannelGitHub,
				Kind:        "issue",
				Locator:     "https://github.com/C360Studio/semteams/issues/271",
				OriginID:    "github://c360studio/semteams/issues/271",
				RetrievedAt: observedAt,
				SourceClass: research.SourceClassPrimary,
				RawBody:     "#271 Evidence contract & mechanical derivation check: enforce citations across github, web, and semsource.",
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal github payload: %v", err)
	}

	trajKV := &fakeTrajectoryKVStore{}
	objStore := &fakeObjectStore{}
	addTrajectoryEntry(t, trajKV, objStore, trajectoryEntrySpec{
		LoopID:          "loop-gather-1",
		CallID:          "call_gather_gh_1",
		ExecutionID:     "exec-gh-1",
		ToolName:        "github_read",
		Content:         string(githubPayload),
		ObservedAt:      observedAt,
		StorageInstance: "agent-content-store",
	})
	addTrajectoryEntry(t, trajKV, objStore, trajectoryEntrySpec{
		LoopID:      "loop-gather-1",
		CallID:      "call_gather_web_1",
		ExecutionID: "exec-web-1",
		ToolName:    "web_search",
		Content:     "[stub] web_search results for: nats compare\n[1] NATS Documentation — Comparison with Other Systems\n    https://docs.nats.io/compare\n    NATS vs MQTT, Kafka, and other messaging systems for IoT and edge.\n",
		DispatchArguments: map[string]any{
			"query": "nats compare",
		},
		ObservedAt:      observedAt,
		StorageInstance: "agent-content-store",
	})
	addTrajectoryEntry(t, trajKV, objStore, trajectoryEntrySpec{
		LoopID:      "loop-gather-1",
		CallID:      "call_gather_ss_1",
		ExecutionID: "exec-ss-1",
		ToolName:    "graph_query",
		Content:     `{"entity_id":"c360.semteams.semsource.git.repo.semstreams","summary":"Frozen SemStreams dependency supplies graph-ingest and rule-processor."}`,
		DispatchArguments: map[string]any{
			"entity_id": "c360.semteams.semsource.git.repo.semstreams",
		},
		ObservedAt:      observedAt,
		StorageInstance: "agent-content-store",
	})

	entities := &fakeEntityTripleReader{
		entities: map[string]map[string]any{
			planLoopEntityID: {
				predicateRunEntityID:         runEntityID,
				predicateLoopRun:             "run-100",
				predicatePlanRevision:        1,
				predicateGatherCompletedLoop: []string{"loop-gather-1"},
			},
			gatherLoopEntityID: {
				predicateRunEntityID:           runEntityID,
				predicateLoopRun:               "run-100",
				predicateLineagePlanLoopEntity: planLoopEntityID,
			},
			synthLoopEntityID: {
				predicateRunEntityID:     runEntityID,
				predicateLoopRun:         "run-100",
				predicateLineagePlanLoop: "loop-plan-1",
			},
		},
	}

	resolver := &NATSToolResultResolver{
		entityReader:   entities,
		platform:       platform,
		storageBuckets: map[string]string{"agent-content-store": defaultAgentContentBucket},
		trajectoriesKV: trajKV,
		contentStores:  map[string]byteObjectStore{defaultAgentContentBucket: objStore},
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
						"source_class":    "primary",
						"quote":           "enforce citations across github, web, and semsource",
						"tool_result_ref": "call_gather_gh_1",
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
						"source_class":    "primary",
						"quote":           "Frozen SemStreams dependency supplies graph-ingest and rule-processor.",
						"tool_result_ref": "call_gather_ss_1",
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
						"source_class":    "official",
						"quote":           "NATS vs MQTT, Kafka, and other messaging systems for IoT and edge.",
						"tool_result_ref": "call_gather_web_1",
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
	_, payloadData, pubCalls := pub.snapshot()
	if pubCalls != 1 {
		t.Errorf("expected 1 payload publish call, got %d", pubCalls)
	}

	// Verify server-side stamps on the published payload:
	// - origin_id derived from (channel, locator)
	// - retrieved_at stamped from trajectory ObservedAt
	// - kind stamped ("issue" for github_read, "entity" for semsource, "snippet" for web_search)
	var published research.Artifact
	if err := json.Unmarshal(payloadData, &published); err != nil {
		t.Fatalf("decode published artifact: %v", err)
	}
	ghEv := published.Actors[0].Evidence[0]
	if ghEv.OriginID != "github://c360studio/semteams/issues/271" {
		t.Errorf("github OriginID = %q, want github://c360studio/semteams/issues/271", ghEv.OriginID)
	}
	if ghEv.Kind != "issue" {
		t.Errorf("github Kind = %q, want issue", ghEv.Kind)
	}
	if !ghEv.RetrievedAt.Equal(observedAt) {
		t.Errorf("github RetrievedAt = %v, want %v", ghEv.RetrievedAt, observedAt)
	}
	webEv := published.IntegrationPoints[0].Evidence[0]
	if webEv.Kind != research.EvidenceKindSnippet {
		t.Errorf("web_search Kind = %q, want %q", webEv.Kind, research.EvidenceKindSnippet)
	}

	// Verify that FilteredKeys was only called with per-loop TrajectoryFactPrefix filters, never an unfiltered scan.
	for _, filter := range trajKV.recordedCalls {
		if !strings.HasPrefix(filter, "v1.") || filter == "v1.>" || filter == ">" || filter == "*" {
			t.Errorf("unexpected broad trajectory key filter %q", filter)
		}
	}
}

func TestExecute_DerivationCheck_RejectsUnknownToolResultRef(t *testing.T) {
	exec, tp, pub, _ := newExecutor(t)
	exec.SetToolResultResolver(&NATSToolResultResolver{
		platform:       types.PlatformMeta{Org: "c360", Platform: "semteams"},
		trajectoriesKV: &fakeTrajectoryKVStore{entries: map[string][]byte{}},
		contentStore:   &fakeObjectStore{blobs: map[string][]byte{}},
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

	trajKV := &fakeTrajectoryKVStore{}
	objStore := &fakeObjectStore{}
	addTrajectoryEntry(t, trajKV, objStore, trajectoryEntrySpec{
		LoopID:      "loop-other-run",
		CallID:      "call_gather_web_1",
		ExecutionID: "exec-foreign-1",
		ToolName:    "web_search",
		Content:     "[1] NATS Docs\n    https://docs.nats.io/compare\n    NATS vs MQTT, Kafka, and other messaging systems for IoT and edge.\n",
	})

	entities := &fakeEntityTripleReader{
		entities: map[string]map[string]any{
			"c360.semteams.agentic-loop.agent.execution.loop-other-run": {
				predicateRunEntityID: agentic.ChainExecutionEntityID("c360", "semteams", "run-other"),
				predicateLoopRun:     "run-other",
			},
			"c360.semteams.agentic-loop.agent.execution.loop-abc": {
				predicateRunEntityID: agentic.ChainExecutionEntityID("c360", "semteams", "run-current"),
				predicateLoopRun:     "run-current",
			},
		},
	}
	exec.SetToolResultResolver(&NATSToolResultResolver{
		entityReader:   entities,
		platform:       platform,
		trajectoriesKV: trajKV,
		contentStore:   objStore,
	})

	args := defaultArtifactArgs()
	// Reference the foreign loop explicitly via <loop_id>/<call_id>.
	args["actors"].([]any)[0].(map[string]any)["evidence"].([]any)[0].(map[string]any)["tool_result_ref"] = "loop-other-run/call_gather_web_1"

	call := defaultCall(args)
	call.Metadata = map[string]any{
		agentic.MetadataKeyRunID:       "run-current",
		agentic.MetadataKeyRunEntityID: agentic.ChainExecutionEntityID("c360", "semteams", "run-current"),
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

	run1EntityID := agentic.ChainExecutionEntityID("c360", "semteams", "run-1")
	run2EntityID := agentic.ChainExecutionEntityID("c360", "semteams", "run-2")

	trajKV := &fakeTrajectoryKVStore{}
	objStore := &fakeObjectStore{}
	addTrajectoryEntry(t, trajKV, objStore, trajectoryEntrySpec{
		LoopID:      "loop-run1-gather",
		CallID:      "call_gather_web_1",
		ExecutionID: "exec-run1-web",
		ToolName:    "web_search",
		Content:     "[1] Old\n    https://example.com/old\n    Old run content that does not match.\n",
	})
	addTrajectoryEntry(t, trajKV, objStore, trajectoryEntrySpec{
		LoopID:      "loop-run2-gather",
		CallID:      "call_gather_web_1",
		ExecutionID: "exec-run2-web",
		ToolName:    "web_search",
		Content:     "[1] NATS Docs\n    https://docs.nats.io/compare\n    NATS vs MQTT, Kafka, and other messaging systems for IoT and edge.\n",
	})

	entities := &fakeEntityTripleReader{
		entities: map[string]map[string]any{
			"c360.semteams.agentic-loop.agent.execution.loop-run1-gather": {
				predicateRunEntityID: run1EntityID,
				predicateLoopRun:     "run-1",
			},
			"c360.semteams.agentic-loop.agent.execution.loop-run2-plan": {
				predicateRunEntityID:         run2EntityID,
				predicateLoopRun:             "run-2",
				predicatePlanRevision:        1,
				predicateGatherCompletedLoop: []string{"loop-run2-gather"},
			},
			"c360.semteams.agentic-loop.agent.execution.loop-run2-gather": {
				predicateRunEntityID: run2EntityID,
				predicateLoopRun:     "run-2",
			},
			"c360.semteams.agentic-loop.agent.execution.loop-run2-synth": {
				predicateRunEntityID:     run2EntityID,
				predicateLoopRun:         "run-2",
				predicateLineagePlanLoop: "loop-run2-plan",
			},
		},
	}
	exec.SetToolResultResolver(&NATSToolResultResolver{
		entityReader:   entities,
		platform:       platform,
		trajectoriesKV: trajKV,
		contentStore:   objStore,
	})

	call := defaultCall(defaultArtifactArgs())
	call.LoopID = "loop-run2-synth"
	call.Metadata = map[string]any{
		agentic.MetadataKeyRunID:       "run-2",
		agentic.MetadataKeyRunEntityID: run2EntityID,
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

func TestExecute_DerivationCheck_C1_RejectsSelfAuthoredFailedAndEchoedTools(t *testing.T) {
	platform := types.PlatformMeta{Org: "c360", Platform: "semteams"}

	cases := []struct {
		name      string
		spec      trajectoryEntrySpec
		wantError string
	}{
		{
			name: "rejects read_loop_result (model reading prior loop prose)",
			spec: trajectoryEntrySpec{
				LoopID:      "loop-abc",
				CallID:      "call_gather_web_1",
				ExecutionID: "exec-1",
				ToolName:    "read_loop_result",
				Content:     "https://docs.nats.io/compare NATS vs MQTT, Kafka, and other messaging systems for IoT and edge.",
			},
			wantError: "not an allowed retrieval tool",
		},
		{
			name: "rejects scratchpad write/read",
			spec: trajectoryEntrySpec{
				LoopID:      "loop-abc",
				CallID:      "call_gather_web_1",
				ExecutionID: "exec-2",
				ToolName:    "scratchpad",
				Content:     "https://docs.nats.io/compare NATS vs MQTT, Kafka, and other messaging systems for IoT and edge.",
			},
			wantError: "not an allowed retrieval tool",
		},
		{
			name: "rejects decide terminal tool",
			spec: trajectoryEntrySpec{
				LoopID:      "loop-abc",
				CallID:      "call_gather_web_1",
				ExecutionID: "exec-3",
				ToolName:    "decide",
				Content:     "https://docs.nats.io/compare NATS vs MQTT, Kafka, and other messaging systems for IoT and edge.",
			},
			wantError: "not an allowed retrieval tool",
		},
		{
			name: "rejects bash echo command",
			spec: trajectoryEntrySpec{
				LoopID:      "loop-abc",
				CallID:      "call_gather_web_1",
				ExecutionID: "exec-4",
				ToolName:    "bash",
				Content:     "https://docs.nats.io/compare NATS vs MQTT, Kafka, and other messaging systems for IoT and edge.",
				DispatchArguments: map[string]any{
					"command": "echo 'https://docs.nats.io/compare NATS vs MQTT, Kafka, and other messaging systems for IoT and edge.'",
				},
			},
			wantError: "unless command invokes curl or wget",
		},
		{
			name: "rejects failed tool execution (404 / curl error)",
			spec: trajectoryEntrySpec{
				LoopID:      "loop-abc",
				CallID:      "call_gather_web_1",
				ExecutionID: "exec-5",
				ToolName:    "http_request",
				Status:      agentic.TrajectoryStatusFailed,
				Error:       "404 Not Found: https://docs.nats.io/compare NATS vs MQTT, Kafka, and other messaging systems for IoT and edge.",
				Content:     "404 Not Found: https://docs.nats.io/compare NATS vs MQTT, Kafka, and other messaging systems for IoT and edge.",
			},
			wantError: "failed and cannot be cited as evidence",
		},
		{
			name: "rejects quote echoed from dispatch arguments (web_search query echo)",
			spec: trajectoryEntrySpec{
				LoopID:      "loop-abc",
				CallID:      "call_gather_web_1",
				ExecutionID: "exec-6",
				ToolName:    "web_search",
				Content:     "[1] NATS Docs\n    https://docs.nats.io/compare\n    NATS vs MQTT, Kafka, and other messaging systems for IoT and edge.\n",
				DispatchArguments: map[string]any{
					"query": "NATS vs MQTT, Kafka, and other messaging systems for IoT and edge.",
				},
			},
			wantError: "dispatch arguments (query/command echo is not retrieved evidence)",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			exec, tp, pub, _ := newExecutor(t)
			trajKV := &fakeTrajectoryKVStore{}
			objStore := &fakeObjectStore{}
			addTrajectoryEntry(t, trajKV, objStore, tc.spec)

			exec.SetToolResultResolver(&NATSToolResultResolver{
				platform:       platform,
				trajectoriesKV: trajKV,
				contentStore:   objStore,
			})

			res, err := exec.Execute(context.Background(), defaultCall(defaultArtifactArgs()))
			if err != nil {
				t.Fatalf("Execute err: %v", err)
			}
			if res.ErrorKind != agentic.ToolErrorInvalidArgs {
				t.Errorf("ErrorKind = %v, want %v (err=%s)", res.ErrorKind, agentic.ToolErrorInvalidArgs, res.Error)
			}
			if !strings.Contains(res.Error, tc.wantError) {
				t.Errorf("Result.Error = %q, want substring %q", res.Error, tc.wantError)
			}
			if len(tp.snapshot()) != 0 || pub.calls != 0 {
				t.Errorf("expected no side effects on rejected citation")
			}
		})
	}

	t.Run("accepts bash curl piped into head/sed/grep/jq with quoted multi-param URL", func(t *testing.T) {
		for _, cmd := range []string{
			"curl -sL 'https://www.sec.gov/cgi-bin/browse-edgar?action=getcompany&CIK=CMG&type=10-Q' | head -200",
			"curl -sL https://www.rfc-editor.org/rfc/rfc6455.html | sed -n '500,700p'",
			"curl -sL https://api.example.com/v1/items | jq '.items[]'",
		} {
			if !isBashFetchCommand(map[string]any{"command": cmd}, false) {
				t.Errorf("isBashFetchCommand(%q) = false, want true", cmd)
			}
		}
		for _, bad := range []string{
			"curl -sL https://docs.nats.io/compare | python3 -c 'print(1)'",
			"curl -sL https://docs.nats.io/compare; echo injected",
			"head -200 /etc/hosts",
		} {
			if isBashFetchCommand(map[string]any{"command": bad}, false) {
				t.Errorf("isBashFetchCommand(%q) = true, want false", bad)
			}
		}
	})
}

func TestExecute_DerivationCheck_M1_SameItemCooccurrenceAndBoundedURL(t *testing.T) {
	platform := types.PlatformMeta{Org: "c360", Platform: "semteams"}

	t.Run("rejects web_search cross-block citation (locator in [1], quote in [2])", func(t *testing.T) {
		exec, _, pub, _ := newExecutor(t)
		trajKV := &fakeTrajectoryKVStore{}
		objStore := &fakeObjectStore{}
		addTrajectoryEntry(t, trajKV, objStore, trajectoryEntrySpec{
			LoopID:      "loop-abc",
			CallID:      "call_gather_web_1",
			ExecutionID: "exec-1",
			ToolName:    "web_search",
			Content: "[1] First Result\n    https://docs.nats.io/compare\n    Summary of the first result only.\n\n" +
				"[2] Second Result\n    https://other.example.com/mqtt\n    NATS vs MQTT, Kafka, and other messaging systems for IoT and edge.\n",
		})
		exec.SetToolResultResolver(&NATSToolResultResolver{
			platform:       platform,
			trajectoriesKV: trajKV,
			contentStore:   objStore,
		})

		res, _ := exec.Execute(context.Background(), defaultCall(defaultArtifactArgs()))
		if res.ErrorKind != agentic.ToolErrorInvalidArgs || !strings.Contains(res.Error, "does not appear in the web_search result block") {
			t.Errorf("expected cross-block web_search rejection, got kind=%v err=%q", res.ErrorKind, res.Error)
		}
		if pub.calls != 0 {
			t.Errorf("expected 0 publishes")
		}
	})

	t.Run("rejects web_search citation claiming kind=body", func(t *testing.T) {
		exec, _, _, _ := newExecutor(t)
		trajKV := &fakeTrajectoryKVStore{}
		objStore := &fakeObjectStore{}
		addTrajectoryEntry(t, trajKV, objStore, trajectoryEntrySpec{
			LoopID:      "loop-abc",
			CallID:      "call_gather_web_1",
			ExecutionID: "exec-1",
			ToolName:    "web_search",
			Content:     "[1] First Result\n    https://docs.nats.io/compare\n    NATS vs MQTT, Kafka, and other messaging systems for IoT and edge.\n",
		})
		exec.SetToolResultResolver(&NATSToolResultResolver{
			platform:       platform,
			trajectoriesKV: trajKV,
			contentStore:   objStore,
		})

		args := defaultArtifactArgs()
		args["actors"].([]any)[0].(map[string]any)["evidence"].([]any)[0].(map[string]any)["kind"] = "body"
		res, _ := exec.Execute(context.Background(), defaultCall(args))
		if res.ErrorKind != agentic.ToolErrorInvalidArgs || !strings.Contains(res.Error, "only provide search-result snippets") {
			t.Errorf("expected web_search kind=body rejection, got kind=%v err=%q", res.ErrorKind, res.Error)
		}
	})

	t.Run("rejects URL prefix match (https://docs.nats.io/compare vs https://docs.nats.io/compare-other)", func(t *testing.T) {
		exec, _, _, _ := newExecutor(t)
		trajKV := &fakeTrajectoryKVStore{}
		objStore := &fakeObjectStore{}
		addTrajectoryEntry(t, trajKV, objStore, trajectoryEntrySpec{
			LoopID:      "loop-abc",
			CallID:      "call_gather_web_1",
			ExecutionID: "exec-1",
			ToolName:    "web_search",
			Content:     "[1] Other Page\n    https://docs.nats.io/compare-other\n    NATS vs MQTT, Kafka, and other messaging systems for IoT and edge.\n",
		})
		exec.SetToolResultResolver(&NATSToolResultResolver{
			platform:       platform,
			trajectoriesKV: trajKV,
			contentStore:   objStore,
		})

		res, _ := exec.Execute(context.Background(), defaultCall(defaultArtifactArgs()))
		if res.ErrorKind != agentic.ToolErrorInvalidArgs || !strings.Contains(res.Error, "no result block matching locator") {
			t.Errorf("expected URL prefix rejection, got kind=%v err=%q", res.ErrorKind, res.Error)
		}
	})

	t.Run("rejects multi-item JSON cross-element match and JSON map-key match", func(t *testing.T) {
		exec, _, _, _ := newExecutor(t)
		trajKV := &fakeTrajectoryKVStore{}
		objStore := &fakeObjectStore{}
		addTrajectoryEntry(t, trajKV, objStore, trajectoryEntrySpec{
			LoopID:      "loop-abc",
			CallID:      "call_ss_1",
			ExecutionID: "exec-ss-1",
			ToolName:    "graph_query",
			Content: `{"entities":[` +
				`{"id":"c360.semteams.semsource.git.repo.alpha","summary":"Alpha entity summary only."},` +
				`{"id":"c360.semteams.semsource.git.repo.beta","summary":"Frozen SemStreams dependency supplies graph-ingest."}` +
				`]}`,
		})
		exec.SetToolResultResolver(&NATSToolResultResolver{
			platform:       platform,
			trajectoriesKV: trajKV,
			contentStore:   objStore,
		})

		args := defaultArtifactArgs()
		ev := []any{
			map[string]any{
				"channel":         "semsource",
				"locator":         "semsource://c360.semteams.semsource.git.repo.alpha",
				"source_class":    "primary",
				"quote":           "Frozen SemStreams dependency supplies graph-ingest.",
				"tool_result_ref": "call_ss_1",
			},
		}
		args["actors"] = []any{map[string]any{"name": "A", "role": "R", "evidence": ev}}
		args["integration_points"] = []any{map[string]any{"from": "A", "to": "B", "data": "d", "direction": "read", "evidence": ev}}

		res, _ := exec.Execute(context.Background(), defaultCall(args))
		if res.ErrorKind != agentic.ToolErrorInvalidArgs || !strings.Contains(res.Error, "does not appear in the result item") {
			t.Errorf("expected JSON multi-item cross-element rejection, got kind=%v err=%q", res.ErrorKind, res.Error)
		}
	})
}

func TestExecute_DerivationCheck_Revision2PreservesRevision1AndChecksProducerRevision(t *testing.T) {
	exec, tp, pub, _ := newExecutor(t)
	platform := types.PlatformMeta{Org: "c360", Platform: "semteams"}
	runEntityID := agentic.ChainExecutionEntityID("c360", "semteams", "run-rev2")

	trajKV := &fakeTrajectoryKVStore{}
	objStore := &fakeObjectStore{}
	addTrajectoryEntry(t, trajKV, objStore, trajectoryEntrySpec{
		LoopID:      "loop-gather-rev1",
		CallID:      "call_gather_rev1",
		ExecutionID: "exec-rev1",
		ToolName:    "web_search",
		Content:     "[1] NATS Compare\n    https://docs.nats.io/compare\n    NATS vs MQTT, Kafka, and other messaging systems for IoT and edge.\n",
	})
	addTrajectoryEntry(t, trajKV, objStore, trajectoryEntrySpec{
		LoopID:      "loop-gather-rev2",
		CallID:      "call_gather_rev2",
		ExecutionID: "exec-rev2",
		ToolName:    "web_search",
		Content:     "[1] MQTT Spec\n    https://mqtt.org/specification\n    MQTT v5.0 protocol specification for lightweight pub/sub messaging.\n",
	})

	entities := &fakeEntityTripleReader{
		entities: map[string]map[string]any{
			"c360.semteams.agentic-loop.agent.execution.loop-plan-rev1": {
				predicateRunEntityID:         runEntityID,
				predicatePlanRevision:        1,
				predicateGatherCompletedLoop: []string{"loop-gather-rev1"},
			},
			"c360.semteams.agentic-loop.agent.execution.loop-gather-rev1": {
				predicateRunEntityID:           runEntityID,
				predicateLineagePlanLoopEntity: "c360.semteams.agentic-loop.agent.execution.loop-plan-rev1",
			},
			"c360.semteams.agentic-loop.agent.execution.loop-synth-rev1": {
				predicateRunEntityID:      runEntityID,
				predicateArtifactRevision: 1,
				predicateLineagePlanLoop:  "loop-plan-rev1",
			},
			"c360.semteams.agentic-loop.agent.execution.loop-plan-rev2": {
				predicateRunEntityID:            runEntityID,
				predicatePlanRevision:           2,
				predicateLineagePriorSynthesize: "loop-synth-rev1",
				predicateGatherCompletedLoop:    []string{"loop-gather-rev2"},
			},
			"c360.semteams.agentic-loop.agent.execution.loop-gather-rev2": {
				predicateRunEntityID:           runEntityID,
				predicateLineagePlanLoopEntity: "c360.semteams.agentic-loop.agent.execution.loop-plan-rev2",
			},
			"c360.semteams.agentic-loop.agent.execution.loop-synth-rev2": {
				predicateRunEntityID:            runEntityID,
				predicateLineagePlanLoop:        "loop-plan-rev2",
				predicateLineagePriorSynthesize: "loop-synth-rev1",
			},
		},
	}
	exec.SetToolResultResolver(&NATSToolResultResolver{
		entityReader:   entities,
		platform:       platform,
		trajectoriesKV: trajKV,
		contentStore:   objStore,
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

	// Verify rendered markdown includes ref=, retrieved=, kind, and quoted Quote so revision-2+ can round-trip citations.
	pathTriple := triplesByPredicate(tp.snapshot())[predicateArtifactPath]
	mdBody, err := readFileContent(t, pathTriple.Object.(string))
	if err != nil {
		t.Fatalf("read rendered markdown: %v", err)
	}
	for _, wantSub := range []string{"ref=call_gather_rev1", "ref=call_gather_rev2", "snippet", "retrieved=2026-10-16T12:00:00Z"} {
		if !strings.Contains(mdBody, wantSub) {
			t.Errorf("rendered markdown missing %q:\n%s", wantSub, mdBody)
		}
	}

	// Now verify that lying about revision (claiming revision 2 on a revision 1 tool result) is rejected!
	argsBadRev := map[string]any{
		"revision": 2,
		"actors": []any{
			map[string]any{
				"name": "NATS Broker",
				"role": "Mislabeled revision",
				"evidence": []any{
					map[string]any{
						"channel":         "web",
						"locator":         "https://docs.nats.io/compare",
						"source_class":    "official",
						"quote":           "NATS vs MQTT, Kafka, and other messaging systems for IoT and edge.",
						"tool_result_ref": "call_gather_rev1",
						"revision":        2, // mismatch: produced by rev 1 loop
					},
				},
			},
		},
		"integration_points": args["integration_points"],
		"tasks":              args["tasks"],
		"open_gaps":          args["open_gaps"],
	}
	resBad, _ := exec.Execute(context.Background(), agentic.ToolCall{
		ID:        "call-emit-rev2-bad",
		Name:      ToolName,
		Arguments: argsBadRev,
		LoopID:    "loop-synth-rev2",
		Metadata:  call.Metadata,
	})
	if resBad.ErrorKind != agentic.ToolErrorInvalidArgs || !strings.Contains(resBad.Error, "does not match producing loop") {
		t.Errorf("expected revision mismatch rejection, got kind=%v err=%q", resBad.ErrorKind, resBad.Error)
	}
}

func TestExecute_DerivationCheck_M5_ErrorKindClassificationAndDigestCheck(t *testing.T) {
	platform := types.PlatformMeta{Org: "c360", Platform: "semteams"}

	t.Run("NATS KV filter error surfaces as ToolErrorNetwork", func(t *testing.T) {
		exec, _, _, _ := newExecutor(t)
		exec.SetToolResultResolver(&NATSToolResultResolver{
			platform:       platform,
			trajectoriesKV: &fakeTrajectoryKVStore{filterErr: errors.New("jetstream timeout")},
			contentStore:   &fakeObjectStore{},
		})

		res, _ := exec.Execute(context.Background(), defaultCall(defaultArtifactArgs()))
		if res.ErrorKind != agentic.ToolErrorNetwork {
			t.Errorf("ErrorKind = %v, want %v (err=%s)", res.ErrorKind, agentic.ToolErrorNetwork, res.Error)
		}
	})

	t.Run("EntityReader error surfaces as ToolErrorNetwork", func(t *testing.T) {
		exec, _, _, _ := newExecutor(t)
		exec.SetToolResultResolver(&NATSToolResultResolver{
			platform:       platform,
			entityReader:   &fakeEntityTripleReader{err: errors.New("graph-query unreachable")},
			trajectoriesKV: &fakeTrajectoryKVStore{},
			contentStore:   &fakeObjectStore{},
		})

		res, _ := exec.Execute(context.Background(), defaultCall(defaultArtifactArgs()))
		if res.ErrorKind != agentic.ToolErrorNetwork {
			t.Errorf("ErrorKind = %v, want %v (err=%s)", res.ErrorKind, agentic.ToolErrorNetwork, res.Error)
		}
	})

	t.Run("EvidenceDigest mismatch surfaces as ToolErrorInternal", func(t *testing.T) {
		exec, _, _, _ := newExecutor(t)
		trajKV := &fakeTrajectoryKVStore{}
		objStore := &fakeObjectStore{}
		addTrajectoryEntry(t, trajKV, objStore, trajectoryEntrySpec{
			LoopID:         "loop-abc",
			CallID:         "call_gather_web_1",
			ExecutionID:    "exec-1",
			ToolName:       "web_search",
			Content:        "[1] NATS\n    https://docs.nats.io/compare\n    NATS vs MQTT, Kafka, and other messaging systems for IoT and edge.\n",
			DigestOverride: "0000000000000000000000000000000000000000000000000000000000000000",
		})
		exec.SetToolResultResolver(&NATSToolResultResolver{
			platform:       platform,
			trajectoriesKV: trajKV,
			contentStore:   objStore,
		})

		res, _ := exec.Execute(context.Background(), defaultCall(defaultArtifactArgs()))
		if res.ErrorKind != agentic.ToolErrorInternal || !strings.Contains(res.Error, "digest mismatch") {
			t.Errorf("expected ToolErrorInternal digest mismatch, got kind=%v err=%q", res.ErrorKind, res.Error)
		}
	})

	t.Run("typed-nil NATSToolResultResolver does not panic", func(t *testing.T) {
		exec, _, pub, _ := newExecutor(t)
		var typedNil *NATSToolResultResolver
		exec.SetToolResultResolver(typedNil)

		res, err := exec.Execute(context.Background(), defaultCall(defaultArtifactArgs()))
		if err != nil || res.Error != "" {
			t.Fatalf("typed-nil resolver should be treated as nil, got err=%v res.Error=%q", err, res.Error)
		}
		if pub.calls != 1 {
			t.Errorf("expected 1 publish, got %d", pub.calls)
		}
	})
}

func TestNATSToolResultResolver_HttpRequestAndBashCurlSupport(t *testing.T) {
	platform := types.PlatformMeta{Org: "c360", Platform: "semteams"}
	loopID := "loop-traj-1"

	trajKV := &fakeTrajectoryKVStore{}
	objStore := &fakeObjectStore{}
	addTrajectoryEntry(t, trajKV, objStore, trajectoryEntrySpec{
		LoopID:      loopID,
		CallID:      "call_http_1",
		ExecutionID: "exec-http-1",
		ToolName:    "http_request",
		Content:     "HTTP 200 OK\n\nSemEngine durable execution contract handles attempt journals.",
		DispatchArguments: map[string]any{
			"url": "https://docs.c360.io/semengine/durability",
		},
	})
	addTrajectoryEntry(t, trajKV, objStore, trajectoryEntrySpec{
		LoopID:      loopID,
		CallID:      "call_curl_1",
		ExecutionID: "exec-curl-1",
		ToolName:    "bash",
		Content:     "JetStream key-value store backs the reactive graph projection.",
		DispatchArguments: map[string]any{
			"command": "curl -s https://docs.c360.io/semengine/kv",
		},
	})

	resolver := &NATSToolResultResolver{
		platform:       platform,
		trajectoriesKV: trajKV,
		contentStore:   objStore,
	}

	exec, _, pub, _ := newExecutor(t)
	exec.SetToolResultResolver(resolver)

	args := defaultArtifactArgs()
	args["actors"] = []any{
		map[string]any{
			"name": "SemEngine",
			"role": "substrate",
			"evidence": []any{
				map[string]any{
					"channel":         "web",
					"locator":         "https://docs.c360.io/semengine/durability",
					"source_class":    "official",
					"quote":           "SemEngine durable execution contract handles attempt journals.",
					"tool_result_ref": loopID + "/call_http_1",
				},
			},
		},
	}
	args["integration_points"] = []any{
		map[string]any{
			"from":      "SemTeams",
			"to":        "SemEngine",
			"data":      "kv projections",
			"direction": "write",
			"evidence": []any{
				map[string]any{
					"channel":         "web",
					"locator":         "https://docs.c360.io/semengine/kv",
					"source_class":    "official",
					"quote":           "JetStream key-value store backs the reactive graph projection.",
					"tool_result_ref": loopID + "/call_curl_1",
				},
			},
		},
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
