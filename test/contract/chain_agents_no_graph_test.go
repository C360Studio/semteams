package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestChainAgentsCannotReadGraph asserts that every chain-agent role
// the active rule packs spawn has its graph-read tools omitted from
// the publish_agent action's allowed tools. Chain agents (researchers,
// reviewers) reason from web evidence + their own loop state per
// ADR-041 addendum 2026-05-15; the graph is internal harness state,
// not a reasoning surface.
//
// Ops roles (ADR-027) ARE whitelisted graph readers — observing
// harness state is their named job. The walk skips any spawn whose
// role carries the `ops-` prefix.
//
// Originally pinned by TestADR041_ChainAgentsCannotReadGraph in
// adr041_persona_dirs_test.go (retired in ADR-042 MVP-7 alongside
// the legacy researcher-{plan,gather,synthesize,architect} +
// reviewer-{spec,qa} corpora that file's other tests pinned). The
// MVP-7 rule directories that survive are configs/rules/research/
// + configs/rules/coordinator/; configs/rules/ops/ is excluded
// because every spawn there is an ops-prefixed role (the very
// whitelist this test honors).
func TestChainAgentsCannotReadGraph(t *testing.T) {
	forbiddenForChain := map[string]bool{
		"query_entity":        true,
		"query_entities":      true,
		"query_by_type":       true,
		"query_relationships": true,
		"query_neighbors":     true,
		"summarize_graph":     true,
		"search_graph":        true,
	}

	isOpsRole := func(role string) bool {
		return strings.HasPrefix(role, "ops-")
	}

	ruleDirs := []string{
		"../../configs/rules/research",
		"../../configs/rules/coordinator",
	}

	var ruleFiles []string
	for _, dir := range ruleDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read dir %s: %v", dir, err)
		}
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".json") {
				ruleFiles = append(ruleFiles, filepath.Join(dir, e.Name()))
			}
		}
	}
	if len(ruleFiles) == 0 {
		t.Fatal("no chain-agent rule files found — test scope is broken")
	}

	for _, rulePath := range ruleFiles {
		data, err := os.ReadFile(rulePath)
		if err != nil {
			t.Errorf("read %s: %v", rulePath, err)
			continue
		}
		var r struct {
			OnEnter []struct {
				Type  string   `json:"type"`
				Role  string   `json:"role"`
				Tools []string `json:"tools"`
			} `json:"on_enter"`
		}
		if err := json.Unmarshal(data, &r); err != nil {
			t.Errorf("unmarshal %s: %v", rulePath, err)
			continue
		}
		for _, a := range r.OnEnter {
			if a.Type != "publish_agent" {
				continue
			}
			if isOpsRole(a.Role) {
				continue
			}
			for _, tool := range a.Tools {
				if forbiddenForChain[tool] {
					t.Errorf("%s: publish_agent role=%q tools includes forbidden graph-read %q — chain agents do not query the graph (ADR-041 addendum 2026-05-15)",
						filepath.Base(rulePath), a.Role, tool)
				}
			}
		}
	}
}

// TestOpsPersonaDirsExist asserts the ops-* persona corpora the
// ADR-027 ops agent depends on are present on disk with at least
// one fragment file. Sibling to the retired
// TestADR041_OpsProgressObserverPersonaGuardrails which pinned
// content-level invariants; this presence check is the minimum
// structural guarantee. Allows any fragment filename (the `ops/`
// corpus historically opens with `05-semteams-identity.md`, not
// `00-identity.md`, because upstream's ops persona layers on top).
func TestOpsPersonaDirsExist(t *testing.T) {
	root := "../../configs/personas/fragments"
	for _, dir := range []string{
		"ops",
		"ops-chain-observer",
		"ops-progress-observer",
	} {
		dirPath := filepath.Join(root, dir)
		entries, err := os.ReadDir(dirPath)
		if err != nil {
			t.Errorf("expected ops persona dir at %s: %v", dirPath, err)
			continue
		}
		hasFragment := false
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".md") {
				hasFragment = true
				break
			}
		}
		if !hasFragment {
			t.Errorf("ops persona dir %s has no .md fragments", dirPath)
		}
	}
}
