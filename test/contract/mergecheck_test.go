package contract

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// repository-governance / merge-gate:
// scripts/merge-check.sh delegates to `ateam review gate` and runs from a throwaway git
// repository with SemTeams's .agents/review-policy.json and a fake gh first on PATH.
// The fake serves canned answers from files and refuses any call whose arguments differ
// from the reads `ateam review gate` makes.

const fakeGH = `#!/bin/sh
d="$FAKE_GH_DIR"
echo "$*" >> "$d/argv.log"
args=" $* "
need() { case "$args" in *" $1 "*) ;; *) echo "fake gh: '$*' lacks '$1'" >&2; exit 3 ;; esac; }
case "$1 $2" in
  "label list")
    key=labels; need "--json name"
    # gh 2.97 prints nothing at all, not [], when --search matches no label (measured 2026-10-01).
    case "$args" in *" --search "*) exit 0 ;; esac ;;
  "issue list") key=issues; need "--label class:flake"; need "--state open"; need "--limit 100" ;;
  "pr view")
    key=pr
    [ "$3" = "$FAKE_GH_PR" ] || { echo "fake gh: pr view of '$3', want '$FAKE_GH_PR'" >&2; exit 3; }
    need "--json closingIssuesReferences" ;;
  *)
    case "$*" in
      "api repos/{owner}/{repo}/rules/branches/main") key=rules ;;
      "api repos/{owner}/{repo}/rulesets/"*) key=ruleset ;;
      "api repos/{owner}/{repo}/pulls/$FAKE_GH_PR") key=pull ;;
      # Without --paginate gh returns the first page only.
      "api --paginate repos/{owner}/{repo}/pulls/$FAKE_GH_PR/files?per_page=100") key=files ;;
      "api --paginate repos/{owner}/{repo}/pulls/$FAKE_GH_PR/commits?per_page=100") key=commits ;;
      "api --paginate repos/{owner}/{repo}/issues/$FAKE_GH_PR/comments?per_page=100") key=comments ;;
      "api --paginate repos/{owner}/{repo}/commits/"*"?per_page=100")
        sha="${3#repos/*/commits/}"
        sha="${sha%?per_page=100}"
        if [ -e "$d/commit_$sha.fail" ] || [ -e "$d/commit_$sha.json" ]; then
          key="commit_$sha"
        else
          key=commit
        fi ;;
      *) echo "fake gh: unexpected call: $*" >&2; exit 3 ;;
    esac ;;
esac
if [ -e "$d/$key.fail" ]; then echo "HTTP 502: Bad Gateway (fake)" >&2; exit 1; fi
cat "$d/$key.json"
`

const (
	flakeRepo  = "https://github.com/C360Studio/semteams/issues/"
	otherRepo  = "https://github.com/C360Studio/semstreams/issues/"
	mergeBase  = "03121ae8066ec58a2ecc5a95c38c4447389eb0a3"
	mergeHead  = "99d19a4cfa55e59c62dd49f79bd420e653675767"
	mergeNext  = "7413d0c05c25db0e9143f7a466a64954855aa979"
	mergeMain  = "dc854c651fbf82695f4d552badc36340e2f7c472"
	mergeAfter = "0209eb7f6ebaf44e2dd87c401d61a7a48763799b"
)

func issueJSON(n int) string {
	return fmt.Sprintf(`{"number":%d,"url":"%s%d","title":"flaky test %d"}`, n, flakeRepo, n, n)
}

func refJSON(url string, n int) string {
	return fmt.Sprintf(`{"number":%d,"url":"%s%d","repository":{"name":"x","owner":{"login":"C360Studio"}}}`, n, url, n)
}

func jsonList(items ...string) string { return "[" + strings.Join(items, ",") + "]" }

// ghState is what the fake gh answers. Every field holds literal JSON so a case can plant
// an answer of the wrong shape.
type ghState struct {
	labels, issues, pr, rules, ruleset string
	pull, files, commits, comments     string
	commitDetails                      map[string]string
	fail                               []string
	noPolicy                           bool
}

const (
	requiredRule = `{"type":"required_status_checks","ruleset_id":24272345,"ruleset_source":"C360Studio/semteams",` +
		`"parameters":{"strict_required_status_checks_policy":true,"do_not_enforce_on_create":false,` +
		`"required_status_checks":[{"context":"Required","integration_id":15368}]}}`
	otherRules = `{"type":"deletion","ruleset_id":24272345},{"type":"non_fast_forward","ruleset_id":24272345}`
)

func commitEntryJSON(sha, msg string, parents ...string) string {
	ps := make([]string, len(parents))
	for i, p := range parents {
		ps[i] = fmt.Sprintf(`{"sha":%q}`, p)
	}
	b, _ := json.Marshal(msg)
	return fmt.Sprintf(`{"sha":%q,"commit":{"message":%s},"parents":%s}`, sha, b, jsonList(ps...))
}

func commitDetailJSON(sha, msg string, parents []string, files ...string) string {
	ps := make([]string, len(parents))
	for i, p := range parents {
		ps[i] = fmt.Sprintf(`{"sha":%q}`, p)
	}
	b, _ := json.Marshal(msg)
	return fmt.Sprintf(`{"sha":%q,"commit":{"message":%s},"parents":%s,"files":%s}`, sha, b, jsonList(ps...), filesJSON(files...))
}

func commentJSON(bodies ...string) string {
	entries := make([]string, len(bodies))
	for i, body := range bodies {
		b, _ := json.Marshal(body)
		entries[i] = fmt.Sprintf(`{"body":%s}`, b)
	}
	return jsonList(entries...)
}

func healthy() ghState {
	return ghState{
		labels:   jsonList(`{"name":"bug"}`, `{"name":"class:flake"}`),
		issues:   jsonList(),
		pr:       `{"closingIssuesReferences":[]}`,
		rules:    jsonList(otherRules, requiredRule),
		ruleset:  `{"id":24272345,"name":"main","enforcement":"active"}`,
		pull:     pullJSONWithHead(false, "User", "", 1, 1, mergeHead),
		files:    filesJSON("docs/a.md"),
		commits:  jsonList(commitEntryJSON(mergeHead, "docs: update a", mergeBase)),
		comments: jsonList(),
		commitDetails: map[string]string{
			mergeHead: commitDetailJSON(mergeHead, "docs: update a", []string{mergeBase}, "docs/a.md"),
		},
	}
}

func pullJSON(draft bool, userType, body string, changedFiles int) string {
	return pullJSONWithHead(draft, userType, body, changedFiles, 1, mergeHead)
}

func pullJSONWithHead(draft bool, userType, body string, changedFiles, commits int, headSHA string) string {
	b, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf(`{"number":12,"state":"open","draft":%t,"user":{"login":"x","type":%q},"body":%s,"changed_files":%d,"commits":%d,"head":{"sha":%q},"base":{"ref":"main"}}`,
		draft, userType, b, changedFiles, commits, headSHA)
}

func filesJSON(names ...string) string {
	entries := make([]string, len(names))
	for i, n := range names {
		entries[i] = fmt.Sprintf(`{"filename":%q,"status":"modified","additions":1,"deletions":0}`, n)
	}
	return jsonList(entries...)
}

type mergeRun struct {
	out    string
	status int
	argv   string
}

func findAteamBinary(t *testing.T) string {
	t.Helper()
	if p, err := exec.LookPath("ateam"); err == nil {
		return p
	}
	if home := os.Getenv("HOME"); home != "" {
		cand := filepath.Join(home, ".local", "bin", "ateam")
		if info, err := os.Stat(cand); err == nil && !info.IsDir() {
			return cand
		}
	}
	t.Fatal("ateam binary not found on PATH or $HOME/.local/bin/ateam")
	return ""
}

func writeGHFixtures(t *testing.T, st ghState) string {
	t.Helper()
	dir := t.TempDir()
	for key, body := range map[string]string{
		"labels":   st.labels,
		"issues":   st.issues,
		"pr":       st.pr,
		"rules":    st.rules,
		"ruleset":  st.ruleset,
		"pull":     st.pull,
		"files":    st.files,
		"commits":  st.commits,
		"comments": st.comments,
		"commit":   commitDetailJSON(mergeHead, "feat: change", []string{mergeBase}, "scripts/x.sh"),
	} {
		if err := os.WriteFile(filepath.Join(dir, key+".json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for sha, detail := range st.commitDetails {
		if err := os.WriteFile(filepath.Join(dir, "commit_"+sha+".json"), []byte(detail), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, key := range st.fail {
		if err := os.WriteFile(filepath.Join(dir, key+".fail"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func runMergeCheck(t *testing.T, st ghState, pr string, actionsEnv ...string) mergeRun {
	t.Helper()
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("findRepoRoot: %v", err)
	}
	root := copyScript(t, "merge-check.sh")
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "-C", root, "remote", "add", "origin", "https://github.com/C360Studio/semteams.git").CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %v\n%s", err, out)
	}
	if !st.noPolicy {
		policyBytes, err := os.ReadFile(filepath.Join(repoRoot, ".agents", "review-policy.json"))
		if err != nil {
			t.Fatalf("read .agents/review-policy.json: %v", err)
		}
		if err := os.MkdirAll(filepath.Join(root, ".agents"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, ".agents", "review-policy.json"), policyBytes, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	dir := writeGHFixtures(t, st)

	ateamBin := findAteamBinary(t)
	tools := map[string]string{"gh": fakeGH}
	extra := []string{"FAKE_GH_DIR=" + dir, "FAKE_GH_PR=" + pr}
	var env []string
	for _, kv := range fakeBin(t, tools, extra...) {
		switch {
		case strings.HasPrefix(kv, "GITHUB_ACTIONS="),
			strings.HasPrefix(kv, "GITHUB_EVENT_NAME="),
			strings.HasPrefix(kv, "GITHUB_SERVER_URL="),
			strings.HasPrefix(kv, "GITHUB_REPOSITORY="),
			strings.HasPrefix(kv, "ATEAM_FORGE="),
			strings.HasPrefix(kv, "ATEAM="):
			continue
		case strings.HasPrefix(kv, "PATH="):
			kv = kv + ":" + filepath.Dir(ateamBin)
		}
		env = append(env, kv)
	}
	args := []string{filepath.Join(root, "scripts", "merge-check.sh")}
	if pr != "" {
		args = append(args, pr)
	}
	cmd := exec.Command("bash", args...)
	cmd.Dir = root
	cmd.Env = append(env, actionsEnv...)
	out, err := cmd.CombinedOutput()
	r := mergeRun{out: string(out)}
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		r.status = exit.ExitCode()
	case err != nil:
		t.Fatalf("run merge-check.sh: %v", err)
	}
	argv, _ := os.ReadFile(filepath.Join(dir, "argv.log"))
	r.argv = string(argv)
	return r
}

func (r mergeRun) requirePass(t *testing.T, fragments ...string) {
	t.Helper()
	if r.status != 0 {
		t.Fatalf("exit %d, want 0\n%s", r.status, r.out)
	}
	r.requireOutput(t, fragments...)
}

func (r mergeRun) requireFail(t *testing.T, fragments ...string) {
	t.Helper()
	if r.status == 0 {
		t.Fatalf("exit 0, want non-zero\n%s", r.out)
	}
	r.requireOutput(t, fragments...)
}

func (r mergeRun) requireOutput(t *testing.T, fragments ...string) {
	t.Helper()
	for _, f := range fragments {
		if !strings.Contains(r.out, f) {
			t.Errorf("output does not contain %q:\n%s", f, r.out)
		}
	}
}

func (r mergeRun) requireReadPullAndFiles(t *testing.T) {
	t.Helper()
	for _, call := range []string{
		"api repos/{owner}/{repo}/pulls/12\n",
		"api --paginate repos/{owner}/{repo}/pulls/12/files?per_page=100\n",
	} {
		if !strings.Contains(r.argv, call) {
			t.Errorf("the run did not call gh %q:\n%s", strings.TrimSpace(call), r.argv)
		}
	}
}

func TestMergeCheckKnownFlake(t *testing.T) {
	open40 := healthy()
	open40.issues = jsonList(issueJSON(40))
	open40And52 := healthy()
	open40And52.issues = jsonList(issueJSON(40), issueJSON(52))
	closes := func(st ghState, refs ...string) ghState {
		st.pr = `{"closingIssuesReferences":` + jsonList(refs...) + `}`
		return st
	}

	t.Run("open flake and an unrelated pull request", func(t *testing.T) {
		runMergeCheck(t, closes(open40, refJSON(flakeRepo, 7)), "12").requireFail(t, "#40")
	})
	t.Run("pull request that closes the only open flake", func(t *testing.T) {
		r := runMergeCheck(t, closes(open40, refJSON(flakeRepo, 40)), "12")
		r.requirePass(t, "closing #40 exempts pull request #12", "::warning::")
		if !strings.Contains(r.out[strings.Index(r.out, "::warning::"):], "#40") {
			t.Errorf("the ::warning:: line does not name #40:\n%s", r.out)
		}
	})
	t.Run("two open flakes and a pull request that closes one", func(t *testing.T) {
		r := runMergeCheck(t, closes(open40And52, refJSON(flakeRepo, 40)), "12")
		r.requireFail(t, "#52")
		if strings.Contains(r.out, "\n  #40 ") {
			t.Errorf("#40 is closed by the pull request but reported as not closed:\n%s", r.out)
		}
	})
	t.Run("two open flakes and a pull request that closes both", func(t *testing.T) {
		runMergeCheck(t, closes(open40And52, refJSON(flakeRepo, 40), refJSON(flakeRepo, 52)), "12").
			requirePass(t, "closing #40 exempts", "closing #52 exempts")
	})
	t.Run("same number in another repository", func(t *testing.T) {
		runMergeCheck(t, closes(open40, refJSON(otherRepo, 40)), "12").requireFail(t, "#40")
	})
	t.Run("pull request on another base", func(t *testing.T) {
		st := open40
		st.pr = `{"baseRefName":"claude/stacked","closingIssuesReferences":[]}`
		runMergeCheck(t, st, "12").requireFail(t, "#40")
	})
	t.Run("no open flake", func(t *testing.T) {
		r := runMergeCheck(t, healthy(), "12")
		r.requirePass(t, "no open class:flake issue")
		r.requireReadPullAndFiles(t)
	})
	t.Run("label missing", func(t *testing.T) {
		st := open40
		st.labels = jsonList(`{"name":"bug"}`)
		runMergeCheck(t, st, "12").requireFail(t, "label class:flake is missing")
	})
	t.Run("a read fails", func(t *testing.T) {
		for _, tc := range []struct{ key, read string }{
			{"issues", "open class:flake issues"}, {"pr", "pull request #12"}, {"labels", "labels"},
		} {
			for _, how := range []string{"fails", "not a list", "null"} {
				st := open40
				switch how {
				case "fails":
					st.fail = []string{tc.key}
				case "null":
					switch tc.key {
					case "issues":
						st.issues = "null"
					case "labels":
						st.labels = "null"
					case "pr":
						st.pr = `{"closingIssuesReferences":null}`
					}
				default:
					bad := `{"message":"Not Found","status":"404"}`
					switch tc.key {
					case "issues":
						st.issues = bad
					case "labels":
						st.labels = bad
					case "pr":
						st.pr = `{"closingIssuesReferences":` + bad + `}`
					}
				}
				r := runMergeCheck(t, st, "12")
				if r.status != 2 {
					t.Errorf("%s %s: exit %d, want 2\n%s", tc.read, how, r.status, r.out)
				}
				r.requireFail(t, "unavailable", tc.read)
				if strings.Contains(r.out, "no open class:flake issue") {
					t.Errorf("%s %s: reported no known flake:\n%s", tc.read, how, r.out)
				}
			}
		}
	})
	t.Run("list may be cut short", func(t *testing.T) {
		st := healthy()
		var hundred []string
		for n := 1000; n < 1100; n++ {
			hundred = append(hundred, issueJSON(n))
		}
		st.issues = jsonList(hundred...)
		runMergeCheck(t, st, "12").requireFail(t, "may be incomplete")
	})
	t.Run("push run", func(t *testing.T) {
		r := runMergeCheck(t, open40, "", "GITHUB_ACTIONS=true", "GITHUB_EVENT_NAME=push")
		r.requirePass(t, "push run", "known-flake check does not apply")
		if strings.Contains(r.argv, "issue list") || strings.Contains(r.argv, "pr view") || strings.Contains(r.argv, "/pulls/") {
			t.Errorf("a push run read issues, a pull request or its files:\n%s", r.argv)
		}
		if !strings.Contains(r.argv, "rules/branches/main") {
			t.Errorf("a push run did not read the rules in force on main:\n%s", r.argv)
		}
	})
	t.Run("push event named outside Actions", func(t *testing.T) {
		r := runMergeCheck(t, open40, "12", "GITHUB_EVENT_NAME=push")
		r.requireFail(t, "pull-request run for #12 (outside GitHub Actions; GITHUB_EVENT_NAME not read)", "#40")
	})
	t.Run("pull-request run without a number", func(t *testing.T) {
		runMergeCheck(t, open40, "", "GITHUB_ACTIONS=true", "GITHUB_EVENT_NAME=pull_request").
			requireFail(t, "a pull request number is required")
	})
	t.Run("local run without a number", func(t *testing.T) {
		runMergeCheck(t, healthy(), "").requireFail(t, "a pull request number is required")
	})
	t.Run("event with no rule", func(t *testing.T) {
		runMergeCheck(t, healthy(), "12", "GITHUB_ACTIONS=true", "GITHUB_EVENT_NAME=merge_group").
			requireFail(t, "merge_group")
	})
	t.Run("no event inside Actions", func(t *testing.T) {
		runMergeCheck(t, healthy(), "12", "GITHUB_ACTIONS=true").requireFail(t, "the event is missing")
	})
}

func TestMergeCheckUpToDateRule(t *testing.T) {
	t.Run("setting turned off in GitHub", func(t *testing.T) {
		st := healthy()
		st.rules = jsonList(otherRules, strings.Replace(requiredRule, `"strict_required_status_checks_policy":true`, `"strict_required_status_checks_policy":false`, 1))
		runMergeCheck(t, st, "12").requireFail(t, "strict_required_status_checks_policy=false", "ruleset_id=24272345")
	})
	t.Run("no rule requires the Required check", func(t *testing.T) {
		for _, legacy := range []string{"CI Status Check", "Verify"} {
			st := healthy()
			st.rules = jsonList(otherRules, strings.Replace(requiredRule, `"context":"Required"`, `"context":"`+legacy+`"`, 1))
			runMergeCheck(t, st, "12").requireFail(t, "no rule in force on main requires Required")
		}
	})
	t.Run("ruleset not active", func(t *testing.T) {
		for _, enforcement := range []string{"evaluate", "disabled"} {
			st := healthy()
			st.ruleset = `{"id":24272345,"name":"main","enforcement":"` + enforcement + `"}`
			r := runMergeCheck(t, st, "12")
			r.requireFail(t, "enforcement="+enforcement)
			if !strings.Contains(r.argv, "rulesets/24272345") {
				t.Errorf("did not read the ruleset the rule names:\n%s", r.argv)
			}
		}
	})
	t.Run("field the gate does not read", func(t *testing.T) {
		st := healthy()
		st.rules = jsonList(otherRules, strings.Replace(requiredRule, `"parameters":{`, `"surprise":{"new":1},"parameters":{"another_new_field":[1,2],`, 1))
		st.ruleset = `{"id":24272345,"enforcement":"active","bypass_actors":[{"actor_id":1}],"new_field":"x"}`
		r := runMergeCheck(t, st, "12")
		r.requirePass(t, "enforcement=active")
		r.requireReadPullAndFiles(t)
	})
}

func review(draft bool, author, body string, files ...string) ghState {
	st := healthy()
	st.pull = pullJSON(draft, author, body, len(files))
	st.files = filesJSON(files...)
	return st
}

func codeWithRecord(author, body, reviewer string) ghState {
	st := review(false, author, body, "docs/a.md", "scripts/x.sh")
	st.commits = jsonList(commitEntryJSON(mergeHead, "feat: add script", mergeBase))
	st.comments = commentJSON(fmt.Sprintf("reviewed-by: %s\nReviewed at %s\nVerdict: APPROVE", reviewer, mergeHead[:7]))
	return st
}

const codeFilePrefix = "merge-check: code file: "

func (r mergeRun) requireCodeNames(t *testing.T, want ...string) {
	t.Helper()
	var got []string
	for _, line := range strings.Split(r.out, "\n") {
		if name, ok := strings.CutPrefix(line, codeFilePrefix); ok {
			got = append(got, name)
		}
	}
	slices.Sort(got)
	want = slices.Clone(want)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("code names printed %q, want %q\n%s", got, want, r.out)
	}
}

func (r mergeRun) requireNotInOutput(t *testing.T, fragments ...string) {
	t.Helper()
	for _, f := range fragments {
		if strings.Contains(r.out, f) {
			t.Errorf("output contains %q:\n%s", f, r.out)
		}
	}
}

func TestMergeCheckReview(t *testing.T) {
	const implClaude = "Summary.\n\nimplemented-by: claude (opus, medium)\n"

	t.Run("documents only", func(t *testing.T) {
		r := runMergeCheck(t, review(false, "User", "", "docs/a.md", "openspec/config.yaml"), "12")
		r.requirePass(t, "documents only")
		r.requireCodeNames(t)
	})
	t.Run("one code file among documents", func(t *testing.T) {
		r := runMergeCheck(t, review(false, "User", implClaude, "docs/a.md", "scripts/x.sh"), "12")
		r.requireFail(t, "is a code pull request", "no line starts reviewed-by:")
		r.requireCodeNames(t, "scripts/x.sh")
	})
	t.Run("names near the rule", func(t *testing.T) {
		docs := []string{"AGENTS.md", ".claude/skills/preflight/SKILL.md", "x/.claude/agents/a.md",
			"x/.agents/contracts/a.md", "openspec/changes/x/.openspec.yaml", "openspec/changes/x/meta.json"}
		first := runMergeCheck(t, review(false, "User", "", docs...), "12")
		first.requirePass(t, "documents only")
		first.requireCodeNames(t)

		code := []string{"README.MD", "docs/a.md.txt", "openspecs/a.yaml", "docs/openspec/a.yaml", "openspec/x.sh",
			"Taskfile.yml", "ui/src/routes/+page.svelte", ".claude/agents/go-reviewer.md",
			".agents/contracts/go-reviewer.md", ".codex/agents/go-reviewer.toml"}
		st := review(false, "User", "", append(slices.Clone(docs), code...)...)
		entries := strings.TrimSuffix(st.files, "]") + `,{"filename":"scripts/x.sh","status":"removed","additions":0,"deletions":9}]`
		st.files = entries
		st.pull = pullJSON(false, "User", "", len(docs)+len(code)+1)
		second := runMergeCheck(t, st, "12")
		second.requireFail(t, "is a code pull request")
		second.requireNotInOutput(t, "documents only")
		wantCode := append(slices.Clone(code), "scripts/x.sh")
		slices.Sort(wantCode)
		if len(wantCode) > 10 {
			wantCode = wantCode[:10]
		}
		second.requireCodeNames(t, wantCode...)
	})
	t.Run("renamed file", func(t *testing.T) {
		st := review(false, "User", "")
		st.pull = pullJSON(false, "User", "", 1)
		st.files = `[{"filename":"docs/x.md","previous_filename":"scripts/x.sh","status":"renamed","additions":0,"deletions":0}]`
		r := runMergeCheck(t, st, "12")
		r.requireFail(t, "is a code pull request")
		r.requireCodeNames(t, "scripts/x.sh")
	})
	t.Run("code file on the second page", func(t *testing.T) {
		var md []string
		for n := range 100 {
			md = append(md, fmt.Sprintf("docs/n%03d.md", n))
		}
		for _, pages := range []struct{ how, files string }{
			{"joined", filesJSON(append(slices.Clone(md), "go.mod")...)},
			{"one after the other", filesJSON(md...) + filesJSON("go.mod")},
		} {
			st := review(false, "User", "")
			st.pull = pullJSON(false, "User", "", 101)
			st.files = pages.files
			r := runMergeCheck(t, st, "12")
			if r.status == 0 {
				t.Errorf("%s: exit 0, want non-zero\n%s", pages.how, r.out)
			}
			r.requireOutput(t, "is a code pull request")
			r.requireCodeNames(t, "go.mod")
		}
	})
	t.Run("file list incomplete", func(t *testing.T) {
		var md []string
		for n := range 100 {
			md = append(md, fmt.Sprintf("docs/n%03d.md", n))
		}
		st := review(false, "User", "", md...)
		st.pull = pullJSON(false, "User", "", 101)
		r := runMergeCheck(t, st, "12")
		r.requireFail(t, "unavailable", "incomplete", "100 of 101 files")
		r.requireNotInOutput(t, "documents only")
	})
	t.Run("no changed file", func(t *testing.T) {
		r := runMergeCheck(t, review(false, "User", ""), "12")
		r.requirePass(t, "documents only")
		r.requireCodeNames(t)
	})

	const (
		noRun      = "editing the description starts no run"
		mustName   = "the implemented-by: line must name claude, codex, or gemini"
		exactlyOne = "the reviewed-by: line must name exactly one of claude, codex, and gemini"
	)
	t.Run("cross-agent frontier peers pass with review record", func(t *testing.T) {
		for _, tc := range []struct{ body, reviewer string }{
			{implClaude + "reviewed-by: codex (gpt-6-astra, high)\n", "codex"},
			{"Summary.\n\nimplemented-by: gemini (gemini-4-argon, high)\nreviewed-by: claude (opus, high)\n", "claude"},
			{"Summary.\n\nimplemented-by: claude (opus, high)\nreviewed-by: gemini (gemini-4-argon, high)\n", "gemini"},
			{"Summary.\n\nimplemented-by: codex (gpt-6-sol, medium)\nreviewed-by: gemini (pro, high)\n", "gemini"},
		} {
			r := runMergeCheck(t, codeWithRecord("User", tc.body, tc.reviewer), "12")
			r.requirePass(t, "strength: pass", "reviewing agent: "+tc.reviewer+" (record at "+mergeHead+")")
			r.requireNotInOutput(t, noRun)
		}
	})
	t.Run("weaker reviewer or missing effort fails policy check", func(t *testing.T) {
		for _, tc := range []struct{ name, body, reviewer, says string }{
			{"workhorse reviews frontier", "implemented-by: claude (opus, medium)\nreviewed-by: gemini (gemini-3.8-flash, high)\n", "gemini", "weaker than implementer"},
			{"lower effort band at same tier", "implemented-by: claude (opus, high)\nreviewed-by: codex (gpt-6-astra, medium)\n", "codex", "weaker than implementer"},
			{"missing effort", "implemented-by: claude (opus)\nreviewed-by: codex (gpt-6-astra, high)\n", "codex", "names no effort"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				runMergeCheck(t, codeWithRecord("User", tc.body, tc.reviewer), "12").requireFail(t, tc.says, noRun)
			})
		}
	})
	t.Run("missing or stale review record comment fails", func(t *testing.T) {
		body := implClaude + "reviewed-by: codex (gpt-6-astra, high)\n"
		t.Run("no review record comment", func(t *testing.T) {
			st := codeWithRecord("User", body, "codex")
			st.comments = jsonList()
			runMergeCheck(t, st, "12").requireFail(t, "no review record comment names reviewed-by: codex")
		})
		t.Run("later code commit invalidates record", func(t *testing.T) {
			st := codeWithRecord("User", body, "codex")
			st.pull = pullJSONWithHead(false, "User", body, 2, 2, mergeNext)
			st.commits = jsonList(
				commitEntryJSON(mergeHead, "feat: add script", mergeBase),
				commitEntryJSON(mergeNext, "fix: edit script", mergeHead),
			)
			st.commitDetails[mergeNext] = commitDetailJSON(mergeNext, "fix: edit script", []string{mergeHead}, "scripts/x.sh")
			runMergeCheck(t, st, "12").requireFail(t, "older than content commit "+mergeNext)
		})
		t.Run("later docs-only archive commit carries record over", func(t *testing.T) {
			st := codeWithRecord("User", body, "codex")
			st.pull = pullJSONWithHead(false, "User", body, 2, 2, mergeNext)
			st.commits = jsonList(
				commitEntryJSON(mergeHead, "feat: add script", mergeBase),
				commitEntryJSON(mergeNext, "chore: archive openspec change", mergeHead),
			)
			st.commitDetails[mergeNext] = commitDetailJSON(mergeNext, "chore: archive openspec change", []string{mergeHead}, "openspec/changes/archive/2026-10-26-x/tasks.md")
			runMergeCheck(t, st, "12").requirePass(t, "reviewing agent: codex (record at "+mergeHead+")")
		})
		t.Run("disjoint merge of origin/main carries record over", func(t *testing.T) {
			st := codeWithRecord("User", body, "codex")
			st.pull = pullJSONWithHead(false, "User", body, 2, 2, mergeAfter)
			st.commits = jsonList(
				commitEntryJSON(mergeHead, "feat: add script", mergeBase),
				commitEntryJSON(mergeAfter, "Merge remote-tracking branch 'origin/main'", mergeHead, mergeMain),
			)
			st.commitDetails[mergeAfter] = commitDetailJSON(mergeAfter, "Merge remote-tracking branch 'origin/main'", []string{mergeHead, mergeMain}, "README.md")
			runMergeCheck(t, st, "12").requirePass(t, "reviewing agent: codex (record at "+mergeHead+")")
		})
	})
	t.Run("no reviewed-by line", func(t *testing.T) {
		runMergeCheck(t, codeWithRecord("User", implClaude, "codex"), "12").
			requireFail(t, "no line starts reviewed-by:", "the line to write: reviewed-by: codex", noRun)
	})
	t.Run("implementer line missing or naming no agent", func(t *testing.T) {
		for _, body := range []string{
			"reviewed-by: codex (gpt-6-astra, high)\n",
			"implemented-by: opus (go-developer); fable (orchestrating session)\nreviewed-by: codex (gpt-6-astra, high)\n",
		} {
			t.Run(strings.SplitN(body, "\n", 2)[0], func(t *testing.T) {
				runMergeCheck(t, codeWithRecord("User", body, "codex"), "12").requireFail(t, mustName, noRun)
			})
		}
	})
	t.Run("same agent implements and reviews", func(t *testing.T) {
		for _, tc := range []struct{ body, reviewer, says string }{
			{"implemented-by: claude (opus, medium)\nreviewed-by: Claude (opus, high)\n", "claude", "another agent (codex or gemini) reviews"},
			{"implemented-by: gemini (gemini-4-argon, medium)\nreviewed-by: Gemini (gemini-4-argon, high)\n", "gemini", "another agent (claude or codex) reviews"},
		} {
			runMergeCheck(t, codeWithRecord("User", tc.body, tc.reviewer), "12").requireFail(t, tc.says)
		}
	})
	t.Run("reviewer line names no agent, both, or a longer word", func(t *testing.T) {
		for _, reviewer := range []string{"gpt-6-astra", "codex, then claude", "codex2", "gemini2"} {
			t.Run(reviewer, func(t *testing.T) {
				r := runMergeCheck(t, codeWithRecord("User", "implemented-by: claude (opus, medium)\nreviewed-by: "+reviewer+" (opus, high)\n", "codex"), "12")
				r.requireFail(t, exactlyOne)
				r.requireNotInOutput(t, "reviewing agent:")
			})
		}
	})
	t.Run("both agents implemented", func(t *testing.T) {
		runMergeCheck(t, codeWithRecord("User", "implemented-by: codex (gpt-6-sol, medium); claude (review fixes)\nreviewed-by: claude (opus, high)\n", "claude"), "12").
			requirePass(t, "reviewing agent: claude")
	})
	t.Run("pull request by a bot", func(t *testing.T) {
		for _, body := range []string{
			"Bumps x from 1 to 2.\n\nreviewed-by: codex (gpt-6-astra, high)\n",
			"Bumps x from 1 to 2.\n\nimplemented-by: codex (gpt-6-sol, medium)\nreviewed-by: codex (gpt-6-astra, high)\n",
		} {
			t.Run(strings.Fields(strings.SplitN(body, "\n\n", 2)[1])[0], func(t *testing.T) {
				runMergeCheck(t, codeWithRecord("Bot", body, "codex"), "12").requirePass(t, "reviewing agent: codex")
			})
		}
	})
	t.Run("line not written once at the start of a line", func(t *testing.T) {
		for _, tc := range []struct{ body, says string }{
			{implClaude + "reviewed-by: codex (gpt-6-astra, high)\nreviewed-by: codex (gpt-6-astra, high)\n", "2 lines start reviewed-by:"},
			{implClaude + "- reviewed-by: codex (gpt-6-astra, high)\n", "no line starts reviewed-by:"},
			{implClaude + "implemented-by: claude (fable, medium)\nreviewed-by: codex (gpt-6-astra, high)\n", "2 lines start implemented-by:"},
		} {
			t.Run(tc.says, func(t *testing.T) {
				runMergeCheck(t, codeWithRecord("User", tc.body, "codex"), "12").requireFail(t, tc.says)
			})
		}
	})
	t.Run("carriage returns", func(t *testing.T) {
		body := strings.ReplaceAll("Summary.\n\nimplemented-by: claude (opus, medium)\nreviewed-by: codex (gpt-6-astra, high)\n", "\n", "\r\n")
		runMergeCheck(t, codeWithRecord("User", body, "codex"), "12").requirePass(t, "reviewing agent: codex")
	})
	t.Run("draft", func(t *testing.T) {
		runMergeCheck(t, review(true, "User", implClaude, "docs/a.md", "scripts/x.sh"), "12").
			requirePass(t, "no line starts reviewed-by:", "does not fail a draft")
	})
	t.Run("a read for the review check fails", func(t *testing.T) {
		const notFound = `{"message":"Not Found","status":"404"}`
		for _, tc := range []struct {
			name, read string
			edit       func(*ghState)
		}{
			{"pull request read fails", "pull request #12 for the review check",
				func(st *ghState) { st.fail = []string{"pull"} }},
			{"pull request read returns an error object", "pull request #12 for the review check",
				func(st *ghState) { st.pull = notFound }},
			{"draft whose file count is not a number", "pull request #12 for the review check",
				func(st *ghState) {
					st.pull = strings.Replace(pullJSON(true, "User", "", 1), `"changed_files":1`, `"changed_files":"1"`, 1)
				}},
			{"files read fails", "the files of pull request #12",
				func(st *ghState) { st.fail = []string{"files"} }},
			{"files read fails on a draft", "the files of pull request #12",
				func(st *ghState) { st.pull = pullJSON(true, "User", "", 1); st.fail = []string{"files"} }},
			{"files read returns an error object on a draft", "the files of pull request #12",
				func(st *ghState) { st.pull = pullJSON(true, "User", "", 1); st.files = notFound }},
			{"second page of files is not a list of files", "the files of pull request #12",
				func(st *ghState) {
					st.pull = pullJSON(false, "User", "", 2)
					st.files = filesJSON("docs/a.md") + jsonList(notFound)
				}},
			{"commits read fails on code PR", "the commits of pull request #12",
				func(st *ghState) {
					*st = codeWithRecord("User", implClaude+"reviewed-by: codex (gpt-6-astra, high)\n", "codex")
					st.fail = []string{"commits"}
				}},
			{"comments read fails on code PR", "the comments of pull request #12",
				func(st *ghState) {
					*st = codeWithRecord("User", implClaude+"reviewed-by: codex (gpt-6-astra, high)\n", "codex")
					st.fail = []string{"comments"}
				}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				st := healthy()
				tc.edit(&st)
				r := runMergeCheck(t, st, "12")
				if r.status != 2 {
					t.Errorf("exit %d, want 2\n%s", r.status, r.out)
				}
				r.requireFail(t, "unavailable", tc.read)
				r.requireNotInOutput(t, "documents only", "merge-check: ok")
			})
		}
	})
	t.Run("a name or count that is never documents only", func(t *testing.T) {
		for _, tc := range []struct{ name, pull, files string }{
			{"empty file name", pullJSON(false, "User", "", 1), `[{"filename":"","status":"added"}]`},
			{"file name that is a line feed", pullJSON(false, "User", "", 1), `[{"filename":"\n","status":"added"}]`},
			{"file count that is not an integer", strings.Replace(pullJSON(false, "User", "", 1), `"changed_files":1`, `"changed_files":1.5`, 1),
				filesJSON("docs/a.md")},
		} {
			t.Run(tc.name, func(t *testing.T) {
				st := healthy()
				st.pull, st.files = tc.pull, tc.files
				r := runMergeCheck(t, st, "12")
				r.requireFail(t)
				r.requireNotInOutput(t, "documents only", "merge-check: ok")
			})
		}
	})
	t.Run("push run and the review check", func(t *testing.T) {
		r := runMergeCheck(t, review(false, "User", "", "scripts/x.sh"), "", "GITHUB_ACTIONS=true", "GITHUB_EVENT_NAME=push")
		r.requirePass(t, "the review check does not apply")
		if strings.Contains(r.argv, "/pulls/") {
			t.Errorf("a push run read a pull request or its files:\n%s", r.argv)
		}
	})
}
