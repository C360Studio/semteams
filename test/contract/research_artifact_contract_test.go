package contract

import (
	"testing"

	"github.com/c360studio/semteams/cmd/semteams/research"
)

// TestResearchArtifactCompatibleWithRuntimeCatalog checks the product payload's
// schema and collision compatibility with the framework catalog. The actual
// bootstrap overlay is exercised by cmd/semteams/payload_registry_test.go.
func TestResearchArtifactCompatibleWithRuntimeCatalog(t *testing.T) {
	t.Parallel()

	reg := newRuntimePayloadRegistry(t)
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
