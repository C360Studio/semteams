package sandboxfleet

import (
	"strings"
	"testing"
)

// sigForRecipe is the canonical signature used across recipe tests.
// Stable across runs — Canonicalize is the same deterministic
// machine the tool executor uses, so any drift in canonicalization
// surfaces in both Compose tests and the broader executor tests.
func sigForRecipe(t *testing.T) TargetSignature {
	t.Helper()
	sig, err := Canonicalize(CanonicalizeInput{
		Command:   "task test:integration",
		RepoURL:   "https://github.com/c360studio/semteams",
		RepoRef:   "main",
		Toolchain: map[string]string{"go": "1.26.0"},
		BaseImage: "ubuntu:24.04",
	})
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	return sig
}

func TestCompose_DefaultShallowClone(t *testing.T) {
	sig := sigForRecipe(t)
	recipe, err := Compose(RecipeIntent{
		Source: SourceIntent{Kind: SourceKindGit},
	}, sig, "/workspace")
	if err != nil {
		t.Fatalf("compose: %v", err)
	}
	want := "git clone --depth=1 --single-branch --branch main https://github.com/c360studio/semteams /workspace"
	if recipe.CloneCommand != want {
		t.Errorf("clone_command =\n  %q\nwant\n  %q", recipe.CloneCommand, want)
	}
}

// TestCompose_BranchBeforeURL pins the git-grammar fix that PR 3.2
// patched via persona prose (`--branch <ref>` BEFORE the URL). With
// the composer owning this, the persona's CLI grammar warning
// retires and the failure mode it was preventing
// (`fatal: Too many arguments`) becomes structurally impossible.
func TestCompose_BranchBeforeURL(t *testing.T) {
	sig := sigForRecipe(t)
	recipe, _ := Compose(RecipeIntent{
		Source: SourceIntent{Kind: SourceKindGit},
	}, sig, "/workspace")
	branchIdx := strings.Index(recipe.CloneCommand, "--branch")
	urlIdx := strings.Index(recipe.CloneCommand, sig.RepoURL)
	if branchIdx < 0 || urlIdx < 0 {
		t.Fatalf("clone_command missing --branch or url: %q", recipe.CloneCommand)
	}
	if branchIdx >= urlIdx {
		t.Errorf("--branch must come BEFORE url; got idx(--branch)=%d idx(url)=%d in %q",
			branchIdx, urlIdx, recipe.CloneCommand)
	}
}

func TestCompose_FullCloneWhenDepthNegative(t *testing.T) {
	sig := sigForRecipe(t)
	recipe, _ := Compose(RecipeIntent{
		Source: SourceIntent{Kind: SourceKindGit, Depth: -1},
	}, sig, "/workspace")
	if strings.Contains(recipe.CloneCommand, "--depth") {
		t.Errorf("Depth=-1 should omit --depth flag; got %q", recipe.CloneCommand)
	}
}

func TestCompose_AllBranchesOmitsSingleBranch(t *testing.T) {
	sig := sigForRecipe(t)
	recipe, _ := Compose(RecipeIntent{
		Source: SourceIntent{Kind: SourceKindGit, AllBranches: true},
	}, sig, "/workspace")
	if strings.Contains(recipe.CloneCommand, "--single-branch") {
		t.Errorf("AllBranches=true should omit --single-branch; got %q", recipe.CloneCommand)
	}
}

func TestCompose_SourceNoneEmitsNone(t *testing.T) {
	sig := sigForRecipe(t)
	recipe, _ := Compose(RecipeIntent{
		Source: SourceIntent{Kind: SourceKindNone},
	}, sig, "/workspace")
	if recipe.CloneCommand != "none" {
		t.Errorf("clone_command = %q, want %q", recipe.CloneCommand, "none")
	}
}

func TestCompose_SourceUnknownKindErrors(t *testing.T) {
	sig := sigForRecipe(t)
	_, err := Compose(RecipeIntent{
		Source: SourceIntent{Kind: "subversion"},
	}, sig, "/workspace")
	if err == nil {
		t.Fatalf("expected error on unknown source kind")
	}
}

func TestCompose_AptStepIdempotentBatched(t *testing.T) {
	sig := sigForRecipe(t)
	recipe, _ := Compose(RecipeIntent{
		Source: SourceIntent{Kind: SourceKindGit},
		Dependencies: []Dependency{
			{Kind: DependencyApt, Packages: []string{"curl", "git", "ca-certificates"}},
		},
	}, sig, "/workspace")
	if len(recipe.InstallSteps) != 1 {
		t.Fatalf("apt should batch into 1 step; got %d", len(recipe.InstallSteps))
	}
	step := recipe.InstallSteps[0]
	// Idempotency flags the composer owns:
	if !strings.Contains(step, "apt-get update") {
		t.Errorf("missing apt-get update in: %q", step)
	}
	if !strings.Contains(step, "-y") {
		t.Errorf("missing -y flag (composer owns idempotency): %q", step)
	}
	if !strings.Contains(step, "--no-install-recommends") {
		t.Errorf("missing --no-install-recommends (composer owns lean install): %q", step)
	}
	// Packages sorted for stable hashing across input-order drift:
	want := "ca-certificates curl git"
	if !strings.Contains(step, want) {
		t.Errorf("packages should be sorted (%q); got: %q", want, step)
	}
}

func TestCompose_AptEmptyPackagesErrors(t *testing.T) {
	sig := sigForRecipe(t)
	_, err := Compose(RecipeIntent{
		Source:       SourceIntent{Kind: SourceKindNone},
		Dependencies: []Dependency{{Kind: DependencyApt}},
	}, sig, "/workspace")
	if err == nil {
		t.Fatalf("expected error when apt packages empty")
	}
}

func TestCompose_GoModDownloadUsesWorkspace(t *testing.T) {
	sig := sigForRecipe(t)
	recipe, _ := Compose(RecipeIntent{
		Source:       SourceIntent{Kind: SourceKindGit},
		Dependencies: []Dependency{{Kind: DependencyGoModDownload}},
	}, sig, "/srv/work")
	if recipe.InstallSteps[0] != "cd /srv/work && go mod download" {
		t.Errorf("go_mod_download should cd into workspace; got %q", recipe.InstallSteps[0])
	}
}

func TestCompose_PipInstallRequirementsVsSpec(t *testing.T) {
	sig := sigForRecipe(t)
	tests := []struct {
		name        string
		manifest    string
		wantContain string
	}{
		{"requirements file", "requirements.txt", "pip install --no-cache-dir -r requirements.txt"},
		{"flag spec", "-e .[test]", "pip install --no-cache-dir -e .[test]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recipe, err := Compose(RecipeIntent{
				Source:       SourceIntent{Kind: SourceKindNone},
				Dependencies: []Dependency{{Kind: DependencyPipInstall, Manifest: tt.manifest}},
			}, sig, "/workspace")
			if err != nil {
				t.Fatalf("compose: %v", err)
			}
			if !strings.Contains(recipe.InstallSteps[0], tt.wantContain) {
				t.Errorf("step %q missing substring %q", recipe.InstallSteps[0], tt.wantContain)
			}
		})
	}
}

func TestCompose_PipInstallEmptyManifestErrors(t *testing.T) {
	sig := sigForRecipe(t)
	_, err := Compose(RecipeIntent{
		Source:       SourceIntent{Kind: SourceKindNone},
		Dependencies: []Dependency{{Kind: DependencyPipInstall}},
	}, sig, "/workspace")
	if err == nil {
		t.Fatalf("expected error on empty pip manifest")
	}
}

// TestCompose_ToolchainGoInterpolatesLiteralVersion pins PR 3.3
// reviewer REC-3: the composer reads the literal version from the
// canonical signature, NOT a shell-variable placeholder. Without
// this assertion, the toolchain_go composer could regress to
// `${TOOLCHAIN_GO_VERSION}` and the failure would only surface at
// real-LLM smoke time as a curl 404.
func TestCompose_ToolchainGoInterpolatesLiteralVersion(t *testing.T) {
	sig := sigForRecipe(t) // has Toolchain["go"]="1.26.0"
	recipe, err := Compose(RecipeIntent{
		Source:       SourceIntent{Kind: SourceKindNone},
		Dependencies: []Dependency{{Kind: DependencyToolchainGo}},
	}, sig, "/workspace")
	if err != nil {
		t.Fatalf("compose: %v", err)
	}
	if !strings.Contains(recipe.InstallSteps[0], "go1.26.0.linux-amd64") {
		t.Errorf("toolchain_go should interpolate literal version 1.26.0; got %q", recipe.InstallSteps[0])
	}
	if strings.Contains(recipe.InstallSteps[0], "${") {
		t.Errorf("toolchain_go must not leak a shell-variable placeholder; got %q", recipe.InstallSteps[0])
	}
}

// TestCompose_ToolchainGoErrorsWhenVersionMissing pins fail-fast:
// declaring toolchain_go without the corresponding signature entry
// is an error at Compose time, not a silent unfilled-template wedge
// at runtime.
func TestCompose_ToolchainGoErrorsWhenVersionMissing(t *testing.T) {
	sig, _ := Canonicalize(CanonicalizeInput{
		Command:   "task t",
		BaseImage: "ubuntu:24.04",
	}) // no toolchain
	_, err := Compose(RecipeIntent{
		Source:       SourceIntent{Kind: SourceKindNone},
		Dependencies: []Dependency{{Kind: DependencyToolchainGo}},
	}, sig, "/workspace")
	if err == nil {
		t.Fatalf("expected error when toolchain_go declared without sig.Toolchain[\"go\"]")
	}
	if !strings.Contains(err.Error(), "toolchain_go") {
		t.Errorf("error should name toolchain_go; got %v", err)
	}
}

// TestCompose_ToolchainNodeExtractsMajor pins that toolchain_node
// derives the nodesource major-version digit from the canonical
// "major.minor.patch" form (e.g. "22.10.0" → setup_22.x).
func TestCompose_ToolchainNodeExtractsMajor(t *testing.T) {
	sig, _ := Canonicalize(CanonicalizeInput{
		Command:   "task t",
		BaseImage: "ubuntu:24.04",
		Toolchain: map[string]string{"node": "22.10.0"},
	})
	recipe, err := Compose(RecipeIntent{
		Source:       SourceIntent{Kind: SourceKindNone},
		Dependencies: []Dependency{{Kind: DependencyToolchainNode}},
	}, sig, "/workspace")
	if err != nil {
		t.Fatalf("compose: %v", err)
	}
	if !strings.Contains(recipe.InstallSteps[0], "setup_22.x") {
		t.Errorf("toolchain_node should emit setup_22.x for 22.10.0; got %q", recipe.InstallSteps[0])
	}
	if strings.Contains(recipe.InstallSteps[0], "${") {
		t.Errorf("toolchain_node must not leak a shell-variable placeholder; got %q", recipe.InstallSteps[0])
	}
}

func TestCompose_RawPassesThrough(t *testing.T) {
	sig := sigForRecipe(t)
	recipe, _ := Compose(RecipeIntent{
		Source: SourceIntent{Kind: SourceKindNone},
		Dependencies: []Dependency{
			{Kind: DependencyRaw, Command: "cargo install ripgrep --locked"},
		},
	}, sig, "/workspace")
	if recipe.InstallSteps[0] != "cargo install ripgrep --locked" {
		t.Errorf("raw should pass through verbatim; got %q", recipe.InstallSteps[0])
	}
}

func TestCompose_RawEmptyCommandErrors(t *testing.T) {
	sig := sigForRecipe(t)
	_, err := Compose(RecipeIntent{
		Source:       SourceIntent{Kind: SourceKindNone},
		Dependencies: []Dependency{{Kind: DependencyRaw}},
	}, sig, "/workspace")
	if err == nil {
		t.Fatalf("expected error on empty raw command")
	}
}

func TestCompose_DependencyOrderPreserved(t *testing.T) {
	sig := sigForRecipe(t)
	recipe, _ := Compose(RecipeIntent{
		Source: SourceIntent{Kind: SourceKindGit},
		Dependencies: []Dependency{
			{Kind: DependencyApt, Packages: []string{"build-essential"}},
			{Kind: DependencyRaw, Command: "install-go.sh"},
			{Kind: DependencyGoModDownload},
		},
	}, sig, "/workspace")
	if len(recipe.InstallSteps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(recipe.InstallSteps))
	}
	if !strings.HasPrefix(recipe.InstallSteps[0], "apt-get update") {
		t.Errorf("step 0 should be apt; got %q", recipe.InstallSteps[0])
	}
	if recipe.InstallSteps[1] != "install-go.sh" {
		t.Errorf("step 1 should be raw; got %q", recipe.InstallSteps[1])
	}
	if !strings.Contains(recipe.InstallSteps[2], "go mod download") {
		t.Errorf("step 2 should be go mod download; got %q", recipe.InstallSteps[2])
	}
}

func TestCompose_VolumeMountsUseSignaturePrefix(t *testing.T) {
	sig := sigForRecipe(t)
	recipe, err := Compose(RecipeIntent{
		Source: SourceIntent{Kind: SourceKindGit},
		Mounts: []Mount{
			{VolumeSuffix: "workspace", Path: "/workspace"},
			{VolumeSuffix: "deps", Path: "/root/.cache"},
		},
	}, sig, "/workspace")
	if err != nil {
		t.Fatalf("compose: %v", err)
	}
	if len(recipe.VolumeMounts) != 2 {
		t.Fatalf("expected 2 mounts, got %d", len(recipe.VolumeMounts))
	}
	wantPrefix := "semteams-tenant-" + sig.Prefix() + "-"
	for i, m := range recipe.VolumeMounts {
		if !strings.HasPrefix(m, wantPrefix) {
			t.Errorf("mount[%d] %q missing prefix %q", i, m, wantPrefix)
		}
	}
}

func TestCompose_VolumeMountInvalidSuffixErrors(t *testing.T) {
	sig := sigForRecipe(t)
	_, err := Compose(RecipeIntent{
		Source: SourceIntent{Kind: SourceKindNone},
		Mounts: []Mount{{VolumeSuffix: "Has Caps", Path: "/x"}},
	}, sig, "/workspace")
	if err == nil {
		t.Fatalf("expected error on invalid volume_suffix")
	}
}

// TestCompose_MountPathRejectsAdversarialInput defends the downstream
// `docker run -v <vol>:<path>` substitution against shell-injection
// by a malicious-or-confused plan persona. Per PR 3.3 reviewer
// recommendation REC-1 + REC-6. Adding a new attack shape? Pin it
// here first so the composer's guard rejects it from day one.
func TestCompose_MountPathRejectsAdversarialInput(t *testing.T) {
	sig := sigForRecipe(t)
	cases := []struct {
		name string
		path string
	}{
		{"relative path", "workspace"},
		{"colon injection (close volume spec, append docker flags)", "/x:ro,Z --privileged /etc"},
		{"comma injection (docker bind-mount option separator)", "/x,rw,Z"},
		{"shell semicolon", "/x;rm -rf /"},
		{"shell pipe", "/x|cat /etc/shadow"},
		{"command substitution backtick", "/x`whoami`"},
		{"command substitution dollar-paren", "/x$(id)"},
		{"dotdot traversal", "/x/../etc"},
		{"bare dotdot", "/.."},
		{"whitespace in path", "/has space"},
		{"tab in path", "/has\ttab"},
		{"newline in path", "/has\nlf"},
		{"null byte", "/has\x00null"},
		{"single quote", "/x'y"},
		{"double quote", "/x\"y"},
		{"angle bracket redirect", "/x>out"},
		{"backslash escape", "/x\\y"},
		{"glob star", "/x*"},
		{"brace expansion", "/x{a,b}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Compose(RecipeIntent{
				Source: SourceIntent{Kind: SourceKindNone},
				Mounts: []Mount{{VolumeSuffix: "workspace", Path: tc.path}},
			}, sig, "/workspace")
			if err == nil {
				t.Fatalf("expected error on adversarial mount path %q", tc.path)
			}
		})
	}
}

// TestCompose_MountPathAcceptsLegitimatePaths pins the positive side
// of REC-1 — the validator must NOT reject paths the plan persona
// legitimately needs.
func TestCompose_MountPathAcceptsLegitimatePaths(t *testing.T) {
	sig := sigForRecipe(t)
	cases := []string{
		"/workspace",
		"/root/.cache",
		"/var/lib/postgresql/data",
		"/home/runner/work/repo",
		"/tmp/work-dir",
		"/opt/app-data",
	}
	for _, path := range cases {
		t.Run(path, func(t *testing.T) {
			_, err := Compose(RecipeIntent{
				Source: SourceIntent{Kind: SourceKindNone},
				Mounts: []Mount{{VolumeSuffix: "workspace", Path: path}},
			}, sig, "/workspace")
			if err != nil {
				t.Errorf("legitimate path %q rejected: %v", path, err)
			}
		})
	}
}

func TestCompose_ExpectedSmokeStringDeterministic(t *testing.T) {
	sig := sigForRecipe(t)
	recipe, _ := Compose(RecipeIntent{
		Source: SourceIntent{Kind: SourceKindNone},
		Smoke: SmokeIntent{
			Command: "go test ./...",
			Expects: SmokeExpects{ExitCode: 0, StdoutContains: "PASS"},
		},
	}, sig, "/workspace")
	want := "exit 0; stdout contains PASS"
	if recipe.ExpectedSmokeSignature != want {
		t.Errorf("expected_smoke = %q, want %q", recipe.ExpectedSmokeSignature, want)
	}
}

// TestCompose_ExpectedSmokeStringDefault pins zero-value semantics
// (per PR 3.3 reviewer NIT-3): empty Expects renders the canonical
// default "exit 0". Without this assertion, a refactor that flipped
// the default to "exit non-zero" or stripped the exit clause entirely
// could go undetected.
func TestCompose_ExpectedSmokeStringDefault(t *testing.T) {
	sig := sigForRecipe(t)
	recipe, _ := Compose(RecipeIntent{
		Source: SourceIntent{Kind: SourceKindNone},
		Smoke:  SmokeIntent{},
	}, sig, "/workspace")
	if recipe.ExpectedSmokeSignature != "exit 0" {
		t.Errorf("default Expects should render %q; got %q", "exit 0", recipe.ExpectedSmokeSignature)
	}
}

func TestCompose_ExpectedSmokeStringOnlyExitCode(t *testing.T) {
	sig := sigForRecipe(t)
	recipe, _ := Compose(RecipeIntent{
		Source: SourceIntent{Kind: SourceKindNone},
		Smoke:  SmokeIntent{Expects: SmokeExpects{ExitCode: 42}},
	}, sig, "/workspace")
	if recipe.ExpectedSmokeSignature != "exit 42" {
		t.Errorf("expected_smoke = %q, want %q", recipe.ExpectedSmokeSignature, "exit 42")
	}
}

// TestCompose_WorkspaceDefaults pins reviewer H3 invariant (workspace
// is a side-output of provisioning, not an input). Empty workspace
// falls back to /workspace deterministically.
func TestCompose_WorkspaceDefaults(t *testing.T) {
	sig := sigForRecipe(t)
	recipe, _ := Compose(RecipeIntent{
		Source: SourceIntent{Kind: SourceKindGit},
	}, sig, "")
	if !strings.HasSuffix(recipe.CloneCommand, "/workspace") {
		t.Errorf("empty workspace should default to /workspace; got %q", recipe.CloneCommand)
	}
}

// TestCompose_DeterministicAcrossRuns is the property test for plan-
// hash stability: same RecipeIntent, same sig → identical Recipe
// across calls. The composer is pure; if this ever fails the hash
// pinning in the executor's planHash function regresses too.
func TestCompose_DeterministicAcrossRuns(t *testing.T) {
	sig := sigForRecipe(t)
	intent := RecipeIntent{
		Source: SourceIntent{Kind: SourceKindGit},
		Dependencies: []Dependency{
			{Kind: DependencyApt, Packages: []string{"git", "curl"}},
			{Kind: DependencyGoModDownload},
		},
		Mounts: []Mount{{VolumeSuffix: "workspace", Path: "/workspace"}},
		Smoke:  SmokeIntent{Command: "go test", Expects: SmokeExpects{StdoutContains: "ok"}},
	}
	r1, _ := Compose(intent, sig, "/workspace")
	r2, _ := Compose(intent, sig, "/workspace")
	if r1.CloneCommand != r2.CloneCommand {
		t.Errorf("clone non-deterministic: %q vs %q", r1.CloneCommand, r2.CloneCommand)
	}
	if strings.Join(r1.InstallSteps, "|") != strings.Join(r2.InstallSteps, "|") {
		t.Errorf("install_steps non-deterministic")
	}
	if strings.Join(r1.VolumeMounts, "|") != strings.Join(r2.VolumeMounts, "|") {
		t.Errorf("volume_mounts non-deterministic")
	}
	if r1.ExpectedSmokeSignature != r2.ExpectedSmokeSignature {
		t.Errorf("smoke non-deterministic")
	}
}
