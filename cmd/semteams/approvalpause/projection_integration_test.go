//go:build integration

package approvalpause

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/c360studio/semstreams/pkg/projection"
	"github.com/c360studio/semteams/cmd/semteams/vocab"
	"github.com/stretchr/testify/require"
)

// Exercises the production constructor and frozen exact-reader over real NATS.
// The graph owner is a typed protocol fixture; the browser run gate additionally
// qualifies the actual graph-ingest owner. This new boundary pins NATS 2.14.4.
func TestNATSProjectionLiveBoundary(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithNATSVersion("2.14.4"))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	vocab.Register()
	for _, tt := range []struct {
		name      string
		readBody  []byte
		readErr   error
		mutate    func(*durableGraph)
		wantError bool
		transient bool
		unknown   bool
	}{
		{name: "paired history"},
		{name: "entity not found", readErr: errs.ClassifiedCode(errs.ErrorInvalid, graph.ErrorCodeEntityNotFound, errors.New("not found")), wantError: true},
		{name: "invalid request", readErr: errs.ClassifiedCode(errs.ErrorInvalid, graph.ErrorCodeInvalidRequest, errors.New("invalid request")), wantError: true},
		{name: "transient read", readErr: errs.ClassifiedCode(errs.ErrorTransient, "state_unavailable", errors.New("unavailable")), wantError: true, transient: true},
		{name: "missing envelope", readBody: []byte(`{}`), wantError: true},
		{name: "null entity", readBody: []byte(`{"entity":null,"kvRevision":3}`), wantError: true},
		{name: "zero revision", readBody: []byte(`{"entity":{"id":"` + replayRunID + `"},"kvRevision":0}`), wantError: true},
		{name: "wrong identity", readBody: []byte(`{"entity":{"id":"c360.ops.chain.agent.execution.other","triples":[]},"kvRevision":3}`), wantError: true},
		{name: "wrong type", mutate: func(g *durableGraph) { g.wrongType = true }, wantError: true},
		{name: "malformed tuple", mutate: func(g *durableGraph) {
			g.triples = []message.Triple{{Subject: replayRunID, Predicate: MarkerApprovalPending, Object: "loop-ref"}}
		}, wantError: true},
		{name: "ambiguous mutation reply", mutate: func(g *durableGraph) { g.reply = []byte(`{`) }, wantError: true, unknown: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			g := &durableGraph{}
			if tt.mutate != nil {
				tt.mutate(g)
			}
			readRequests := make(chan string, 8)
			query, err := tc.Client.SubscribeForRequests(ctx, "graph.ingest.query.entity", func(c context.Context, data []byte) ([]byte, error) {
				var req struct {
					ID string `json:"id"`
				}
				if err := json.Unmarshal(data, &req); err != nil {
					return nil, err
				}
				readRequests <- req.ID
				if tt.readErr != nil {
					return nil, tt.readErr
				}
				if tt.readBody != nil {
					return tt.readBody, nil
				}
				exact, err := g.ReadExactEntity(c, req.ID)
				if err != nil {
					return nil, err
				}
				return json.Marshal(exact)
			})
			require.NoError(t, err)
			defer query.Unsubscribe()
			mutation, err := tc.Client.SubscribeForRequests(ctx, approvalReconcileSubject, func(c context.Context, data []byte) ([]byte, error) {
				return g.RequestClassified(c, approvalReconcileSubject, data, projectionTimeout)
			})
			require.NoError(t, err)
			defer mutation.Unsubscribe()
			require.NoError(t, tc.Client.GetConnection().FlushWithContext(ctx))
			p, err := NewNATSProjection(tc.Client, testOrg, testPlatform)
			require.NoError(t, err)
			_, err = p.RecordApproval(ctx, replayRunID, replayLoopID, "a", true)
			if tt.wantError {
				require.Error(t, err)
				require.Equal(t, tt.transient, errs.IsTransient(err))
				if tt.unknown {
					var failure *projection.MutationError
					require.ErrorAs(t, err, &failure)
					require.Equal(t, projection.CommitUnknown, failure.Commit)
				}
			} else {
				require.NoError(t, err)
				_, err = p.RecordApproval(ctx, replayRunID, replayLoopID, "b", true)
				require.NoError(t, err)
				replacement, err := NewNATSProjection(tc.Client, testOrg, testPlatform)
				require.NoError(t, err)
				already, err := replacement.RecordApproval(ctx, replayRunID, replayLoopID, "a", false)
				require.NoError(t, err)
				require.True(t, already)
				pending, _ := g.ReadPredicateValues(ctx, replayRunID, MarkerApprovalPending)
				answers, _ := g.ReadPredicateValues(ctx, replayRunID, MarkerApprovalAnswered)
				require.Len(t, pending, 2)
				require.Len(t, answers, 2)
				require.Equal(t, 0, outstanding(g.triples))
			}
			select {
			case id := <-readRequests:
				require.Equal(t, replayRunID, id)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
		})
	}
	t.Run("cancel in flight", func(t *testing.T) {
		entered, release := make(chan struct{}), make(chan struct{})
		query, err := tc.Client.SubscribeForRequests(ctx, "graph.ingest.query.entity", func(context.Context, []byte) ([]byte, error) {
			close(entered)
			<-release
			return nil, errors.New("released")
		})
		require.NoError(t, err)
		defer query.Unsubscribe()
		require.NoError(t, tc.Client.GetConnection().FlushWithContext(ctx))
		p, err := NewNATSProjection(tc.Client, testOrg, testPlatform)
		require.NoError(t, err)
		canceled, stop := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { _, err := p.RecordApproval(canceled, replayRunID, replayLoopID, "c", false); done <- err }()
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		stop()
		select {
		case err := <-done:
			require.ErrorIs(t, err, context.Canceled)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		close(release)
	})
}
