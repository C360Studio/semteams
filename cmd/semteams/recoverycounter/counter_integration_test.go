//go:build integration

package recoverycounter_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	agentictools "github.com/c360studio/semstreams/processor/agentic-tools"
	"github.com/c360studio/semstreams/types"
	agvocab "github.com/c360studio/semstreams/vocabulary/agentic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semteams/cmd/semteams/chain"
	"github.com/c360studio/semteams/cmd/semteams/recoverycounter"
)

// Production NATS subjects the Counter and its dependencies hit. Stubs
// reference these constants so a refactor that flips the subject in
// upstream agentic-tools / graph-ingest must also flip this test —
// exactly the property the smoke #8 root-cause class teaches us to
// enforce (silent subject drift = silent feature break).
const (
	graphQueryEntitySubject = "graph.query.entity"
	graphMutationAddSubject = "graph.mutation.triple.add"
)

// stubGraph runs two stub responders backing the Counter's dependencies:
//
//   - graph.query.entity (request/reply): returns the configured triples
//     for a given entity_id. Tracks how many times each entity was
//     queried so cycle ordering can be asserted.
//   - graph.mutation.triple.add (request/reply): captures every triple
//     write. Returns Success=true so the publisher's retry path doesn't
//     trip. Reads of the chain entity that follow a write see the
//     updated triple set so cycle progression evolves naturally.
type stubGraph struct {
	mu       sync.Mutex
	entities map[string]map[string]any
	written  []message.Triple
}

func newStubGraph(initial map[string]map[string]any) *stubGraph {
	if initial == nil {
		initial = map[string]map[string]any{}
	}
	return &stubGraph{entities: initial}
}

func (s *stubGraph) handleQuery(_ context.Context, data []byte) ([]byte, error) {
	var req map[string]string
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, err
	}
	id := req["id"]
	s.mu.Lock()
	defer s.mu.Unlock()
	triples := []map[string]any{}
	if m, ok := s.entities[id]; ok {
		for pred, obj := range m {
			triples = append(triples, map[string]any{
				"predicate": pred,
				"object":    obj,
			})
		}
	}
	resp := map[string]any{
		"id":      id,
		"triples": triples,
	}
	return json.Marshal(resp)
}

func (s *stubGraph) handleAdd(_ context.Context, data []byte) ([]byte, error) {
	var req graph.AddTripleRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.written = append(s.written, req.Triple)
	// Mirror the write into entity state so a subsequent query sees
	// the updated triple set — Counter does this internally when its
	// own writes inform later reads, and a faithful stub must too.
	if _, ok := s.entities[req.Triple.Subject]; !ok {
		s.entities[req.Triple.Subject] = map[string]any{}
	}
	s.entities[req.Triple.Subject][req.Triple.Predicate] = req.Triple.Object
	resp := graph.AddTripleResponse{MutationResponse: graph.MutationResponse{Success: true}}
	return json.Marshal(resp)
}

func (s *stubGraph) writesByPredicateSubject(predicate, subject string) []message.Triple {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []message.Triple
	for _, t := range s.written {
		if t.Predicate == predicate && t.Subject == subject {
			out = append(out, t)
		}
	}
	return out
}

// stubParentReader implements chain.ParentReader against an in-memory
// ancestry map. Used so the test doesn't need a graph-ingest stub that
// returns agent.loop.parent — keeps the wire-format test focused on
// the predicates the Counter writes.
type stubParentReader struct {
	parents map[string]string // child loop_id → parent entity_id
}

func (s *stubParentReader) ReadParent(_ context.Context, loopID string) (string, error) {
	return s.parents[loopID], nil
}

// TestCounter_LiveSubjects_FullCapEngagement drives the Counter
// against real NATS with stubs on the two production subjects it
// hits (graph.query.entity for the chain-entity read, and
// graph.mutation.triple.add for every triple write via
// agentictools.NATSTriplePublisher). Walks four consecutive
// insufficient verdicts at threshold=3 to exercise both branches:
//
//   - Cycles 1-3 (within budget): chain.recovery.count lands on the
//     chain entity, chain.recovery.proceed="true" lands on the
//     reviewer entity (the per-cycle gate sentinel).
//   - Cycle 4 (over budget): chain.recovery.count lands; instead of
//     proceed, chain.recovery.exhausted="true" lands on the chain
//     entity (the cap-hit marker rule_02 stops on).
//
// Catches three classes of bug not covered by unit tests:
//
//  1. Subject drift — if a refactor flips the upstream agentic-tools
//     triple-publisher subject or the graph-query subject, this test
//     breaks at "stub never received the request" rather than silently
//     no-opping in production.
//  2. Cap-engagement branch — no Playwright spec drives 4+ insufficient
//     verdicts on research-reviewer; the existing
//     research-mode-transition.spec.ts only walks one recovery cycle
//     (count 0→1, well under threshold).
//  3. Wire-format symmetry — the Counter both reads and writes
//     chain.recovery.count; a string-vs-number mismatch in the wire
//     shape would surface here at "cycle 2 reads count back as 0
//     because the cycle 1 write was the wrong type."
//
// Build tag `integration`. Run via `go test -tags integration ./cmd/semteams/recoverycounter/...`.
func TestCounter_LiveSubjects_FullCapEngagement(t *testing.T) {
	tc := natsclient.NewTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	platform := types.PlatformMeta{Org: "c360", Platform: "test"}
	chainEntityID := agentic.ChainExecutionEntityID(platform.Org, platform.Platform, "dispatch_root")

	// Initial state: empty chain entity (no recovery_count yet — first
	// cycle starts at 0). Reviewer entities pre-populated with
	// coordinator.next_action=insufficient so the Counter's filter
	// passes.
	const numCycles = 4
	reviewerLoopIDs := make([]string, numCycles)
	reviewerEntityIDs := make([]string, numCycles)
	initialEntities := map[string]map[string]any{}
	for i := 0; i < numCycles; i++ {
		loopID := fmt.Sprintf("reviewer_%d", i+1)
		reviewerLoopIDs[i] = loopID
		reviewerEntityIDs[i] = agentic.LoopExecutionEntityID(platform.Org, platform.Platform, loopID)
		initialEntities[reviewerEntityIDs[i]] = map[string]any{
			agvocab.CoordinatorNextAction: "insufficient",
		}
	}
	stub := newStubGraph(initialEntities)

	// Wire stub responders on the production subjects.
	querySub, err := tc.Client.SubscribeForRequests(ctx, graphQueryEntitySubject, stub.handleQuery)
	require.NoError(t, err, "stub subscribe to %s failed", graphQueryEntitySubject)
	t.Cleanup(func() { _ = querySub.Unsubscribe() })

	addSub, err := tc.Client.SubscribeForRequests(ctx, graphMutationAddSubject, stub.handleAdd)
	require.NoError(t, err, "stub subscribe to %s failed", graphMutationAddSubject)
	t.Cleanup(func() { _ = addSub.Unsubscribe() })

	// Settle: SubscribeForRequests is async wire-protocol register;
	// natsclient has no exported Flush. Mirrors chainpause's
	// decision_handler_integration_test.go pattern.
	time.Sleep(50 * time.Millisecond)

	// Production wiring: real NATSTriplePublisher (writes to
	// graph.mutation.triple.add) + real chain.NATSEntityReader (reads
	// from graph.query.entity). Parent reader stubbed in-memory; all 4
	// reviewers share dispatch_root as their parent so they walk to the
	// same chain entity (otherwise each reviewer would be its own chain
	// root and the count would never accumulate across cycles).
	dispatchRootEntityID := agentic.LoopExecutionEntityID(platform.Org, platform.Platform, "dispatch_root")
	parents := map[string]string{}
	for _, loopID := range reviewerLoopIDs {
		parents[loopID] = dispatchRootEntityID
	}

	publisher := agentictools.NewNATSTriplePublisher(tc.Client)
	entityReader := chain.NewNATSEntityReader(tc.Client, "")
	resolver := chain.NewResolver(&stubParentReader{parents: parents}, platform)

	const threshold = 3
	c := recoverycounter.NewCounter(publisher, resolver, entityReader, platform, threshold, nil)

	// Drive four consecutive insufficient verdicts. Each cycle's reviewer
	// is a fresh entity; the chain entity accumulates recovery_count
	// across cycles via the stubGraph's write-back behaviour.
	for i := 0; i < numCycles; i++ {
		ev := &agentic.LoopCompletedEvent{
			LoopID:  reviewerLoopIDs[i],
			Role:    "research-reviewer",
			Outcome: agentic.OutcomeSuccess,
		}
		err := c.HandleLoopCompleted(ctx, ev)
		require.NoErrorf(t, err, "cycle %d HandleLoopCompleted errored", i+1)
	}

	// Cycle 1-3: count writes land per cycle; proceed lands on each
	// reviewer entity; no exhausted yet.
	for i := 0; i < 3; i++ {
		expectedCount := fmt.Sprintf("%d", i+1)
		// Each cycle writes the new count to the chain entity. With
		// stubGraph mirroring writes into entity state, the cycle N read
		// sees N-1 and writes N — so the chain entity carries 4 distinct
		// count writes in order, each Object matching the cycle index.
		writes := stub.writesByPredicateSubject(chain.PredicateRecoveryCount, chainEntityID)
		require.Lenf(t, writes, numCycles, "expected %d count writes after all cycles, got %d", numCycles, len(writes))
		assert.Equalf(t, expectedCount, writes[i].Object, "cycle %d count write Object mismatch", i+1)

		// Proceed sentinel must land on this cycle's reviewer entity.
		proceedWrites := stub.writesByPredicateSubject(chain.PredicateRecoveryProceed, reviewerEntityIDs[i])
		assert.Lenf(t, proceedWrites, 1, "cycle %d (within budget) should write proceed once on reviewer %s", i+1, reviewerEntityIDs[i])
		if len(proceedWrites) == 1 {
			assert.Equalf(t, "true", proceedWrites[0].Object, "cycle %d proceed Object should be \"true\"", i+1)
		}
	}

	// Cycle 4: count=4 lands; 4 > 3 → no proceed on reviewer 4;
	// exhausted=true lands on chain entity instead.
	cycle4Proceed := stub.writesByPredicateSubject(chain.PredicateRecoveryProceed, reviewerEntityIDs[3])
	assert.Empty(t, cycle4Proceed, "cycle 4 (over budget) must not write proceed — its absence is the gate")

	exhaustedWrites := stub.writesByPredicateSubject(chain.PredicateRecoveryExhausted, chainEntityID)
	require.Len(t, exhaustedWrites, 1, "cycle 4 (over budget) should write exhausted once on chain entity")
	assert.Equal(t, "true", exhaustedWrites[0].Object, "exhausted Object should be \"true\"")

	// Sanity: exhausted must NOT land on any reviewer entity (it's a
	// chain-entity-only marker per ADR-040 §addendum).
	for i, reviewerID := range reviewerEntityIDs {
		ex := stub.writesByPredicateSubject(chain.PredicateRecoveryExhausted, reviewerID)
		assert.Emptyf(t, ex, "exhausted leaked onto reviewer %d entity %s — should land only on chain entity", i+1, reviewerID)
	}
}
