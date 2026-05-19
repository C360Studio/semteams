package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/c360studio/semteams/cmd/semteams/devviaspec"
	"github.com/c360studio/semteams/cmd/semteams/research"
)

// TestJourneyFixtureEmitArgs_Validate walks every mock-llm journey
// fixture under test/fixtures/journeys and round-trips each
// `emit_research_artifact` / `emit_plan` tool call's arguments_json
// blob through the same parse + Validate path the production executor
// uses.
//
// Why this exists: the fixtures encode the canonical "this is how a
// model following the persona contract calls the tool" example. If the
// fixture args drift away from the tool's schema (missing required
// field, wrong nested key shape, type mismatch), the Playwright e2e
// fails at runtime — but only after spinning up the full Docker stack.
// This test catches drift in <1s without booting anything.
//
// Coverage is structural well-formedness only — it doesn't check that
// the args are a *good* example of the persona contract (that's a
// reviewer judgment), only that they're a *valid* example the executor
// accepts. The emit_dev_via_spec_artifact branch retired alongside the
// dev-via-spec arc in the ADR-042 MVP-7 follow-up sweep.
//
// Loop ID and produced_at are server-supplied at runtime; the test
// injects synthetic values so Validate() doesn't reject on missing
// loop_id (mirrors what parseArgsIntoPlan / parseArgsIntoArtifact do
// at execution time).
func TestJourneyFixtureEmitArgs_Validate(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir("../fixtures/journeys")
	if err != nil {
		t.Fatalf("read journeys dir: %v", err)
	}

	sawFixture := false
	for _, ent := range entries {
		if ent.IsDir() || filepath.Ext(ent.Name()) != ".yaml" {
			continue
		}
		path := filepath.Join("../fixtures/journeys", ent.Name())
		t.Run(ent.Name(), func(t *testing.T) {
			t.Parallel()
			validateJourneyFixture(t, path)
		})
		sawFixture = true
	}
	if !sawFixture {
		t.Fatal("no journey fixtures found under test/fixtures/journeys")
	}
}

// journeyFixture is the minimal YAML shape needed to walk a fixture's
// tool calls. Other fields (description, name, completion turns,
// arguments unrelated to emit_*) are intentionally not deserialised
// — keeping the type narrow means a fixture-format change unrelated
// to emit_* args can't break this test.
type journeyFixture struct {
	Responses []struct {
		ToolCall *struct {
			Name          string `yaml:"name"`
			ArgumentsJSON string `yaml:"arguments_json"`
		} `yaml:"tool_call,omitempty"`
	} `yaml:"responses"`
}

func validateJourneyFixture(t *testing.T, path string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var fx journeyFixture
	if err := yaml.Unmarshal(data, &fx); err != nil {
		t.Fatalf("parse YAML %s: %v", path, err)
	}

	emitTurnsSeen := 0
	for i, resp := range fx.Responses {
		if resp.ToolCall == nil {
			continue
		}
		switch resp.ToolCall.Name {
		case "emit_research_artifact":
			emitTurnsSeen++
			validateResearchArtifactArgs(t, path, i, resp.ToolCall.ArgumentsJSON)
		case "emit_plan":
			emitTurnsSeen++
			validatePlanArgs(t, path, i, resp.ToolCall.ArgumentsJSON)
		}
	}

	t.Logf("%s: validated %d emit_* turn(s)", filepath.Base(path), emitTurnsSeen)
}

// validateResearchArtifactArgs round-trips the LLM-supplied args
// through the same shape parseArgsIntoArtifact (in
// cmd/semteams/tools/emitartifact/executor.go) uses at runtime. Server-
// supplied loop_id and produced_at are injected so Validate() doesn't
// reject for legitimate-at-runtime reasons.
func validateResearchArtifactArgs(t *testing.T, path string, idx int, raw string) {
	t.Helper()

	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		t.Errorf("%s [response %d emit_research_artifact]: arguments_json invalid JSON: %v", path, idx, err)
		return
	}
	args["loop_id"] = "fixture-loop-id"
	args["produced_at"] = time.Now().UTC()

	bytes, err := json.Marshal(args)
	if err != nil {
		t.Errorf("%s [response %d emit_research_artifact]: re-marshal: %v", path, idx, err)
		return
	}

	var artifact research.Artifact
	if err := json.Unmarshal(bytes, &artifact); err != nil {
		t.Errorf("%s [response %d emit_research_artifact]: unmarshal into research.Artifact: %v", path, idx, err)
		return
	}
	if err := artifact.Validate(); err != nil {
		t.Errorf("%s [response %d emit_research_artifact]: Validate(): %v", path, idx, err)
	}
}

// validatePlanArgs round-trips the LLM-supplied args through the same
// shape parseArgsIntoPlan uses. Mirrors validateResearchArtifactArgs.
func validatePlanArgs(t *testing.T, path string, idx int, raw string) {
	t.Helper()

	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		t.Errorf("%s [response %d emit_plan]: arguments_json invalid JSON: %v", path, idx, err)
		return
	}
	args["loop_id"] = "fixture-loop-id"
	args["produced_at"] = time.Now().UTC()

	bytes, err := json.Marshal(args)
	if err != nil {
		t.Errorf("%s [response %d emit_plan]: re-marshal: %v", path, idx, err)
		return
	}

	var plan devviaspec.Plan
	if err := json.Unmarshal(bytes, &plan); err != nil {
		t.Errorf("%s [response %d emit_plan]: unmarshal into devviaspec.Plan: %v", path, idx, err)
		return
	}
	if err := plan.Validate(); err != nil {
		t.Errorf("%s [response %d emit_plan]: Validate(): %v", path, idx, err)
	}
}
