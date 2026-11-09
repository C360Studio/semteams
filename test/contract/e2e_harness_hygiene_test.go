package contract

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type journeyFixtureDoc struct {
	Name      string `yaml:"name"`
	Responses []struct {
		ToolCall struct {
			Name string `yaml:"name"`
		} `yaml:"tool_call"`
	} `yaml:"responses"`
}

type migrationMatrixScenario struct {
	Name    string `json:"name"`
	Fixture string `json:"fixture"`
	Spec    string `json:"spec"`
	Config  string `json:"config"`
}

func checkStackUpPollsDispatchAndLoops(taskfileText string) error {
	idx := strings.Index(taskfileText, "test:e2e:agentic:stack:up:")
	if idx < 0 {
		return fmt.Errorf("ui/Taskfile.yml missing test:e2e:agentic:stack:up task")
	}
	block := taskfileText[idx:]
	if next := strings.Index(block, "\n  test:e2e:agentic:cleanup:"); next > 0 {
		block = block[:next]
	}
	if !strings.Contains(block, "/teams-dispatch/commands") {
		return fmt.Errorf("stack:up readiness loop must poll /teams-dispatch/commands")
	}
	if !strings.Contains(block, "/teams-dispatch/loops") {
		return fmt.Errorf("stack:up readiness loop must poll /teams-dispatch/loops (#420 cold-start 503)")
	}
	return nil
}

func checkResearchArtifactRegex(specText string) error {
	const prefix = "toMatch("
	idx := strings.Index(specText, "artifacts\\/research\\/")
	if idx < 0 {
		return fmt.Errorf("research-mvp.spec.ts missing artifacts/research regex")
	}
	start := strings.LastIndex(specText[:idx], prefix)
	end := strings.Index(specText[idx:], ");")
	if start < 0 || end < 0 {
		return fmt.Errorf("could not isolate toMatch regex literal in research-mvp.spec.ts")
	}
	literal := strings.TrimSpace(specText[start+len(prefix) : idx+end])
	if len(literal) < 2 || literal[0] != '/' || literal[len(literal)-1] != '/' {
		return fmt.Errorf("unexpected regex literal %q", literal)
	}
	re, err := regexp.Compile(literal[1 : len(literal)-1])
	if err != nil {
		return fmt.Errorf("compile %s: %w", literal, err)
	}
	for _, wantMatch := range []string{
		"/artifacts/research/2026-11-09-mqtt-vs-nats.md",
		".artifacts/research/2026-11-09-mqtt-vs-nats.md",
		"artifacts/research/2026-11-09-mqtt-vs-nats.md",
	} {
		if !re.MatchString(wantMatch) {
			return fmt.Errorf("research artifact regex %s rejected valid artifact path %q", literal, wantMatch)
		}
	}
	for _, wantReject := range []string{
		"/tmp/not-artifacts/2026-11-09.txt",
		"/artifacts/research/subdir-without-md",
		"xartifacts/research/2026-11-09-mqtt-vs-nats.md",
	} {
		if re.MatchString(wantReject) {
			return fmt.Errorf("research artifact regex %s accepted invalid path %q", literal, wantReject)
		}
	}
	return nil
}

func checkRootDispatchLoopSelection(specName, specText string) error {
	if strings.Contains(specText, "parent_loop_id") {
		return fmt.Errorf("%s must not filter GET /teams-dispatch/loops by parent_loop_id (omitted on summary wire JSON)", specName)
	}
	if !strings.Contains(specText, `l.task_id?.startsWith("dispatch-")`) {
		return fmt.Errorf("%s must filter listLoops root loop by l.task_id?.startsWith(\"dispatch-\")", specName)
	}
	if !strings.Contains(specText, "task_id?: string;") {
		return fmt.Errorf("%s LoopSummary interface must declare task_id?: string", specName)
	}
	return nil
}

func checkApproval4cAdvertisedToolParity(
	ruleTools []string,
	fixtureDoc journeyFixtureDoc,
	taskfileText string,
	matrix []migrationMatrixScenario,
) error {
	if len(fixtureDoc.Responses) < 3 {
		return fmt.Errorf("%s has %d responses, want >= 3", fixtureDoc.Name, len(fixtureDoc.Responses))
	}
	gatedTool := fixtureDoc.Responses[2].ToolCall.Name
	if gatedTool == "" || gatedTool == "decide" {
		return fmt.Errorf("%s response[2] must call a non-decide gated tool, got %q", fixtureDoc.Name, gatedTool)
	}
	if !slices.Contains(ruleTools, gatedTool) {
		return fmt.Errorf("%s response[2] calls %q which is not advertised in research/06 tools %v (#420)", fixtureDoc.Name, gatedTool, ruleTools)
	}
	const derivedCfg = "e2e-flow-bootstrap-approval-4c.gen.json"
	if !strings.Contains(taskfileText, "AGENTIC_CONFIG: "+derivedCfg) ||
		!strings.Contains(taskfileText, `"`+gatedTool+`"`) {
		return fmt.Errorf("ui/Taskfile.yml must derive %s gating %q", derivedCfg, gatedTool)
	}
	for _, sc := range matrix {
		if sc.Name == fixtureDoc.Name {
			if sc.Config != derivedCfg {
				return fmt.Errorf("matrix scenario %s config = %q, want %q", sc.Name, sc.Config, derivedCfg)
			}
			return nil
		}
	}
	return fmt.Errorf("scenario %s missing from semstreams-migration-matrix.json", fixtureDoc.Name)
}

func loadResearch06Tools(t *testing.T, repoRoot string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot, "configs", "rules", "research", "06-needs-clarification-replan.json"))
	if err != nil {
		t.Fatalf("read research/06 rule: %v", err)
	}
	var doc struct {
		OnEnter []struct {
			Tools []string `json:"tools"`
		} `json:"on_enter"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil || len(doc.OnEnter) == 0 {
		t.Fatalf("unmarshal research/06 rule: %v (on_enter=%d)", err, len(doc.OnEnter))
	}
	return doc.OnEnter[0].Tools
}

func loadJourneyFixture(t *testing.T, repoRoot, fileName string) journeyFixtureDoc {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot, "test", "fixtures", "journeys", fileName))
	if err != nil {
		t.Fatalf("read %s: %v", fileName, err)
	}
	var doc journeyFixtureDoc
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal %s: %v", fileName, err)
	}
	return doc
}

// spec: openspec/specs/framework-adoption/spec.md#Requirement: Frozen framework adoption preserves qualified product behavior
func TestE2EHarnessHygiene_Issue420(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("findRepoRoot: %v", err)
	}

	taskfileBytes, err := os.ReadFile(filepath.Join(repoRoot, "ui", "Taskfile.yml"))
	if err != nil {
		t.Fatalf("read ui/Taskfile.yml: %v", err)
	}
	taskfileText := string(taskfileBytes)

	t.Run("stack_up_polls_dispatch_and_loops", func(t *testing.T) {
		if err := checkStackUpPollsDispatchAndLoops(taskfileText); err != nil {
			t.Fatal(err)
		}
		regressed := strings.ReplaceAll(taskfileText, "/teams-dispatch/loops", "/teams-dispatch/removed")
		if err := checkStackUpPollsDispatchAndLoops(regressed); err == nil {
			t.Fatal("expected checkStackUpPollsDispatchAndLoops to reject missing /teams-dispatch/loops probe")
		}
	})

	t.Run("research_mvp_accepts_compose_and_dot_artifact_paths", func(t *testing.T) {
		specBytes, readErr := os.ReadFile(filepath.Join(repoRoot, "ui", "e2e", "agentic", "research-mvp.spec.ts"))
		if readErr != nil {
			t.Fatalf("read research-mvp.spec.ts: %v", readErr)
		}
		if err := checkResearchArtifactRegex(string(specBytes)); err != nil {
			t.Fatal(err)
		}
		if err := checkResearchArtifactRegex(`expect(p).toMatch(/\.artifacts\/research\/.+\.md$/);`); err == nil {
			t.Fatal("expected checkResearchArtifactRegex to reject dot-only .artifacts regex")
		}
	})

	t.Run("task_board_specs_select_root_dispatch_loop", func(t *testing.T) {
		for _, specName := range []string{"task-story-trace.spec.ts", "task-title-aliases.spec.ts"} {
			specBytes, readErr := os.ReadFile(filepath.Join(repoRoot, "ui", "e2e", "agentic", specName))
			if readErr != nil {
				t.Fatalf("read %s: %v", specName, readErr)
			}
			if err := checkRootDispatchLoopSelection(specName, string(specBytes)); err != nil {
				t.Fatal(err)
			}
		}
		if err := checkRootDispatchLoopSelection("regressed.spec.ts", "l => !initialIds.has(l.loop_id)"); err == nil {
			t.Fatal("expected checkRootDispatchLoopSelection to reject unfiltered listLoops.find")
		}
	})

	t.Run("approval_4c_journeys_gate_advertised_recovery_tool", func(t *testing.T) {
		ruleTools := loadResearch06Tools(t, repoRoot)
		matrixBytes, readErr := os.ReadFile(filepath.Join(repoRoot, "scripts", "semstreams-migration-matrix.json"))
		if readErr != nil {
			t.Fatalf("read semstreams-migration-matrix.json: %v", readErr)
		}
		var matrix []migrationMatrixScenario
		if unmarshalErr := json.Unmarshal(matrixBytes, &matrix); unmarshalErr != nil {
			t.Fatalf("unmarshal semstreams-migration-matrix.json: %v", unmarshalErr)
		}
		for _, fixtureFile := range []string{"approval-pause.yaml", "approval-resume.yaml"} {
			fixtureDoc := loadJourneyFixture(t, repoRoot, fixtureFile)
			if err := checkApproval4cAdvertisedToolParity(ruleTools, fixtureDoc, taskfileText, matrix); err != nil {
				t.Fatal(err)
			}
		}
		unadvertised := loadJourneyFixture(t, repoRoot, "approval-pause.yaml")
		unadvertised.Responses[2].ToolCall.Name = "create_rule"
		if err := checkApproval4cAdvertisedToolParity(ruleTools, unadvertised, taskfileText, matrix); err == nil {
			t.Fatal("expected checkApproval4cAdvertisedToolParity to reject unadvertised create_rule tool call")
		}
	})
}
