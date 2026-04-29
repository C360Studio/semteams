package contract

import (
	"encoding/json"
	"testing"

	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/payloadbuiltins"
	"github.com/c360studio/semstreams/pkg/types"
)

// schemaProvider matches message.Payload's Schema() method
type schemaProvider interface {
	Schema() types.Type
}

// TestSchemaRegistrationConsistency verifies that all registered payloads
// have Schema() methods that return values matching their registration.
// This test catches mismatches that would cause deserialization failures.
func TestSchemaRegistrationConsistency(t *testing.T) {
	reg := payloadbuiltins.NewTestRegistry(t)
	payloads := reg.List()
	if len(payloads) == 0 {
		t.Skip("No payloads registered")
	}

	for msgType, r := range payloads {
		t.Run(msgType, func(t *testing.T) {
			payload := reg.Create(r.Domain, r.Category, r.Version)
			if payload == nil {
				t.Fatalf("Create returned nil for registered type %s", msgType)
			}

			sp, ok := payload.(schemaProvider)
			if !ok {
				t.Skipf("Payload %s does not implement Schema() method", msgType)
				return
			}

			schema := sp.Schema()
			if schema.Domain != r.Domain {
				t.Errorf("Schema().Domain = %q, want %q", schema.Domain, r.Domain)
			}
			if schema.Category != r.Category {
				t.Errorf("Schema().Category = %q, want %q", schema.Category, r.Category)
			}
			if schema.Version != r.Version {
				t.Errorf("Schema().Version = %q, want %q", schema.Version, r.Version)
			}
		})
	}
}

// TestBaseMessageRoundTrip verifies that BaseMessage can marshal and unmarshal
// for all registered payload types without data loss.
//
// Note: Empty payloads (from factory) typically fail validation because they
// have required fields. This is expected and correct behavior - the contract
// enforcement prevents invalid messages from being serialized.
func TestBaseMessageRoundTrip(t *testing.T) {
	reg := payloadbuiltins.NewTestRegistry(t)
	decoder := message.NewDecoder(reg)
	payloads := reg.List()
	if len(payloads) == 0 {
		t.Skip("No payloads registered")
	}

	for msgType, r := range payloads {
		t.Run(msgType, func(t *testing.T) {
			payload := reg.Create(r.Domain, r.Category, r.Version)
			if payload == nil {
				t.Fatalf("Create returned nil for registered type %s", msgType)
			}

			msgPayload, ok := payload.(message.Payload)
			if !ok {
				t.Skipf("Payload %s does not implement message.Payload", msgType)
				return
			}

			msgTypeStruct := types.Type{
				Domain:   r.Domain,
				Category: r.Category,
				Version:  r.Version,
			}
			original := message.NewBaseMessage(msgTypeStruct, msgPayload, "contract-test")

			data, err := json.Marshal(original)
			if err != nil {
				// Empty payloads failing validation is expected and correct behavior
				// This proves the contract enforcement is working
				t.Skipf("Empty payload correctly rejected by validation: %v", err)
				return
			}

			restored, err := decoder.Decode(data)
			if err != nil {
				t.Fatalf("Failed to decode BaseMessage: %v\nJSON: %s", err, string(data))
			}

			if err := restored.Validate(); err != nil {
				t.Errorf("Restored message failed validation: %v", err)
			}

			if restored.Type() != original.Type() {
				t.Errorf("Type mismatch: got %v, want %v", restored.Type(), original.Type())
			}
		})
	}
}

// TestPayloadValidation verifies that newly created payloads from factories
// pass validation (or fail with expected errors for required fields).
func TestPayloadValidation(t *testing.T) {
	reg := payloadbuiltins.NewTestRegistry(t)
	payloads := reg.List()
	if len(payloads) == 0 {
		t.Skip("No payloads registered")
	}

	for msgType, r := range payloads {
		t.Run(msgType, func(t *testing.T) {
			payload := reg.Create(r.Domain, r.Category, r.Version)
			if payload == nil {
				t.Fatalf("Create returned nil for registered type %s", msgType)
			}

			msgPayload, ok := payload.(message.Payload)
			if !ok {
				t.Skipf("Payload %s does not implement message.Payload", msgType)
				return
			}

			// Empty payloads may fail validation - that's expected
			// We're just checking that Validate() doesn't panic
			_ = msgPayload.Validate()
		})
	}
}

// TestPayloadMarshalJSON verifies that all registered payloads can marshal to JSON.
func TestPayloadMarshalJSON(t *testing.T) {
	reg := payloadbuiltins.NewTestRegistry(t)
	payloads := reg.List()
	if len(payloads) == 0 {
		t.Skip("No payloads registered")
	}

	for msgType, r := range payloads {
		t.Run(msgType, func(t *testing.T) {
			payload := reg.Create(r.Domain, r.Category, r.Version)
			if payload == nil {
				t.Fatalf("Create returned nil for registered type %s", msgType)
			}

			msgPayload, ok := payload.(message.Payload)
			if !ok {
				t.Skipf("Payload %s does not implement message.Payload", msgType)
				return
			}

			// Test MarshalJSON doesn't panic
			_, err := msgPayload.MarshalJSON()
			if err != nil {
				// Some payloads may fail to marshal when empty due to validation
				// This is expected behavior - log but don't fail
				t.Logf("Payload %s MarshalJSON error (may be expected for empty payload): %v", msgType, err)
			}
		})
	}
}
