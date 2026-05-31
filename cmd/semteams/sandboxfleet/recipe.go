package sandboxfleet

import (
	"fmt"
	"sort"
	"strings"
)

// RecipeIntent captures the structured provisioning intent the plan
// persona supplies. It deliberately does NOT contain shell strings:
// CLI grammar (git's positional ordering, apt-get's idempotency
// flags, docker volume-name conventions) is deterministic given the
// intent, so encoding it in code instead of LLM prose retires a
// class of fragile persona-prose fixes — see
// [[personas-should-not-author-shell]] and ADR-042 §addendum PR 3.3.
//
// The composer Compose translates RecipeIntent into a Recipe, whose
// fields are the verbatim shell strings the execute persona's bash
// tool runs. The contract downstream of Compose is identical to the
// pre-PR-3.3 shape (same clone_command/install_steps/volume_mounts
// triple shape stamped on run + plan-loop entities), so rule 02b's
// spawn-prompt substitution and the execute persona's shell-emission
// loop are unchanged.
//
// Escape hatch: DependencyRaw is the explicit "I don't fit a modeled
// kind" valve. Use it for things the composer does not (yet) model;
// surfacing it in real-LLM smoke is the prompt to add a structured
// kind on the third recurrence.
type RecipeIntent struct {
	Source       SourceIntent
	Dependencies []Dependency
	Mounts       []Mount
	Smoke        SmokeIntent
	// DockerSocketMount is the conservative gate for targets whose
	// measurement command needs Docker (testcontainers, docker-compose-
	// driven tests). Default false; the plan persona flips it true only
	// when the target description names docker / testcontainers
	// explicitly.
	DockerSocketMount bool
}

// SourceIntent describes what the tenant's workspace is populated
// from. Kind=git clones from RepoURL@RepoRef; kind=none leaves the
// workspace empty (self-contained shell-script targets).
//
// Zero-value semantics are tuned for the typical case: a SourceIntent
// of {Kind: git} produces the cheapest clone (shallow depth=1, single
// branch). Explicit opt-outs:
//
//   - Depth < 0 → full clone (no --depth flag). For measurements that
//     read git history (blame, log).
//   - Depth > 0 → that depth (e.g. 50 for "last few commits").
//   - AllBranches=true → omit --single-branch (fetch all refs).
type SourceIntent struct {
	Kind        SourceKind
	Depth       int
	AllBranches bool
}

// SourceKind enumerates the source-of-truth choices for the tenant
// workspace. Extend by adding a new constant + a Compose branch; do
// not pass raw shell through here.
type SourceKind string

// SourceKind enum values. Persona-supplied as JSON strings; the
// composer branches on these. Mirror this set in the persona +
// rule prose when adding a new kind so the LLM knows it exists.
const (
	SourceKindGit  SourceKind = "git"
	SourceKindNone SourceKind = "none"
)

// Dependency is one install step in structured form. The composer
// branches on Kind to emit the right shell line; unknown kinds error
// at Compose time (the executor surfaces as ToolErrorInvalidArgs).
//
// The Packages / Manifest / Command field semantics depend on Kind:
//   - DependencyApt: Packages is the apt-get package list.
//   - DependencyGoModDownload: no fields (workspace must contain go.mod).
//   - DependencyNPMCI: no fields (workspace must contain package-lock.json).
//   - DependencyPipInstall: Manifest is "requirements.txt" or a
//     pyproject extras spec ("-e .[test]").
//   - DependencyRaw: Command is the verbatim shell line. Escape hatch.
type Dependency struct {
	Kind     DependencyKind
	Packages []string
	Manifest string
	Command  string
}

// DependencyKind enumerates the structured install-step shapes the
// composer knows. The set is intentionally small at v1 — the bar to
// add a new kind is "DependencyRaw used twice for the same shape."
type DependencyKind string

// DependencyKind enum values. Each maps to a composeDependency case
// that renders the canonical idempotent shell line. Update the
// persona's allowed-kinds list AND the executor's ListTools schema
// enum when widening this set.
const (
	DependencyApt           DependencyKind = "apt"
	DependencyGoModDownload DependencyKind = "go_mod_download"
	DependencyNPMCI         DependencyKind = "npm_ci"
	DependencyPipInstall    DependencyKind = "pip_install"
	DependencyToolchainGo   DependencyKind = "toolchain_go"
	DependencyToolchainNode DependencyKind = "toolchain_node"
	DependencyRaw           DependencyKind = "raw"
)

// Mount describes one volume mount. The composer derives the full
// volume name from the tenant signature
// ("semteams-tenant-<sig-prefix>-<suffix>"), so VolumeSuffix is the
// caller-supplied differentiator only — typically "workspace" or
// "deps". Path is the in-container mount point.
type Mount struct {
	VolumeSuffix string
	Path         string
}

// SmokeIntent is the verify-phase grading contract in structured
// form. Command is the actual shell line the verify persona runs
// (legitimately LLM-authored — this IS the user-intent verbatim, not
// a wrapper composed around it; see
// [[personas-should-not-author-shell]] exception clause). Expects is
// the structured grading shape — the composer derives the legacy
// expected_smoke_signature string from it for downstream
// compatibility, but new consumers should read Expects directly.
type SmokeIntent struct {
	Command string
	Expects SmokeExpects
}

// SmokeExpects encodes the grading rule structurally so the composer
// can render a deterministic expected_smoke_signature string and so
// future Go-side grading (a substrate-side verify executor) can read
// fields instead of regex-parsing prose.
type SmokeExpects struct {
	// ExitCode is the required exit status. Zero means "exit 0";
	// callers that intend a non-zero pass-criterion must set it
	// explicitly. The composer always emits "exit <N>" in the legacy
	// string.
	ExitCode int
	// StdoutContains is an optional substring match on the smoke
	// command's stdout. Empty means "no stdout assertion."
	StdoutContains string
}

// Recipe is the composer output: verbatim shell strings + mount
// specs the execute persona's bash loop will run, plus the legacy
// expected_smoke_signature string for verify-phase audit. Field
// shapes match the pre-PR-3.3 plan-triple shape so downstream
// substitution / persona prose stay unchanged.
type Recipe struct {
	// CloneCommand is the literal "git clone …" the execute persona
	// runs, or "none" when SourceKind=none. The composer always emits
	// --branch BEFORE the URL (git's positional grammar).
	CloneCommand string
	// InstallSteps is the ordered list of shell lines, one per
	// Dependency in input order. Composer-emitted lines include
	// idempotency flags (apt-get -y --no-install-recommends, pip
	// install --no-cache-dir, etc.).
	InstallSteps []string
	// VolumeMounts is the docker-CLI volume-mount syntax list
	// ("<volume>:<container-path>"), one per Mount. Volume names
	// derived from the tenant signature prefix; deterministic across
	// runs.
	VolumeMounts []string
	// ExpectedSmokeSignature is the legacy verify-phase grading
	// string derived from SmokeExpects (e.g. "exit 0; stdout contains
	// PASS"). Preserved for downstream consumers that still read the
	// string shape.
	ExpectedSmokeSignature string
}

// ComposerVersion is folded into plan_hash so any semantic-affecting
// change to the composer's output (new flag, retired idempotency
// quirk, kind retired/re-aliased) deterministically rotates cached
// plan_hashes — operators on the next bootstrap arc re-provision
// rather than reuse a tenant that was built under the old composer.
// Bump when the composed shell semantics change in a way that should
// invalidate cache hits; do NOT bump for pure refactors / comment
// edits / test additions. Per PR 3.3 reviewer REC-4.
const ComposerVersion = 1

// Compose deterministically renders RecipeIntent into a Recipe given
// the canonicalized signature (for volume naming + toolchain version
// lookup) and the container-internal workspace path (for clone
// target). Returns an error when the intent is internally
// inconsistent (unknown kind, missing required fields) — the
// executor surfaces these as ToolErrorInvalidArgs so the plan
// persona can re-author with a specific message.
//
// Pure function: no side effects, no network, no time. Stable across
// runs given identical inputs — this is a hashed contributor to
// plan_hash via the composed strings.
func Compose(in RecipeIntent, sig TargetSignature, workspace string) (Recipe, error) {
	if workspace = strings.TrimSpace(workspace); workspace == "" {
		workspace = "/workspace"
	}

	cloneCmd, err := composeClone(in.Source, sig, workspace)
	if err != nil {
		return Recipe{}, fmt.Errorf("compose clone: %w", err)
	}

	installSteps, err := composeInstallSteps(in.Dependencies, sig, workspace)
	if err != nil {
		return Recipe{}, fmt.Errorf("compose install steps: %w", err)
	}

	volumeMounts, err := composeVolumeMounts(in.Mounts, sig)
	if err != nil {
		return Recipe{}, fmt.Errorf("compose volume mounts: %w", err)
	}

	return Recipe{
		CloneCommand:           cloneCmd,
		InstallSteps:           installSteps,
		VolumeMounts:           volumeMounts,
		ExpectedSmokeSignature: composeExpectedSmoke(in.Smoke.Expects),
	}, nil
}

// composeClone renders SourceIntent into a git-clone shell line, or
// "none" for SourceKindNone. Git grammar pinned in code: --branch
// goes BEFORE the URL (git's positional ordering is
// `git clone [options] <url> [<dir>]`), --depth defaults to 1
// (shallow), --single-branch defaults to true (cuts unrelated
// refs). The pre-PR-3.3 persona-prose CLI grammar warning retires
// with this function.
func composeClone(src SourceIntent, sig TargetSignature, workspace string) (string, error) {
	switch src.Kind {
	case "", SourceKindNone:
		return "none", nil
	case SourceKindGit:
		if sig.RepoURL == "" {
			return "", fmt.Errorf("source.kind=git requires repo_url in the canonical signature")
		}
		if sig.RepoRef == "" {
			return "", fmt.Errorf("source.kind=git requires repo_ref in the canonical signature")
		}
		parts := []string{"git", "clone"}
		// Depth zero-value = shallow (1); negative = full clone (omit
		// --depth); positive = literal depth.
		switch {
		case src.Depth < 0:
			// full clone — no --depth flag
		case src.Depth == 0:
			parts = append(parts, "--depth=1")
		default:
			parts = append(parts, fmt.Sprintf("--depth=%d", src.Depth))
		}
		if !src.AllBranches {
			parts = append(parts, "--single-branch")
		}
		parts = append(parts, "--branch", sig.RepoRef, sig.RepoURL, workspace)
		return strings.Join(parts, " "), nil
	default:
		return "", fmt.Errorf("source.kind %q: unknown (expected one of: git, none)", src.Kind)
	}
}

// composeInstallSteps walks the Dependencies in input order and
// emits one shell line per dependency. Order is preserved
// (apt-before-toolchain-before-app-deps is the persona's
// responsibility); idempotency flags (-y, --no-cache-dir) are the
// composer's. Signature passes through so toolchain_* kinds can
// interpolate the literal version from sig.Toolchain (per PR 3.3
// reviewer REC-3 — emitting `${TOOLCHAIN_GO_VERSION}` as a shell
// var was a latent wedge because nothing sets it downstream).
func composeInstallSteps(deps []Dependency, sig TargetSignature, workspace string) ([]string, error) {
	steps := make([]string, 0, len(deps))
	for i, dep := range deps {
		line, err := composeDependency(dep, sig, workspace)
		if err != nil {
			return nil, fmt.Errorf("dependency[%d]: %w", i, err)
		}
		steps = append(steps, line)
	}
	return steps, nil
}

// composeDependency renders one Dependency into a shell line.
// Composer owns flag-correctness (apt-get -y, npm ci --silent-ish,
// pip --no-cache-dir); persona owns intent. Toolchain kinds read
// the literal version from sig.Toolchain — composer fails fast if
// the persona declared the kind but omitted the toolchain entry
// (silent template-string fallback would 404 the curl downstream).
func composeDependency(dep Dependency, sig TargetSignature, workspace string) (string, error) {
	switch dep.Kind {
	case DependencyApt:
		if len(dep.Packages) == 0 {
			return "", fmt.Errorf("apt: packages must be non-empty")
		}
		// Sort packages for stable hashing under input-order drift.
		// apt-get treats them as a set, so this changes no behavior.
		pkgs := append([]string(nil), dep.Packages...)
		sort.Strings(pkgs)
		// Emit update + install as a single shell line; apt-get is
		// allergic to outdated indexes and the install-step granularity
		// shouldn't penalize the persona for that.
		return "apt-get update && apt-get install -y --no-install-recommends " + strings.Join(pkgs, " "), nil
	case DependencyGoModDownload:
		return "cd " + workspace + " && go mod download", nil
	case DependencyNPMCI:
		return "cd " + workspace + " && npm ci --no-audit --no-fund", nil
	case DependencyPipInstall:
		manifest := strings.TrimSpace(dep.Manifest)
		if manifest == "" {
			return "", fmt.Errorf("pip_install: manifest must be non-empty (requirements.txt or pyproject extras spec)")
		}
		// pip install -r <manifest> when manifest looks like a file;
		// otherwise treat manifest as a pip spec ("-e .[test]"). Test
		// is "starts with '-'" — pip's CLI uses '-' for its flags so
		// callers passing a flag spec bypass -r.
		if strings.HasPrefix(manifest, "-") {
			return "cd " + workspace + " && pip install --no-cache-dir " + manifest, nil
		}
		return "cd " + workspace + " && pip install --no-cache-dir -r " + manifest, nil
	case DependencyToolchainGo:
		// Read the literal version from the canonical signature.
		// Erroring here is the right failure mode — silently emitting
		// an unfilled shell-var template would defer the failure to a
		// curl 404 inside the tenant (smoke #10c class).
		version := strings.TrimSpace(sig.Toolchain["go"])
		if version == "" {
			return "", fmt.Errorf("toolchain_go: signature toolchain[\"go\"] required (persona named the kind but didn't supply the version)")
		}
		return "curl -fsSL https://go.dev/dl/go" + version + ".linux-amd64.tar.gz -o /tmp/go.tgz && tar -C /usr/local -xzf /tmp/go.tgz && rm /tmp/go.tgz", nil
	case DependencyToolchainNode:
		// Nodesource setup script keys on the major version digit;
		// extract from the canonical "<major>.<minor>.<patch>" form.
		// Operators wanting nvm-style installs use DependencyRaw.
		full := strings.TrimSpace(sig.Toolchain["node"])
		if full == "" {
			return "", fmt.Errorf("toolchain_node: signature toolchain[\"node\"] required (persona named the kind but didn't supply the version)")
		}
		major, _, _ := strings.Cut(full, ".")
		if major == "" {
			return "", fmt.Errorf("toolchain_node: signature toolchain[\"node\"]=%q: no major-version prefix", full)
		}
		return "curl -fsSL https://deb.nodesource.com/setup_" + major + ".x | bash - && apt-get install -y --no-install-recommends nodejs", nil
	case DependencyRaw:
		cmd := strings.TrimSpace(dep.Command)
		if cmd == "" {
			return "", fmt.Errorf("raw: command must be non-empty")
		}
		// Escape hatch: pass-through. If the same shape appears in
		// real-LLM smoke twice, add a structured kind.
		return cmd, nil
	default:
		return "", fmt.Errorf("kind %q: unknown (expected one of: apt, go_mod_download, npm_ci, pip_install, toolchain_go, toolchain_node, raw)", dep.Kind)
	}
}

// composeVolumeMounts renders Mount specs into docker-CLI volume
// strings. Volume name derives from sig.Prefix() so two arcs
// targeting the same tenant share volumes (workspace persistence +
// dep cache reuse across provisions). Suffix is caller-supplied:
// "workspace", "deps", or any operator-named differentiator.
//
// Path validation defends the downstream `docker run -v <vol>:<path>`
// invocation against shell injection by an over-creative or
// adversarial plan persona. The path lands verbatim in a shell line
// that downstream substitution puts inside `sh -c '<volume_mount>'`;
// a path containing `:` would close the volume spec and let the
// remainder append flags (e.g. `/x:ro,Z --privileged`). Rejecting
// the small set of shell-significant characters up front is cheaper
// and less error-prone than the alternative (shell-escape every
// substitution downstream). Per PR 3.3 reviewer recommendation REC-1.
func composeVolumeMounts(mounts []Mount, sig TargetSignature) ([]string, error) {
	out := make([]string, 0, len(mounts))
	for i, m := range mounts {
		suffix := strings.TrimSpace(m.VolumeSuffix)
		if suffix == "" {
			return nil, fmt.Errorf("mount[%d]: volume_suffix must be non-empty", i)
		}
		path := strings.TrimSpace(m.Path)
		if path == "" {
			return nil, fmt.Errorf("mount[%d]: path must be non-empty", i)
		}
		if !volumeSuffixPattern.MatchString(suffix) {
			return nil, fmt.Errorf("mount[%d]: volume_suffix %q must match [a-z0-9-]+ (docker volume name constraints)", i, suffix)
		}
		if err := validateMountPath(path); err != nil {
			return nil, fmt.Errorf("mount[%d]: %w", i, err)
		}
		// "semteams-tenant-<prefix>-<suffix>:<path>"
		out = append(out, "semteams-tenant-"+sig.Prefix()+"-"+suffix+":"+path)
	}
	return out, nil
}

// validateMountPath rejects in-container paths whose contents could
// inject shell tokens once substituted into a downstream
// `docker run -v <vol>:<path>` invocation. The validations:
//
//   - Must be absolute (leading "/"). Relative paths are at best
//     confusing inside a fresh container and at worst depend on
//     CWD the persona has no control over.
//   - Must not contain ":" — colon is the docker volume-spec
//     delimiter; embedding one lets a malicious path append docker
//     flags (`/x:ro,Z --privileged ...`).
//   - Must not contain "," — comma is docker's bind-mount option
//     separator (`/x,rw,Z`).
//   - Must not contain shell-significant chars (whitespace, ';',
//     '&', '|', '$', backticks, quotes, '<', '>', '\\', '*',
//     '?', '(', ')', '{', '}', '[', ']', '!', '#'). These would
//     either be interpreted by the wrapping shell or be benign-
//     looking but enable trickery once the downstream prompt
//     embeds the spec.
//   - Must not contain ".." segments — directory traversal is
//     never the right intent for a fresh tenant mount.
//   - Must not contain non-printable / control characters.
func validateMountPath(path string) error {
	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("path %q: must be absolute (leading '/')", path)
	}
	if strings.Contains(path, "..") {
		return fmt.Errorf("path %q: must not contain '..' segments (directory traversal)", path)
	}
	const forbidden = ":,;&|$`\"'<>\\*?(){}[]!#"
	if idx := strings.IndexAny(path, forbidden); idx >= 0 {
		return fmt.Errorf("path %q: contains shell-significant character %q (docker -v spec injection guard)", path, path[idx])
	}
	for _, r := range path {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return fmt.Errorf("path %q: must not contain whitespace", path)
		}
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("path %q: contains non-printable character", path)
		}
	}
	return nil
}

// composeExpectedSmoke renders the structured grading rule into the
// legacy string shape downstream verify-phase audit reads. The
// emitted string is deterministic over the inputs so plan_hash stays
// stable across prose variants the persona might have authored.
func composeExpectedSmoke(e SmokeExpects) string {
	parts := []string{fmt.Sprintf("exit %d", e.ExitCode)}
	if s := strings.TrimSpace(e.StdoutContains); s != "" {
		parts = append(parts, "stdout contains "+s)
	}
	return strings.Join(parts, "; ")
}
