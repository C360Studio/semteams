package contract

import (
	"testing"

	"github.com/c360studio/semstreams/payloadbuiltins"

	"github.com/c360studio/semteams/cmd/semteams/research"
)

// TestResearchArtifactRegisteredInLiveRegistry asserts that the
// SemTeams-local research.Artifact payload is reachable through the
// live registry after the same registration sequence cmd/semteams/
// main.go performs at boot. Catches:
//
//   - the product-payload registration call going missing from main.go
//   - schema metadata drift (Domain / Category / Version)
//   - a future framework-side registration colliding with research.artifact.v1
//
// R3.1 of ADR-031.
func TestResearchArtifactRegisteredInLiveRegistry(t *testing.T) {
	t.Parallel()

	reg := payloadbuiltins.NewTestRegistry(t)
	if err := research.RegisterPayloads(reg); err != nil {
		t.Fatalf("research.RegisterPayloads: %v", err)
	}

	payload := reg.Create(research.Domain, research.CategoryArtifact, research.SchemaVersion)
	if payload == nil {
		t.Fatalf("Create(%q, %q, %q) returned nil — not registered",
			research.Domain, research.CategoryArtifact, research.SchemaVersion)
	}

	artifact, ok := payload.(*research.Artifact)
	if !ok {
		t.Fatalf("Create returned %T, want *research.Artifact", payload)
	}

	schema := artifact.Schema()
	if schema.Domain != research.Domain {
		t.Errorf("Schema().Domain: got %q, want %q", schema.Domain, research.Domain)
	}
	if schema.Category != research.CategoryArtifact {
		t.Errorf("Schema().Category: got %q, want %q", schema.Category, research.CategoryArtifact)
	}
	if schema.Version != research.SchemaVersion {
		t.Errorf("Schema().Version: got %q, want %q", schema.Version, research.SchemaVersion)
	}
}
