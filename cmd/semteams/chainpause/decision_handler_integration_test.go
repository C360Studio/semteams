//go:build integration

package chainpause

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNATSPauseDataReader_LiveSubject verifies the chainpause decision
// handler's pause-data reader reaches a responder on its production
// subject and decodes the response shape. Companion to
// chain/resolver_integration_test.go's TestNATSEntityReader_LiveSubject —
// same bug class would surface here if either reader's subject drifts
// from the upstream graph-query / graph-ingest contract (smoke #8 root
// cause).
//
// Build tag `integration`. Run via `go test -tags integration ./cmd/semteams/chainpause/...`.
func TestNATSPauseDataReader_LiveSubject(t *testing.T) {
	tc := natsclient.NewTestClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	const failedLoopEntityID = "c360.test.agent.agentic-loop.execution.failed-loop-id"
	// beta.160 exact-read envelope (see chain.DecodeExactEntityTriples):
	// {entity: {id, triples}, kvRevision} — the bare shape decodes to
	// zero triples silently.
	stubResponse := []byte(`{
		"entity": {
			"id": "` + failedLoopEntityID + `",
			"triples": [
				{"predicate": "chain.paused.role", "object": "builder"},
				{"predicate": "chain.paused.original-model", "object": "claude-sonnet"},
				{"predicate": "chain.paused.cause", "object": "max_iterations"}
			]
		},
		"kvRevision": 7
	}`)

	var lastRequest map[string]string
	sub, err := tc.Client.SubscribeForRequests(
		ctx,
		// Reference the production constant: a refactor that flips
		// DefaultGraphQueryEntitySubject in decision_handler.go must also
		// flip this stub or the test fails — exactly the property
		// the smoke #8 root-cause class teaches us to enforce.
		DefaultGraphQueryEntitySubject,
		func(_ context.Context, data []byte) ([]byte, error) {
			if uErr := json.Unmarshal(data, &lastRequest); uErr != nil {
				return nil, uErr
			}
			return stubResponse, nil
		},
	)
	require.NoError(t, err, "stub subscribe to %s failed", DefaultGraphQueryEntitySubject)
	t.Cleanup(func() { _ = sub.Unsubscribe() })

	// Settle: SubscribeForRequests is async wire-protocol register;
	// natsclient has no exported Flush. Mirrors upstream's own
	// natsclient/request_integration_test.go pattern.
	time.Sleep(50 * time.Millisecond)

	// Empty subject → constructor falls back to DefaultGraphQueryEntitySubject;
	// matches what main.go passes for production wiring.
	reader := NewNATSPauseDataReader(tc.Client, "")
	role, model, err := reader.ReadPauseData(ctx, failedLoopEntityID)

	require.NoError(t, err, "ReadPauseData against live NATS+stub responder failed — likely subject mismatch in chainpause/decision_handler.go")
	require.NotNil(t, lastRequest, "stub never received the request — subject mismatch in NATSPauseDataReader")
	assert.Equal(t, failedLoopEntityID, lastRequest["id"], "request payload's id field doesn't match what the reader was asked for")

	// Reader extracts role + model from the chain.paused.* triples.
	assert.Equal(t, "builder", role)
	assert.Equal(t, "claude-sonnet", model)
}

// TestNATSPauseDataReader_NotFoundReturnsEmpty verifies the
// RequestClassified migration: when the handler returns an Invalid-class
// error (graph-ingest's "entity doesn't exist" classification),
// ReadPauseData returns ("", "", nil) so DecisionHandler.retry's
// documented fallback (role="dispatch", model="claude-haiku") kicks
// in cleanly. Pre-migration this code path silently mis-decoded the
// legacy `error: not found: <id>` body shape as JSON and surfaced a
// confusing "invalid character 'e'" decode error — same bug class
// chain/resolver.go fixed in bae5706.
//
// The stub uses errs.NewInvalid + SubscribeForRequests' error-return
// path to produce the exact wire shape RequestClassified is designed
// to classify (carrier-encoded error, not legacy text body). Without
// the migration this test would either time out (timeout class) or
// fail decode (silent-corruption class) — both regressions from the
// no-op semantics RequestClassified delivers.
func TestNATSPauseDataReader_NotFoundReturnsEmpty(t *testing.T) {
	tc := natsclient.NewTestClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	const missingEntityID = "c360.test.agent.agentic-loop.execution.does-not-exist"
	var lastRequest map[string]string
	sub, err := tc.Client.SubscribeForRequests(
		ctx,
		DefaultGraphQueryEntitySubject,
		func(_ context.Context, data []byte) ([]byte, error) {
			// Capture the request payload so a constructor regression
			// that flips `{"id": ...}` to a different shape is caught
			// here too (symmetric with TestNATSPauseDataReader_LiveSubject
			// — smoke #8 root-cause prevention argument applies to both
			// the happy path AND the not-found path).
			_ = json.Unmarshal(data, &lastRequest)
			// Return an Invalid-class err mirroring graph-ingest's
			// handleQueryEntityNATS contract for "entity doesn't exist."
			// errs.Classified(errs.ErrorInvalid, ...) is the canonical
			// shape from upstream natsclient/doc.go that
			// classifiedFromHeader on the client side reconstructs as
			// IsInvalid(err) == true.
			return nil, errs.Classified(errs.ErrorInvalid, fmt.Errorf("not found: %s", missingEntityID))
		},
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sub.Unsubscribe() })

	time.Sleep(50 * time.Millisecond)

	reader := NewNATSPauseDataReader(tc.Client, "")
	role, model, err := reader.ReadPauseData(ctx, missingEntityID)

	require.NoError(t, err, "Invalid-class err should surface as empty role+model+nil err; pre-RequestClassified migration this would fail decode")
	require.NotNil(t, lastRequest, "stub never received the request — subject mismatch in NATSPauseDataReader (regression class smoke #8)")
	assert.Equal(t, missingEntityID, lastRequest["id"], "request payload's id field doesn't match what the reader was asked for")
	assert.Equal(t, "", role, "missing entity should return empty role so retry() falls back to dispatch")
	assert.Equal(t, "", model, "missing entity should return empty model so retry() falls back to claude-haiku")
}
