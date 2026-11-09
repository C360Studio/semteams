package contract

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type ciWorkflowFile struct {
	Jobs map[string]ciWorkflowJob `yaml:"jobs"`
}

type ciWorkflowJob struct {
	Steps []ciWorkflowStep `yaml:"steps"`
}

type ciWorkflowStep struct {
	Name string `yaml:"name"`
	Uses string `yaml:"uses"`
	Run  string `yaml:"run"`
}

// ciStepMapping pairs a normalized step from .github/workflows/ci.yml with the
// corresponding read-only command fragment required in scripts/validate.sh.
type ciStepMapping struct {
	job              string
	ciRun            string
	validateFragment string
}

func expectedCIStepMappings() []ciStepMapping {
	return []ciStepMapping{
		{job: "go", ciRun: "task lint", validateFragment: "task lint"},
		{job: "go", ciRun: "task test:race", validateFragment: "task test:race"},
		{job: "go", ciRun: "task test:browser-runner", validateFragment: "task test:browser-runner"},
		{job: "go", ciRun: "task test:integration", validateFragment: "task test:integration"},
		{job: "go", ciRun: "go build ./...", validateFragment: `go build -o "$TMP_DIR/bin/" ./...`},
		{
			job:              "go",
			ciRun:            "task schema:generate\ntask schema:check-changes",
			validateFragment: `go run ./cmd/openapi-generator -out "$TMP_DIR/schemas" -openapi "$TMP_DIR/specs/openapi.v3.yaml"`,
		},
		{job: "ui", ciRun: "npm run lint", validateFragment: "npm run lint"},
		{job: "ui", ciRun: "npm run check", validateFragment: "npm run check"},
		{job: "ui", ciRun: "npm run test:unit -- --run", validateFragment: "npm run test:unit -- --run"},
		{job: "ui", ciRun: "npm run generate-types:check", validateFragment: "npm run generate-types:check"},
		{job: "ui", ciRun: "npm run build", validateFragment: "npm run build"},
		{job: "governance", ciRun: "task ateam:doctor", validateFragment: "task ateam:doctor"},
		{job: "governance", ciRun: "task openspec:validate", validateFragment: "task openspec:validate"},
		{job: "governance", ciRun: "task test:spec", validateFragment: "task test:spec"},
		{job: "governance", ciRun: "task publish:verify-test", validateFragment: "task publish:verify-test"},
	}
}

func isToolInstallStep(run string) bool {
	trimmed := strings.TrimSpace(run)
	if trimmed == "npm ci" {
		return true
	}
	lines := strings.Split(trimmed, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "go install ") && !strings.HasPrefix(line, "GOTOOLCHAIN=auto go install ") && !strings.HasPrefix(line, "npm install ") {
			return false
		}
	}
	return len(lines) > 0
}

func verifyCIWorkflowParity(ciYAML []byte, validateScript string) error {
	var wf ciWorkflowFile
	if err := yaml.Unmarshal(ciYAML, &wf); err != nil {
		return fmt.Errorf("unmarshal ci.yml: %w", err)
	}

	var actualSteps []struct {
		job string
		run string
	}
	for _, jobName := range []string{"go", "ui", "governance"} {
		job, ok := wf.Jobs[jobName]
		if !ok {
			return fmt.Errorf("ci.yml missing required job %q", jobName)
		}
		for _, step := range job.Steps {
			run := strings.TrimSpace(step.Run)
			if run == "" || isToolInstallStep(run) {
				continue
			}
			actualSteps = append(actualSteps, struct {
				job string
				run string
			}{job: jobName, run: run})
		}
	}

	expected := expectedCIStepMappings()
	if len(actualSteps) != len(expected) {
		return fmt.Errorf("ci.yml validation step count = %d, want %d (actual=%+v)", len(actualSteps), len(expected), actualSteps)
	}

	lastIdx := -1
	for i, exp := range expected {
		act := actualSteps[i]
		if act.job != exp.job || act.run != exp.ciRun {
			return fmt.Errorf("step %d mismatch: ci.yml has (%s: %q), expected (%s: %q)", i, act.job, act.run, exp.job, exp.ciRun)
		}
		idx := strings.Index(validateScript, exp.validateFragment)
		if idx == -1 {
			return fmt.Errorf("scripts/validate.sh missing fragment %q for CI step (%s: %q)", exp.validateFragment, exp.job, exp.ciRun)
		}
		if idx <= lastIdx {
			return fmt.Errorf("scripts/validate.sh fragment %q appears out of CI order", exp.validateFragment)
		}
		lastIdx = idx
	}

	return nil
}

// spec: openspec/specs/repository-governance/spec.md#Requirement: Repository CI runs the obvious repository checks
func TestValidateTaskMirrorsRepositoryCI(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("findRepoRoot: %v", err)
	}

	ciBytes, err := os.ReadFile(filepath.Join(repoRoot, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("read ci.yml: %v", err)
	}
	scriptBytes, err := os.ReadFile(filepath.Join(repoRoot, "scripts", "validate.sh"))
	if err != nil {
		t.Fatalf("read scripts/validate.sh: %v", err)
	}
	scriptText := string(scriptBytes)

	if err := verifyCIWorkflowParity(ciBytes, scriptText); err != nil {
		t.Fatalf("CI parity check failed: %v", err)
	}

	t.Run("detects_added_ci_step_drift", func(t *testing.T) {
		driftedCI := strings.Replace(
			string(ciBytes),
			"      - name: Lint\n        run: task lint",
			"      - name: Lint\n        run: task lint\n      - name: Extra Check\n        run: task extra:check",
			1,
		)
		if err := verifyCIWorkflowParity([]byte(driftedCI), scriptText); err == nil {
			t.Fatal("expected parity check to fail when ci.yml adds an unmapped step, but it passed")
		}
	})

	t.Run("detects_missing_validate_step_drift", func(t *testing.T) {
		driftedScript := strings.Replace(scriptText, "task publish:verify-test", "# removed", 1)
		if err := verifyCIWorkflowParity(ciBytes, driftedScript); err == nil {
			t.Fatal("expected parity check to fail when scripts/validate.sh omits a CI step, but it passed")
		}
	})

	t.Run("detects_missing_test_spec_drift", func(t *testing.T) {
		driftedScript := strings.Replace(scriptText, "task test:spec\n", "", 1)
		if err := verifyCIWorkflowParity(ciBytes, driftedScript); err == nil {
			t.Fatal("expected parity check to fail when scripts/validate.sh omits task test:spec, but it passed")
		}
		driftedCI := strings.Replace(string(ciBytes), "        run: task test:spec\n", "", 1)
		if err := verifyCIWorkflowParity([]byte(driftedCI), scriptText); err == nil {
			t.Fatal("expected parity check to fail when ci.yml omits task test:spec, but it passed")
		}
	})

	t.Run("detects_out_of_order_validate_step_drift", func(t *testing.T) {
		driftedScript := strings.Replace(scriptText, "task lint\ntask test:race", "task test:race\ntask lint", 1)
		if err := verifyCIWorkflowParity(ciBytes, driftedScript); err == nil {
			t.Fatal("expected parity check to fail when scripts/validate.sh reorders CI steps, but it passed")
		}
	})
}

type worktreeFixture struct {
	rootDir  string
	binDir   string
	logFile  string
	snapshot string
}

func newValidateWorktreeFixture(t *testing.T) *worktreeFixture {
	t.Helper()

	base := t.TempDir()
	repoDir := filepath.Join(base, "repo")
	binDir := filepath.Join(base, "bin")
	logFile := filepath.Join(base, "commands.log")

	for _, dir := range []string{
		filepath.Join(repoDir, "schemas"),
		filepath.Join(repoDir, "specs"),
		filepath.Join(repoDir, "ui", "node_modules"),
		filepath.Join(repoDir, "ui", "src", "lib", "types"),
		binDir,
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	writeFile := func(rel, content string) {
		t.Helper()
		full := filepath.Join(repoDir, rel)
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	writeFile(".gitignore", "ui/node_modules/\nui/build/\nui/.svelte-kit/\n.ateam/\n")
	writeFile("schemas/agentic-loop.v1.json", "{\"$id\":\"agentic-loop.v1.json\"}\n")
	writeFile("specs/openapi.v3.yaml", "openapi: 3.0.3\n")
	writeFile("ui/src/lib/types/api.generated.ts", "export interface paths {}\n")
	writeFile("tracked.txt", "committed-tracked\n")
	writeFile("staged.txt", "committed-staged\n")

	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@example.invalid",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@example.invalid",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
		}
		return string(out)
	}

	runGit("init", "--quiet")
	runGit("add", ".gitignore", "schemas/agentic-loop.v1.json", "specs/openapi.v3.yaml", "ui/src/lib/types/api.generated.ts", "tracked.txt", "staged.txt")
	runGit("commit", "--quiet", "-m", "initial")

	// Create staged, unstaged, and untracked changes to prove dirty-worktree preservation.
	writeFile("staged.txt", "staged-modification\n")
	runGit("add", "staged.txt")
	writeFile("tracked.txt", "unstaged-modification\n")
	writeFile("untracked.txt", "untracked-content\n")

	writeValidateStubs(t, binDir)

	fx := &worktreeFixture{
		rootDir: repoDir,
		binDir:  binDir,
		logFile: logFile,
	}
	fx.snapshot = fx.captureSnapshot(t)
	return fx
}

func writeValidateStubs(t *testing.T, binDir string) {
	t.Helper()

	writeStub := func(name, body string) {
		t.Helper()
		p := filepath.Join(binDir, name)
		if err := os.WriteFile(p, []byte("#!/usr/bin/env bash\nset -euo pipefail\n"+body+"\n"), 0755); err != nil {
			t.Fatalf("write stub %s: %v", name, err)
		}
	}

	writeStub("go", `
echo "go $*" >> "$VALIDATE_TEST_LOG"
if [ "${1:-}" = "build" ]; then
  # Verify -o points outside VALIDATE_REPO_ROOT
  out_dir="${3:-}"
  case "$out_dir" in
    "$VALIDATE_REPO_ROOT"*)
      echo "go build output inside repo root: $out_dir" >&2
      exit 2
      ;;
  esac
  mkdir -p "$out_dir"
  touch "$out_dir/semteams"
  exit 0
fi
if [ "${1:-}" = "run" ] && [ "${2:-}" = "./cmd/openapi-generator" ]; then
  out_dir=""
  openapi_out=""
  shift 2
  while [ $# -gt 0 ]; do
    case "$1" in
      -out) out_dir="$2"; shift 2 ;;
      -openapi) openapi_out="$2"; shift 2 ;;
      *) shift ;;
    esac
  done
  case "$out_dir" in
    "$VALIDATE_REPO_ROOT"*)
      echo "openapi-generator -out inside repo root: $out_dir" >&2
      exit 2
      ;;
  esac
  case "$openapi_out" in
    "$VALIDATE_REPO_ROOT"*)
      echo "openapi-generator -openapi inside repo root: $openapi_out" >&2
      exit 2
      ;;
  esac
  mkdir -p "$out_dir" "$(dirname "$openapi_out")"
  printf '%s' "${FAKE_SCHEMA_CONTENT:-{\"\$id\":\"agentic-loop.v1.json\"}}" > "$out_dir/agentic-loop.v1.json"
  printf '\n' >> "$out_dir/agentic-loop.v1.json"
  printf '%s' "${FAKE_OPENAPI_CONTENT:-openapi: 3.0.3}" > "$openapi_out"
  printf '\n' >> "$openapi_out"
  exit 0
fi
`)
	writeStub("gofmt", `echo "gofmt $*" >> "$VALIDATE_TEST_LOG"`)
	writeStub("task", `
echo "task $*" >> "$VALIDATE_TEST_LOG"
if [ "${FAKE_FAIL_STEP:-}" = "task $*" ]; then
  echo "simulated failure in task $*" >&2
  exit 1
fi
`)
	writeStub("revive", `echo "revive $*" >> "$VALIDATE_TEST_LOG"`)
	writeStub("python3", `echo "python3 $*" >> "$VALIDATE_TEST_LOG"`)
	writeStub("node", `echo "node $*" >> "$VALIDATE_TEST_LOG"`)
	writeStub("npm", `
echo "npm $*" >> "$VALIDATE_TEST_LOG"
mkdir -p "$VALIDATE_REPO_ROOT/ui/.svelte-kit" "$VALIDATE_REPO_ROOT/ui/build"
echo "generated-build-artifact" > "$VALIDATE_REPO_ROOT/ui/build/index.js"
if [ "${FAKE_FAIL_STEP:-}" = "npm $*" ]; then
  echo "simulated failure in npm $*" >&2
  exit 1
fi
`)
	writeStub("docker", `
echo "docker $*" >> "$VALIDATE_TEST_LOG"
if [ "${FAKE_DOCKER_DOWN:-}" = "1" ]; then
  exit 1
fi
`)
	writeStub("openspec", `echo "openspec $*" >> "$VALIDATE_TEST_LOG"`)
	writeStub("ateam", `echo "ateam $*" >> "$VALIDATE_TEST_LOG"`)
	writeStub("jq", `echo "jq $*" >> "$VALIDATE_TEST_LOG"`)
}

func (fx *worktreeFixture) captureSnapshot(t *testing.T) string {
	t.Helper()
	var b strings.Builder

	run := func(name string, args ...string) {
		t.Helper()
		cmd := exec.Command(name, args...)
		cmd.Dir = fx.rootDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s %v: %v\n%s", name, args, err, string(out))
		}
		b.WriteString("=== " + name + " " + strings.Join(args, " ") + " ===\n")
		b.Write(out)
	}

	run("git", "status", "--porcelain", "--untracked-files=all")
	run("git", "diff")
	run("git", "diff", "--cached")

	for _, rel := range []string{
		"schemas/agentic-loop.v1.json",
		"specs/openapi.v3.yaml",
		"ui/src/lib/types/api.generated.ts",
		"tracked.txt",
		"staged.txt",
		"untracked.txt",
	} {
		data, err := os.ReadFile(filepath.Join(fx.rootDir, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		b.WriteString("=== file " + rel + " ===\n")
		b.Write(data)
	}
	return b.String()
}

func (fx *worktreeFixture) assertWorktreeUnchanged(t *testing.T) {
	t.Helper()
	got := fx.captureSnapshot(t)
	if got != fx.snapshot {
		t.Fatalf("worktree state changed!\n--- want\n%s\n--- got\n%s", fx.snapshot, got)
	}
	for _, transient := range []string{"ui/build", "ui/.svelte-kit", ".ateam"} {
		if _, err := os.Stat(filepath.Join(fx.rootDir, transient)); !os.IsNotExist(err) {
			t.Fatalf("expected transient directory %s to be cleaned up when it did not exist before validate", transient)
		}
	}
}

// spec: openspec/specs/repository-governance/spec.md#Requirement: task validate is the read-only local mirror of Repository CI
func TestValidateScriptCommandCompositionAndWorktreePreservation(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("findRepoRoot: %v", err)
	}
	scriptPath := filepath.Join(repoRoot, "scripts", "validate.sh")

	t.Run("passing_run_executes_all_lanes_and_preserves_dirty_worktree", func(t *testing.T) {
		fx := newValidateWorktreeFixture(t)
		cmd := exec.Command("bash", scriptPath)
		cmd.Env = []string{
			"PATH=" + fx.binDir + ":/usr/bin:/bin",
			"HOME=" + os.Getenv("HOME"),
			"TMPDIR=" + t.TempDir(),
			"VALIDATE_REPO_ROOT=" + fx.rootDir,
			"VALIDATE_TEST_LOG=" + fx.logFile,
		}
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("validate.sh failed: %v\n%s", err, string(out))
		}

		fx.assertWorktreeUnchanged(t)

		logBytes, err := os.ReadFile(fx.logFile)
		if err != nil {
			t.Fatalf("read command log: %v", err)
		}
		lines := strings.Split(strings.TrimSpace(string(logBytes)), "\n")
		wantPrefixes := []string{
			"docker info",
			"task lint",
			"task test:race",
			"task test:browser-runner",
			"task test:integration",
			"go build -o ",
			"go run ./cmd/openapi-generator -out ",
			"npm run lint",
			"npm run check",
			"npm run test:unit -- --run",
			"npm run generate-types:check",
			"npm run build",
			"task ateam:doctor",
			"task openspec:validate",
			"task test:spec",
			"task publish:verify-test",
		}
		if len(lines) != len(wantPrefixes) {
			t.Fatalf("logged command count = %d, want %d\nlog:\n%s", len(lines), len(wantPrefixes), string(logBytes))
		}
		for i, want := range wantPrefixes {
			if !strings.HasPrefix(lines[i], want) {
				t.Errorf("command[%d] = %q, want prefix %q", i, lines[i], want)
			}
		}
		logText := string(logBytes)
		for _, forbidden := range []string{"playwright", "test:e2e", "smoke"} {
			if strings.Contains(logText, forbidden) {
				t.Errorf("validate command log must never contain %q, got:\n%s", forbidden, logText)
			}
		}
	})

	t.Run("schema_drift_fails_and_preserves_dirty_worktree", func(t *testing.T) {
		fx := newValidateWorktreeFixture(t)
		cmd := exec.Command("bash", scriptPath)
		cmd.Env = []string{
			"PATH=" + fx.binDir + ":/usr/bin:/bin",
			"HOME=" + os.Getenv("HOME"),
			"TMPDIR=" + t.TempDir(),
			"VALIDATE_REPO_ROOT=" + fx.rootDir,
			"VALIDATE_TEST_LOG=" + fx.logFile,
			`FAKE_SCHEMA_CONTENT={"$id":"drifted.v1.json"}`,
		}
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("expected validate.sh to fail on schema drift, but it passed:\n%s", string(out))
		}
		if !strings.Contains(string(out), "generated schemas/OpenAPI drifted") {
			t.Fatalf("expected schema drift message, got:\n%s", string(out))
		}
		fx.assertWorktreeUnchanged(t)
	})

	t.Run("openapi_drift_fails_and_preserves_dirty_worktree", func(t *testing.T) {
		fx := newValidateWorktreeFixture(t)
		cmd := exec.Command("bash", scriptPath)
		cmd.Env = []string{
			"PATH=" + fx.binDir + ":/usr/bin:/bin",
			"HOME=" + os.Getenv("HOME"),
			"TMPDIR=" + t.TempDir(),
			"VALIDATE_REPO_ROOT=" + fx.rootDir,
			"VALIDATE_TEST_LOG=" + fx.logFile,
			"FAKE_OPENAPI_CONTENT=openapi: 3.1.0-drifted",
		}
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("expected validate.sh to fail on OpenAPI drift, but it passed:\n%s", string(out))
		}
		if !strings.Contains(string(out), "generated schemas/OpenAPI drifted") {
			t.Fatalf("expected OpenAPI drift message, got:\n%s", string(out))
		}
		fx.assertWorktreeUnchanged(t)
	})

	t.Run("mid_lane_failure_stops_and_preserves_dirty_worktree", func(t *testing.T) {
		fx := newValidateWorktreeFixture(t)
		cmd := exec.Command("bash", scriptPath)
		cmd.Env = []string{
			"PATH=" + fx.binDir + ":/usr/bin:/bin",
			"HOME=" + os.Getenv("HOME"),
			"TMPDIR=" + t.TempDir(),
			"VALIDATE_REPO_ROOT=" + fx.rootDir,
			"VALIDATE_TEST_LOG=" + fx.logFile,
			"FAKE_FAIL_STEP=npm run check",
		}
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("expected validate.sh to fail when npm run check fails, but it passed:\n%s", string(out))
		}
		fx.assertWorktreeUnchanged(t)
	})
}

func TestValidateScriptPrerequisiteFailures(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("findRepoRoot: %v", err)
	}
	scriptPath := filepath.Join(repoRoot, "scripts", "validate.sh")

	for _, missingTool := range []string{"go", "task", "revive", "node", "npm", "docker", "python3", "openspec", "ateam", "jq"} {
		t.Run("missing_"+missingTool, func(t *testing.T) {
			fx := newValidateWorktreeFixture(t)
			// Build a clean directory of symlinked system utilities plus stubs, omitting missingTool.
			isolatedBin := filepath.Join(t.TempDir(), "isolated-bin")
			if err := os.MkdirAll(isolatedBin, 0755); err != nil {
				t.Fatalf("mkdir isolated-bin: %v", err)
			}
			for _, sysTool := range []string{"bash", "sh", "dirname", "pwd", "mktemp", "rm", "mkdir", "cat", "diff", "git", "touch"} {
				sysPath, lookErr := exec.LookPath(sysTool)
				if lookErr != nil {
					continue
				}
				_ = os.Symlink(sysPath, filepath.Join(isolatedBin, sysTool))
			}
			entries, err := os.ReadDir(fx.binDir)
			if err != nil {
				t.Fatalf("readdir binDir: %v", err)
			}
			for _, entry := range entries {
				if entry.Name() == missingTool {
					continue
				}
				_ = os.Symlink(filepath.Join(fx.binDir, entry.Name()), filepath.Join(isolatedBin, entry.Name()))
			}

			cmd := exec.Command("bash", scriptPath)
			cmd.Env = []string{
				"PATH=" + isolatedBin,
				"HOME=" + os.Getenv("HOME"),
				"TMPDIR=" + t.TempDir(),
				"VALIDATE_REPO_ROOT=" + fx.rootDir,
				"VALIDATE_TEST_LOG=" + fx.logFile,
			}
			out, runErr := cmd.CombinedOutput()
			if runErr == nil {
				t.Fatalf("expected validate.sh to fail when %s is missing, but it passed:\n%s", missingTool, string(out))
			}
			if !strings.Contains(string(out), "missing required prerequisite(s)") || !strings.Contains(string(out), missingTool) {
				t.Fatalf("expected missing prerequisite error naming %q, got:\n%s", missingTool, string(out))
			}
			if _, statErr := os.Stat(fx.logFile); !os.IsNotExist(statErr) {
				t.Fatalf("expected no commands to execute when prerequisite %q is missing", missingTool)
			}
			fx.assertWorktreeUnchanged(t)
		})
	}

	t.Run("docker_daemon_unreachable", func(t *testing.T) {
		fx := newValidateWorktreeFixture(t)
		cmd := exec.Command("bash", scriptPath)
		cmd.Env = []string{
			"PATH=" + fx.binDir + ":/usr/bin:/bin",
			"HOME=" + os.Getenv("HOME"),
			"TMPDIR=" + t.TempDir(),
			"VALIDATE_REPO_ROOT=" + fx.rootDir,
			"VALIDATE_TEST_LOG=" + fx.logFile,
			"FAKE_DOCKER_DOWN=1",
		}
		out, runErr := cmd.CombinedOutput()
		if runErr == nil {
			t.Fatalf("expected validate.sh to fail when docker daemon is unreachable, but it passed:\n%s", string(out))
		}
		if !strings.Contains(string(out), "docker daemon is not reachable") {
			t.Fatalf("expected docker daemon error message, got:\n%s", string(out))
		}
		fx.assertWorktreeUnchanged(t)
	})

	t.Run("missing_ui_node_modules", func(t *testing.T) {
		fx := newValidateWorktreeFixture(t)
		if err := os.RemoveAll(filepath.Join(fx.rootDir, "ui", "node_modules")); err != nil {
			t.Fatalf("remove ui/node_modules: %v", err)
		}
		fx.snapshot = fx.captureSnapshot(t)

		cmd := exec.Command("bash", scriptPath)
		cmd.Env = []string{
			"PATH=" + fx.binDir + ":/usr/bin:/bin",
			"HOME=" + os.Getenv("HOME"),
			"TMPDIR=" + t.TempDir(),
			"VALIDATE_REPO_ROOT=" + fx.rootDir,
			"VALIDATE_TEST_LOG=" + fx.logFile,
		}
		out, runErr := cmd.CombinedOutput()
		if runErr == nil {
			t.Fatalf("expected validate.sh to fail when ui/node_modules is missing, but it passed:\n%s", string(out))
		}
		if !strings.Contains(string(out), "missing required prerequisite: ui/node_modules") {
			t.Fatalf("expected ui/node_modules prerequisite error, got:\n%s", string(out))
		}
		fx.assertWorktreeUnchanged(t)
	})
}

// spec: openspec/specs/repository-governance/spec.md#Requirement: task validate is the read-only local mirror of Repository CI
func TestValidateTaskAndDocsContract(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("findRepoRoot: %v", err)
	}

	taskfileBytes, err := os.ReadFile(filepath.Join(repoRoot, "Taskfile.yml"))
	if err != nil {
		t.Fatalf("read Taskfile.yml: %v", err)
	}
	taskfileText := string(taskfileBytes)
	if !strings.Contains(taskfileText, "validate:") || !strings.Contains(taskfileText, "scripts/validate.sh") {
		t.Fatal("Taskfile.yml must define top-level 'validate' task invoking scripts/validate.sh")
	}
	for _, wantTask := range []string{
		"tools:ateam:",
		"doctor:", "scripts/doctor.sh",
		"ateam:doctor:", "env -u ATEAM_SESSION_ID -u CLAUDE_SESSION_ID -u CODEX_SESSION_ID -u GEMINI_SESSION_ID scripts/ateam doctor --check",
		"fmt:", "fmt:check:", "tidy:check:",
		"cleanup-roots:check:", "scripts/cleanup-roots-check.sh",
		"merge:check:", "scripts/merge-check.sh",
		"mutate:check:", `- "scripts/ateam mutate {{.CLI_ARGS}}"`,
		"spec:check:", "spec:queue:",
		"test:spec:", `- scripts/ateam test-spec --base "${TEST_SPEC_BASE:-origin/main}"`,
		"verify:", "scripts/verify.sh",
	} {
		if !strings.Contains(taskfileText, wantTask) {
			t.Errorf("Taskfile.yml missing required SemEngine parity entry %q", wantTask)
		}
	}
	verifyScriptBytes, err := os.ReadFile(filepath.Join(repoRoot, "scripts", "verify.sh"))
	if err != nil {
		t.Fatalf("read scripts/verify.sh: %v", err)
	}
	if !strings.Contains(string(verifyScriptBytes), "validate.sh") {
		t.Fatal("scripts/verify.sh must delegate to scripts/validate.sh")
	}

	pkgBytes, err := os.ReadFile(filepath.Join(repoRoot, "ui", "package.json"))
	if err != nil {
		t.Fatalf("read ui/package.json: %v", err)
	}
	if err := verifyGenerateTypesCheckScript(pkgBytes); err != nil {
		t.Fatalf("ui/package.json generate-types:check contract: %v", err)
	}

	agentsBytes, err := os.ReadFile(filepath.Join(repoRoot, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	agentsText := strings.ReplaceAll(string(agentsBytes), "\r\n", "\n")
	wantBlock := "Before pushing:\n\n```bash\ntask validate\n```"
	if !strings.Contains(agentsText, wantBlock) {
		t.Errorf("AGENTS.md must collapse 'Before pushing:' to a single 'task validate' block")
	}

	claudeBytes, err := os.ReadFile(filepath.Join(repoRoot, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	if !strings.Contains(string(claudeBytes), "@AGENTS.md") {
		t.Errorf("CLAUDE.md must import @AGENTS.md")
	}
}

var (
	generateTypesDelegateRe    = regexp.MustCompile(`(?:^|[;&|({]\s*|bash\s+-c\s+['"]\s*)(?:npm|pnpm|yarn|bun)\s+(?:--[^\s]+\s+)*(?:run\s+)?generate-types(?::from-url)?(?:\s|$|[;&|)'"])`)
	trackedGeneratedRedirectRe = regexp.MustCompile(`(?:>{1,2}|\b(?:cp|mv|install|tee)\b[^;&|]*)\s*["']?(?:\./)?src/lib/types/api\.generated\.ts\b`)
	mktempDirVarRe             = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]*)=\$\(\s*mktemp\s+-d\b`)
	outputFlagRe               = regexp.MustCompile(`(?:\s|^)(--output|-o)(?:=|\s+)(\\"[^\\"]*\\"|\\"[^"]*|"[^"]*"|'[^']*'|[^\s;"']+)`)
)

func verifyGenerateTypesCheckScript(pkgBytes []byte) error {
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(pkgBytes, &pkg); err != nil {
		return fmt.Errorf("unmarshal ui/package.json: %w", err)
	}
	script := strings.TrimSpace(pkg.Scripts["generate-types:check"])
	if script == "" {
		return fmt.Errorf("missing or empty scripts[\"generate-types:check\"]")
	}
	if generateTypesDelegateRe.MatchString(script) {
		return fmt.Errorf("must not delegate to mutating generate-types script")
	}
	if trackedGeneratedRedirectRe.MatchString(script) {
		return fmt.Errorf("must not redirect or copy output to tracked src/lib/types/api.generated.ts")
	}
	return verifyGenerateTypesOutputTargets(script)
}

func verifyGenerateTypesOutputTargets(script string) error {
	var tempVars []string
	for _, m := range mktempDirVarRe.FindAllStringSubmatch(script, -1) {
		tempVars = append(tempVars, m[1])
	}
	matches := outputFlagRe.FindAllStringSubmatch(script, -1)
	hasLongOutput := false
	for _, m := range matches {
		flag, target := m[1], trimShellQuotes(m[2])
		if flag == "--output" {
			hasLongOutput = true
		}
		if strings.Contains(target, "src/lib/types/api.generated.ts") || strings.Contains(target, "..") {
			return fmt.Errorf("%s target %q must not write tracked src/lib/types/api.generated.ts or traverse parent dirs", flag, target)
		}
		if !targetUsesTempDir(target, tempVars) {
			if len(tempVars) == 0 {
				return fmt.Errorf("%s target %q must write inside a mktemp -d directory", flag, target)
			}
			return fmt.Errorf("%s target %q must reference mktemp -d directory ($%s)", flag, target, strings.Join(tempVars, " or $"))
		}
	}
	if len(tempVars) == 0 && !strings.Contains(script, "mktemp -d") {
		return fmt.Errorf("must allocate a temporary directory via mktemp -d")
	}
	if !hasLongOutput {
		return fmt.Errorf("must pass --output writing into the mktemp -d temporary directory")
	}
	return nil
}

func trimShellQuotes(s string) string {
	s = strings.TrimPrefix(strings.TrimSuffix(s, `\"`), `\"`)
	return strings.Trim(s, `"'`)
}

func targetUsesTempDir(target string, tempVars []string) bool {
	if strings.HasPrefix(target, "$(mktemp -d") && strings.Contains(target, "/") {
		return true
	}
	for _, v := range tempVars {
		if strings.HasPrefix(target, "$"+v+"/") || strings.HasPrefix(target, "${"+v+"}/") {
			return true
		}
	}
	return false
}

func mutateGenerateTypesCheckJSON(t *testing.T, basePkgBytes []byte, script string, remove bool) []byte {
	t.Helper()
	var raw map[string]any
	if err := json.Unmarshal(basePkgBytes, &raw); err != nil {
		t.Fatalf("unmarshal ui/package.json: %v", err)
	}
	scripts, ok := raw["scripts"].(map[string]any)
	if !ok {
		t.Fatal("ui/package.json missing scripts object")
	}
	if remove {
		delete(scripts, "generate-types:check")
	} else {
		scripts["generate-types:check"] = script
	}
	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		t.Fatalf("marshal mutated package.json: %v", err)
	}
	return out
}

// spec: openspec/specs/repository-governance/spec.md#Requirement: task validate is the read-only local mirror of Repository CI
func TestGenerateTypesCheckGuardSensitivity(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("findRepoRoot: %v", err)
	}
	pkgBytes, err := os.ReadFile(filepath.Join(repoRoot, "ui", "package.json"))
	if err != nil {
		t.Fatalf("read ui/package.json: %v", err)
	}
	if err := verifyGenerateTypesCheckScript(pkgBytes); err != nil {
		t.Fatalf("clean ui/package.json should pass: %v", err)
	}

	cases := []struct {
		name, script, wantSub string
		raw                   []byte
		remove                bool
	}{
		{name: "invalid_json", raw: []byte("{not-json"), wantSub: "unmarshal"},
		{name: "missing_script_entry", remove: true, wantSub: "generate-types:check"},
		{name: "inline_overwrite_long_flag", script: "openapi-typescript ${OPENAPI_SPEC_PATH:-../specs/openapi.v3.yaml} --output src/lib/types/api.generated.ts", wantSub: "src/lib/types/api.generated.ts"},
		{name: "inline_overwrite_equals_flag", script: "openapi-typescript ../specs/openapi.v3.yaml --output=./src/lib/types/api.generated.ts", wantSub: "src/lib/types/api.generated.ts"},
		{name: "inline_overwrite_short_flag", script: "openapi-typescript ../specs/openapi.v3.yaml -o src/lib/types/api.generated.ts", wantSub: "src/lib/types/api.generated.ts"},
		{name: "mktemp_present_but_second_output_overwrites", script: `bash -c 'tmpdir=$(mktemp -d); openapi-typescript ../specs/openapi.v3.yaml --output "$tmpdir/api.generated.ts"; openapi-typescript ../specs/openapi.v3.yaml --output src/lib/types/api.generated.ts'`, wantSub: "src/lib/types/api.generated.ts"},
		{name: "path_traversal_out_of_tmpdir", script: `bash -c 'tmpdir=$(mktemp -d); openapi-typescript ../specs/openapi.v3.yaml --output "$tmpdir/../src/lib/types/api.generated.ts"'`, wantSub: "src/lib/types/api.generated.ts"},
		{name: "shell_redirect_overwrites_tracked_file", script: `bash -c 'tmpdir=$(mktemp -d); openapi-typescript ../specs/openapi.v3.yaml --output "$tmpdir/api.generated.ts"; cat "$tmpdir/api.generated.ts" > src/lib/types/api.generated.ts'`, wantSub: "src/lib/types/api.generated.ts"},
		{name: "cp_overwrites_tracked_file", script: `bash -c 'tmpdir=$(mktemp -d); openapi-typescript ../specs/openapi.v3.yaml --output "$tmpdir/api.generated.ts"; cp "$tmpdir/api.generated.ts" src/lib/types/api.generated.ts'`, wantSub: "src/lib/types/api.generated.ts"},
		{name: "delegates_to_npm_run_generate_types", script: "npm run generate-types && git diff --exit-code src/lib/types/api.generated.ts", wantSub: "generate-types"},
		{name: "delegates_via_bash_c_npm_run_generate_types", script: `bash -c 'tmpdir=$(mktemp -d); openapi-typescript ../specs/openapi.v3.yaml --output "$tmpdir/api.generated.ts"; npm --silent run generate-types'`, wantSub: "generate-types"},
		{name: "wrapper_without_mktemp_or_output", script: "./scripts/custom-typegen.sh", wantSub: "mktemp"},
		{name: "mktemp_without_output_flag", script: `bash -c 'tmpdir=$(mktemp -d); ./scripts/custom-typegen.sh "$tmpdir"'`, wantSub: "--output"},
		{name: "output_not_in_mktemp_dir", script: `bash -c 'tmpdir=$(mktemp -d); openapi-typescript ../specs/openapi.v3.yaml --output /tmp/fixed-api.generated.ts'`, wantSub: "tmpdir"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := tc.raw
			if input == nil {
				input = mutateGenerateTypesCheckJSON(t, pkgBytes, tc.script, tc.remove)
			}
			err := verifyGenerateTypesCheckScript(input)
			if err == nil {
				t.Fatalf("expected verifyGenerateTypesCheckScript to fail for %s, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("error %q missing expected substring %q", err.Error(), tc.wantSub)
			}
		})
	}
}
