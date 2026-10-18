// Package semsource registers parked semsource wire payload types
// (semsource.{entity,status,manifest,predicates}.v1) so the product payload
// catalog preserves its registered schema and BaseMessage decoding surface
// without taking a Go-module dependency on semsource.
//
// Under ADR-060, SemSource is a standalone external service with its own NATS
// and graph (SemSource ADR-0006 removed headless mode), and
// configs/flow-bootstrap.json does not subscribe graph-ingest to shared-NATS
// graph.ingest.{entity,status,manifest,predicates} streams. Retaining these
// registrations preserves runtime-composition catalog parity (#282/#285) and
// does not claim active headless-host operation or SemSource dogfood readiness.
package semsource

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/payloadregistry"
)

// Compile-time interface compliance checks.
var (
	_ graph.Graphable = (*EntityPayload)(nil)
	_ message.Payload = (*EntityPayload)(nil)
	_ message.Payload = (*StatusPayload)(nil)
	_ message.Payload = (*ManifestPayload)(nil)
	_ message.Payload = (*PredicateSchemaPayload)(nil)
)

// RegisterPayloads registers the four semsource wire payload types with the
// supplied registry. Called from cmd/semteams/product_tools.go's
// registerProductPayloads to preserve product payload catalog compatibility.
func RegisterPayloads(reg *payloadregistry.Registry) error {
	return errors.Join(
		reg.Register(&payloadregistry.Registration{
			Domain:      "semsource",
			Category:    "entity",
			Version:     "v1",
			Description: "Entity streamed from a semsource ingestion instance",
			Factory: func() any {
				return &EntityPayload{}
			},
			Example: map[string]any{
				"id":         "org.platform.domain.system.type.instance",
				"triples":    []any{},
				"updated_at": time.Now().UTC().Format(time.RFC3339),
			},
		}),
		reg.Register(&payloadregistry.Registration{
			Domain:      "semsource",
			Category:    "status",
			Version:     "v1",
			Description: "Semsource instance status heartbeat",
			Factory: func() any {
				return &StatusPayload{}
			},
		}),
		reg.Register(&payloadregistry.Registration{
			Domain:      "semsource",
			Category:    "manifest",
			Version:     "v1",
			Description: "Source manifest listing all configured ingestion sources",
			Factory:     func() any { return &ManifestPayload{} },
		}),
		reg.Register(&payloadregistry.Registration{
			Domain:      "semsource",
			Category:    "predicates",
			Version:     "v1",
			Description: "Predicate schema advertising predicates emitted per source type",
			Factory:     func() any { return &PredicateSchemaPayload{} },
		}),
	)
}

// EntityPayload carries a graph entity received from semsource.
// It implements graph.Graphable so graph-ingest can persist it directly,
// and message.Payload so the component framework can deserialize it from wire.
type EntityPayload struct {
	// ID is the six-part federated entity identifier.
	ID string `json:"id"`

	// TripleData are the semantic facts that make up this entity's current state.
	TripleData []message.Triple `json:"triples"`

	// UpdatedAt records when the entity was last modified in semsource.
	UpdatedAt time.Time `json:"updated_at"`
}

// EntityID satisfies graph.Graphable.
func (p *EntityPayload) EntityID() string {
	return p.ID
}

// Triples satisfies graph.Graphable.
func (p *EntityPayload) Triples() []message.Triple {
	return p.TripleData
}

// Schema satisfies message.Payload.
func (p *EntityPayload) Schema() message.Type {
	return message.Type{Domain: "semsource", Category: "entity", Version: "v1"}
}

// Validate satisfies message.Payload.
func (p *EntityPayload) Validate() error {
	if p.ID == "" {
		return errors.New("semsource entity payload: id is required")
	}
	return nil
}

// MarshalJSON satisfies json.Marshaler.
func (p *EntityPayload) MarshalJSON() ([]byte, error) {
	type Alias EntityPayload
	return json.Marshal((*Alias)(p))
}

// UnmarshalJSON satisfies json.Unmarshaler.
func (p *EntityPayload) UnmarshalJSON(data []byte) error {
	type Alias EntityPayload
	return json.Unmarshal(data, (*Alias)(p))
}

// StatusPayload is the semsource heartbeat message.
type StatusPayload struct {
	Status   string `json:"status"`
	Sources  int    `json:"sources"`
	Entities int    `json:"entities"`
	Uptime   string `json:"uptime,omitempty"`
}

// Schema returns the message type for semsource status heartbeats.
func (p *StatusPayload) Schema() message.Type {
	return message.Type{Domain: "semsource", Category: "status", Version: "v1"}
}

// Validate checks the payload for correctness.
func (p *StatusPayload) Validate() error {
	if strings.TrimSpace(p.Status) == "" {
		return errors.New("semsource status payload: status is required")
	}
	if p.Sources < 0 {
		return errors.New("semsource status payload: sources must be non-negative")
	}
	if p.Entities < 0 {
		return errors.New("semsource status payload: entities must be non-negative")
	}
	return nil
}

// MarshalJSON satisfies json.Marshaler.
func (p *StatusPayload) MarshalJSON() ([]byte, error) {
	type Alias StatusPayload
	return json.Marshal((*Alias)(p))
}

// UnmarshalJSON satisfies json.Unmarshaler.
func (p *StatusPayload) UnmarshalJSON(data []byte) error {
	type Alias StatusPayload
	return json.Unmarshal(data, (*Alias)(p))
}
