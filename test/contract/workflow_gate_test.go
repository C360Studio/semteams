package contract

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// repository-governance › "Merge check is its own required workflow" and "One unconditional repository CI
// workflow reports one aggregate" (ateam v0.3.4 layout, C360Studio/ateam#250 and #255). Each assertion
// below has a sensitivity row that mutates the real workflow text and expects the violation.

const (
	ateamPin = "v0.3.5"
	// The job guard in ci.yml: push events run, draft pull requests skip.
	ciDraftGuard = "github.event_name != 'pull_request' || !github.event.pull_request.draft"
	// merge-check.yml has only pull_request events, so its guard is upstream's shorter form.
	mergeCheckDraftGuard = "${{ !github.event.pull_request.draft }}"
)

// draftName is the name a job carries so that a skipped draft run is never a check-run with the
// required context's name (GitHub counts a skipped check as passing, ateam#250).
func draftName(name string) string {
	return "${{ github.event.pull_request.draft && '" + name + " (draft, skipped)' || '" + name + "' }}"
}

type gateWorkflow struct {
	On struct {
		PullRequest *struct {
			Types []string `yaml:"types"`
		} `yaml:"pull_request"`
		Push              any `yaml:"push"`
		PullRequestTarget any `yaml:"pull_request_target"`
	} `yaml:"on"`
	Concurrency struct {
		Group            string `yaml:"group"`
		CancelInProgress any    `yaml:"cancel-in-progress"`
	} `yaml:"concurrency"`
	Permissions map[string]string  `yaml:"permissions"`
	Jobs        map[string]gateJob `yaml:"jobs"`
}

type gateJob struct {
	Name        string            `yaml:"name"`
	If          string            `yaml:"if"`
	Needs       []string          `yaml:"needs"`
	Permissions map[string]string `yaml:"permissions"`
	Steps       []gateStep        `yaml:"steps"`
}

type gateStep struct {
	Uses string            `yaml:"uses"`
	Run  string            `yaml:"run"`
	With map[string]any    `yaml:"with"`
	Env  map[string]string `yaml:"env"`
}

func parseGateWorkflow(file string, data []byte) (gateWorkflow, []string) {
	var wf gateWorkflow
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return wf, []string{file + ": parse: " + err.Error()}
	}
	return wf, nil
}

func mergeCheckWorkflowViolations(data []byte) []string {
	const file = "merge-check.yml"
	wf, v := parseGateWorkflow(file, data)
	if v != nil {
		return v
	}
	bad := func(format string, args ...any) { v = append(v, file+": "+fmt.Sprintf(format, args...)) }

	if wf.On.PullRequest == nil {
		bad("no pull_request trigger")
	} else if want := []string{"opened", "synchronize", "reopened", "ready_for_review", "edited"}; !slices.Equal(wf.On.PullRequest.Types, want) {
		bad("pull_request types are %q, want %q (edited is how the reviewer's description edit reruns the gate)", wf.On.PullRequest.Types, want)
	}
	if wf.On.Push != nil || wf.On.PullRequestTarget != nil {
		bad("triggers on push or pull_request_target; want pull_request only")
	}
	if want := "merge-check-${{ github.event.pull_request.number }}"; wf.Concurrency.Group != want {
		bad("concurrency group is %q, want %q", wf.Concurrency.Group, want)
	}
	if wf.Concurrency.CancelInProgress != false {
		bad("concurrency cancel-in-progress is %v, want false (runs for one pull request queue)", wf.Concurrency.CancelInProgress)
	}
	if want := map[string]string{"contents": "read"}; !mapsEqual(wf.Permissions, want) {
		bad("workflow permissions are %v, want %v", wf.Permissions, want)
	}
	if len(wf.Jobs) != 1 {
		bad("has %d jobs, want only merge-check", len(wf.Jobs))
	}
	job, ok := wf.Jobs["merge-check"]
	if !ok {
		bad("no merge-check job")
		return v
	}
	if want := draftName("Merge check"); job.Name != want {
		bad("merge-check name is %q, want the draft rename %q", job.Name, want)
	}
	if job.If != mergeCheckDraftGuard {
		bad("merge-check if is %q, want the draft guard %q", job.If, mergeCheckDraftGuard)
	}
	if want := map[string]string{"contents": "read", "issues": "read", "pull-requests": "read"}; !mapsEqual(job.Permissions, want) {
		bad("merge-check permissions are %v, want read-only %v", job.Permissions, want)
	}
	if len(job.Steps) == 0 || !strings.HasPrefix(job.Steps[0].Uses, "actions/checkout@") {
		bad("merge-check does not start with a checkout")
	} else {
		with := job.Steps[0].With
		if ref := fmt.Sprint(with["ref"]); ref != "${{ github.event.pull_request.base.sha }}" {
			bad("checkout ref is %q, want the base commit", ref)
		}
		if with["persist-credentials"] != false {
			bad("checkout persist-credentials is %v, want false", with["persist-credentials"])
		}
	}
	install := `GOBIN="$RUNNER_TEMP" go install github.com/c360studio/ateam/cmd/ateam@` + ateamPin
	if !slices.ContainsFunc(job.Steps, func(s gateStep) bool { return strings.TrimSpace(s.Run) == install }) {
		bad("no step runs %q", install)
	}
	var gate *gateStep
	for i := range job.Steps {
		if strings.TrimSpace(job.Steps[i].Run) == `scripts/merge-check.sh "$PR_NUMBER"` {
			gate = &job.Steps[i]
		}
	}
	if gate == nil {
		bad("no step runs scripts/merge-check.sh \"$PR_NUMBER\"")
		return v
	}
	if got, want := gate.Env["ATEAM_REQUIRED_CHECKS"], "Required,Merge check"; got != want {
		bad("ATEAM_REQUIRED_CHECKS is %q, want %q", got, want)
	}
	if got, want := gate.Env["ATEAM"], "${{ runner.temp }}/ateam"; got != want {
		bad("ATEAM is %q, want %q", got, want)
	}
	return v
}

func ciWorkflowGateViolations(data []byte) []string {
	const file = "ci.yml"
	wf, v := parseGateWorkflow(file, data)
	if v != nil {
		return v
	}
	bad := func(format string, args ...any) { v = append(v, file+": "+fmt.Sprintf(format, args...)) }

	if wf.On.PullRequest == nil {
		bad("no pull_request trigger")
	} else if want := []string{"opened", "synchronize", "reopened", "ready_for_review"}; !slices.Equal(wf.On.PullRequest.Types, want) {
		bad("pull_request types are %q, want %q (an edited trigger reruns every lane on a description edit)", wf.On.PullRequest.Types, want)
	}
	lanes := map[string]string{"go": "Go", "ui": "UI", "governance": "Governance"}
	for id := range wf.Jobs {
		if id == "merge-check" {
			bad("merge-check job is still in ci.yml; it belongs to merge-check.yml")
		} else if _, ok := lanes[id]; !ok && id != "required" {
			bad("unexpected job %q", id)
		}
	}
	for id, display := range lanes {
		job, ok := wf.Jobs[id]
		if !ok {
			bad("no %s job", id)
			continue
		}
		if job.Name != draftName(display) {
			bad("%s name is %q, want the draft rename %q", id, job.Name, draftName(display))
		}
		if job.If != ciDraftGuard {
			bad("%s if is %q, want the draft guard %q", id, job.If, ciDraftGuard)
		}
	}
	req, ok := wf.Jobs["required"]
	if !ok {
		bad("no required job")
		return v
	}
	if req.Name != draftName("Required") {
		bad("required name is %q, want the draft rename %q", req.Name, draftName("Required"))
	}
	if want := "always() && (" + ciDraftGuard + ")"; req.If != want {
		bad("required if is %q, want %q", req.If, want)
	}
	if got, want := slices.Sorted(slices.Values(req.Needs)), []string{"go", "governance", "ui"}; !slices.Equal(got, want) {
		bad("required needs %q, want exactly %q (Required must not depend on Merge check)", req.Needs, want)
	}
	var run strings.Builder
	for _, s := range req.Steps {
		run.WriteString(s.Run)
	}
	// Fail closed: each lane must equal success, so failure, cancelled, skipped and a missing result all fail.
	for id := range lanes {
		if !strings.Contains(run.String(), `"${{ needs.`+id+`.result }}" != "success"`) {
			bad("required step does not fail unless needs.%s.result is success", id)
		}
	}
	if !strings.Contains(run.String(), "exit 1") || strings.Contains(run.String(), "exit 0") {
		bad("required step must exit 1 on a failed lane and never exit 0")
	}
	if strings.Contains(run.String(), "needs.merge-check") {
		bad("required step still reads needs.merge-check")
	}
	return v
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, x := range b {
		if a[k] != x {
			return false
		}
	}
	return true
}

func readWorkflow(t *testing.T, name string) string {
	t.Helper()
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("findRepoRoot: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(repoRoot, ".github", "workflows", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

type workflowMutation struct {
	name     string
	old, new string
	wants    []string
}

func runWorkflowMutations(t *testing.T, text string, check func([]byte) []string, rows []workflowMutation) {
	t.Helper()
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(text, tc.old) {
				t.Fatalf("mutation does not apply: %q not in the workflow", tc.old)
			}
			requireViolation(t, check([]byte(strings.Replace(text, tc.old, tc.new, 1))), tc.wants...)
		})
	}
}

func TestMergeCheckWorkflow(t *testing.T) {
	text := readWorkflow(t, "merge-check.yml")
	requireNoViolations(t, "merge-check.yml", mergeCheckWorkflowViolations([]byte(text)))

	runWorkflowMutations(t, text, mergeCheckWorkflowViolations, []workflowMutation{
		{"edited dropped", "ready_for_review, edited]", "ready_for_review]", []string{"pull_request types"}},
		{"push trigger added", "on:\n", "on:\n  push:\n    branches: [main]\n", []string{"push"}},
		{"concurrency cancels", "cancel-in-progress: false", "cancel-in-progress: true", []string{"cancel-in-progress"}},
		{"concurrency group not per pull request", "group: merge-check-${{ github.event.pull_request.number }}", "group: merge-check", []string{"concurrency group"}},
		{"draft guard dropped", "    if: " + mergeCheckDraftGuard + "\n", "", []string{"draft guard"}},
		{"rename dropped", "name: " + draftName("Merge check"), "name: Merge check", []string{"draft rename"}},
		{"head checked out", "ref: ${{ github.event.pull_request.base.sha }}", "ref: ${{ github.event.pull_request.head.sha }}", []string{"checkout ref"}},
		{"credentials persisted", "persist-credentials: false", "persist-credentials: true", []string{"persist-credentials"}},
		{"job permission writes", "pull-requests: read", "pull-requests: write", []string{"read-only"}},
		{"workflow permission writes", "permissions:\n  contents: read", "permissions:\n  contents: write", []string{"workflow permissions"}},
		{"ateam pin drifts", "ateam@" + ateamPin, "ateam@v0.3.3", []string{"go install"}},
		{"Merge check dropped from ATEAM_REQUIRED_CHECKS", "ATEAM_REQUIRED_CHECKS: Required,Merge check", "ATEAM_REQUIRED_CHECKS: Required", []string{"ATEAM_REQUIRED_CHECKS"}},
		{"Required dropped from ATEAM_REQUIRED_CHECKS", "ATEAM_REQUIRED_CHECKS: Required,Merge check", "ATEAM_REQUIRED_CHECKS: Merge check", []string{"ATEAM_REQUIRED_CHECKS"}},
	})
}

func TestCIWorkflowGate(t *testing.T) {
	text := readWorkflow(t, "ci.yml")
	requireNoViolations(t, "ci.yml", ciWorkflowGateViolations([]byte(text)))

	const needs = "needs: [go, ui, governance]"
	runWorkflowMutations(t, text, ciWorkflowGateViolations, []workflowMutation{
		{"required still needs merge-check", needs, "needs: [go, ui, governance, merge-check]", []string{"required needs"}},
		{"required lane dropped", needs, "needs: [go, ui]", []string{"required needs"}},
		{"merge-check job left in ci.yml", "\n  required:\n", "\n  merge-check:\n    name: Merge check\n    runs-on: ubuntu-latest\n    steps:\n      - run: scripts/merge-check.sh\n\n  required:\n", []string{"merge-check job is still in ci.yml"}},
		{"draft guard dropped on required", "if: always() && (" + ciDraftGuard + ")", "if: ${{ always() }}", []string{"required if"}},
		{"rename dropped on required", "name: " + draftName("Required"), "name: Required", []string{"required name", "draft rename"}},
		{"draft guard dropped on go", "name: " + draftName("Go") + "\n    if: " + ciDraftGuard + "\n", "name: " + draftName("Go") + "\n", []string{"go if"}},
		{"rename dropped on governance", "name: " + draftName("Governance"), "name: Governance", []string{"governance name"}},
		{"edited added to ci triggers", "ready_for_review]", "ready_for_review, edited]", []string{"pull_request types"}},
		{"go lane compared to failure only", `"${{ needs.go.result }}" != "success"`, `"${{ needs.go.result }}" == "failure"`, []string{"needs.go.result is success"}},
		{"required step exits 0", "            exit 1\n", "            exit 0\n", []string{"exit 1"}},
	})
}
