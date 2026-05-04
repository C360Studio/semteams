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
	Domain             = "verification"
	CategoryCommitment = "commitment"
	SchemaVersion      = "v1"
)

// ApproachKind enumerates the closed set of verification approaches
// the framework recognises. CLOSED enum: each value has structurally
// distinct framework semantics (different lifecycle, different
// reviewer prompts, different evidence-checker shapes), so adding a
// 6th IS an architectural decision that requires ADR amendment, not
// silent operator extension.
type ApproachKind string

// ApproachKind values.
const (
	// ApproachInProcessUnit — tests against in-language fakes/mocks.
	// No external infrastructure. Cheapest, most common. Goodhart-
	// vulnerable without evidence rules. Harness MUST be empty;
	// runtime MAY be set (e.g. go-testing for the language target).
	ApproachInProcessUnit ApproachKind = "in-process-unit"

	// ApproachProcessLocalTestcontainer — tests boot/tear infra in
	// the test process (Go: testcontainers-go, Java: Testcontainers,
	// Python: testcontainers-python). Real bytes, fast iteration.
	// Harness REQUIRED (names the testcontainer-style catalog entry).
	// Runtime REQUIRED.
	ApproachProcessLocalTestcontainer ApproachKind = "process-local-testcontainer"

	// ApproachExternalSidecar — tests against a long-lived
	// docker-compose service. Operator pre-boots; chain connects.
	// Real bytes, real protocol. Harness REQUIRED (names a sidecar-
	// style catalog entry). Runtime REQUIRED.
	ApproachExternalSidecar ApproachKind = "external-sidecar"

	// ApproachBrowserFlow — Playwright-style human-flow simulation
	// against a known stack. Substance is "the right pages render +
	// the right network calls happen". Harness REQUIRED (names the
	// docker-compose stack as a browser-fixture). Runtime REQUIRED
	// (typically playwright-typescript).
	ApproachBrowserFlow ApproachKind = "browser-flow"

	// ApproachStaticAnalysis — type checker, linter, structural
	// property check. No execution. Substance is structural. Harness
	// MUST be empty; runtime MAY be set.
	ApproachStaticAnalysis ApproachKind = "static-analysis"
)

// IsValid reports whether k is one of the closed-enum values.
func (k ApproachKind) IsValid() bool {
	switch k {
	case ApproachInProcessUnit, ApproachProcessLocalTestcontainer,
		ApproachExternalSidecar, ApproachBrowserFlow, ApproachStaticAnalysis:
		return true
	}
	return false
}

// RequiresHarness reports whether an approach must name a harness
// catalog entry. testcontainer/sidecar/browser-flow all need a
// real backing target; unit and static-analysis don't.
func (k ApproachKind) RequiresHarness() bool {
	switch k {
	case ApproachProcessLocalTestcontainer, ApproachExternalSidecar, ApproachBrowserFlow:
		return true
	}
	return false
}

// RequiresRuntime reports whether the approach must name a test
// runtime. Same set as RequiresHarness today — every real-stack
// approach needs a language/test-runner pairing. Kept as a separate
// predicate because static-analysis MAY name a runtime (e.g. for
// language-specific linters) without needing a harness.
//
// TODO(R3.7.2.c): collapse this back into RequiresHarness if
// static-analysis still doesn't name a runtime by the time the
// runtime registry ships. Don't survive on inertia.
func (k ApproachKind) RequiresRuntime() bool {
	return k.RequiresHarness()
}

// ConventionRefType discriminates the union shape of ConventionRef.
// Closed enum: filepath (brownfield) or template_id (greenfield).
type ConventionRefType string

// ConventionRefType values.
const (
	ConventionFilepath   ConventionRefType = "filepath"
	ConventionTemplateID ConventionRefType = "template_id"
)

// IsValid reports whether the type is one of the closed-enum values.
func (t ConventionRefType) IsValid() bool {
	return t == ConventionFilepath || t == ConventionTemplateID
}

// ConventionRef points at the convention this commitment models its
// tests after. Discriminated union by Type:
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
type ConventionRef struct {
	Type ConventionRefType `json:"type"`
	Path string            `json:"path,omitempty"`
	ID   string            `json:"id,omitempty"`
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

// Commitment is the typed payload an architect emits on the
// dev_via_spec.artifact (R3.7.2.b) to commit to a verification
// surface. One artifact may carry many commitments — typically a
// unit-level one for in-language behaviour and an integration-
// level one against a real harness.
//
// Schema: verification.commitment.v1.
type Commitment struct {
	// Target is the natural-language description of WHAT is
	// verified by this commitment. Reviewer judges adequacy
	// against the artifact's overall integration_points.
	Target string `json:"target"`

	// Approach is the closed-enum kind. Drives required-field
	// validation: ApproachInProcessUnit forbids Harness; others
	// require it.
	Approach ApproachKind `json:"approach"`

	// Harness names a catalog entry by its `name` field. Required
	// when Approach.RequiresHarness(); empty otherwise. The
	// existence of the named harness is verified at the schema-
	// gate (R3.7.2.e) via catalog lookup, not here.
	Harness string `json:"harness,omitempty"`

	// Runtime names a registered test runtime (e.g.
	// "java-junit-testcontainers", "go-testing-net",
	// "playwright-typescript"). Required when
	// Approach.RequiresRuntime(); the registry is shipped
	// piecewise per language (R3.7.2.c first; additions are pure
	// adds).
	Runtime string `json:"runtime,omitempty"`

	// Convention points at the test pattern this commitment
	// models its rendered tests after. Required for every
	// approach (greenfield → template_id; brownfield → filepath).
	Convention ConventionRef `json:"convention"`

	// Evidence is the structurally-checkable assertions the
	// evidence gate runs post-build. May be empty in v1 if the
	// commitment relies entirely on the framework's per-runtime
	// template (which is itself structurally constrained), but
	// then the reviewer will likely flag the commitment as
	// under-specified for non-template approaches.
	Evidence []EvidenceRule `json:"evidence,omitempty"`
}

// Schema implements message.Payload.
func (c *Commitment) Schema() message.Type {
	return message.Type{Domain: Domain, Category: CategoryCommitment, Version: SchemaVersion}
}

// Validate enforces structural well-formedness only — closed-enum
// membership, required-field presence per Approach, ConventionRef
// shape. Catalog-bound checks (does Harness exist? does Runtime
// support this family?) are the schema gate's job (R3.7.2.e), not
// Validate's, because Validate runs without access to the catalog.
//
// The split mirrors research.Artifact.Validate / dev_via_spec
// .Artifact.Validate: structural always; semantic at the
// reviewer/gate boundary.
func (c *Commitment) Validate() error {
	if c.Target == "" {
		return fmt.Errorf("target required")
	}
	if !c.Approach.IsValid() {
		return fmt.Errorf("approach %q is not a recognised ApproachKind", c.Approach)
	}
	if c.Approach.RequiresHarness() && c.Harness == "" {
		return fmt.Errorf("approach %q requires a harness", c.Approach)
	}
	if !c.Approach.RequiresHarness() && c.Harness != "" {
		return fmt.Errorf("approach %q must not name a harness (got %q)", c.Approach, c.Harness)
	}
	if c.Approach.RequiresRuntime() && c.Runtime == "" {
		return fmt.Errorf("approach %q requires a runtime", c.Approach)
	}
	if err := c.Convention.Validate(); err != nil {
		return fmt.Errorf("convention: %w", err)
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

// Validate enforces ConventionRef's discriminated-union shape:
// Type closed-enum, exactly-one populated path/id field, no
// cross-population.
func (r *ConventionRef) Validate() error {
	if !r.Type.IsValid() {
		return fmt.Errorf("type %q is not one of {filepath, template_id}", r.Type)
	}
	switch r.Type {
	case ConventionFilepath:
		if r.Path == "" {
			return fmt.Errorf("type=filepath requires non-empty path")
		}
		if r.ID != "" {
			return fmt.Errorf("type=filepath must not set id (got %q)", r.ID)
		}
	case ConventionTemplateID:
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
func (c *Commitment) MarshalJSON() ([]byte, error) {
	type Alias Commitment
	return json.Marshal((*Alias)(c))
}

// UnmarshalJSON implements json.Unmarshaler. See MarshalJSON.
func (c *Commitment) UnmarshalJSON(data []byte) error {
	type Alias Commitment
	return json.Unmarshal(data, (*Alias)(c))
}

// RegisterPayloads registers verification.commitment.v1 with the
// supplied payloadregistry. Call from cmd/semteams/main.go's
// registerProductPayloads after research.RegisterPayloads /
// devviaspec.RegisterPayloads. Same pattern as the other product-
// local payloads.
func RegisterPayloads(reg *payloadregistry.Registry) error {
	return reg.Register(&payloadregistry.Registration{
		Factory:     func() any { return &Commitment{} },
		Domain:      Domain,
		Category:    CategoryCommitment,
		Version:     SchemaVersion,
		Description: "SemTeams verification commitment — architect's structured commitment to a verification surface (target / approach / harness / runtime / convention / evidence). ADR-033 §addendum 2026-05-04.",
	})
}
