package contract

import (
	"encoding/json"
	"testing"

	"github.com/c360studio/semteams/cmd/semteams/tools/bootstrapworkspace"
	"github.com/c360studio/semteams/cmd/semteams/tools/emitspecartifact"
)

// TestSidecarJSONShape pins the top-level shape of <slug>.checks.json:
// must contain a "checks" array and an optional "harnesses" map.
// The SidecarPayload type in emitspecartifact is the authoritative schema.
func TestSidecarJSONShape(t *testing.T) {
	const sidecarJSON = `{
  "checks": [
    {
      "target": "connection established",
      "runtime": "process-local-testcontainer",
      "test_harness": "meshtasticd-3.x",
      "test_runtime": "java-junit-testcontainers",
      "ref": {"type": "filepath", "path": "src/test/java/FooIT.java"},
      "evidence": [{"kind": "surefire_passing_count", "args": {"min": 1}}]
    }
  ],
  "harnesses": {
    "meshtasticd-3.x": {
      "id": "meshtasticd-3.x",
      "image": "meshtastic/meshtasticd:3.5.0",
      "ports": {"meshtastic-protobuf": 4403}
    }
  }
}`

	var payload emitspecartifact.SidecarPayload
	if err := json.Unmarshal([]byte(sidecarJSON), &payload); err != nil {
		t.Fatalf("SidecarPayload unmarshal: %v", err)
	}

	if len(payload.Checks) != 1 {
		t.Errorf("Checks len = %d, want 1", len(payload.Checks))
	}
	if len(payload.Harnesses) != 1 {
		t.Errorf("Harnesses len = %d, want 1", len(payload.Harnesses))
	}

	h, ok := payload.Harnesses["meshtasticd-3.x"]
	if !ok {
		t.Fatalf("harnesses missing meshtasticd-3.x")
	}
	if h.ID != "meshtasticd-3.x" {
		t.Errorf("harness.ID = %q, want meshtasticd-3.x", h.ID)
	}
	if h.Image != "meshtastic/meshtasticd:3.5.0" {
		t.Errorf("harness.Image = %q, want meshtastic/meshtasticd:3.5.0", h.Image)
	}
	if port, ok := h.Ports["meshtastic-protobuf"]; !ok || port != 4403 {
		t.Errorf("harness.Ports[meshtastic-protobuf] = %d (ok=%v), want 4403", port, ok)
	}
}

// TestWorkspaceFilePaths pins the workspace-relative paths bootstrapworkspace
// uses for sidecar projection. Downstream slices (evidence preprocessor,
// builder persona) hard-code these paths — changing them is a breaking change.
func TestWorkspaceFilePaths(t *testing.T) {
	if bootstrapworkspace.ChecksFilename != ".evidence/checks.json" {
		t.Errorf("ChecksFilename = %q, want .evidence/checks.json", bootstrapworkspace.ChecksFilename)
	}
	if bootstrapworkspace.TestHarnessManifestFilename != ".test-harness/manifest.json" {
		t.Errorf("TestHarnessManifestFilename = %q, want .test-harness/manifest.json",
			bootstrapworkspace.TestHarnessManifestFilename)
	}
}

// TestSidecarHarnessesMapKeying verifies that the harnesses map is keyed by
// catalog ID (the TestHarness.Name / ResolvedManifest.ID field), not by
// image or any other field. This is the lookup contract for the builder's
// Testcontainers code.
func TestSidecarHarnessesMapKeying(t *testing.T) {
	const sidecarJSON = `{
  "checks": [{"target":"t","runtime":"in-process-unit","ref":{"type":"filepath","path":"x.go"}}],
  "harnesses": {
    "harness-a": {"id": "harness-a", "image": "img-a:1.0"},
    "harness-b": {"id": "harness-b", "image": "img-b:2.0"}
  }
}`

	var payload emitspecartifact.SidecarPayload
	if err := json.Unmarshal([]byte(sidecarJSON), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for key, manifest := range payload.Harnesses {
		if key != manifest.ID {
			t.Errorf("map key %q != manifest.ID %q — harnesses must be keyed by catalog ID", key, manifest.ID)
		}
	}
}
