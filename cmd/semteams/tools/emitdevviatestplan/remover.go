package emitdevviatestplan

// Product-local triple-remove path for plan upsert (ADR-044 §addendum
// Slice 6). The emit executor must REPLACE the prior plan on a re-plan
// (revision > 1), but upstream's agentic-tools.TriplePublisher is
// add-only by design. This mirrors the exact pattern upstream's own
// write_todos tool uses for the identical "upsert a triple set" need
// (processor/agentic-tools/write_todos.go natsTodoWriter): publish a
// graph.RemoveTripleRequest to graph.mutation.triple.remove via
// request-reply, treating a missing subject as success.
//
// Framework-alignment posture (ADR-044 §addendum Slice 6): tools that
// upsert wire their own remove writer rather than widening
// TriplePublisher. Migration target — if upstream ever lifts
// RemoveByPredicate onto TriplePublisher, collapse this onto it.
//
// Perf note (go-reviewer R3): clearPriorPlan issues one synchronous
// remove RPC per predicate (~10 plan-level + 9 per task), each a CAS
// rewrite of the same hot run-entity key, before the single batched
// AddTriplesBatch. Fine for v1 — re-plans are rare and bounded by
// plan.lisa_retry_budget (≤5). If upstream ever exposes a batched or
// predicate-PREFIX remove, collapse the per-predicate loop in
// clearPriorPlan onto it.

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/natsclient"
)

// removeTripleSubject is the graph-ingest NATS surface for triple
// removal (must match upstream graph/mutations wiring + write_todos).
const removeTripleSubject = "graph.mutation.triple.remove"

// removeTimeout bounds a single remove request-reply. Matches
// write_todos' 5s budget — a triple remove is a fast KV op.
const removeTimeout = 5 * time.Second

// tripleRemover clears a single (subject, predicate) row so the
// executor can upsert. Narrow on purpose: the executor only ever
// needs remove-by-predicate, never arbitrary graph mutation.
type tripleRemover interface {
	RemoveByPredicate(ctx context.Context, subject, predicate string) error
}

// natsTripleRemover is the production tripleRemover. Wraps the same
// natsclient.Client the add-publisher uses.
type natsTripleRemover struct {
	client *natsclient.Client
}

// NewNATSTripleRemover constructs the production remover. Returns nil
// when client is nil so the caller can decide whether a nil remover
// is acceptable (first-emit-only deployments) or a wiring error.
func NewNATSTripleRemover(client *natsclient.Client) tripleRemover {
	if client == nil {
		return nil
	}
	return &natsTripleRemover{client: client}
}

// RemoveByPredicate removes the (subject, predicate) row. A missing
// subject/predicate is success (nothing to clear) — graph-ingest
// returns a success-only RemoveTripleResponse for a no-op remove,
// mirroring write_todos.
func (r *natsTripleRemover) RemoveByPredicate(ctx context.Context, subject, predicate string) error {
	req := graph.RemoveTripleRequest{Subject: subject, Predicate: predicate}
	reqData, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal remove-triple request: %w", err)
	}
	respData, err := r.client.RequestWithRetryClassified(ctx, removeTripleSubject, reqData, removeTimeout, natsclient.DefaultRetryConfig())
	if err != nil {
		return fmt.Errorf("request %s: %w", removeTripleSubject, err)
	}
	// ADR-060: handler failures arrive as the classified err above; the
	// response body is success-only.
	var resp graph.RemoveTripleResponse
	if err := json.Unmarshal(respData, &resp); err != nil {
		return fmt.Errorf("unmarshal remove response: %w", err)
	}
	return nil
}
