package contract

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Steps 1–8 of .agents/README.md's read-only parity smoke, held by a contract test and its
// paired sensitivity test (adapted from SemEngine's harness-boundaries spec, "Agent setup and
// rule-index parity", and SemVocab/SemSparql's agentsetup_test.go). Expected roles, tool lists,
// sandbox modes, model/effort pins, and caps are constants here rather than read from .agents/README.md.
const (
	maxAgentsMDBytes    = 24000
	maxClaudeMDLines    = 10
	maxRoleAdapterLines = 20
	maxSkillLines       = 25
)

type agentRoleSpec struct {
	name         string
	readOnly     bool
	claudeModel  string
	claudeEffort string
	codexModel   string
	codexEffort  string
}

var (
	agentRoleSpecs = []agentRoleSpec{
		{name: "architect", readOnly: true, claudeModel: "opus", claudeEffort: "medium", codexModel: "gpt-6-astra", codexEffort: "medium"},
		{name: "go-developer", readOnly: false, claudeModel: "opus", claudeEffort: "medium", codexModel: "gpt-6-sol", codexEffort: "medium"},
		{name: "go-reviewer", readOnly: true, claudeModel: "opus", claudeEffort: "high", codexModel: "gpt-6-astra", codexEffort: "high"},
		{name: "svelte-developer", readOnly: false, claudeModel: "opus", claudeEffort: "medium", codexModel: "gpt-6-sol", codexEffort: "medium"},
		{name: "svelte-reviewer", readOnly: true, claudeModel: "opus", claudeEffort: "high", codexModel: "gpt-6-astra", codexEffort: "high"},
		{name: "technical-writer", readOnly: false, claudeModel: "sonnet", claudeEffort: "medium", codexModel: "gpt-6-sol", codexEffort: "medium"},
	}
	readOnlyClaudeTools = []string{"Read", "Bash", "Grep", "Glob", "Skill"}
	writeClaudeTools    = []string{"Read", "Edit", "Write", "Bash", "Grep", "Glob", "Skill"}
	sharedSkills        = []struct{ canonical, claude string }{
		{"handoff", "handoff"},
		{"kv-or-stream", "kv-or-stream"},
		{"new-payload", "new-payload"},
		{"orchestration-check", "orchestration-check"},
		{"pickup", "pickup"},
		{"ponytail", "ponytail"},
		{"preflight", "preflight"},
		{"query-pattern", "query-pattern"},
		{"review-pr", "review-pr"},
	}
	expectedRuleDomains = []string{
		"1. Go, composition root, and runtime ownership",
		"2. Packs, rules, predicates, and personas",
		"3. Svelte UI and observable work",
		"4. Testing, E2E, and verification",
		"5. Workflow, review, and merge gates",
	}
)

func TestAgentSetupParity(t *testing.T) {
	requireNoAgentSetupViolations(t, "agent setup parity", agentSetupViolations("../.."))
}

func TestSharedWorkProtocolParity(t *testing.T) {
	TestAgentSetupParity(t)
}

func TestAgentSetupParitySensitivity(t *testing.T) {
	base := copyAgentSetupFixture(t, "../..")
	edit := func(rel string, fn func(string) string) map[string]string {
		m := make(map[string]string, len(base))
		for k, v := range base {
			m[k] = v
		}
		if fn == nil {
			delete(m, rel)
		} else {
			m[rel] = fn(m[rel])
		}
		return m
	}
	root := writeAgentSetupTree(t, base)
	requireNoAgentSetupViolations(t, "clean fixture", agentSetupViolations(root))

	for _, tc := range []struct {
		name  string
		files map[string]string
		wants []string
	}{
		{"missing canonical contract", edit(".agents/contracts/architect.md", nil),
			[]string{".agents/contracts/architect.md", "missing"}},
		{"missing codex adapter", edit(".codex/agents/go-reviewer.toml", nil),
			[]string{".codex/agents/go-reviewer.toml", "missing"}},
		{"wrong contract path in claude adapter", edit(".claude/agents/go-reviewer.md", func(s string) string {
			return strings.ReplaceAll(s, "go-reviewer.md", "go-developer.md")
		}), []string{".claude/agents/go-reviewer.md", ".agents/contracts/go-reviewer.md"}},
		{"read-only claude adapter granted Edit", edit(".claude/agents/architect.md", func(s string) string {
			return strings.Replace(s, "tools: Read, Bash, Grep, Glob, Skill", "tools: Read, Edit, Bash, Grep, Glob, Skill", 1)
		}), []string{".claude/agents/architect.md", "tools"}},
		{"claude adapter lists LSP tool", edit(".claude/agents/go-developer.md", func(s string) string {
			return strings.Replace(s, "Skill", "Skill, LSP", 1)
		}), []string{".claude/agents/go-developer.md", "tools"}},
		{"read-only codex adapter missing sandbox_mode", edit(".codex/agents/go-reviewer.toml", func(s string) string {
			return strings.Replace(s, `sandbox_mode = "read-only"`+"\n", "", 1)
		}), []string{".codex/agents/go-reviewer.toml", "sandbox_mode"}},
		{"write codex adapter sets sandbox_mode", edit(".codex/agents/go-developer.toml", func(s string) string {
			return s + "sandbox_mode = \"read-only\"\n"
		}), []string{".codex/agents/go-developer.toml", "sandbox_mode"}},
		{"write codex adapter sets literal-string sandbox_mode", edit(".codex/agents/go-developer.toml", func(s string) string {
			return s + "sandbox_mode = 'read-only'\n"
		}), []string{".codex/agents/go-developer.toml", "sandbox_mode"}},
		{"write codex adapter sets sandbox_mode with inline comment", edit(".codex/agents/go-developer.toml", func(s string) string {
			return s + "sandbox_mode = \"read-only\" # override\n"
		}), []string{".codex/agents/go-developer.toml", "sandbox_mode"}},
		{"CLAUDE.md drops @AGENTS.md import", edit("CLAUDE.md", func(string) string {
			return "# SemTeams\n\nCustom rules here.\n"
		}), []string{"CLAUDE.md", "@AGENTS.md"}},
		{"CLAUDE.md adds section heading", edit("CLAUDE.md", func(s string) string {
			return s + "\n## Local Rule\n"
		}), []string{"CLAUDE.md", "section heading"}},
		{"AGENTS.md reaches 24000 bytes", edit("AGENTS.md", func(s string) string {
			return s + strings.Repeat("x", maxAgentsMDBytes)
		}), []string{"AGENTS.md", "24000"}},
		{"AGENTS.md and rules-index.md row count mismatch", edit("AGENTS.md", func(s string) string {
			return strings.Replace(s, "| `verify` | Pinned `revive` (`revive.toml`), worktree-safe `gofmt`, `go mod tidy -diff`, and `go vet` pass with zero errors | `task lint` (`task validate`) |\n", "", 1)
		}), []string{"AGENTS.md", ".agents/rules-index.md", "1. Go, composition root, and runtime ownership"}},
		{"role adapter contains checkbox", edit(".claude/agents/go-developer.md", func(s string) string {
			return s + "- [ ] copied checklist item\n"
		}), []string{".claude/agents/go-developer.md", "checkbox"}},
		{"role adapter exceeds line limit", edit(".codex/agents/go-developer.toml", func(s string) string {
			return s + strings.Repeat("# extra line\n", 15)
		}), []string{".codex/agents/go-developer.toml", "lines"}},
		{"missing review policy", edit(".agents/review-policy.json", nil),
			[]string{".agents/review-policy.json", "missing"}},
		{"skill adapter points to wrong canonical skill", edit(".claude/skills/preflight/SKILL.md", func(s string) string {
			return strings.ReplaceAll(s, "preflight/SKILL.md", "handoff/SKILL.md")
		}), []string{".claude/skills/preflight/SKILL.md", "../../../.agents/skills/preflight/SKILL.md"}},
		{"new canonical skill missing claude adapter", edit(".agents/skills/new-review-skill/SKILL.md", func(string) string {
			return "# New Skill\n"
		}), []string{".claude/skills/new-review-skill/SKILL.md", "missing Claude skill adapter"}},
		{"orphan claude skill adapter without canonical skill", edit(".claude/skills/orphan-skill/SKILL.md", func(string) string {
			return "# Orphan\n"
		}), []string{".claude/skills/orphan-skill/SKILL.md", "missing canonical skill"}},
		{"claude model mismatch", edit(".claude/agents/technical-writer.md", func(s string) string {
			return strings.Replace(s, "model: sonnet", "model: opus", 1)
		}), []string{".claude/agents/technical-writer.md", "model", "sonnet"}},
		{"claude effort mismatch", edit(".claude/agents/go-reviewer.md", func(s string) string {
			return strings.Replace(s, "effort: high", "effort: medium", 1)
		}), []string{".claude/agents/go-reviewer.md", "effort", "high"}},
		{"codex effort mismatch", edit(".codex/agents/go-developer.toml", func(s string) string {
			return strings.Replace(s, `model_reasoning_effort = "medium"`, `model_reasoning_effort = "high"`, 1)
		}), []string{".codex/agents/go-developer.toml", "model_reasoning_effort", "medium"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeAgentSetupTree(t, tc.files)
			requireAgentSetupViolation(t, agentSetupViolations(dir), tc.wants...)
		})
	}
}

func requireNoAgentSetupViolations(t *testing.T, label string, violations []string) {
	t.Helper()
	if len(violations) > 0 {
		t.Fatalf("%s violations:\n  - %s", label, strings.Join(violations, "\n  - "))
	}
}

func requireAgentSetupViolation(t *testing.T, violations []string, wants ...string) {
	t.Helper()
	if len(violations) == 0 {
		t.Fatalf("expected violation containing %v, got none", wants)
	}
	for _, v := range violations {
		allMatch := true
		for _, w := range wants {
			if !strings.Contains(v, w) {
				allMatch = false
				break
			}
		}
		if allMatch {
			return
		}
	}
	t.Fatalf("expected a violation containing all of %v, got:\n  - %s", wants, strings.Join(violations, "\n  - "))
}

func writeAgentSetupTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", full, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
	return dir
}

func copyAgentSetupFixture(t *testing.T, root string) map[string]string {
	t.Helper()
	rels := []string{
		"AGENTS.md",
		"CLAUDE.md",
		".agents/README.md",
		".agents/protocol.md",
		".agents/rules-index.md",
		".agents/review-policy.json",
		".agents/contracts/developer.md",
		".agents/contracts/reviewer.md",
	}
	for _, r := range agentRoleSpecs {
		rels = append(rels,
			".agents/contracts/"+r.name+".md",
			".claude/agents/"+r.name+".md",
			".codex/agents/"+r.name+".toml",
		)
	}
	for _, s := range discoverSkills(root) {
		rels = append(rels,
			".agents/skills/"+s.canonical+"/SKILL.md",
			".claude/skills/"+s.claude+"/SKILL.md",
		)
	}
	files := make(map[string]string, len(rels))
	for _, rel := range rels {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		files[rel] = string(data)
	}
	return files
}

func agentSetupViolations(root string) []string {
	var out []string
	out = append(out, checkRequiredAgentFiles(root)...)
	for _, role := range agentRoleSpecs {
		out = append(out, checkClaudeRoleAdapter(root, role)...)
		out = append(out, checkCodexRoleAdapter(root, role)...)
	}
	out = append(out, checkRulesAndSizeParity(root)...)
	out = append(out, checkSkillAdapters(root)...)
	return out
}

func checkRequiredAgentFiles(root string) []string {
	var out []string
	required := []string{
		"AGENTS.md",
		"CLAUDE.md",
		".agents/README.md",
		".agents/protocol.md",
		".agents/rules-index.md",
		".agents/review-policy.json",
		".agents/contracts/developer.md",
		".agents/contracts/reviewer.md",
	}
	for _, r := range agentRoleSpecs {
		required = append(required,
			".agents/contracts/"+r.name+".md",
			".claude/agents/"+r.name+".md",
			".codex/agents/"+r.name+".toml",
		)
	}
	for _, rel := range required {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil || info.IsDir() || info.Size() == 0 {
			out = append(out, fmt.Sprintf("%s: missing or empty required agent file", rel))
		}
	}
	return out
}

var contractPathPattern = regexp.MustCompile(`\.agents/contracts/[A-Za-z0-9_.-]+\.md`)

func checkClaudeRoleAdapter(root string, role agentRoleSpec) []string {
	rel := ".claude/agents/" + role.name + ".md"
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return nil
	}
	text := string(data)
	var out []string
	out = append(out, checkAdapterCommon(rel, role.name, text)...)

	rest, ok := strings.CutPrefix(text, "---\n")
	if !ok {
		return append(out, fmt.Sprintf("%s: missing YAML frontmatter", rel))
	}
	fmText, _, ok := strings.Cut(rest, "\n---\n")
	if !ok {
		return append(out, fmt.Sprintf("%s: unterminated YAML frontmatter", rel))
	}
	var fm struct {
		Name   string `yaml:"name"`
		Tools  string `yaml:"tools"`
		Model  string `yaml:"model"`
		Effort string `yaml:"effort"`
	}
	if err := yaml.Unmarshal([]byte(fmText), &fm); err != nil {
		return append(out, fmt.Sprintf("%s: invalid frontmatter: %v", rel, err))
	}
	if fm.Name != role.name {
		out = append(out, fmt.Sprintf("%s: frontmatter name = %q, want %q", rel, fm.Name, role.name))
	}
	if fm.Model != role.claudeModel {
		out = append(out, fmt.Sprintf("%s: frontmatter model = %q, want %q", rel, fm.Model, role.claudeModel))
	}
	if fm.Effort != role.claudeEffort {
		out = append(out, fmt.Sprintf("%s: frontmatter effort = %q, want %q", rel, fm.Effort, role.claudeEffort))
	}
	var gotTools []string
	for _, tok := range strings.Split(fm.Tools, ",") {
		if t := strings.TrimSpace(tok); t != "" {
			gotTools = append(gotTools, t)
		}
	}
	wantTools := writeClaudeTools
	if role.readOnly {
		wantTools = readOnlyClaudeTools
	}
	if !slices.Equal(gotTools, wantTools) {
		out = append(out, fmt.Sprintf("%s: frontmatter tools = %v, want %v", rel, gotTools, wantTools))
	}
	return out
}

func checkCodexRoleAdapter(root string, role agentRoleSpec) []string {
	rel := ".codex/agents/" + role.name + ".toml"
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return nil
	}
	text := string(data)
	var out []string
	out = append(out, checkAdapterCommon(rel, role.name, text)...)

	fields, err := parseFlatTOMLStrings(text)
	if err != nil {
		return append(out, fmt.Sprintf("%s: invalid TOML: %v", rel, err))
	}
	if fields["name"] != role.name {
		out = append(out, fmt.Sprintf("%s: name = %q, want %q", rel, fields["name"], role.name))
	}
	if fields["model"] != role.codexModel {
		out = append(out, fmt.Sprintf("%s: model = %q, want %q", rel, fields["model"], role.codexModel))
	}
	if fields["model_reasoning_effort"] != role.codexEffort {
		out = append(out, fmt.Sprintf("%s: model_reasoning_effort = %q, want %q", rel, fields["model_reasoning_effort"], role.codexEffort))
	}
	sandboxVal, hasSandbox := fields["sandbox_mode"]
	if role.readOnly && sandboxVal != "read-only" {
		out = append(out, fmt.Sprintf("%s: read-only role must set sandbox_mode = \"read-only\" (got %q)", rel, sandboxVal))
	}
	if !role.readOnly && hasSandbox {
		out = append(out, fmt.Sprintf("%s: write role must not set sandbox_mode (got %q)", rel, sandboxVal))
	}
	return out
}

func parseFlatTOMLStrings(text string) (map[string]string, error) {
	out := make(map[string]string)
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, rhs, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("line %d: missing '='", i+1)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("line %d: empty key", i+1)
		}
		if _, exists := out[key]; exists {
			return nil, fmt.Errorf("line %d: duplicate key %q", i+1, key)
		}
		rhs = strings.TrimSpace(rhs)
		if strings.HasPrefix(rhs, `"""`) || strings.HasPrefix(rhs, `'''`) {
			delim := rhs[:3]
			rest := rhs[3:]
			if endIdx := strings.Index(rest, delim); endIdx >= 0 {
				out[key] = rest[:endIdx]
				continue
			}
			var buf strings.Builder
			buf.WriteString(rest)
			closed := false
			for i+1 < len(lines) {
				i++
				if endIdx := strings.Index(lines[i], delim); endIdx >= 0 {
					buf.WriteString("\n")
					buf.WriteString(lines[i][:endIdx])
					closed = true
					break
				}
				buf.WriteString("\n")
				buf.WriteString(lines[i])
			}
			if !closed {
				return nil, fmt.Errorf("unterminated multiline string for %q", key)
			}
			out[key] = buf.String()
			continue
		}
		val, err := parseTOMLSingleLineString(rhs)
		if err != nil {
			return nil, fmt.Errorf("line %d (%s): %w", i+1, key, err)
		}
		out[key] = val
	}
	return out, nil
}

func parseTOMLSingleLineString(rhs string) (string, error) {
	if len(rhs) < 2 {
		return "", fmt.Errorf("invalid string literal %q", rhs)
	}
	quote := rhs[0]
	if quote != '"' && quote != '\'' {
		return "", fmt.Errorf("expected quoted string, got %q", rhs)
	}
	escaped := false
	for i := 1; i < len(rhs); i++ {
		ch := rhs[i]
		if quote == '"' && ch == '\\' && !escaped {
			escaped = true
			continue
		}
		if ch == quote && !escaped {
			trailing := strings.TrimSpace(rhs[i+1:])
			if trailing != "" && !strings.HasPrefix(trailing, "#") {
				return "", fmt.Errorf("unexpected trailing tokens %q", trailing)
			}
			return rhs[1:i], nil
		}
		escaped = false
	}
	return "", fmt.Errorf("unterminated string literal %q", rhs)
}

func checkAdapterCommon(rel, roleName, text string) []string {
	var out []string
	wantContract := ".agents/contracts/" + roleName + ".md"
	matches := contractPathPattern.FindAllString(text, -1)
	if len(matches) == 0 || !slices.Equal(matches, []string{wantContract}) {
		out = append(out, fmt.Sprintf("%s: contract references %v, want exactly [%s]", rel, matches, wantContract))
	}
	wantReadFirst := fmt.Sprintf("Your first action is to read `%s` fully", wantContract)
	if !strings.Contains(text, wantReadFirst) {
		out = append(out, fmt.Sprintf("%s: missing %q", rel, wantReadFirst))
	}
	lines := strings.Count(strings.TrimRight(text, "\n"), "\n") + 1
	if lines > maxRoleAdapterLines {
		out = append(out, fmt.Sprintf("%s: %d lines exceeds max %d lines", rel, lines, maxRoleAdapterLines))
	}
	if strings.Contains(text, "- [ ]") || strings.Contains(text, "- [x]") {
		out = append(out, fmt.Sprintf("%s: contains copied checklist checkbox", rel))
	}
	return out
}

type ruleSectionCount struct {
	title string
	rows  int
}

func checkRulesAndSizeParity(root string) []string {
	var out []string
	claudeData, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err == nil {
		claudeText := string(claudeData)
		if !strings.Contains(claudeText, "@AGENTS.md") {
			out = append(out, "CLAUDE.md: missing @AGENTS.md import")
		}
		lines := strings.Count(strings.TrimRight(claudeText, "\n"), "\n") + 1
		if lines > maxClaudeMDLines {
			out = append(out, fmt.Sprintf("CLAUDE.md: %d lines exceeds max %d lines", lines, maxClaudeMDLines))
		}
		if strings.Contains(claudeText, "\n## ") || strings.Contains(claudeText, "\n|") {
			out = append(out, "CLAUDE.md: must not carry its own section heading or rule table")
		}
	}

	agentsData, errA := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	indexData, errI := os.ReadFile(filepath.Join(root, ".agents", "rules-index.md"))
	if errA != nil || errI != nil {
		return out
	}
	if len(agentsData) >= maxAgentsMDBytes {
		out = append(out, fmt.Sprintf("AGENTS.md: %d bytes is not strictly under %d bytes", len(agentsData), maxAgentsMDBytes))
	}
	agentsCounts := parseRuleSectionCounts(string(agentsData), "### ")
	indexCounts := parseRuleSectionCounts(string(indexData), "## ")
	if len(agentsCounts) != len(expectedRuleDomains) || len(indexCounts) != len(expectedRuleDomains) {
		return append(out, fmt.Sprintf("AGENTS.md (%d sections) and .agents/rules-index.md (%d sections) must both have %d numbered rule sections",
			len(agentsCounts), len(indexCounts), len(expectedRuleDomains)))
	}
	for i, wantTitle := range expectedRuleDomains {
		a, b := agentsCounts[i], indexCounts[i]
		if a.title != wantTitle || b.title != wantTitle {
			out = append(out, fmt.Sprintf("rule section %d: AGENTS.md = %q, .agents/rules-index.md = %q, want %q", i+1, a.title, b.title, wantTitle))
			continue
		}
		if a.rows == 0 || a.rows != b.rows {
			out = append(out, fmt.Sprintf("AGENTS.md and .agents/rules-index.md disagree on row count in %q: AGENTS.md has %d, .agents/rules-index.md has %d",
				wantTitle, a.rows, b.rows))
		}
	}
	return out
}

func parseRuleSectionCounts(text, prefix string) []ruleSectionCount {
	var sections []ruleSectionCount
	cur := -1
	for _, line := range strings.Split(text, "\n") {
		if title, ok := strings.CutPrefix(line, prefix); ok && len(title) >= 3 && title[0] >= '1' && title[0] <= '9' && title[1] == '.' {
			sections = append(sections, ruleSectionCount{title: strings.TrimSpace(title)})
			cur = len(sections) - 1
			continue
		}
		if cur >= 0 && strings.HasPrefix(line, "## ") && prefix != "## " {
			cur = -1
			continue
		}
		if cur < 0 || !strings.HasPrefix(line, "| ") {
			continue
		}
		if strings.HasPrefix(line, "| ---") || strings.HasPrefix(line, "| Gate |") || strings.HasPrefix(line, "| Rule |") {
			continue
		}
		sections[cur].rows++
	}
	return sections
}

func discoverSkills(root string) []struct{ canonical, claude string } {
	skills := make([]struct{ canonical, claude string }, 0, len(sharedSkills))
	seen := make(map[string]bool, len(sharedSkills))
	for _, s := range sharedSkills {
		skills = append(skills, s)
		seen[s.canonical] = true
	}
	entries, err := os.ReadDir(filepath.Join(root, ".agents", "skills"))
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() || seen[entry.Name()] {
				continue
			}
			skills = append(skills, struct{ canonical, claude string }{entry.Name(), entry.Name()})
			seen[entry.Name()] = true
		}
	}
	return skills
}

func checkSkillAdapters(root string) []string {
	var out []string
	discovered := discoverSkills(root)
	seenClaude := make(map[string]bool, len(discovered))
	for _, s := range discovered {
		seenClaude[s.claude] = true
		canonRel := ".agents/skills/" + s.canonical + "/SKILL.md"
		if info, err := os.Stat(filepath.Join(root, filepath.FromSlash(canonRel))); err != nil || info.IsDir() || info.Size() == 0 {
			out = append(out, fmt.Sprintf("%s: missing canonical skill", canonRel))
		}
		adapterRel := ".claude/skills/" + s.claude + "/SKILL.md"
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(adapterRel)))
		if err != nil {
			out = append(out, fmt.Sprintf("%s: missing Claude skill adapter", adapterRel))
			continue
		}
		text := string(data)
		wantLink := "../../../.agents/skills/" + s.canonical + "/SKILL.md"
		if !strings.Contains(text, wantLink) || !strings.Contains(text, "fully") {
			out = append(out, fmt.Sprintf("%s: must link to %s and instruct to read it fully", adapterRel, wantLink))
		}
		lines := strings.Count(strings.TrimRight(text, "\n"), "\n") + 1
		if lines > maxSkillLines {
			out = append(out, fmt.Sprintf("%s: %d lines exceeds max %d lines", adapterRel, lines, maxSkillLines))
		}
		if strings.Contains(text, "- [ ]") || strings.Contains(text, "- [x]") {
			out = append(out, fmt.Sprintf("%s: contains copied checklist checkbox", adapterRel))
		}
	}
	if claudeEntries, err := os.ReadDir(filepath.Join(root, ".claude", "skills")); err == nil {
		for _, entry := range claudeEntries {
			if !entry.IsDir() || seenClaude[entry.Name()] {
				continue
			}
			adapterRel := ".claude/skills/" + entry.Name() + "/SKILL.md"
			canonRel := ".agents/skills/" + entry.Name() + "/SKILL.md"
			if info, err := os.Stat(filepath.Join(root, filepath.FromSlash(canonRel))); err != nil || info.IsDir() || info.Size() == 0 {
				out = append(out, fmt.Sprintf("%s: missing canonical skill %s", adapterRel, canonRel))
			}
		}
	}
	return out
}
