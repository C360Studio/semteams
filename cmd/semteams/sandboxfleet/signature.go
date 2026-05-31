package sandboxfleet

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/c360studio/semstreams/message"
)

// TargetSignature captures the canonical fields that identify a tenant
// container. Two targets that canonicalize to the same TargetSignature
// share a tenant; different signatures get different tenants.
//
// All fields are post-canonicalization. Callers MUST go through
// Canonicalize — direct struct-literal construction bypasses the
// normalizers and breaks cache-key correctness across prose-framing
// variants. Hash is deterministic only on canonicalized input.
//
// Per ADR-042 §addendum 2026-05-29 §A and review H2 fix:
// cache-key correctness is load-bearing for a long-running shared
// substrate. Two users describing the same target two different ways
// must produce the same Hash (no waste); materially-different targets
// must produce distinct Hashes (no silent collision / measurement
// corruption).
type TargetSignature struct {
	Command   string            `json:"command"`
	RepoURL   string            `json:"repo_url,omitempty"`
	RepoRef   string            `json:"repo_ref,omitempty"`
	Toolchain map[string]string `json:"toolchain,omitempty"`
	BaseImage string            `json:"base_image"`
}

// CanonicalizeInput is the raw, LLM-supplied input shape. Every field
// goes through a deterministic Go normalizer; nothing here is trusted
// as-canonical. The plan persona fills the typed fields from the
// user's prose; the tool executor calls Canonicalize before stamping
// triples or hashing.
type CanonicalizeInput struct {
	// Command is the measurement / smoke command verbatim. Examples:
	// "task test:integration", "pytest -q", "npm test".
	Command string

	// RepoURL is optional — empty means a self-contained target with
	// no repo (e.g. tenant for shell-script profiling). When present,
	// accepted forms: https://host/path[.git][/], git@host:owner/repo[.git].
	RepoURL string

	// RepoRef is optional, required when RepoURL is non-empty. Forms:
	// 40-char commit SHA (preferred), tag (e.g. "v1.2.3"), branch
	// (e.g. "main"). The canonicalizer treats SHAs as immutable and
	// movable refs as movable — registry freshness checks catch
	// upstream drift on movable refs.
	RepoRef string

	// Toolchain pins language toolchain versions. Keys are toolchain
	// names ("go", "node", "python", ...); values are version strings
	// (any of "1.26", "v1.26", "1.26.0"). The canonicalizer lowercases
	// keys and normalizes versions to full semver. Empty toolchains
	// are accepted (substrate-only targets, e.g. shell-script tenants).
	Toolchain map[string]string

	// BaseImage is the Docker image:tag. Must be explicit:
	// ":latest" is rejected, plain image with no tag is rejected
	// (cache correctness). Examples: "ubuntu:24.04",
	// "golang:1.26-bookworm", "node:22-alpine".
	BaseImage string
}

// Canonicalize normalizes a raw plan-supplied target into the
// deterministic TargetSignature shape. Returns an error if any
// required normalization fails (unparseable repo URL, ":latest" base
// image, missing required field). The plan persona surfaces returned
// errors as decide(needs_clarification) per the rule 01 contract.
func Canonicalize(in CanonicalizeInput) (TargetSignature, error) {
	cmd, err := canonicalizeCommand(in.Command)
	if err != nil {
		return TargetSignature{}, err
	}

	repoURL, repoRef, err := canonicalizeRepo(in.RepoURL, in.RepoRef)
	if err != nil {
		return TargetSignature{}, err
	}

	toolchain, err := canonicalizeToolchain(in.Toolchain)
	if err != nil {
		return TargetSignature{}, err
	}

	image, err := canonicalizeBaseImage(in.BaseImage)
	if err != nil {
		return TargetSignature{}, err
	}

	return TargetSignature{
		Command:   cmd,
		RepoURL:   repoURL,
		RepoRef:   repoRef,
		Toolchain: toolchain,
		BaseImage: image,
	}, nil
}

// Hash returns a stable hex SHA-256 of the canonicalized signature.
// The same TargetSignature value produces the same Hash across
// process restarts, Go versions, and host OS — the substrate's
// cross-run cache key.
//
// Deterministic via encoding/json's struct-field-order marshal and
// sorted map-key marshal. Toolchain map keys are sorted by Go's
// json package; lowercased + version-normalized at canonicalize
// time so two equivalent Toolchain maps marshal identically.
func (s TargetSignature) Hash() string {
	b, _ := json.Marshal(s)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Prefix returns the first 12 hex characters of Hash. Used as the
// tenant container-name suffix; full hashes are too long for
// container names (Docker caps at 63 chars and hash-only would
// hit the limit when combined with naming prefixes). 12 hex chars
// = 48 bits of entropy — collision probability across O(10^6)
// concurrent tenants is negligible.
func (s TargetSignature) Prefix() string {
	return s.Hash()[:12]
}

// ContainerName derives the Docker container name for this tenant:
// "semteams-tenant-<prefix>". Stable across canonicalize-Hash runs.
// Per ADR-042 §addendum §A / sandbox-bootstrap pack README open
// question #6 — this is the package's canonical convention.
func (s TargetSignature) ContainerName() string {
	return "semteams-tenant-" + s.Prefix()
}

// canonicalizeCommand lowercases, collapses runs of whitespace, and
// trims surrounding punctuation. Examples:
//
//	"Task Test:Integration"  → "task test:integration"
//	"  pytest    -q"         → "pytest -q"
//	"npm test;"              → "npm test"
func canonicalizeCommand(raw string) (string, error) {
	c := strings.TrimSpace(raw)
	if c == "" {
		return "", fmt.Errorf("command must not be empty")
	}
	c = strings.ToLower(c)
	c = whitespaceRun.ReplaceAllString(c, " ")
	c = strings.TrimRight(c, ".;,")
	c = strings.TrimSpace(c)
	if c == "" {
		return "", fmt.Errorf("command empty after canonicalize")
	}
	return c, nil
}

// canonicalizeRepo accepts ssh and https forms, returns (url, ref).
// Empty URL is legal (self-contained target); non-empty URL requires
// a non-empty ref. Examples:
//
//	"git@github.com:Owner/Repo.git", "v1.2.3"
//	  → "https://github.com/owner/repo", "v1.2.3"
//
//	"https://GitHub.com/Owner/Repo/", "main"
//	  → "https://github.com/owner/repo", "main"
func canonicalizeRepo(rawURL, rawRef string) (string, string, error) {
	if rawURL = strings.TrimSpace(rawURL); rawURL == "" {
		if strings.TrimSpace(rawRef) != "" {
			return "", "", fmt.Errorf("repo_ref %q supplied without repo_url", rawRef)
		}
		return "", "", nil
	}

	u, err := parseRepoURL(rawURL)
	if err != nil {
		return "", "", err
	}

	ref := strings.TrimSpace(rawRef)
	if ref == "" {
		return "", "", fmt.Errorf("repo_ref must be non-empty when repo_url is set")
	}

	return u, ref, nil
}

// parseRepoURL accepts both ssh ("git@host:path") and https forms
// and returns a canonical "https://host/path" without trailing
// slash and without ".git" suffix. Per PR 3.3 reviewer REC-7,
// embedded whitespace is rejected — url.Parse accepts URLs with
// spaces in the path, but a space in the canonicalized URL would
// mis-position the workspace argument in `git clone <url> <ws>`
// (git would parse the post-space token as the workspace and our
// /workspace arg as a positional overflow).
func parseRepoURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if strings.ContainsAny(raw, " \t\n\r") {
		return "", fmt.Errorf("repo_url %q: contains whitespace (would break shell-positional git clone)", raw)
	}

	// ssh form: git@host:owner/repo[.git]
	if rest, ok := strings.CutPrefix(raw, "git@"); ok {
		colonIdx := strings.Index(rest, ":")
		if colonIdx < 1 || colonIdx == len(rest)-1 {
			return "", fmt.Errorf("ssh repo url %q: missing host:path delimiter", raw)
		}
		host := strings.ToLower(rest[:colonIdx])
		path := rest[colonIdx+1:]
		path = strings.TrimSuffix(path, ".git")
		path = strings.TrimSuffix(path, "/")
		path = strings.ToLower(path)
		if path == "" {
			return "", fmt.Errorf("ssh repo url %q: empty path after canonicalize", raw)
		}
		return "https://" + host + "/" + path, nil
	}

	// https / http form
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("repo_url %q: %w", raw, err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", fmt.Errorf("repo_url %q: scheme %q not supported (use https or ssh)", raw, u.Scheme)
	}
	if u.Host == "" {
		return "", fmt.Errorf("repo_url %q: empty host", raw)
	}
	path := strings.TrimSuffix(u.Path, "/")
	path = strings.TrimSuffix(path, ".git")
	path = strings.ToLower(path)
	if path == "" || path == "/" {
		return "", fmt.Errorf("repo_url %q: empty path after canonicalize", raw)
	}
	host := strings.ToLower(u.Host)
	return "https://" + host + path, nil
}

// canonicalizeToolchain lowercases keys, normalizes version strings
// to full semver-like form ("1.26" → "1.26.0", "v1.26.0" → "1.26.0").
// Empty input returns nil (so a zero-valued Toolchain is JSON-omitted).
func canonicalizeToolchain(raw map[string]string) (map[string]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(raw))

	// Walk in sorted key order so that any error is deterministic
	// across runs (helps callers reproduce LLM-induced bugs).
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		key := strings.TrimSpace(strings.ToLower(k))
		if key == "" {
			return nil, fmt.Errorf("toolchain has empty key")
		}
		val, err := normalizeVersion(raw[k])
		if err != nil {
			return nil, fmt.Errorf("toolchain[%s]: %w", key, err)
		}
		out[key] = val
	}
	return out, nil
}

// normalizeVersion strips a leading "v" and pads missing patch
// digits with ".0" so "1.26", "v1.26", and "1.26.0" all canonicalize
// to "1.26.0". Tolerates pre-release suffixes (e.g. "1.26.0-rc.1");
// rejects empty input.
func normalizeVersion(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "", fmt.Errorf("version must not be empty")
	}
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")

	// Split off pre-release / build metadata at the first '-' or '+'.
	core := v
	suffix := ""
	if idx := strings.IndexAny(v, "-+"); idx >= 0 {
		core = v[:idx]
		suffix = v[idx:]
	}

	if !versionCorePattern.MatchString(core) {
		return "", fmt.Errorf("version %q: not a recognized semver-like form", raw)
	}

	parts := strings.Split(core, ".")
	for len(parts) < 3 {
		parts = append(parts, "0")
	}
	return strings.Join(parts, ".") + suffix, nil
}

// canonicalizeBaseImage enforces an explicit image:tag (or
// image:tag@sha256:...) with neither ":latest" nor a missing tag.
// Lowercases the host + path portion; preserves tag case (tags can
// be SemVer with a leading "v" that we keep as-is).
func canonicalizeBaseImage(raw string) (string, error) {
	img := strings.TrimSpace(raw)
	if img == "" {
		return "", fmt.Errorf("base_image must not be empty")
	}

	// Split off optional digest (@sha256:...). Anything after the
	// "@" is preserved verbatim — Docker accepts the digest case-
	// sensitively as a content-address.
	digest := ""
	if at := strings.Index(img, "@"); at >= 0 {
		digest = img[at:]
		img = img[:at]
	}

	colon := strings.LastIndex(img, ":")
	if colon < 0 {
		return "", fmt.Errorf("base_image %q: missing :tag (cache correctness requires explicit tag)", raw)
	}
	// Reject port:tag confusion ("host:5000/img" with no tag).
	// Heuristic: if the segment after the last "/" contains no ":",
	// there's no tag. The colon we found is inside an authority.
	lastSlash := strings.LastIndex(img, "/")
	if lastSlash > colon {
		return "", fmt.Errorf("base_image %q: missing :tag", raw)
	}

	name := img[:colon]
	tag := img[colon+1:]
	if tag == "" {
		return "", fmt.Errorf("base_image %q: empty tag", raw)
	}
	if strings.EqualFold(tag, "latest") {
		return "", fmt.Errorf("base_image %q: :latest forbidden (cache correctness requires a pinned tag)", raw)
	}
	name = strings.ToLower(name)
	return name + ":" + tag + digest, nil
}

// EntityID builds the canonical 6-part entity ID for a tenant
// registry record:
//
//	{org}.{platform}.sandbox.tenant.execution.{Hash()}
//
// Sibling to {org}.{platform}.agent.agentic-loop.execution.{loopID}
// and {org}.{platform}.agent.chain.execution.{chainID}. Validated
// against message.IsValidEntityID; panics on malformed input (org
// or platform empty / contains dots) — these are operator-config
// errors that should fail loudly at boot, mirroring upstream's
// LoopExecutionEntityID convention.
func (s TargetSignature) EntityID(org, platform string) string {
	id, err := s.TryEntityID(org, platform)
	if err != nil {
		panic(fmt.Sprintf("sandboxfleet.TargetSignature.EntityID: %s", err))
	}
	return id
}

// TryEntityID is the error-returning variant of EntityID. Use this
// in runtime tool-executor paths where a panic would silently crash
// the dispatch goroutine; surface the error as
// agentic.ToolErrorInternal instead. Boot-time and post-completion
// callers can keep using the panicking EntityID.
func (s TargetSignature) TryEntityID(org, platform string) (string, error) {
	if org == "" || strings.Contains(org, ".") {
		return "", fmt.Errorf("org %q must be non-empty and contain no dots", org)
	}
	if platform == "" || strings.Contains(platform, ".") {
		return "", fmt.Errorf("platform %q must be non-empty and contain no dots", platform)
	}
	id := fmt.Sprintf("%s.%s.sandbox.tenant.execution.%s", org, platform, s.Hash())
	if !message.IsValidEntityID(id) {
		return "", fmt.Errorf("constructed tenant entity id %q failed IsValidEntityID", id)
	}
	return id, nil
}

var (
	whitespaceRun      = regexp.MustCompile(`\s+`)
	versionCorePattern = regexp.MustCompile(`^[0-9]+(\.[0-9]+){0,2}$`)
	// volumeSuffixPattern restricts caller-supplied volume-name
	// suffixes to docker-volume-name-legal characters (lowercase
	// alnum + hyphen). Used by recipe.composeVolumeMounts when
	// rendering Mount.VolumeSuffix into the full
	// "semteams-tenant-<prefix>-<suffix>" volume name.
	volumeSuffixPattern = regexp.MustCompile(`^[a-z0-9-]+$`)
)
