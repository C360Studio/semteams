//go:build integration

package chainpause

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/c360studio/semstreams/natsclient"
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
	stubResponse := []byte(`{
		"id": "` + failedLoopEntityID + `",
		"triples": [
			{"predicate": "chain.paused.role", "object": "dev-via-spec-builder"},
			{"predicate": "chain.paused.original_model", "object": "claude-sonnet"},
			{"predicate": "chain.paused.cause", "object": "max_iterations"}
		]
	}`)

	var lastRequest map[string]string
	sub, err := tc.Client.SubscribeForRequests(
		ctx,
		// Mirror the production subject used by the reader; if this
		// drifts from decision_handler.go's literal the test fails.
		"graph.query.entity",
		func(_ context.Context, data []byte) ([]byte, error) {
			if uErr := json.Unmarshal(data, &lastRequest); uErr != nil {
				return nil, uErr
			}
			return stubResponse, nil
		},
	)
	require.NoError(t, err, "stub subscribe to graph.query.entity failed")
	t.Cleanup(func() { _ = sub.Unsubscribe() })

	reader := NewNATSPauseDataReader(tc.Client)
	role, model, err := reader.ReadPauseData(ctx, failedLoopEntityID)

	require.NoError(t, err, "ReadPauseData against live NATS+stub responder failed — likely subject mismatch in chainpause/decision_handler.go")
	require.NotNil(t, lastRequest, "stub never received the request — subject mismatch in NATSPauseDataReader")
	assert.Equal(t, failedLoopEntityID, lastRequest["id"], "request payload's id field doesn't match what the reader was asked for")

	// Reader extracts role + model from the chain.paused.* triples.
	assert.Equal(t, "dev-via-spec-builder", role)
	assert.Equal(t, "claude-sonnet", model)
}
