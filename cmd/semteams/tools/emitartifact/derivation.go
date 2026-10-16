package emitartifact

import (
	"context"
	"crypto/sha256"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/types"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semteams/cmd/semteams/chain"
	"github.com/c360studio/semteams/cmd/semteams/research"
	"github.com/c360studio/semteams/cmd/semteams/runanchor"
)

const (
	// agentContentBucket is the JetStream ObjectStore bucket where
	// agentic-loop persists full trajectory evidence bodies.
	agentContentBucket = "AGENT_CONTENT"

	// predicateRunEntityID is the canonical 3-segment triple predicate stamped
	// on loop entities pointing at their enclosing run entity ID
	// (org.platform.agent.chain.execution.<run_id>).
	predicateRunEntityID = "agent.run.entity-id"

	// predicateRunID is the read-only fallback triple predicate stamped on
	// loop entities holding the bare run root loop ID (never emitted by this package).
	predicateRunID = "agent.run"
)

// ResolvedToolResult holds the resolved output, arguments, and loop lineage of
// a prior tool execution referenced by an EvidenceRecord.ToolResultRef.
type ResolvedToolResult struct {
	Ref               string
	CallID            string
	ExecutionID       string
	LoopID            string
	ToolName          string
	Content           string
	DispatchArguments map[string]any
}

// ToolResultResolver resolves a tool_result_ref (a ToolCall.ID,
// ToolCall.ExecutionID, or "<loop_id>/<call_id>") within the scope of the
// calling tool execution.
//
// Implementations MUST verify that the resolved tool execution belongs to the
// same research run as call (either same LoopID or same run anchor entity ID)
// and return an error if the reference does not exist or belongs to a
// different run.
type ToolResultResolver interface {
	ResolveToolResult(ctx context.Context, call agentic.ToolCall, ref string) (*ResolvedToolResult, error)
}

// VerifyDerivation mechanically verifies every EvidenceRecord cited across an
// Artifact's Actors and IntegrationPoints against prior tool results from the
// same research run:
//  1. tool_result_ref resolves to a real completed tool execution in the
//     current research run (same loop or same run anchor across plan, gather,
//     synthesize, and revision-2+ retry loops).
//  2. The cited locator or canonical origin_id is present in the resolved tool
//     output or dispatch arguments (and for structured github_read record
//     outputs, matches a returned EvidenceRecord).
//  3. The cited quote is a verbatim substring of the resolved tool output
//     (and for structured github_read record outputs, of the matched record's
//     RawBody).
func VerifyDerivation(ctx context.Context, resolver ToolResultResolver, call agentic.ToolCall, a *research.Artifact) error {
	if resolver == nil || a == nil {
		return nil
	}

	cache := make(map[string]*ResolvedToolResult)
	resolve := func(ref string) (*ResolvedToolResult, error) {
		if cached, ok := cache[ref]; ok {
			return cached, nil
		}
		res, err := resolver.ResolveToolResult(ctx, call, ref)
		if err != nil {
			return nil, err
		}
		if res == nil {
			return nil, fmt.Errorf("tool_result_ref %q resolved to nil result", ref)
		}
		cache[ref] = res
		return res, nil
	}

	for i, actor := range a.Actors {
		for j, ev := range actor.Evidence {
			res, err := resolve(ev.ToolResultRef)
			if err != nil {
				return fmt.Errorf("actors[%d] (%s) evidence[%d]: %w", i, actor.Name, j, err)
			}
			if err := verifyCitationAgainstResult(ev, res); err != nil {
				return fmt.Errorf("actors[%d] (%s) evidence[%d]: %w", i, actor.Name, j, err)
			}
		}
	}

	for i, ip := range a.IntegrationPoints {
		for j, ev := range ip.Evidence {
			res, err := resolve(ev.ToolResultRef)
			if err != nil {
				return fmt.Errorf("integration_points[%d] (%s -> %s) evidence[%d]: %w", i, ip.From, ip.To, j, err)
			}
			if err := verifyCitationAgainstResult(ev, res); err != nil {
				return fmt.Errorf("integration_points[%d] (%s -> %s) evidence[%d]: %w", i, ip.From, ip.To, j, err)
			}
		}
	}

	return nil
}

// evidenceRecordsEnvelope matches structured tool outputs like github_read
// that return {"records": [...]}.
type evidenceRecordsEnvelope struct {
	Records []research.EvidenceRecord `json:"records"`
}

func verifyCitationAgainstResult(ev research.EvidenceRecord, res *ResolvedToolResult) error {
	// Structured record envelope (e.g., github_read): match by OriginID or
	// Locator against a returned record, then verify Quote against that
	// record's RawBody.
	var env evidenceRecordsEnvelope
	if err := json.Unmarshal([]byte(res.Content), &env); err == nil && len(env.Records) > 0 {
		var matched []*research.EvidenceRecord
		for idx := range env.Records {
			rec := &env.Records[idx]
			if (ev.OriginID != "" && rec.OriginID == ev.OriginID) ||
				(ev.Locator != "" && rec.Locator == ev.Locator) {
				matched = append(matched, rec)
			}
		}
		if len(matched) == 0 {
			return fmt.Errorf("tool_result_ref %q does not contain record for locator %q or origin_id %q",
				ev.ToolResultRef, ev.Locator, ev.OriginID)
		}
		for _, rec := range matched {
			body := rec.RawBody
			if body == "" {
				body = rec.Quote
			}
			if strings.Contains(body, ev.Quote) {
				return nil
			}
		}
		return fmt.Errorf("quote %q is not a verbatim substring of tool_result_ref %q record %q",
			ev.Quote, ev.ToolResultRef, matched[0].OriginID)
	}

	// Unstructured / generic tool output (web_search, http_request,
	// graph_query, graph_search, graph_summary, bash, etc.).
	if !resultContainsLocatorOrOrigin(res, ev.Locator, ev.OriginID) {
		return fmt.Errorf("tool_result_ref %q does not reference locator %q or origin_id %q",
			ev.ToolResultRef, ev.Locator, ev.OriginID)
	}
	if !contentContainsVerbatimQuote(res.Content, ev.Quote) {
		return fmt.Errorf("quote %q is not a verbatim substring of tool_result_ref %q output",
			ev.Quote, ev.ToolResultRef)
	}
	return nil
}

func resultContainsLocatorOrOrigin(res *ResolvedToolResult, locator, originID string) bool {
	candidates := []string{locator, originID}
	for _, c := range []string{locator, originID} {
		if strings.HasPrefix(strings.ToLower(c), "semsource://") {
			candidates = append(candidates, c[len("semsource://"):])
		}
	}
	for _, cand := range candidates {
		if cand == "" {
			continue
		}
		if strings.Contains(res.Content, cand) || jsonStringsContain(res.Content, cand) || mapContainsSubstring(res.DispatchArguments, cand) {
			return true
		}
	}
	return false
}

func contentContainsVerbatimQuote(content, quote string) bool {
	if quote == "" {
		return false
	}
	if strings.Contains(content, quote) {
		return true
	}
	return jsonStringsContain(content, quote)
}

func jsonStringsContain(rawJSON, needle string) bool {
	if needle == "" || len(rawJSON) == 0 {
		return false
	}
	var decoded any
	if err := json.Unmarshal([]byte(rawJSON), &decoded); err != nil {
		return false
	}
	return valueContainsSubstring(decoded, needle)
}

func mapContainsSubstring(m map[string]any, needle string) bool {
	if needle == "" || len(m) == 0 {
		return false
	}
	return valueContainsSubstring(m, needle)
}

func valueContainsSubstring(v any, needle string) bool {
	switch typed := v.(type) {
	case string:
		return strings.Contains(typed, needle)
	case map[string]any:
		for k, val := range typed {
			if strings.Contains(k, needle) || valueContainsSubstring(val, needle) {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if valueContainsSubstring(item, needle) {
				return true
			}
		}
	}
	return false
}

// kvStore abstracts a JetStream KeyValue bucket for NATSToolResultResolver so
// unit tests can exercise resolution and multi-run disambiguation without a
// live NATS server.
type kvStore interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Keys(ctx context.Context) ([]string, error)
}

// byteObjectStore abstracts a JetStream ObjectStore bucket for reading
// AGENT_CONTENT trajectory evidence bodies.
type byteObjectStore interface {
	GetBytes(ctx context.Context, key string) ([]byte, error)
}

type jetStreamKVAdapter struct {
	bucket jetstream.KeyValue
}

func (a jetStreamKVAdapter) Get(ctx context.Context, key string) ([]byte, error) {
	entry, err := a.bucket.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	return entry.Value(), nil
}

func (a jetStreamKVAdapter) Keys(ctx context.Context) ([]string, error) {
	lister, err := a.bucket.ListKeys(ctx)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return nil, nil
		}
		return nil, err
	}
	var keys []string
	for k := range lister.Keys() {
		keys = append(keys, k)
	}
	return keys, nil
}

// NATSToolResultResolver resolves tool_result_ref citations against the
// runtime's TOOL_CALL_OUTCOMES KV bucket and AGENT_TRAJECTORIES /
// AGENT_CONTENT trajectory persistence, verifying run lineage via
// chain.EntityTripleReader.
type NATSToolResultResolver struct {
	natsClient   *natsclient.Client
	entityReader chain.EntityTripleReader
	platform     types.PlatformMeta

	// Optional test overrides; when nil, opened lazily from natsClient.
	outcomesKV     kvStore
	trajectoriesKV kvStore
	contentStore   byteObjectStore
}

// NewNATSToolResultResolver constructs a NATSToolResultResolver backed by
// natsClient and entityReader. Returns nil if natsClient is nil and no test
// stores are injected.
func NewNATSToolResultResolver(
	natsClient *natsclient.Client,
	entityReader chain.EntityTripleReader,
	platform types.PlatformMeta,
) *NATSToolResultResolver {
	if natsClient == nil {
		return nil
	}
	return &NATSToolResultResolver{
		natsClient:   natsClient,
		entityReader: entityReader,
		platform:     platform,
	}
}

type completedOutcomeRecord struct {
	Version     string             `json:"version"`
	ExecutionID string             `json:"execution_id"`
	RequestID   string             `json:"request_id"`
	CallID      string             `json:"call_id"`
	CallOrdinal uint32             `json:"call_ordinal"`
	Fingerprint string             `json:"fingerprint"`
	Result      agentic.ToolResult `json:"result"`
}

type trajectoryToolCompletionBody struct {
	Result            agentic.ToolResult `json:"result"`
	DispatchArguments map[string]any     `json:"dispatch_arguments"`
}

// ResolveToolResult resolves ref within call's research run.
func (r *NATSToolResultResolver) ResolveToolResult(ctx context.Context, call agentic.ToolCall, ref string) (*ResolvedToolResult, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("tool_result_ref is required")
	}

	hintLoopID, targetID := splitToolResultRef(ref)
	callerRunID, callerRunEntityID := r.resolveCallerRun(ctx, call)

	candidates, err := r.collectCandidates(ctx, ref, hintLoopID, targetID)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("tool_result_ref %q not found in TOOL_CALL_OUTCOMES or AGENT_TRAJECTORIES", ref)
	}

	var foreignLoopID string
	var foreignRun string
	for _, cand := range candidates {
		sameRun, prodRun := r.belongsToCallerRun(ctx, call.LoopID, callerRunID, callerRunEntityID, cand.LoopID)
		if sameRun {
			r.enrichDispatchArgsFromTrajectory(ctx, cand)
			return cand, nil
		}
		foreignLoopID = cand.LoopID
		foreignRun = prodRun
	}

	if foreignRun != "" {
		return nil, fmt.Errorf("tool_result_ref %q (loop %q, run %q) does not belong to current research run %q",
			ref, foreignLoopID, foreignRun, firstNonEmpty(callerRunEntityID, callerRunID, call.LoopID))
	}
	return nil, fmt.Errorf("tool_result_ref %q (loop %q) does not belong to current research run %q",
		ref, foreignLoopID, firstNonEmpty(callerRunEntityID, callerRunID, call.LoopID))
}

func splitToolResultRef(ref string) (hintLoopID, targetID string) {
	if idx := strings.IndexByte(ref, '/'); idx > 0 && idx < len(ref)-1 {
		return strings.TrimSpace(ref[:idx]), strings.TrimSpace(ref[idx+1:])
	}
	return "", ref
}

func (r *NATSToolResultResolver) resolveCallerRun(ctx context.Context, call agentic.ToolCall) (runID, runEntityID string) {
	runID, runEntityID = runanchor.Anchor(call, r.platform.Org, r.platform.Platform)
	if (runID == "" || runEntityID == "") && call.LoopID != "" && r.entityReader != nil {
		loopEntityID := agentic.LoopExecutionEntityID(r.platform.Org, r.platform.Platform, call.LoopID)
		if triples, err := r.entityReader.ReadEntity(ctx, loopEntityID); err == nil && triples != nil {
			if runEntityID == "" {
				runEntityID, _ = triples[predicateRunEntityID].(string)
			}
			if runID == "" {
				runID, _ = triples[predicateRunID].(string)
			}
		}
	}
	return runID, runEntityID
}

func (r *NATSToolResultResolver) belongsToCallerRun(
	ctx context.Context,
	callerLoopID, callerRunID, callerRunEntityID, producerLoopID string,
) (bool, string) {
	if producerLoopID != "" && producerLoopID == callerLoopID {
		return true, callerRunEntityID
	}
	if producerLoopID == "" || r.entityReader == nil {
		return false, ""
	}
	prodLoopEntityID := agentic.LoopExecutionEntityID(r.platform.Org, r.platform.Platform, producerLoopID)
	triples, err := r.entityReader.ReadEntity(ctx, prodLoopEntityID)
	if err != nil || triples == nil {
		return false, ""
	}
	prodRunEntityID, _ := triples[predicateRunEntityID].(string)
	prodRunID, _ := triples[predicateRunID].(string)

	if callerRunEntityID != "" && prodRunEntityID == callerRunEntityID {
		return true, prodRunEntityID
	}
	if callerRunID != "" && prodRunID == callerRunID {
		return true, prodRunID
	}
	return false, firstNonEmpty(prodRunEntityID, prodRunID)
}

func (r *NATSToolResultResolver) collectCandidates(
	ctx context.Context,
	rawRef, hintLoopID, targetID string,
) ([]*ResolvedToolResult, error) {
	var out []*ResolvedToolResult
	seen := make(map[string]bool)
	addCandidate := func(cand *ResolvedToolResult) {
		if cand == nil {
			return
		}
		if hintLoopID != "" && cand.LoopID != "" && cand.LoopID != hintLoopID {
			return
		}
		key := cand.LoopID + "|" + cand.ExecutionID + "|" + cand.CallID
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, cand)
	}

	if outcomesKV := r.getOutcomesKV(ctx); outcomesKV != nil {
		// 1. Direct lookup if targetID is an execution_id.
		for _, candidateExecID := range []string{targetID, rawRef} {
			if candidateExecID == "" {
				continue
			}
			if data, err := outcomesKV.Get(ctx, outcomeKeyForExecutionID(candidateExecID)); err == nil {
				if cand, ok := decodeOutcomeCandidate(rawRef, data); ok {
					addCandidate(cand)
				}
			}
		}

		// 2. Scan TOOL_CALL_OUTCOMES keys for matching CallID or ExecutionID.
		if keys, err := outcomesKV.Keys(ctx); err == nil {
			for _, k := range keys {
				data, getErr := outcomesKV.Get(ctx, k)
				if getErr != nil {
					continue
				}
				cand, ok := decodeOutcomeCandidate(rawRef, data)
				if !ok {
					continue
				}
				if matchesTargetID(cand, rawRef, targetID) {
					addCandidate(cand)
				}
			}
		}
	}

	// 3. Also scan AGENT_TRAJECTORIES + AGENT_CONTENT when needed (or when an
	// explicit loop hint is provided).
	if len(out) == 0 {
		if trajKV, contentStore := r.getTrajectoryStores(ctx); trajKV != nil && contentStore != nil {
			keys, err := trajKV.Keys(ctx)
			if err == nil {
				prefix := ""
				if hintLoopID != "" {
					prefix = agentic.TrajectoryFactPrefix(hintLoopID)
				}
				for _, k := range keys {
					if prefix != "" && !strings.HasPrefix(k, prefix) {
						continue
					}
					data, getErr := trajKV.Get(ctx, k)
					if getErr != nil {
						continue
					}
					cand, ok := r.decodeTrajectoryCandidate(ctx, contentStore, rawRef, targetID, data)
					if ok {
						addCandidate(cand)
					}
				}
			}
		}
	}

	return out, nil
}

func (r *NATSToolResultResolver) enrichDispatchArgsFromTrajectory(ctx context.Context, cand *ResolvedToolResult) {
	if cand == nil || len(cand.DispatchArguments) > 0 || cand.LoopID == "" {
		return
	}
	trajKV, contentStore := r.getTrajectoryStores(ctx)
	if trajKV == nil || contentStore == nil {
		return
	}
	keys, err := trajKV.Keys(ctx)
	if err != nil {
		return
	}
	prefix := agentic.TrajectoryFactPrefix(cand.LoopID)
	for _, k := range keys {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		data, getErr := trajKV.Get(ctx, k)
		if getErr != nil {
			continue
		}
		var fact agentic.TrajectoryFactV1
		if err := json.Unmarshal(data, &fact); err != nil {
			continue
		}
		if fact.Kind != agentic.TrajectoryKindToolCompleted || fact.Evidence == nil {
			continue
		}
		if fact.SourceCorrelation != cand.CallID && fact.SourceCorrelation != cand.ExecutionID {
			continue
		}
		bodyBytes, err := contentStore.GetBytes(ctx, fact.Evidence.Key)
		if err != nil {
			continue
		}
		var env agentic.TrajectoryEvidenceV1
		if err := json.Unmarshal(bodyBytes, &env); err != nil || len(env.Body) == 0 {
			continue
		}
		var body trajectoryToolCompletionBody
		if err := json.Unmarshal(env.Body, &body); err != nil {
			continue
		}
		if len(body.DispatchArguments) > 0 {
			cand.DispatchArguments = body.DispatchArguments
			return
		}
	}
}

func decodeOutcomeCandidate(rawRef string, data []byte) (*ResolvedToolResult, bool) {
	var rec completedOutcomeRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, false
	}
	callID := firstNonEmpty(rec.CallID, rec.Result.CallID)
	execID := firstNonEmpty(rec.ExecutionID, rec.Result.ExecutionID)
	if callID == "" && execID == "" {
		return nil, false
	}
	return &ResolvedToolResult{
		Ref:         rawRef,
		CallID:      callID,
		ExecutionID: execID,
		LoopID:      rec.Result.LoopID,
		ToolName:    rec.Result.Name,
		Content:     rec.Result.Content,
	}, true
}

type jetStreamObjectStoreAdapter struct {
	store jetstream.ObjectStore
}

func (a jetStreamObjectStoreAdapter) GetBytes(ctx context.Context, key string) ([]byte, error) {
	return a.store.GetBytes(ctx, key)
}

func (r *NATSToolResultResolver) decodeTrajectoryCandidate(
	ctx context.Context,
	contentStore byteObjectStore,
	rawRef, targetID string,
	data []byte,
) (*ResolvedToolResult, bool) {
	var fact agentic.TrajectoryFactV1
	if err := json.Unmarshal(data, &fact); err != nil {
		return nil, false
	}
	if fact.Kind != agentic.TrajectoryKindToolCompleted || fact.Evidence == nil {
		return nil, false
	}
	bodyBytes, err := contentStore.GetBytes(ctx, fact.Evidence.Key)
	if err != nil {
		return nil, false
	}
	var env agentic.TrajectoryEvidenceV1
	if err := json.Unmarshal(bodyBytes, &env); err != nil || len(env.Body) == 0 {
		return nil, false
	}
	var body trajectoryToolCompletionBody
	if err := json.Unmarshal(env.Body, &body); err != nil {
		return nil, false
	}
	cand := &ResolvedToolResult{
		Ref:               rawRef,
		CallID:            firstNonEmpty(body.Result.CallID, fact.SourceCorrelation),
		ExecutionID:       body.Result.ExecutionID,
		LoopID:            body.Result.LoopID,
		ToolName:          firstNonEmpty(body.Result.Name, fact.ToolPreview),
		Content:           body.Result.Content,
		DispatchArguments: body.DispatchArguments,
	}
	if !matchesTargetID(cand, rawRef, targetID) {
		return nil, false
	}
	return cand, true
}

func matchesTargetID(cand *ResolvedToolResult, rawRef, targetID string) bool {
	for _, want := range []string{targetID, rawRef} {
		if want == "" {
			continue
		}
		if cand.CallID == want || cand.ExecutionID == want {
			return true
		}
	}
	return false
}

func outcomeKeyForExecutionID(executionID string) string {
	sum := sha256.Sum256([]byte(executionID))
	digest := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:]))
	return "v1." + digest
}

func (r *NATSToolResultResolver) getOutcomesKV(ctx context.Context) kvStore {
	if r.outcomesKV != nil {
		return r.outcomesKV
	}
	if r.natsClient == nil {
		return nil
	}
	bucket, err := r.natsClient.GetKeyValueBucket(ctx, graph.BucketToolCallOutcomes)
	if err != nil || bucket == nil {
		return nil
	}
	return jetStreamKVAdapter{bucket: bucket}
}

func (r *NATSToolResultResolver) getTrajectoryStores(ctx context.Context) (kvStore, byteObjectStore) {
	if r.trajectoriesKV != nil && r.contentStore != nil {
		return r.trajectoriesKV, r.contentStore
	}
	if r.natsClient == nil {
		return nil, nil
	}
	trajBucket, err := r.natsClient.GetKeyValueBucket(ctx, agentic.TrajectoryBucketName)
	if err != nil || trajBucket == nil {
		return nil, nil
	}
	js, err := r.natsClient.JetStream()
	if err != nil || js == nil {
		return nil, nil
	}
	objStore, err := js.ObjectStore(ctx, agentContentBucket)
	if err != nil || objStore == nil {
		return nil, nil
	}
	return jetStreamKVAdapter{bucket: trajBucket}, jetStreamObjectStoreAdapter{store: objStore}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
