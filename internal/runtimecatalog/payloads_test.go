package runtimecatalog_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/governance"
	"github.com/c360studio/semstreams/graph/inference"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/payloadregistry"
	"github.com/c360studio/semstreams/pkg/lifecycle"
	gateddagexec "github.com/c360studio/semstreams/processor/gated-dag"
	"github.com/c360studio/semstreams/storage/objectstore"
	"github.com/c360studio/semteams/internal/runtimecatalog"
)

// Valid, populated payloads must decode, rather than treating validation skips
// on zero values as evidence of wire compatibility. Include the internal births
// absent from configured ports: lifecycle harnesses and hierarchy containers.
func TestPayloadOwnersRoundTrip(t *testing.T) {
	registry := payloadregistry.New()
	if err := runtimecatalog.RegisterPayloads(registry); err != nil {
		t.Fatal(err)
	}
	decoder := message.NewDecoder(registry)
	for _, payload := range payloadSamples() {
		t.Run(payload.Schema().Key(), func(t *testing.T) {
			if err := payload.Validate(); err != nil {
				t.Fatalf("invalid fixture: %v", err)
			}
			wire, err := json.Marshal(message.NewBaseMessage(payload.Schema(), payload, "catalog-test"))
			if err != nil {
				t.Fatal(err)
			}
			restored, err := decoder.Decode(wire)
			if err != nil {
				t.Fatal(err)
			}
			if err := restored.Validate(); err != nil {
				t.Fatal(err)
			}
			if reflect.TypeOf(restored.Payload()) != reflect.TypeOf(payload) {
				t.Fatalf("decoded %T, want %T", restored.Payload(), payload)
			}
			before, err := payload.MarshalJSON()
			if err != nil {
				t.Fatal(err)
			}
			after, err := restored.Payload().MarshalJSON()
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) {
				t.Errorf("payload data changed: before %s, after %s", before, after)
			}
		})
	}
}

func payloadSamples() []message.Payload {
	const id = "c360.semteams.agent.lifecycle.run.catalog-test"
	at := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	facts := []message.Triple{{Subject: id, Predicate: "agent.run.phase", Object: "running", Source: "catalog-test", Timestamp: at, Confidence: 1}}
	harness := &lifecycle.HarnessEntity{ID: id, Facts: facts}
	return []message.Payload{
		message.NewGenericJSON(map[string]any{"content": "inspectable"}),
		&agentic.UserResponse{ResponseID: "response-1", ChannelType: "web", ChannelID: "channel-1", Type: agentic.ResponseTypeText, Content: "ready", Timestamp: at},
		&gateddagexec.DispatchMessage{UnitEntityID: id, FanOutWorkflow: "research-gather"},
		&gateddagexec.StallEvent{StalledUnits: []string{id}, FanOutWorkflow: "research-gather", FanOutInstanceID: "fanout-1"},
		objectstore.NewStoredMessage(harness, &message.StorageReference{StorageInstance: "artifacts", Key: "run-1", ContentType: "application/json", Size: 100}, harness.Schema().Key()),
		&governance.VerdictEvent{Decision: governance.DecisionDeny, RuleID: "deny-write", Reason: "read-only", EntityID: id, Timestamp: at},
		harness,
		&inference.ContainerEntity{ID: id, Facts: facts},
	}
}
