package sandboxmanager

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
)

// Profile describes one canonical devcontainer.json + the metadata
// the matcher needs to score it against incoming requirements. The
// actual container internals (base image, devcontainer features,
// postCreateCommand) live in the JSON file at DevcontainerPath; the
// Go-side record carries only what the matcher and admission need:
// what capabilities the profile advertises, what privileges it
// allows, and how long an attestation against it stays fresh.
//
// Profile is a value type. Catalog is the registration surface;
// MatchProfile is the scoring surface. Both are pure functions of
// the catalog and requirements.
type Profile struct {
	// Name is the catalog identifier ("go-backend", "svelte-ui",
	// "full-stack-e2e"). Stamped on attestation as
	// sandbox.attestation.profile = "<name>@<version>".
	Name string

	// Version is a semver-like string the profile author bumps when
	// the underlying devcontainer.json changes in a way that
	// invalidates prior attestations (new base image, feature
	// upgrade, materially different postCreateCommand). Format is
	// free-form by intent — catalog authors pick the convention.
	Version string

	// DevcontainerPath is the repo-relative path to the profile's
	// devcontainer.json. The manager passes it to
	// `devcontainer up --config <path>`. Resolved against the
	// catalog's anchor at LoadCatalog time so relative paths Just
	// Work; absolute paths pass through.
	DevcontainerPath string

	// Languages advertises the language toolchains the materialized
	// container guarantees (post-postCreateCommand). Matched
	// case-insensitively against SandboxRequirements.Languages.
	Languages []string

	// Tools advertises CLI tools the materialized container
	// guarantees. The probe step in RunProbes verifies these are
	// actually present; the catalog claim is the promise, the probe
	// is the attestation.
	Tools []string

	// Services advertises the in-container services the profile
	// stands up (NATS, postgres, etc.). Most profiles have none;
	// full-stack-e2e advertises agentic-e2e stack services.
	Services []string

	// AllowedNetwork is the set of network policies admission will
	// admit on this profile. {restricted, none} for most; public is
	// added explicitly on profiles where the workload requires it.
	AllowedNetwork []NetworkPolicy

	// AllowedPrivileges is the set of privilege classes admission
	// will admit on this profile. Most are empty (no privileges);
	// full-stack-e2e includes docker-socket because the agentic-e2e
	// stack requires DooD.
	AllowedPrivileges []Privilege

	// TTL is how long an attestation against this profile stays
	// fresh. After TTL elapses, the manager re-runs up + probes
	// before returning ready. Zero defaults to 24h (sane for image
	// drift).
	TTL time.Duration
}

// ID returns the canonical "name@version" identifier stamped on
// attestations.
func (p Profile) ID() string {
	return p.Name + "@" + p.Version
}

// DefaultTTL is the fallback TTL when a profile leaves Profile.TTL
// at the zero value. 24 hours matches ADR-043 §Operational state.
// Image upstreams (golang, node) typically publish at most one
// patch release per day, so this catches most drift on next-day
// reuse without paying re-attestation cost intra-day.
const DefaultTTL = 24 * time.Hour

// ResolvedTTL returns p.TTL or DefaultTTL when unset.
func (p Profile) ResolvedTTL() time.Duration {
	if p.TTL <= 0 {
		return DefaultTTL
	}
	return p.TTL
}

// Catalog is the indexed registration surface for profiles. Wraps
// a slice keyed by lowercased Name so MatchProfile and Get are O(N)
// — N is small (3 in MVP). Catalog is read-only after construction;
// Manager holds one for its lifetime.
type Catalog struct {
	profiles []Profile
}

// NewCatalog builds a Catalog from a profile list. Validates that
// names are unique (post-lowercase) and non-empty; that every
// profile has a non-empty Version + DevcontainerPath; and that
// AllowedNetwork is non-empty (default-deny per PR 4.1 finding M5
// requires explicit enumeration). Returns the first failure.
func NewCatalog(profiles []Profile) (*Catalog, error) {
	seen := make(map[string]struct{}, len(profiles))
	for i, p := range profiles {
		name := strings.ToLower(strings.TrimSpace(p.Name))
		if name == "" {
			return nil, fmt.Errorf("profile[%d]: Name must not be empty", i)
		}
		if _, ok := seen[name]; ok {
			return nil, fmt.Errorf("profile[%d]=%q: duplicate name (case-insensitive)", i, p.Name)
		}
		seen[name] = struct{}{}
		if strings.TrimSpace(p.Version) == "" {
			return nil, fmt.Errorf("profile[%q]: Version must not be empty", p.Name)
		}
		if strings.TrimSpace(p.DevcontainerPath) == "" {
			return nil, fmt.Errorf("profile[%q]: DevcontainerPath must not be empty", p.Name)
		}
		if len(p.AllowedNetwork) == 0 {
			return nil, fmt.Errorf("profile[%q]: AllowedNetwork must be non-empty (default-deny — operator must enumerate)", p.Name)
		}
	}
	cat := &Catalog{profiles: make([]Profile, len(profiles))}
	copy(cat.profiles, profiles)
	return cat, nil
}

// Profiles returns a copy of the registered profile list. Stable
// over the catalog's lifetime; tests rely on the order being
// insertion-order.
func (c *Catalog) Profiles() []Profile {
	if c == nil {
		return nil
	}
	out := make([]Profile, len(c.profiles))
	copy(out, c.profiles)
	return out
}

// Get looks up a profile by case-insensitive name. Returns the
// profile + true on hit, zero + false on miss.
func (c *Catalog) Get(name string) (Profile, bool) {
	if c == nil {
		return Profile{}, false
	}
	target := strings.ToLower(strings.TrimSpace(name))
	for _, p := range c.profiles {
		if strings.ToLower(p.Name) == target {
			return p, true
		}
	}
	return Profile{}, false
}

// MatchProfile scores each catalog profile against the requirements
// and returns the highest-scoring profile, or ErrNoProfileMatch
// wrapped with the structured unmet-capability list when nothing
// covers the requirements. Ties broken by catalog order (deterministic).
//
// Scoring:
//
//   - +N points where N = count of language/tool/service requirements
//     satisfied (each separately).
//   - -1 point per advertised capability the requirements did NOT
//     ask for (mild over-provisioning penalty — picks the snuggest
//     fit when two profiles both cover all needs). Network and
//     privileges are admission concerns, not score concerns —
//     they're checked separately so a profile that "covers all
//     needs but doesn't allow docker-socket" deterministically
//     fails admission rather than silently losing a scoring round.
//
// MatchProfile does NOT call AdmissionCheck — it just finds the
// best capability fit. Manager.Request runs admission after match
// so admission denials are loud (their own error class) instead of
// hidden in no-match.
func MatchProfile(req SandboxRequirements, catalog *Catalog) (Profile, error) {
	if catalog == nil || len(catalog.profiles) == 0 {
		return Profile{}, ErrEmptyCatalog
	}

	needLang := lowerSet(req.Languages)
	needTool := lowerSet(req.Tools)
	needSvc := lowerSet(req.Services)

	type scored struct {
		profile Profile
		score   int
		unmet   []string
	}
	scoredAll := make([]scored, 0, len(catalog.profiles))

	for _, p := range catalog.profiles {
		advLang := lowerSet(p.Languages)
		advTool := lowerSet(p.Tools)
		advSvc := lowerSet(p.Services)

		var s int
		var unmet []string

		for k := range needLang {
			if _, ok := advLang[k]; ok {
				s++
			} else {
				unmet = append(unmet, "language:"+k)
			}
		}
		for k := range needTool {
			if _, ok := advTool[k]; ok {
				s++
			} else {
				unmet = append(unmet, "tool:"+k)
			}
		}
		for k := range needSvc {
			if _, ok := advSvc[k]; ok {
				s++
			} else {
				unmet = append(unmet, "service:"+k)
			}
		}

		for k := range advLang {
			if _, ok := needLang[k]; !ok {
				s--
			}
		}
		for k := range advTool {
			if _, ok := needTool[k]; !ok {
				s--
			}
		}
		for k := range advSvc {
			if _, ok := needSvc[k]; !ok {
				s--
			}
		}

		scoredAll = append(scoredAll, scored{profile: p, score: s, unmet: unmet})
	}

	// Pick the highest-scoring entry whose unmet list is empty
	// (covers ALL needs). Ties broken by catalog order.
	var bestIdx = -1
	for i, sc := range scoredAll {
		if len(sc.unmet) > 0 {
			continue
		}
		if bestIdx < 0 || sc.score > scoredAll[bestIdx].score {
			bestIdx = i
		}
	}
	if bestIdx >= 0 {
		return scoredAll[bestIdx].profile, nil
	}

	// No covering profile. Pick the smallest unmet list to include
	// in the error so the Coordinator-facing message highlights what
	// to add to the catalog. Deterministic: catalog order for ties.
	var leastUnmet []string
	for _, sc := range scoredAll {
		if leastUnmet == nil || len(sc.unmet) < len(leastUnmet) {
			leastUnmet = sc.unmet
		}
	}
	sort.Strings(leastUnmet)
	return Profile{}, &NoMatchError{Unmet: leastUnmet}
}

// ErrEmptyCatalog is returned by MatchProfile when the catalog has
// zero profiles. Typically a boot-config error (catalog wasn't
// loaded); the manager surfaces it as a terminal admission failure.
var ErrEmptyCatalog = errors.New("sandboxmanager: profile catalog is empty")

// NoMatchError is returned by MatchProfile when no catalog profile
// covers the requirements. Unmet lists the capabilities (prefixed
// "language:"/"tool:"/"service:") nearest to a match — the operator
// adds a profile that covers them, or the Coordinator re-scopes.
type NoMatchError struct {
	Unmet []string
}

// Error renders a Coordinator-facing message highlighting unmet
// capabilities. The order is sorted+stable so the message is
// reproducible across runs (helpful for log-grep + audit).
func (e *NoMatchError) Error() string {
	if len(e.Unmet) == 0 {
		return "sandboxmanager: no profile match (no unmet detail)"
	}
	return "sandboxmanager: no profile match; unmet capabilities: " + strings.Join(e.Unmet, ", ")
}

// IsNoMatch reports whether err wraps a NoMatchError.
func IsNoMatch(err error) bool {
	var nm *NoMatchError
	return errors.As(err, &nm)
}

func lowerSet(in []string) map[string]struct{} {
	if len(in) == 0 {
		return map[string]struct{}{}
	}
	out := make(map[string]struct{}, len(in))
	for _, s := range in {
		k := strings.ToLower(strings.TrimSpace(s))
		if k == "" {
			continue
		}
		out[k] = struct{}{}
	}
	return out
}

// ProfileAllowsPrivilege reports whether the profile's allowlist
// permits this privilege. Pure on Profile; admission composes this
// with the operator AdmissionPolicy.
func ProfileAllowsPrivilege(p Profile, priv Privilege) bool {
	return slices.Contains(p.AllowedPrivileges, priv)
}

// ProfileAllowsNetwork reports whether the profile's allowlist
// permits this network class. Default-deny: an empty AllowedNetwork
// allows nothing — operators must explicitly enumerate which
// classes a profile supports (per ADR-043 §Decision "default-deny
// on privileges" generalized to network class). The NewCatalog
// constructor enforces non-empty AllowedNetwork at registration
// time so accidentally-empty allowlists fail at boot rather than
// silently denying every request at admission. Per PR 4.1
// go-reviewer finding M5.
func ProfileAllowsNetwork(p Profile, n NetworkPolicy) bool {
	if n == "" {
		n = NetworkRestricted
	}
	return slices.Contains(p.AllowedNetwork, n)
}
