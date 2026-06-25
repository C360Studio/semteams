package runanchor

import (
	"testing"

	"github.com/c360studio/semstreams/agentic"
)

const (
	testOrg      = "c360"
	testPlatform = "ops"
)

// wantEntity is the 6-part chain entity ID the framework would mint for a given
// bare runID under the test org/platform.
func wantEntity(runID string) string {
	return testOrg + "." + testPlatform + ".agent.chain.execution." + runID
}

// wantLoopEntity is the 6-part loop entity ID agentic-loop births for a given
// loopID under the test org/platform.
func wantLoopEntity(loopID string) string {
	return testOrg + "." + testPlatform + ".agent.agentic-loop.execution." + loopID
}

func TestAnchor(t *testing.T) {
	tests := []struct {
		name         string
		meta         map[string]any
		wantRunID    string
		wantEntityID string
	}{
		{
			name: "both keys present returned verbatim",
			meta: map[string]any{
				agentic.MetadataKeyRunID:       "run-123",
				agentic.MetadataKeyRunEntityID: "c360.ops.agent.chain.execution.run-123",
			},
			wantRunID:    "run-123",
			wantEntityID: "c360.ops.agent.chain.execution.run-123",
		},
		{
			name: "only runID present reconstructs entity from org/platform",
			meta: map[string]any{
				agentic.MetadataKeyRunID: "run-456",
			},
			wantRunID:    "run-456",
			wantEntityID: wantEntity("run-456"),
		},
		{
			name:         "nil metadata yields empty anchor",
			meta:         nil,
			wantRunID:    "",
			wantEntityID: "",
		},
		{
			name:         "empty metadata yields empty anchor",
			meta:         map[string]any{},
			wantRunID:    "",
			wantEntityID: "",
		},
		{
			name: "non-string runID ignored",
			meta: map[string]any{
				agentic.MetadataKeyRunID: 42,
			},
			wantRunID:    "",
			wantEntityID: "",
		},
		{
			name: "dotted runID cannot reconstruct entity (left empty, not panicked)",
			meta: map[string]any{
				agentic.MetadataKeyRunID: "bad.run.id",
			},
			wantRunID:    "bad.run.id",
			wantEntityID: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			call := agentic.ToolCall{Metadata: tc.meta}
			gotRunID, gotEntityID := Anchor(call, testOrg, testPlatform)
			if gotRunID != tc.wantRunID {
				t.Errorf("runID = %q, want %q", gotRunID, tc.wantRunID)
			}
			if gotEntityID != tc.wantEntityID {
				t.Errorf("runEntityID = %q, want %q", gotEntityID, tc.wantEntityID)
			}
		})
	}
}

func TestChainEntityID(t *testing.T) {
	const pinned = "c360.ops.agent.chain.execution.pinned-chain"

	tests := []struct {
		name    string
		meta    map[string]any
		want    string
		wantErr bool
	}{
		{
			name: "related_loops pin takes precedence over run anchor",
			meta: map[string]any{
				agentic.MetadataKeyRelatedLoops: map[string]any{ChainEntityRoleKey: pinned},
				agentic.MetadataKeyRunEntityID:  "c360.ops.agent.chain.execution.other",
			},
			want: pinned,
		},
		{
			name: "related_loops string map takes precedence over loop fallback",
			meta: map[string]any{
				agentic.MetadataKeyRelatedLoops: map[string]string{ChainEntityRoleKey: pinned},
				"loop_id":                       "loop-front-door",
			},
			want: pinned,
		},
		{
			name: "falls back to run anchor entity when no pin",
			meta: map[string]any{
				agentic.MetadataKeyRunEntityID: "c360.ops.agent.chain.execution.from-anchor",
			},
			want: "c360.ops.agent.chain.execution.from-anchor",
		},
		{
			name: "reconstructs entity from bare runID when only runID present",
			meta: map[string]any{
				agentic.MetadataKeyRunID: "run-789",
			},
			want: wantEntity("run-789"),
		},
		{
			name: "empty pin value falls through to run anchor",
			meta: map[string]any{
				agentic.MetadataKeyRelatedLoops: map[string]any{ChainEntityRoleKey: ""},
				agentic.MetadataKeyRunID:        "run-fallthrough",
			},
			want: wantEntity("run-fallthrough"),
		},
		{
			name: "front-door loop_id metadata targets existing loop entity when no run anchor",
			meta: map[string]any{
				"loop_id": "loop-front-door",
			},
			want: wantLoopEntity("loop-front-door"),
		},
		{
			name: "run anchor wins over front-door loop_id fallback",
			meta: map[string]any{
				agentic.MetadataKeyRunID: "run-wins",
				"loop_id":                "loop-front-door",
			},
			want: wantEntity("run-wins"),
		},
		{
			name: "invalid run anchor blocks front-door loop_id fallback",
			meta: map[string]any{
				agentic.MetadataKeyRunID: "bad.run.id",
				"loop_id":                "loop-front-door",
			},
			wantErr: true,
		},
		{
			name: "non-string run anchor blocks front-door loop_id fallback",
			meta: map[string]any{
				agentic.MetadataKeyRunID: 42,
				"loop_id":                "loop-front-door",
			},
			wantErr: true,
		},
		{
			name:    "no pin and no run anchor errors",
			meta:    map[string]any{},
			wantErr: true,
		},
		{
			name:    "nil metadata errors",
			meta:    nil,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			call := agentic.ToolCall{Metadata: tc.meta}
			got, err := ChainEntityID(call, testOrg, testPlatform)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (returned %q)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("chain entity ID = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestChainEntityID_UsesToolCallLoopIDFallback(t *testing.T) {
	call := agentic.ToolCall{LoopID: "loop-field"}
	got, err := ChainEntityID(call, testOrg, testPlatform)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != wantLoopEntity("loop-field") {
		t.Errorf("chain entity ID = %q, want %q", got, wantLoopEntity("loop-field"))
	}
}
