package verification

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/payloadregistry"
)

// evidenceKindPattern is the structural shape every EvidenceRule.Kind
// must match: lowercase ASCII, digits, and underscores; first char must
// be a letter. Catches typo'd or PascalCase emissions at validate-time
// rather than fail-closing them at the gate (R3.7.2.h) with a worse
// error message after a builder cycle has burned. The pattern is
// deliberately tight — operators authoring new Kind values via the
// R3.7.2.e registry inherit the same constraint by convention.
var evidenceKindPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Payload type metadata (domain.category.version).
const (
	Domain        = "verification"
	CategoryCheck = "check"
	SchemaVersion = "v1"
)

// RuntimeKind enumerates the closed set of verification runtimes
// the framework recognises. CLOSED enum: each value has structurally
// distinct framework semantics (different lifecycle, different
// reviewer prompts, different evidence-checker shapes), so adding a
// 6th IS an architectural decision that requires ADR amendment, not
// silent operator extension.
type RuntimeKind string

// RuntimeKind values.
const (
	// RuntimeInProcessUnit — tests against in-language fakes/mocks.
	// No external infrastructure. Cheapest, most common. Goodhart-
	// vulnerable without evidence rules. TestHarness MUST be empty;
	// runtime MAY be set (e.g. go-testing for the language target).
	RuntimeInProcessUnit RuntimeKind = "in-process-unit"

	// RuntimeProcessLocalTestcontainer — tests boot/tear infra in
	// the test process (Go: testcontainers-go, Java: Testcontainers,
	// Python: testcontainers-python). Real bytes, fast iteration.
	// TestHarness REQUIRED (names the testcontainer-style catalog entry).
	// Runtime REQUIRED.
	RuntimeProcessLocalTestcontainer RuntimeKind = "process-local-testcontainer"

	// RuntimeExternalSidecar — tests against a long-lived
	// docker-compose service. Operator pre-boots; chain connects.
	// Real bytes, real protocol. TestHarness REQUIRED (names a sidecar-
	// style catalog entry). Runtime REQUIRED.
	RuntimeExternalSidecar RuntimeKind = "external-sidecar"

	// RuntimeBrowserFlow — Playwright-style human-flow simulation
	// against a known stack. Substance is "the right pages render +
	// the right network calls happen". TestHarness REQUIRED (names the
	// docker-compose stack as a browser-fixture). Runtime REQUIRED
	// (typically playwright-typescript).
	RuntimeBrowserFlow RuntimeKind = "browser-flow"

	// RuntimeStaticAnalysis — type checker, linter, structural
	// property check. No execution. Substance is structural. TestHarness
	// MUST be empty; runtime MAY be set.
	RuntimeStaticAnalysis RuntimeKind = "static-analysis"
)

// IsValid reports whether k is one of the closed-enum values.
func (k RuntimeKind) IsValid() bool {
	switch k {
	case RuntimeInProcessUnit, RuntimeProcessLocalTestcontainer,
		RuntimeExternalSidecar, RuntimeBrowserFlow, RuntimeStaticAnalysis:
		return true
	}
	return false
}

// RequiresTestHarness reports whether a runtime must name a test_harness
// catalog entry. testcontainer/sidecar/browser-flow all need a
// real backing target; unit and static-analysis don't.
func (k RuntimeKind) RequiresTestHarness() bool {
	switch k {
	case RuntimeProcessLocalTestcontainer, RuntimeExternalSidecar, RuntimeBrowserFlow:
		return true
	}
	return false
}

// RequiresRuntime reports whether the runtime kind must name a test
// runtime. Same set as RequiresTestHarness today — every real-stack
// runtime needs a language/test-runner pairing. Kept as a separate
// predicate because static-analysis MAY name a runtime (e.g. for
// language-specific linters) without needing a test_harness.
//
// TODO(R3.7.2.c): collapse this back into RequiresTestHarness if
// static-analysis still doesn't name a runtime by the time the
// runtime registry ships. Don't survive on inertia.
func (k RuntimeKind) RequiresRuntime() bool {
	return k.RequiresTestHarness()
}

// RefType discriminates the union shape of Ref.
// Closed enum: filepath (brownfield) or template_id (greenfield).
type RefType string

// RefType values.
const (
	RefFilepath   RefType = "filepath"
	RefTemplateID RefType = "template_id"
)

// IsValid reports whether the type is one of the closed-enum values.
func (t RefType) IsValid() bool {
	return t == RefFilepath || t == RefTemplateID
}

// Ref points at the reference this check models its tests after.
// Discriminated union by Type:
//
//   - filepath:    Path is a workspace-relative path to an existing
//     test file the chain should pattern-match against.
//     Used for brownfield work where the project
//     already has a convention (e.g. semteams's own
//     cmd/semteams/sandbox/integration_test.go).
//   - template_id: ID names a framework-shipped test template
//     (e.g. "tcp.binary-protobuf.java-junit-testcontainers.v1").
//     Used for greenfield work or when no project
//     pattern fits.
//
// Exactly one of Path / ID must be populated — Validate enforces.
type Ref struct {
	Type RefType `json:"type"`
	Path string  `json:"path,omitempty"`
	ID   string  `json:"id,omitempty"`
}

// EvidenceRule is one structural assertion the evidence gate runs
// post-build against the workspace. Discriminated by Kind; Args is
// kind-specific. The Kind enum is registry-extensible (R3.7.2.e
// ships the registry primitive); v1 of this struct just carries the
// raw shape and trusts the registry to recognise / dispatch the
// kind. Unknown kinds are fail-closed at gate time, NOT at validate
// time, because the registry is composition-extensible at runtime.
type EvidenceRule struct {
	Kind string         `json:"kind"`
	Args map[string]any `json:"args,omitempty"`
}

// Check is the typed payload an architect emits on the
// dev_via_spec.artifact (R3.7.2.b) to specify a verification
// surface. One artifact may carry many checks — typically a
// unit-level one for in-language behaviour and an integration-
// level one against a real test_harness.
//
// Schema: verification.check.v1.
type Check struct {
	// Target is the natural-language description of WHAT is
	// verified by this check. Reviewer judges adequacy
	// against the artifact's overall integration_points.
	Target string `json:"target"`

	// Runtime is the closed-enum kind. Drives required-field
	// validation: RuntimeInProcessUnit forbids TestHarness; others
	// require it.
	Runtime RuntimeKind `json:"runtime"`

	// TestHarness names a catalog entry by its `name` field. Required
	// when Runtime.RequiresTestHarness(); empty otherwise. The
	// existence of the named test_harness is verified at the schema-
	// gate (R3.7.2.e) via catalog lookup, not here.
	TestHarness string `json:"test_harness,omitempty"`

	// TestRuntime names a registered test runtime (e.g.
	// "java-junit-testcontainers", "go-testing-net",
	// "playwright-typescript"). Required when
	// Runtime.RequiresRuntime(); the registry is shipped
	// piecewise per language (R3.7.2.c first; additions are pure
	// adds).
	TestRuntime string `json:"test_runtime,omitempty"`

	// Ref points at the test pattern this check models its
	// rendered tests after. Required for every runtime (greenfield
	// → template_id; brownfield → filepath).
	Ref Ref `json:"ref"`

	// Evidence is the structurally-checkable assertions the
	// evidence gate runs post-build. Required (≥1 rule) per check —
	// without it, the gate cannot mechanically corroborate the
	// builder's claims and the dvs-qa-reviewer (R3.7.2.j′) will
	// terminate with needs_clarification, regardless of how many
	// tests the builder reports passing. Smoke #8 run-8 surfaced
	// this gap: three checks emitted with no rules, builder
	// reported 22/22, qa-reviewer correctly declined to verify.
	//
	// Registered kinds (cmd/semteams/evidence/kinds_*.go):
	//   - test_file_exists       — assert a test source file exists
	//   - test_uses_build_tag    — assert a Go test gates on a build tag
	//   - surefire_passing_count — assert a Maven surefire suite passes ≥N tests
	//
	// JSON tag stays `omitempty` so a serialised Check round-trips
	// without a noisy null when callers explicitly construct the
	// zero value (rare, mostly tests). Validate is the authoritative
	// gate — serialisation does not enforce.
	Evidence []EvidenceRule `json:"evidence,omitempty"`
}

// Schema implements message.Payload.
func (c *Check) Schema() message.Type {
	return message.Type{Domain: Domain, Category: CategoryCheck, Version: SchemaVersion}
}

// Validate enforces structural well-formedness only — closed-enum
// membership, required-field presence per Runtime, Ref shape.
// Catalog-bound checks (does TestHarness exist? does TestRuntime
// support this family?) are the schema gate's job (R3.7.2.e), not
// Validate's, because Validate runs without access to the catalog.
//
// The split mirrors research.Artifact.Validate / dev_via_spec
// .Artifact.Validate: structural always; semantic at the
// reviewer/gate boundary.
func (c *Check) Validate() error {
	if c.Target == "" {
		return fmt.Errorf("target required")
	}
	if !c.Runtime.IsValid() {
		return fmt.Errorf("runtime %q is not a recognised RuntimeKind", c.Runtime)
	}
	if c.Runtime.RequiresTestHarness() && c.TestHarness == "" {
		return fmt.Errorf("runtime %q requires a test_harness", c.Runtime)
	}
	if !c.Runtime.RequiresTestHarness() && c.TestHarness != "" {
		return fmt.Errorf("runtime %q must not name a test_harness (got %q)", c.Runtime, c.TestHarness)
	}
	if c.Runtime.RequiresRuntime() && c.TestRuntime == "" {
		return fmt.Errorf("runtime %q requires a test_runtime", c.Runtime)
	}
	if err := c.Ref.Validate(); err != nil {
		return fmt.Errorf("ref: %w", err)
	}
	// Smoke #8 run-8: qa-reviewer (R3.7.2.j′) requires ≥1 evidence
	// rule per check to render a verdict — without it there is no
	// mechanical basis to corroborate the builder's claims, and the
	// chain wedges at needs_clarification regardless of test count.
	// Reject empty Evidence at the structural layer so the architect
	// cannot ship a check that the gate cannot verify.
	if len(c.Evidence) == 0 {
		return fmt.Errorf("evidence required (≥1 rule); registered kinds: test_file_exists, test_uses_build_tag, surefire_passing_count")
	}
	for i, e := range c.Evidence {
		if e.Kind == "" {
			return fmt.Errorf("evidence[%d].kind required", i)
		}
		if !evidenceKindPattern.MatchString(e.Kind) {
			return fmt.Errorf("evidence[%d].kind %q must match %s (lowercase, digits, underscores; first char a letter)",
				i, e.Kind, evidenceKindPattern.String())
		}
	}
	return nil
}

// Validate enforces Ref's discriminated-union shape:
// Type closed-enum, exactly-one populated path/id field, no
// cross-population.
func (r *Ref) Validate() error {
	if !r.Type.IsValid() {
		return fmt.Errorf("type %q is not one of {filepath, template_id}", r.Type)
	}
	switch r.Type {
	case RefFilepath:
		if r.Path == "" {
			return fmt.Errorf("type=filepath requires non-empty path")
		}
		if r.ID != "" {
			return fmt.Errorf("type=filepath must not set id (got %q)", r.ID)
		}
	case RefTemplateID:
		if r.ID == "" {
			return fmt.Errorf("type=template_id requires non-empty id")
		}
		if r.Path != "" {
			return fmt.Errorf("type=template_id must not set path (got %q)", r.Path)
		}
	}
	return nil
}

// MarshalJSON implements json.Marshaler. Standard alias-pattern; no
// custom shape vs the struct tags. Present for forward-compat —
// future additive fields can land without breaking consumers that
// explicitly round-trip.
func (c *Check) MarshalJSON() ([]byte, error) {
	type Alias Check
	return json.Marshal((*Alias)(c))
}

// UnmarshalJSON implements json.Unmarshaler. See MarshalJSON.
func (c *Check) UnmarshalJSON(data []byte) error {
	type Alias Check
	return json.Unmarshal(data, (*Alias)(c))
}

// RegisterPayloads registers verification.check.v1 with the
// supplied payloadregistry. Call from cmd/semteams/main.go's
// registerProductPayloads after research.RegisterPayloads /
// devviaspec.RegisterPayloads. Same pattern as the other product-
// local payloads.
func RegisterPayloads(reg *payloadregistry.Registry) error {
	return reg.Register(&payloadregistry.Registration{
		Factory:     func() any { return &Check{} },
		Domain:      Domain,
		Category:    CategoryCheck,
		Version:     SchemaVersion,
		Description: "SemTeams verification check — architect's structured check of a verification surface (target / runtime / test_harness / test_runtime / ref / evidence). ADR-036.",
	})
}
