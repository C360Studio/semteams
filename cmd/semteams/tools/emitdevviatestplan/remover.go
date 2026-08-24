package emitdevviatestplan

// Product-local triple-remove path for plan upsert (ADR-044 §addendum
// Slice 6) — PARKED-DEAD at beta.160.
//
// History: the emit executor must REPLACE the prior plan on a re-plan
// (revision > 1). Through beta.159 this mirrored upstream write_todos'
// remove-then-add over the raw graph.mutation.triple.remove wire. The
// beta.160 graph foundation cutover DELETED that operation (the
// admitted mutation ops are entity.create / entity.reconcile /
// triple.append / entity.delete, all via the typed
// semstreams.graph.mutation/v1 request port), and write_todos itself
// moved to a reconcile of one agent.todo.record literal.
//
// This tool belongs to the PARKED dev-via-test pack (ADR-058): it stays
// compiled but is not registered by any wired bootstrap. Re-wiring the
// pack requires re-authoring this upsert onto a reconcile_predicates
// projection group (the canonical replace-a-group primitive), at which
// point this remover collapses away entirely. Until then the production
// remover fails loudly instead of silently publishing into a dead wire.

import (
	"context"
	"errors"

	"github.com/c360studio/semstreams/natsclient"
)

// errRemoveWireRetired names the beta.160 contract gap explicitly so a
// premature re-wiring of the parked pack surfaces as this error, not a
// NATS timeout on a subject nothing serves.
var errRemoveWireRetired = errors.New(
	"emitdevviatestplan: graph.mutation.triple.remove was retired by the semstreams beta.160 graph cutover; " +
		"re-author the plan upsert as a reconcile_predicates projection group before re-wiring the dev-via-test pack (ADR-058)")

// tripleRemover clears a single (subject, predicate) row so the
// executor can upsert. Narrow on purpose: the executor only ever
// needs remove-by-predicate, never arbitrary graph mutation.
type tripleRemover interface {
	RemoveByPredicate(ctx context.Context, subject, predicate string) error
}

// natsTripleRemover is the production tripleRemover. Parked-dead: see
// the package comment.
type natsTripleRemover struct{}

// NewNATSTripleRemover constructs the production remover. Returns nil
// when client is nil so the caller can decide whether a nil remover
// is acceptable (first-emit-only deployments) or a wiring error.
func NewNATSTripleRemover(client *natsclient.Client) tripleRemover {
	if client == nil {
		return nil
	}
	return &natsTripleRemover{}
}

// RemoveByPredicate fails loudly: the raw remove wire no longer exists
// at beta.160. See errRemoveWireRetired.
func (r *natsTripleRemover) RemoveByPredicate(_ context.Context, _, _ string) error {
	return errRemoveWireRetired
}
