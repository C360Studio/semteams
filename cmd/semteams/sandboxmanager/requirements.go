package sandboxmanager

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// SandboxRequirements is the typed capability contract Coordinator
// emits when it needs a prepared environment. It is the renamed +
// reframed PR 3.3 RecipeIntent — capability contract, NOT recipe.
// Field shapes are domain-shaped (what the work needs):
//
//   - Languages, Tools, Services: capability tokens the work uses.
//     The matcher scores Profiles against the union.
//   - Network: policy class, not a port list. Profiles + admission
//     decide what "public" actually means at materialization time.
//   - Secrets: identifier-shaped names; the operator pre-provisions
//     them via boot config, never per-request.
//   - Mounts: enum classes ("workspace-write" etc.) — not host paths.
//     Host-path bind mounts go through admission as a privileged
//     requirement, not as a first-class field.
//   - Privileges: enum classes (docker-socket, privileged) — gated
//     by admission against per-profile allowlists.
//   - Verification: probes the manager runs after `devcontainer up`
//     to attest verified capabilities. Probe outcomes drive the
//     ready/degraded branching the Coordinator routes on.
//
// Fields are intentionally domain-shaped (NOT container-shaped: no
// image, no mounts as paths, no privileges as docker-CLI flags).
// Those concerns live inside Profiles (catalog policy) or
// AdmissionPolicy (operator policy). Per ADR-043 §Decision: "We do
// NOT let the manager's internals leak into Coordinator prose; we
// do NOT let Coordinator emit container-level intent."
type SandboxRequirements struct {
	Languages    []string      `json:"languages,omitempty"`
	Tools        []string      `json:"tools,omitempty"`
	Services     []string      `json:"services,omitempty"`
	Network      NetworkPolicy `json:"network,omitempty"`
	Secrets      []string      `json:"secrets,omitempty"`
	Mounts       []MountClass  `json:"mounts,omitempty"`
	Privileges   []Privilege   `json:"privileges,omitempty"`
	Verification []VerifyProbe `json:"verification,omitempty"`
}

// NetworkPolicy enumerates the network classes the Coordinator may
// declare. Profiles advertise what they support; admission gates
// public against operator policy.
type NetworkPolicy string

// NetworkPolicy enum values. Wire-form is the lowercased string;
// rule packs and personas reference them by exact value. The zero
// value is NetworkRestricted (safer default than zero-meaning-any).
const (
	NetworkRestricted NetworkPolicy = "restricted"
	NetworkPublic     NetworkPolicy = "public"
	NetworkNone       NetworkPolicy = "none"
)

// MountClass enumerates the structured mount intents Coordinator
// may declare. workspace-write is the typical case (the tenant
// gets a writable workspace volume). Adding host-path bind mounts
// is intentionally NOT a first-class enum value — those go via the
// "privileged" admission path because they expand the blast radius.
type MountClass string

// MountClass enum values. workspace-write is the default for any
// build/test workload; workspace-read is the audit/inspect case
// where the manager mounts a read-only snapshot.
const (
	MountWorkspaceWrite MountClass = "workspace-write"
	MountWorkspaceRead  MountClass = "workspace-read"
)

// Privilege enumerates the privileged capability classes. Each is
// gated by AdmissionPolicy against a per-profile allowlist — default
// deny.
type Privilege string

// Privilege enum values. docker-socket mounts /var/run/docker.sock
// (testcontainers / docker-compose workloads). privileged is full
// --privileged container mode — forbidden by default; operator must
// flip a boot-time flag AND the chain must clear admission-review.
const (
	PrivilegeDockerSocket Privilege = "docker-socket"
	PrivilegePrivileged   Privilege = "privileged"
)

// VerifyProbe is one preflight probe the manager runs inside the
// materialized container before emitting attestation. Probes assert
// verified capabilities: "go version" exits 0 + stdout matches the
// expected major; "task --list" exits 0; etc.
//
// Name is the capability identifier the attestation triples key on
// (sandbox.attestation.verified-<name>). Must be an identifier-like
// token so it composes into the triple-predicate suffix without
// quoting (per [[schema-shape-for-cross-field-constraints]]).
//
// Command is the verbatim shell line run via `devcontainer exec`.
// This is one of the legitimate cases where a string IS user-intent
// (the probe shape IS its command), per the
// [[personas-should-not-author-shell]] exception clause —
// Coordinator authoring "task build" as a probe command is correct,
// not the smell shape PR 3.3 closed.
type VerifyProbe struct {
	Name       string `json:"name"`
	Command    string `json:"command"`
	ExpectExit int    `json:"expect_exit,omitempty"`
}

// Validate enforces the Coordinator-facing contract. Returns the
// first failure. Idempotent: callers may invoke after Normalize
// (Validate does not mutate). Invariants enforced:
//
//   - Capability slices: non-empty tokens, no whitespace, no
//     duplicates (after lowercasing).
//   - Network: empty (zero-value treated as restricted at materialize
//     time) or a known enum value.
//   - Mounts/Privileges: known enum values; no duplicates.
//   - Secrets: identifier-shaped names so the operator pre-config
//     can key them by exact match without quoting (the manager
//     looks them up via the operator's pre-provisioned env vars at
//     `devcontainer up` time).
//   - Probes: unique Names (capability namespace can't collide);
//     non-empty Commands; identifier-shaped Name so it composes
//     into the attestation predicate without quoting.
//
// Validate is the structural seam — admission policy decisions
// (privilege allowlists, public-network operator approval) belong
// in AdmissionCheck, not here.
func (r SandboxRequirements) Validate() error {
	if err := validateTokenSlice("languages", r.Languages); err != nil {
		return err
	}
	if err := validateTokenSlice("tools", r.Tools); err != nil {
		return err
	}
	if err := validateTokenSlice("services", r.Services); err != nil {
		return err
	}
	if err := validateNetwork(r.Network); err != nil {
		return err
	}
	if err := validateSecrets(r.Secrets); err != nil {
		return err
	}
	if err := validateMounts(r.Mounts); err != nil {
		return err
	}
	if err := validatePrivileges(r.Privileges); err != nil {
		return err
	}
	return validateProbes(r.Verification)
}

// Normalize returns a copy with capability slices lowercased,
// trimmed, deduplicated, and sorted; network defaulted to
// restricted when zero; secrets sorted; mounts/privileges
// deduplicated and sorted. Idempotent — Normalize(Normalize(r)) =
// Normalize(r). Hash determinism depends on Normalize: two
// equivalent requirements MUST normalize to the same shape so
// their Hashes match and the per-(profile, requirements_hash)
// cache key collapses to the same lookup.
//
// Does NOT call Validate — callers run Validate first, Normalize
// second; an invalid Requirements should fail Validate before any
// hashing.
func (r SandboxRequirements) Normalize() SandboxRequirements {
	out := SandboxRequirements{
		Languages: normalizeTokens(r.Languages),
		Tools:     normalizeTokens(r.Tools),
		Services:  normalizeTokens(r.Services),
		Network:   r.Network,
		Secrets:   normalizeTokens(r.Secrets),
	}
	if out.Network == "" {
		out.Network = NetworkRestricted
	}
	if len(r.Mounts) > 0 {
		mounts := make([]MountClass, 0, len(r.Mounts))
		seen := make(map[MountClass]struct{}, len(r.Mounts))
		for _, m := range r.Mounts {
			if _, ok := seen[m]; ok {
				continue
			}
			seen[m] = struct{}{}
			mounts = append(mounts, m)
		}
		slices.Sort(mounts)
		out.Mounts = mounts
	}
	if len(r.Privileges) > 0 {
		privs := make([]Privilege, 0, len(r.Privileges))
		seen := make(map[Privilege]struct{}, len(r.Privileges))
		for _, p := range r.Privileges {
			if _, ok := seen[p]; ok {
				continue
			}
			seen[p] = struct{}{}
			privs = append(privs, p)
		}
		slices.Sort(privs)
		out.Privileges = privs
	}
	if len(r.Verification) > 0 {
		probes := make([]VerifyProbe, len(r.Verification))
		for i, p := range r.Verification {
			probes[i] = VerifyProbe{
				Name:       strings.ToLower(strings.TrimSpace(p.Name)),
				Command:    strings.TrimSpace(p.Command),
				ExpectExit: p.ExpectExit,
			}
		}
		sort.Slice(probes, func(i, j int) bool { return probes[i].Name < probes[j].Name })
		out.Verification = probes
	}
	return out
}

// Hash returns a stable hex SHA-256 of the Normalize()'d shape.
// Two equivalent SandboxRequirements (same capabilities described
// in different field order / case) produce the same Hash. Used as
// half of the attestation cache key — the other half is the
// Profile name@version.
//
// Determinism: encoding/json marshals struct fields in declaration
// order; slices are pre-sorted by Normalize. No maps in the
// requirements struct, so no map-key-order risk. We panic on
// json.Marshal failure — under the current struct shape it cannot
// fail, but a future field carrying a custom json.Marshaler with a
// bug would otherwise silently collide every requirement on
// sha256("") (smoke #12 lesson — silent-empty in load-bearing
// substitution surfaces is the failure shape we keep eating).
func (r SandboxRequirements) Hash() string {
	b, err := json.Marshal(r.Normalize())
	if err != nil {
		panic(fmt.Sprintf("sandboxmanager: SandboxRequirements.Hash marshal: %v (programmer error — a field marshaler regressed)", err))
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// HasPrivilege reports whether p is in r.Privileges (post-normalize
// case-insensitive). Helper for admission rules and probe wiring.
func (r SandboxRequirements) HasPrivilege(p Privilege) bool {
	return slices.Contains(r.Privileges, p)
}

func validateTokenSlice(field string, in []string) error {
	for i, t := range in {
		if strings.TrimSpace(t) == "" {
			return fmt.Errorf("%s[%d]: must not be empty / whitespace", field, i)
		}
		if !tokenPattern.MatchString(strings.ToLower(strings.TrimSpace(t))) {
			return fmt.Errorf("%s[%d]=%q: must match [a-z0-9][a-z0-9._-]* (identifier shape)", field, i, t)
		}
	}
	return nil
}

func validateNetwork(n NetworkPolicy) error {
	switch n {
	case "", NetworkRestricted, NetworkPublic, NetworkNone:
		return nil
	default:
		return fmt.Errorf("network %q: must be one of {restricted, public, none}", n)
	}
}

func validateSecrets(in []string) error {
	for i, s := range in {
		if strings.TrimSpace(s) == "" {
			return fmt.Errorf("secrets[%d]: must not be empty / whitespace", i)
		}
		if !secretPattern.MatchString(strings.TrimSpace(s)) {
			return fmt.Errorf("secrets[%d]=%q: must match [A-Z_][A-Z0-9_]* (env-var identifier shape)", i, s)
		}
	}
	return nil
}

func validateMounts(in []MountClass) error {
	for i, m := range in {
		switch m {
		case MountWorkspaceWrite, MountWorkspaceRead:
		default:
			return fmt.Errorf("mounts[%d]=%q: must be one of {workspace-write, workspace-read}", i, m)
		}
	}
	return nil
}

func validatePrivileges(in []Privilege) error {
	for i, p := range in {
		switch p {
		case PrivilegeDockerSocket, PrivilegePrivileged:
		default:
			return fmt.Errorf("privileges[%d]=%q: must be one of {docker-socket, privileged}", i, p)
		}
	}
	return nil
}

func validateProbes(in []VerifyProbe) error {
	seen := make(map[string]struct{}, len(in))
	for i, p := range in {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			return fmt.Errorf("verification[%d].name: must not be empty", i)
		}
		if !tokenPattern.MatchString(strings.ToLower(name)) {
			return fmt.Errorf("verification[%d].name=%q: must match [a-z0-9][a-z0-9._-]* (composes into attestation predicate)", i, name)
		}
		lc := strings.ToLower(name)
		if _, ok := seen[lc]; ok {
			return fmt.Errorf("verification[%d].name=%q: duplicate (post-lowercase)", i, name)
		}
		seen[lc] = struct{}{}
		cmd := strings.TrimSpace(p.Command)
		if cmd == "" {
			return fmt.Errorf("verification[%d].command: must not be empty", i)
		}
		// Reject control characters in probe commands. NUL would
		// truncate the shell line at exec; other controls (ANSI
		// escape \x1b, etc.) round-trip into Verified[name] stdout
		// → triples → LLM prompts → triggers downstream
		// rendering/parsing surprises. Tab/newline excluded — a
		// probe authoring `printf 'a\tb'` is fine. Per PR 4.1
		// finding N3.
		for _, r := range cmd {
			if r == 0 {
				return fmt.Errorf("verification[%d].command: contains NUL byte", i)
			}
			if r < 0x20 && r != '\t' && r != '\n' {
				return fmt.Errorf("verification[%d].command: contains control character %#x", i, r)
			}
			if r == 0x7f {
				return fmt.Errorf("verification[%d].command: contains DEL (0x7f)", i)
			}
		}
	}
	return nil
}

func normalizeTokens(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, t := range in {
		tok := strings.ToLower(strings.TrimSpace(t))
		if tok == "" {
			continue
		}
		if _, ok := seen[tok]; ok {
			continue
		}
		seen[tok] = struct{}{}
		out = append(out, tok)
	}
	sort.Strings(out)
	return out
}

var (
	tokenPattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	secretPattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
)
