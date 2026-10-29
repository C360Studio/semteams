package contract

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"gopkg.in/yaml.v3"
)

// repository-governance › governance checks: `task spec:check` and `task openspec:validate` run
// `openspec validate --all --strict` and then `ateam state spec-check` (design D3 of
// adopt-ateam-v034). The explicit strict step keeps the check fail-closed: without `openspec`,
// `ateam state spec-check` prints "strict validation skipped" and exits 0 (ateam v0.3.4
// internal/state/openspec.go). Hold placement is ateam's `spec-check` since #369 retired the
// repository's own queue script.

const (
	specValidateCmd   = `openspec validate --all --strict`
	specAteamCheckCmd = `ateam state spec-check`
)

func TestSpecCheckWiring(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("findRepoRoot: %v", err)
	}
	t.Run("Taskfile.yml", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Join(repoRoot, "Taskfile.yml"))
		if err != nil {
			t.Fatal(err)
		}
		requireNoViolations(t, "spec:check wiring", specCheckWiringViolations(data, "spec:check"))
	})
	t.Run("taskfiles/openspec.yml", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Join(repoRoot, "taskfiles", "openspec.yml"))
		if err != nil {
			t.Fatal(err)
		}
		requireNoViolations(t, "openspec:validate wiring", specCheckWiringViolations(data, "validate"))
	})

	taskfile := func(extra string, cmds ...string) []byte {
		s := "version: '3'\ntasks:\n  spec:check:\n    desc: d\n" + extra + "    cmds:\n"
		for _, c := range cmds {
			s += "      - " + c + "\n"
		}
		return []byte(s)
	}
	t.Run("clean fixture", func(t *testing.T) {
		requireNoViolations(t, "clean fixture", specCheckWiringViolations(taskfile("", specValidateCmd, specAteamCheckCmd), "spec:check"))
	})
	for _, tc := range []struct {
		name  string
		file  []byte
		wants []string
	}{
		{"ateam spec-check line removed", taskfile("", specValidateCmd),
			[]string{"spec:check", "command 2", `want "` + specAteamCheckCmd + `"`}},
		{"strict validate line removed", taskfile("", specAteamCheckCmd),
			[]string{"spec:check", "command 1", `want "` + specValidateCmd + `"`}},
		{"the two lines swapped", taskfile("", specAteamCheckCmd, specValidateCmd),
			[]string{"spec:check", "command 1", `want "` + specValidateCmd + `"`}},
		{"--strict dropped", taskfile("", "openspec validate --all", specAteamCheckCmd),
			[]string{"spec:check", "command 1", `want "` + specValidateCmd + `"`}},
		{"a third command added", taskfile("", specValidateCmd, specAteamCheckCmd, "scripts/hold-check.sh"),
			[]string{"spec:check", "runs 3 commands, want exactly 2"}},
		{"ignore_error on the task", taskfile("    ignore_error: true\n", specValidateCmd, specAteamCheckCmd),
			[]string{"spec:check", "ignore_error"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requireViolation(t, specCheckWiringViolations(tc.file, "spec:check"), tc.wants...)
		})
	}
}

func specCheckWiringViolations(taskfile []byte, taskName string) []string {
	var tf struct {
		Tasks map[string]yaml.Node `yaml:"tasks"`
	}
	if err := yaml.Unmarshal(taskfile, &tf); err != nil {
		return []string{"Taskfile.yml: parse: " + err.Error()}
	}
	node, ok := tf.Tasks[taskName]
	if !ok || node.Kind != yaml.MappingNode {
		return []string{fmt.Sprintf("Taskfile.yml: %s: no such task", taskName)}
	}
	var violations, cmds []string
	for i := 0; i+1 < len(node.Content); i += 2 {
		switch key, value := node.Content[i].Value, node.Content[i+1]; key {
		case "desc", "summary":
		case "cmds":
			for _, c := range value.Content {
				if c.Kind != yaml.ScalarNode {
					cmds = append(cmds, "<not a plain command string>")
					continue
				}
				cmds = append(cmds, c.Value)
			}
		default:
			violations = append(violations, fmt.Sprintf("Taskfile.yml: %s: key %q can skip a command or discard its exit status; only desc, summary and cmds are allowed", taskName, key))
		}
	}
	wantCmds := []string{specValidateCmd, specAteamCheckCmd}
	for i, want := range wantCmds {
		got := "nothing"
		if i < len(cmds) {
			got = strconv.Quote(cmds[i])
		}
		if got != strconv.Quote(want) {
			violations = append(violations, fmt.Sprintf("Taskfile.yml: %s: command %d is %s, want %q", taskName, i+1, got, want))
		}
	}
	if len(cmds) > len(wantCmds) {
		violations = append(violations, fmt.Sprintf("Taskfile.yml: %s: runs %d commands, want exactly %d: %q", taskName, len(cmds), len(wantCmds), cmds))
	}
	return violations
}
