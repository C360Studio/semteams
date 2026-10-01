package main

import (
	"encoding/json"
	"testing"

	"github.com/c360studio/semstreams/config"
	"github.com/c360studio/semstreams/types"
)

func TestStatefulToolsUseDeclaredLoopAuthorityBucket(t *testing.T) {
	cfg := &config.Config{Components: config.ComponentConfigs{
		"tools": types.ComponentConfig{Name: "agentic-tools", Enabled: true, Config: json.RawMessage(`{"ports":{"inputs":[{"name":"agent_loops","config":{"kind":"kv-read","bucket":"CUSTOM_LOOPS"}}]}}`)},
	}}
	if got := extractLoopsBucket(cfg); got != "CUSTOM_LOOPS" {
		t.Fatalf("bucket = %q, want named agent_loops authority", got)
	}
}
