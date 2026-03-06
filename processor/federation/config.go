// Package federation provides merge policy logic for the federation processor.
// It applies namespace sovereignty, edge union, and provenance tracking to
// incoming graph events for cross-service federation.
package federation

import (
	"fmt"

	"github.com/c360studio/semstreams/component"
)

// MergePolicy controls how incoming entities are merged against the local graph.
type MergePolicy string

const (
	// MergePolicyStandard applies the standard merge rules:
	//   - public.* merges unconditionally
	//   - {org}.* merges only if org matches LocalNamespace
	//   - cross-org entities are rejected
	//   - edges use union semantics
	//   - provenance is always appended
	MergePolicyStandard MergePolicy = "standard"
)

// validMergePolicies is the set of recognized merge policy values.
var validMergePolicies = map[MergePolicy]bool{
	MergePolicyStandard: true,
}

// Config holds configuration for the federation processor.
type Config struct {
	// LocalNamespace is the org namespace this processor is authoritative for
	// (e.g. "acme"). Entities from other orgs are rejected unless they are
	// in the "public" namespace.
	LocalNamespace string `json:"local_namespace" schema:"type:string,description:Org namespace this processor is authoritative for (e.g. acme or public),category:basic,required"`

	// MergePolicy controls the entity merge strategy. Valid values: "standard".
	MergePolicy MergePolicy `json:"merge_policy" schema:"type:string,description:Entity merge strategy,category:basic,enum:standard,default:standard"`

	// InputSubject is the JetStream subject this processor consumes events from.
	InputSubject string `json:"input_subject" schema:"type:string,description:JetStream subject for incoming federation events,default:federation.graph.events,category:basic"`

	// OutputSubject is the JetStream subject merged events are published to.
	OutputSubject string `json:"output_subject" schema:"type:string,description:JetStream subject for merged federation events,default:federation.graph.merged,category:basic"`

	// InputStream is the JetStream stream name for the input subject.
	InputStream string `json:"input_stream" schema:"type:string,description:Input JetStream stream name,default:FEDERATION_EVENTS,category:basic"`

	// OutputStream is the JetStream stream name for the output subject.
	OutputStream string `json:"output_stream" schema:"type:string,description:Output JetStream stream name,default:FEDERATION_MERGED,category:basic"`

	// Ports is the port configuration for inputs and outputs.
	Ports *component.PortConfig `json:"ports,omitempty" schema:"type:ports,description:Port configuration for inputs and outputs,category:basic"`
}

// Validate checks that Config contains all required and valid field values.
func (c Config) Validate() error {
	if c.LocalNamespace == "" {
		return fmt.Errorf("federation config: local_namespace is required")
	}
	if c.MergePolicy == "" {
		return fmt.Errorf("federation config: merge_policy is required")
	}
	if !validMergePolicies[c.MergePolicy] {
		return fmt.Errorf("federation config: unknown merge_policy %q (valid: standard)", c.MergePolicy)
	}
	if c.InputSubject == "" {
		return fmt.Errorf("federation config: input_subject is required")
	}
	if c.OutputSubject == "" {
		return fmt.Errorf("federation config: output_subject is required")
	}
	if c.InputStream == "" {
		return fmt.Errorf("federation config: input_stream is required")
	}
	if c.OutputStream == "" {
		return fmt.Errorf("federation config: output_stream is required")
	}
	return nil
}

// DefaultConfig returns a Config with sensible defaults.
// LocalNamespace is set to "public" as a safe starting point; callers should
// override it with the actual org namespace.
func DefaultConfig() Config {
	return Config{
		LocalNamespace: "public",
		MergePolicy:    MergePolicyStandard,
		InputSubject:   "federation.graph.events",
		OutputSubject:  "federation.graph.merged",
		InputStream:    "FEDERATION_EVENTS",
		OutputStream:   "FEDERATION_MERGED",
		Ports: &component.PortConfig{
			Inputs: []component.PortDefinition{
				{
					Name:        "federation_events_in",
					Type:        "jetstream",
					Subject:     "federation.graph.events",
					StreamName:  "FEDERATION_EVENTS",
					Required:    true,
					Description: "JetStream input for incoming federation graph events",
				},
			},
			Outputs: []component.PortDefinition{
				{
					Name:        "federation_events_out",
					Type:        "jetstream",
					Subject:     "federation.graph.merged",
					StreamName:  "FEDERATION_MERGED",
					Description: "JetStream output for merged federation graph events",
				},
			},
		},
	}
}
