package contract

import (
	"context"
	"testing"

	"github.com/c360studio/semstreams/message"
	agentictools "github.com/c360studio/semstreams/processor/agentic-tools"
	"github.com/c360studio/semstreams/types"

	"github.com/c360studio/semteams/cmd/semteams/tools/emitartifact"
)

// nopTriplePublisher is the minimum agentictools.TriplePublisher needed
// to construct an emitartifact.Executor for schema inspection. ListTools
// never invokes it — it's here purely to satisfy the constructor.
type nopTriplePublisher struct{}

func (nopTriplePublisher) AddTriple(context.Context, message.Triple) error { return nil }

// nopNATSPublisher is the minimum emitartifact.Publisher needed to
// construct an Executor for schema inspection. ListTools never invokes
// it.
type nopNATSPublisher struct{}

func (nopNATSPublisher) Publish(context.Context, string, []byte) error { return nil }

// TestEmitResearchArtifactToolDefinitionShape asserts that the
// emit_research_artifact tool definition matches the LLM-facing
// contract: the tool name and required-fields set are part of the
// stabilised schema and changing them affects every research persona
// prompt downstream. R3.2.1 of ADR-031.
//
// We assert the schema directly (rather than booting the live registry)
// because the framework registry's tool-CRUD surface validates names
// and schemas at the point the LLM enumerates available tools — drift
// at *that* layer manifests as schema mismatch in the planner, which
// this test pins.
func TestEmitResearchArtifactToolDefinitionShape(t *testing.T) {
	t.Parallel()

	exec := emitartifact.NewExecutor(nopTriplePublisher{}, nopNATSPublisher{}, types.PlatformMeta{Org: "c360", Platform: "semteams"}, nil, t.TempDir())
	defs := exec.ListTools()

	if len(defs) != 1 {
		t.Fatalf("ListTools length = %d, want 1", len(defs))
	}
	def := defs[0]

	if def.Name != emitartifact.ToolName {
		t.Errorf("tool name = %q, want %q", def.Name, emitartifact.ToolName)
	}
	if def.Name != "emit_research_artifact" {
		// Pin the literal string too — research personas / mock-llm
		// fixtures reference it by name; renaming it is a coordinated
		// change across rules, personas, and fixtures.
		t.Errorf("tool name string = %q, want %q (renaming requires persona+rule+fixture coordination)", def.Name, "emit_research_artifact")
	}

	props, ok := def.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema missing properties: %#v", def.Parameters)
	}
	for _, key := range []string{"revision", "actors", "integration_points", "tasks", "substrate_mutations"} {
		if _, ok := props[key]; !ok {
			t.Errorf("schema missing field %q (LLM-facing contract)", key)
		}
	}

	// loop_id and produced_at are server-supplied — leaking them into
	// the LLM-facing schema invites the model to lie about the calling
	// loop or wall-clock time.
	for _, forbidden := range []string{"loop_id", "produced_at"} {
		if _, ok := props[forbidden]; ok {
			t.Errorf("schema must not advertise server-supplied field %q", forbidden)
		}
	}

	required, _ := def.Parameters["required"].([]string)
	gotRequired := map[string]bool{}
	for _, r := range required {
		gotRequired[r] = true
	}
	for _, must := range []string{"revision", "actors", "integration_points", "tasks"} {
		if !gotRequired[must] {
			t.Errorf("required-fields set missing %q (have %v)", must, required)
		}
	}
}

// TestEmitResearchArtifactExecutorImplementsToolExecutor compile-time
// asserts the executor satisfies the framework's ToolExecutor interface
// — the same shape the live registry calls during dispatch. A future
// upstream signature change in agentictools.ToolExecutor surfaces here
// before booting the binary.
func TestEmitResearchArtifactExecutorImplementsToolExecutor(t *testing.T) {
	t.Parallel()
	var _ agentictools.ToolExecutor = (*emitartifact.Executor)(nil)
}
