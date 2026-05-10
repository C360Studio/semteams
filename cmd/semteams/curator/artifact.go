// Package curator holds SemTeams-local payload types for the source-curator
// role. The Artifact type is the structured handoff the source-curator emits
// after adding sources, waiting for indexing, and verifying entity IDs. The
// next researcher loop reads it via read_loop_result and queries the verified
// entities directly without re-doing the curator's discovery work.
//
// This is a product-shell payload. The curator concept is SemTeams-specific;
// semstreams has no notion of a curator role. Explicit framework-alignment
// carve-out per ADR-040 §"`emit_curator_artifact` typed payload".
//
// See docs/adr/040-source-curator-role.md for the design rationale and the
// Addendum 2026-05-10 §"SemSource shared source-dir mount" for the
// source_dirs field motivation.
package curator

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/payloadregistry"
)

// Payload type metadata (domain.category.version → "curator.artifact.v1").
const (
	Domain           = "curator"
	CategoryArtifact = "artifact"
	SchemaVersion    = "v1"
)

// AddedSource records one source repo the curator added via add_source_repo.
// All three fields are required — they form the curator's commitment to the
// researcher: "this URL, indexed under this namespace, covers these symbols."
type AddedSource struct {
	URL       string `json:"url"`
	Namespace string `json:"namespace"`
	Covers    string `json:"covers"`
}

// SourceDir records one shared source-dir mount the curator verified as part
// of the indexing-wait phase (ADR-040 addendum 2026-05-10 §item 3). Optional
// at the payload level — deployments without the SemSource shared mount
// omit the entire source_dirs array. When present, all three fields are
// required.
type SourceDir struct {
	Namespace string `json:"namespace"`
	MountPath string `json:"mount_path"`
	Covers    string `json:"covers"`
}

// Artifact is the SemTeams-local payload representing the source-curator's
// terminal output. Emitted via emit_curator_artifact when the curator's
// indexing-wait + entity-ID verification phase completes successfully.
//
// The next researcher loop reads this via read_loop_result and can query
// VerifiedEntityIDs directly without re-doing the curator's discovery work.
// SourceDirs (when present) additionally lets the researcher bash-cat files
// under the shared mount as a fallback when graph queries don't cover the
// case.
//
// Schema: curator.artifact.v1. LoopID and ProducedAt are server-supplied
// by the emit_curator_artifact tool (mirrors research.Artifact and
// devviaspec.Consensus). Source_dirs is omitempty — deployments without
// the SemSource shared mount will not supply it, and the researcher persona
// falls back to graph-only when absent.
//
// Per ADR-040 §"`emit_curator_artifact` typed payload" and addendum
// 2026-05-10 §"SemSource shared source-dir mount".
type Artifact struct {
	LoopID string `json:"loop_id"`

	// AddedSources lists the source repos the curator added this loop.
	// Required (≥1 entry). Each entry is the curator's record of one
	// add_source_repo call: url, namespace, and a short coverage note.
	AddedSources []AddedSource `json:"added_sources"`

	// VerifiedEntityIDs lists the entity IDs the curator confirmed
	// resolve via query_entity after indexing. Required (≥1 entry).
	// These are the curator's commitment to the researcher: "I checked
	// these resolve; you can build on them without re-doing my discovery."
	// Empty verified_entity_ids signals the curator never reached the
	// indexed state — use decide(action="needs_clarification") instead.
	VerifiedEntityIDs []string `json:"verified_entity_ids"`

	// SourceDirs lists the shared source-dir mounts the curator verified
	// as part of the indexing-wait phase. Optional (omitempty). When
	// present, each entry is the curator's commitment to the researcher:
	// "this namespace is mounted at this path; bash-cat works."
	// Deployments without the SemSource shared source-dir mount omit
	// this field. ADR-040 addendum 2026-05-10 §item 3.
	SourceDirs []SourceDir `json:"source_dirs,omitempty"`

	ProducedAt time.Time `json:"produced_at"`
}

// Schema implements message.Payload.
func (a *Artifact) Schema() message.Type {
	return message.Type{Domain: Domain, Category: CategoryArtifact, Version: SchemaVersion}
}

// Validate implements message.Payload. Structural well-formedness only —
// content quality (coverage note accuracy, entity-ID relevance) lives in
// the persona, not here.
//
// Invariants enforced:
//   - loop_id required
//   - added_sources: ≥1 entry; each entry: url, namespace, covers all required
//   - verified_entity_ids: ≥1 entry (empty = not in indexed state; use
//     needs_clarification instead, per ADR-040 §"Curator persona contract")
//   - source_dirs: optional; if present, each entry needs namespace, mount_path,
//     and covers
//   - produced_at: required (non-zero)
func (a *Artifact) Validate() error {
	if a.LoopID == "" {
		return fmt.Errorf("loop_id required")
	}
	if len(a.AddedSources) == 0 {
		return fmt.Errorf("added_sources required (>= 1) — curator must have added at least one source")
	}
	for i, src := range a.AddedSources {
		if src.URL == "" {
			return fmt.Errorf("added_sources[%d].url required", i)
		}
		if src.Namespace == "" {
			return fmt.Errorf("added_sources[%d].namespace required", i)
		}
		if src.Covers == "" {
			return fmt.Errorf("added_sources[%d].covers required", i)
		}
	}
	if len(a.VerifiedEntityIDs) == 0 {
		return fmt.Errorf("verified_entity_ids required (>= 1) — empty means curator never reached the indexed state; use decide(action=\"needs_clarification\") instead")
	}
	for i, dir := range a.SourceDirs {
		if dir.Namespace == "" {
			return fmt.Errorf("source_dirs[%d].namespace required", i)
		}
		if dir.MountPath == "" {
			return fmt.Errorf("source_dirs[%d].mount_path required", i)
		}
		if dir.Covers == "" {
			return fmt.Errorf("source_dirs[%d].covers required", i)
		}
	}
	if a.ProducedAt.IsZero() {
		return fmt.Errorf("produced_at required")
	}
	return nil
}

// MarshalJSON implements json.Marshaler.
func (a *Artifact) MarshalJSON() ([]byte, error) {
	type Alias Artifact
	return json.Marshal((*Alias)(a))
}

// UnmarshalJSON implements json.Unmarshaler.
func (a *Artifact) UnmarshalJSON(data []byte) error {
	type Alias Artifact
	return json.Unmarshal(data, (*Alias)(a))
}

// RegisterPayloads registers SemTeams-local curator payloads with the
// supplied registry. Call from cmd/semteams/product_tools.go after
// payloadbuiltins.Register so the SemTeams-local types layer on top of
// the framework's first-party set.
//
// ADR-040: curator concept is SemTeams-specific; semstreams has no upstream
// primitive to migrate to. Stays product-local.
func RegisterPayloads(reg *payloadregistry.Registry) error {
	return reg.Register(&payloadregistry.Registration{
		Factory:     func() any { return &Artifact{} },
		Domain:      Domain,
		Category:    CategoryArtifact,
		Version:     SchemaVersion,
		Description: "SemTeams curator artifact — structured handoff from source-curator to next researcher. Lists newly-indexed sources, verified entity IDs, and optional shared mount paths. ADR-040.",
	})
}
