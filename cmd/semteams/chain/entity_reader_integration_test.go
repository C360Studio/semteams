//go:build integration

package chain

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/c360studio/semstreams/natsclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNATSEntityReader_LiveSubject verifies the reader's ReadEntity
// call reaches a responder on the production subject and decodes the
// response shape. Catches the class of bug smoke #8 surfaced (PR D
// post-mortem): the reader was sending requests to `graph.query.entity`
// but no agentic config ran the upstream graph-query processor that
// subscribes there. Result: every chain milestone subscriber timed out
// silently and no chain entity triples landed.
//
// Why integration not unit: the contract under test IS the subject the
// reader sends to. A unit test with an in-memory reader is blind to this
// class — it never exercises the wire.
//
// Why a stub responder not real graph-query+graph-ingest: the bug shape
// is "no responder on the subject the reader uses." A stub on the
// expected subject catches that with one moving part. Spinning up the
// real upstream components also tests StaticRouter routing + KV
// roundtrip but is heavier and out of scope for the first integration
// guard. Add a fuller bootstrap test if router/KV regressions surface.
//
// Build tag: `integration`. Run via `go test -tags integration ./cmd/semteams/chain/...`.
func TestNATSEntityReader_LiveSubject(t *testing.T) {
	tc := natsclient.NewTestClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stub responder mirroring upstream graph-query's response shape:
	// {id, triples: [{predicate, object}]}. Asserts the reader sent
	// the right request shape (`{"id": "..."}`) before returning canned
	// data the reader's decoder must handle.
	const stubEntityID = "c360.test.agent.agentic-loop.execution.test-loop-id"
	stubResponse := []byte(`{
		"id": "` + stubEntityID + `",
		"triples": [
			{"predicate": "agent.loop.role", "object": "research-reviewer"},
			{"predicate": "coordinator.decision.next_action", "object": "approved"},
			{"predicate": "lineage.researcher", "object": "researcher-loop-id"},
			{"predicate": "agent.loop.iterations", "object": 3}
		]
	}`)

	var lastRequest map[string]string
	sub, err := tc.Client.SubscribeForRequests(
		ctx,
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
	// natsclient/request_integration_test.go pattern. Without this the
	// reader's first Request can race the server's subject-table
	// propagation, surface as "no responders," and read like a real
	// bug under CI testcontainer cold-start load.
	time.Sleep(50 * time.Millisecond)

	// Empty subject → constructor falls back to DefaultGraphQueryEntitySubject;
	// matches what main.go passes for the production wiring.
	reader := NewNATSEntityReader(tc.Client, "")
	got, err := reader.ReadEntity(ctx, stubEntityID)

	// Wire-format contract: reader must send `{"id": "<entityID>"}` to
	// the subject the upstream graph-query processor subscribes to.
	require.NoError(t, err, "ReadEntity against live NATS+stub responder failed — likely subject mismatch (entity_reader.go uses %q)", DefaultGraphQueryEntitySubject)
	require.NotNil(t, lastRequest, "stub never received the request — subject mismatch in chain.NATSEntityReader")
	assert.Equal(t, stubEntityID, lastRequest["id"], "request payload's id field doesn't match what the reader was asked for")

	// Response-decoding contract: reader must surface every triple the
	// upstream component returns, with predicate→object mapping intact.
	assert.Equal(t, "research-reviewer", got["agent.loop.role"])
	assert.Equal(t, "approved", got["coordinator.decision.next_action"])
	assert.Equal(t, "researcher-loop-id", got["lineage.researcher"])
	assert.EqualValues(t, 3, got["agent.loop.iterations"])
}
