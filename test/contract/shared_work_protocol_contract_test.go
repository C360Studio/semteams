package contract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const maxRuleFileBytes = 24000

func TestSharedWorkProtocolParity(t *testing.T) {
	const heading = "## Shared work protocol (Claude, Codex, and Gemini)"
	agentsPath := "../../AGENTS.md"
	claudePath := "../../CLAUDE.md"

	for _, path := range []string{agentsPath, claudePath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if info.Size() >= maxRuleFileBytes {
			t.Errorf("%s is %d bytes; must stay below %d-byte rule truncation cap", path, info.Size(), maxRuleFileBytes)
		}
	}

	agents := readMarkdownSection(t, agentsPath, heading)
	claude := readMarkdownSection(t, claudePath, heading)

	if agents != claude {
		t.Fatal("AGENTS.md and CLAUDE.md must contain the exact same Shared Work Protocol section")
	}
}

func TestAgentProfilesAndSkillsParity(t *testing.T) {
	root := "../.."

	for _, required := range []string{
		".agents/README.md",
		".agents/protocol.md",
	} {
		path := filepath.Join(root, required)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing %s: %v", required, err)
		}
	}

	roles := []struct {
		name     string
		readOnly bool
	}{
		{name: "architect", readOnly: true},
		{name: "go-developer", readOnly: false},
		{name: "go-reviewer", readOnly: true},
		{name: "svelte-developer", readOnly: false},
		{name: "svelte-reviewer", readOnly: true},
		{name: "technical-writer", readOnly: false},
	}

	for _, role := range roles {
		contractRel := ".agents/contracts/" + role.name + ".md"
		contractPath := filepath.Join(root, contractRel)
		if _, err := os.Stat(contractPath); err != nil {
			t.Errorf("missing canonical contract %s: %v", contractRel, err)
		}

		claudeRel := ".claude/agents/" + role.name + ".md"
		claudeBytes, err := os.ReadFile(filepath.Join(root, claudeRel))
		if err != nil {
			t.Errorf("read %s: %v", claudeRel, err)
		} else {
			claudeText := string(claudeBytes)
			if !strings.Contains(claudeText, contractRel) {
				t.Errorf("%s must reference %s", claudeRel, contractRel)
			}
			if role.readOnly && (strings.Contains(claudeText, "Edit") || strings.Contains(claudeText, "Write")) {
				t.Errorf("read-only role %s in %s must not grant Edit or Write tools", role.name, claudeRel)
			}
		}

		codexRel := ".codex/agents/" + role.name + ".toml"
		codexBytes, err := os.ReadFile(filepath.Join(root, codexRel))
		if err != nil {
			t.Errorf("read %s: %v", codexRel, err)
		} else {
			codexText := string(codexBytes)
			if !strings.Contains(codexText, contractRel) {
				t.Errorf("%s must reference %s", codexRel, contractRel)
			}
			if role.readOnly && !strings.Contains(codexText, `sandbox_mode = "read-only"`) {
				t.Errorf("read-only role %s in %s must set sandbox_mode = \"read-only\"", role.name, codexRel)
			}
		}
	}

	skills := []string{"kv-or-stream", "new-payload", "orchestration-check", "query-pattern"}
	for _, skill := range skills {
		canonicalRel := ".agents/skills/" + skill + "/SKILL.md"
		if _, err := os.Stat(filepath.Join(root, canonicalRel)); err != nil {
			t.Errorf("missing canonical skill %s: %v", canonicalRel, err)
		}

		adapterRel := ".claude/skills/" + skill + "/SKILL.md"
		adapterBytes, err := os.ReadFile(filepath.Join(root, adapterRel))
		if err != nil {
			t.Errorf("read %s: %v", adapterRel, err)
		} else if !strings.Contains(string(adapterBytes), canonicalRel) {
			t.Errorf("%s must point to %s", adapterRel, canonicalRel)
		}
	}
}

func readMarkdownSection(t *testing.T, path, heading string) string {
	t.Helper()

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	text := strings.ReplaceAll(string(body), "\r\n", "\n")
	start := strings.Index(text, heading+"\n")
	if start == -1 {
		t.Fatalf("%s is missing %q", path, heading)
	}

	section := text[start:]
	if next := strings.Index(section[len(heading)+1:], "\n## "); next != -1 {
		section = section[:len(heading)+1+next+1]
	}

	return strings.TrimSpace(section)
}
