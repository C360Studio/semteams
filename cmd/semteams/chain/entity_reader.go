package chain

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/errs"
)

// graphQueryTimeout caps each entity-read NATS request. Mirrors
// chainpause.NATSPauseDataReader's 3s budget.
const graphQueryTimeout = 3 * time.Second

// DefaultGraphQueryEntitySubject is the upstream graph-query
// processor's entity-read request/reply subject. Used as the fallback
// when cmd/semteams/main.go cannot resolve the subject from the
// running graph-query (or graph-ingest) component's port config — see
// portresolver.SubjectOrDefault wiring.
//
// Mirrored by chainpause.DefaultGraphQueryEntitySubject. Operators can
// override per deployment by editing the graph-query component's
// `inputs[name="query_requests"].subject` field; that override flows
// through portresolver into NATSEntityReader's constructor argument.
//
// NOTE: in the SemTeams configs, graph-query's port subject is
// declared as the wildcard `graph.query.>` (a namespace metadata
// declaration). The REAL reason port-layer resolution is fiction
// here: upstream graph-query's setupQueryHandlers
// (processor/graph-query/query.go:19) hardcodes the literal
// `graph.query.entity` as the subscription subject regardless of
// what the operator declares in the port config — it ignores the
// port-config subject for subscription registration entirely (the
// subject in the port block is consulted only by StaticRouter for
// upstream-internal routing). An operator who wanted to rewire the
// entity-read RPC would need to patch graph-query's Go code, not
// just the port config. Constructor still accepts the param so
// tests + future config-overrides work the moment that upstream
// surface gets parameterized.
const DefaultGraphQueryEntitySubject = "graph.query.entity"

// EntityTripleReader returns a flat predicate→object map for a single
// graph entity. The shape is a thin convenience over the
// graph.query.entity request/reply: callers that want one or two
// predicates avoid bespoke JSON decoding at every site.
//
// Returns an empty map (not error) when the entity has no triples;
// callers can distinguish "absent predicate" from "graph error" by
// checking the map vs the error.
type EntityTripleReader interface {
	ReadEntity(ctx context.Context, entityID string) (map[string]any, error)
}

// NATSEntityReader reads any entity's triples via the graph component's
// graph.query.entity request/reply NATS surface. Mirrors the response
// shape used by chainpause/decision_handler.go NATSPauseDataReader.
//
// ADR-053 Phase 5 (semstreams#250) retired the ancestry-walk Resolver
// that used to live alongside this reader; run identity now travels on
// the wire (ToolCall.Metadata / LoopFailedEvent.RunEntityID) and the
// remaining product-shell consumers reconstruct it via cmd/semteams/runanchor.
// What survives here is the single-entity read those consumers still need
// (e.g. the operator-decision path reading a failed loop's run anchor).
type NATSEntityReader struct {
	client  *natsclient.Client
	subject string
}

// NewNATSEntityReader constructs an EntityTripleReader backed by the
// given NATS client. subject is the entity-read RPC subject; empty
// falls back to DefaultGraphQueryEntitySubject. main.go is expected
// to resolve the live subject from the running graph-query (or
// graph-ingest) component's port config via portresolver.
func NewNATSEntityReader(client *natsclient.Client, subject string) *NATSEntityReader {
	if subject == "" {
		subject = DefaultGraphQueryEntitySubject
	}
	return &NATSEntityReader{client: client, subject: subject}
}

// ReadEntity implements EntityTripleReader against a live graph
// component. Uses natsclient.RequestClassified (beta.87+) so the
// handler-side "not found: <id>" failure surfaces as a typed
// errs.IsInvalid error rather than a raw error envelope our JSON
// decoder would mis-parse. Per upstream's ADR-060 contract, request/reply
// handlers return either a success body or a classified error; consumers
// that need to branch on handler failures must use the classified client
// methods rather than raw Request.
//
// An invalid-class err means "entity doesn't exist," which for
// ReadEntity means an empty triple map — callers decide whether an
// absent entity is benign (a loop outside any run) or an error.
func (r *NATSEntityReader) ReadEntity(ctx context.Context, entityID string) (map[string]any, error) {
	req := map[string]string{"id": entityID}
	reqData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal entity query: %w", err)
	}

	queryCtx, cancel := context.WithTimeout(ctx, graphQueryTimeout)
	defer cancel()

	respData, err := r.client.RequestClassified(queryCtx, r.subject, reqData, graphQueryTimeout)
	if err != nil {
		// graph-ingest classifies "entity doesn't exist" as Invalid
		// (see processor/graph-ingest/query.go handleQueryEntityNATS).
		// For our callers that's an empty triple map — they distinguish
		// "absent predicate" from "graph error" by checking the map.
		if errs.IsInvalid(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("graph entity query for %q: %w", entityID, err)
	}

	triples, err := DecodeExactEntityTriples(respData)
	if err != nil {
		return nil, fmt.Errorf("decode entity response for %q: %w", entityID, err)
	}
	return triples, nil
}

// DecodeExactEntityTriples decodes the beta.160 exact-read envelope the
// graph.query.entity responder returns: ExactEntity {entity:{id,triples},
// kvRevision}. The pre-160 bare {id,triples} shape decodes to ZERO triples
// silently — that exact drift starved autoresearch's empirical compare
// (journey-caught 2026-08-14) because every unit test mocks at the
// EntityReader interface, never the wire bytes. entity_reader_test.go pins
// this decode against upstream's literal fixture. kvRevision is surfaced
// for CAS writers; read-only callers ignore it.
func DecodeExactEntityTriples(respData []byte) (map[string]any, error) {
	var resp struct {
		Entity struct {
			ID      string `json:"id"`
			Triples []struct {
				Predicate string `json:"predicate"`
				Object    any    `json:"object"`
			} `json:"triples"`
		} `json:"entity"`
		KVRevision uint64 `json:"kvRevision"`
	}
	if err := json.Unmarshal(respData, &resp); err != nil {
		return nil, err
	}

	out := make(map[string]any, len(resp.Entity.Triples))
	for _, t := range resp.Entity.Triples {
		// Last-wins semantics: if a predicate appears twice the most
		// recent one wins, matching graph-ingest's stamping policy.
		out[t.Predicate] = t.Object
	}
	return out, nil
}
