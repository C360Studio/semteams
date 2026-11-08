// Package runtimecatalog declares the explicit component and payload catalog
// composed by SemTeams. Bootstrap, schema generation, and contract checks share
// this catalog; implementations remain in frozen SemStreams until separately
// qualified extraction or SemEngine adoption.
package runtimecatalog

import (
	"errors"
	"fmt"

	"github.com/c360studio/semstreams/component"
	graphgateway "github.com/c360studio/semstreams/gateway/graph-gateway"
	agenticdispatch "github.com/c360studio/semstreams/processor/agentic-dispatch"
	agenticloop "github.com/c360studio/semstreams/processor/agentic-loop"
	agenticmodel "github.com/c360studio/semstreams/processor/agentic-model"
	agentictools "github.com/c360studio/semstreams/processor/agentic-tools"
	graphingest "github.com/c360studio/semstreams/processor/graph-ingest"
	graphquery "github.com/c360studio/semstreams/processor/graph-query"
	"github.com/c360studio/semstreams/processor/rule"
	"github.com/c360studio/semstreams/storage/objectstore"
)

// RegisterComponents registers the 9 component factories composed by SemTeams's
// shipped bootstrap configurations. It does not enable components. A caller
// must discard the registry on error; registration is not atomic.
func RegisterComponents(registry *component.Registry) error {
	if registry == nil {
		return errors.New("register components: registry cannot be nil")
	}
	registrations := []struct {
		name     string
		register func(*component.Registry) error
	}{
		{"graph-ingest", graphingest.Register},
		{"graph-query", graphquery.Register},
		{"rule-processor", rule.Register},
		{"objectstore", objectstore.Register},
		{"agentic-loop", agenticloop.Register},
		{"agentic-model", func(reg *component.Registry) error { return agenticmodel.Register(reg) }},
		{"agentic-tools", func(reg *component.Registry) error { return agentictools.Register(reg) }},
		{"agentic-dispatch", agenticdispatch.Register},
		{"graph-gateway", graphgateway.Register},
	}
	for _, registration := range registrations {
		if err := registration.register(registry); err != nil {
			return fmt.Errorf("register component %q: %w", registration.name, err)
		}
	}
	return nil
}
