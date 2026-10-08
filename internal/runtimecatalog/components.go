// Package runtimecatalog declares the frozen framework registrations admitted by
// SemTeams. Bootstrap, schema generation and contract checks share this catalog;
// implementations remain in SemStreams until separately qualified extraction.
package runtimecatalog

import (
	"errors"
	"fmt"

	"github.com/c360studio/semstreams/component"
	graphgateway "github.com/c360studio/semstreams/gateway/graph-gateway"
	gatewayhttp "github.com/c360studio/semstreams/gateway/http"
	lifecyclegateway "github.com/c360studio/semstreams/gateway/lifecycle-gateway"
	fileinput "github.com/c360studio/semstreams/input/file"
	"github.com/c360studio/semstreams/input/udp"
	websocketinput "github.com/c360studio/semstreams/input/websocket"
	"github.com/c360studio/semstreams/output/file"
	"github.com/c360studio/semstreams/output/httppost"
	"github.com/c360studio/semstreams/output/websocket"
	agenticdispatch "github.com/c360studio/semstreams/processor/agentic-dispatch"
	agenticgovernance "github.com/c360studio/semstreams/processor/agentic-governance"
	agenticloop "github.com/c360studio/semstreams/processor/agentic-loop"
	agenticmodel "github.com/c360studio/semstreams/processor/agentic-model"
	agentictools "github.com/c360studio/semstreams/processor/agentic-tools"
	gateddagexec "github.com/c360studio/semstreams/processor/gated-dag"
	graphclustering "github.com/c360studio/semstreams/processor/graph-clustering"
	graphembedding "github.com/c360studio/semstreams/processor/graph-embedding"
	graphindex "github.com/c360studio/semstreams/processor/graph-index"
	graphindexspatial "github.com/c360studio/semstreams/processor/graph-index-spatial"
	graphindextemporal "github.com/c360studio/semstreams/processor/graph-index-temporal"
	graphingest "github.com/c360studio/semstreams/processor/graph-ingest"
	graphquery "github.com/c360studio/semstreams/processor/graph-query"
	jsonfilter "github.com/c360studio/semstreams/processor/json_filter"
	jsongeneric "github.com/c360studio/semstreams/processor/json_generic"
	jsonmap "github.com/c360studio/semstreams/processor/json_map"
	"github.com/c360studio/semstreams/processor/rule"
	"github.com/c360studio/semstreams/storage/objectstore"
)

// RegisterComponents preserves all 27 factories advertised at SemStreams
// 8b99efe9c66a, including inactive factories. It does not enable components.
// A caller must discard the registry on error; registration is not atomic.
func RegisterComponents(registry *component.Registry) error {
	if registry == nil {
		return errors.New("register components: registry cannot be nil")
	}
	registrations := []struct {
		name     string
		register func(*component.Registry) error
	}{
		{"udp", udp.Register},
		{"websocket_input", websocketinput.Register},
		{"file_input", fileinput.Register},
		{"json_generic", jsongeneric.Register},
		{"json_filter", jsonfilter.Register},
		{"json_map", jsonmap.Register},
		{"objectstore", objectstore.Register},
		{"file", file.Register},
		{"httppost", httppost.Register},
		{"websocket", websocket.Register},
		{"http", gatewayhttp.Register},
		{"lifecycle-gateway", lifecyclegateway.Register},
		{"graph-ingest", graphingest.Register},
		{"graph-index", graphindex.Register},
		{"graph-gateway", graphgateway.Register},
		{"graph-query", graphquery.Register},
		{"graph-embedding", graphembedding.Register},
		{"graph-clustering", graphclustering.Register},
		{"graph-index-spatial", graphindexspatial.Register},
		{"graph-index-temporal", graphindextemporal.Register},
		{"rule-processor", rule.Register},
		{"gated-dag", gateddagexec.Register},
		{"agentic-dispatch", agenticdispatch.Register},
		{"agentic-governance", func(reg *component.Registry) error { return agenticgovernance.Register(reg) }},
		{"agentic-model", func(reg *component.Registry) error { return agenticmodel.Register(reg) }},
		{"agentic-tools", func(reg *component.Registry) error { return agentictools.Register(reg) }},
		{"agentic-loop", agenticloop.Register},
	}
	for _, registration := range registrations {
		if err := registration.register(registry); err != nil {
			return fmt.Errorf("register component %q: %w", registration.name, err)
		}
	}
	return nil
}
