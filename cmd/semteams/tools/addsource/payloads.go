// Package addsource implements the add_source_* tool executors that bridge
// SemTeams agent loops to the SemSource ingest API per
// semsource/docs/adr/0003-programmatic-source-add-api.md.
//
// Wire-format types are duplicated locally rather than imported from
// semsource. The contract lives upstream — we mirror the field shapes to
// keep semteams's go.mod off semsource's transitive-dep graph. If semsource
// ever extends a payload, this file must update in lockstep; the upstream
// ADR pins the contract enough that drift is low risk.
//
// Subjects (per ADR-0003):
//   - graph.ingest.add.{namespace}    request/reply
//   - graph.ingest.remove.{namespace} request/reply (not used in R2 slice 1)
//   - graph.ingest.status              broadcast (caller subscribes to wait
//     for source_status.phase ∈ {watching, idle})
package addsource

import "time"

// SourceType enumerates the source handler types accepted by SemSource.
// Slice 1 only emits "git"; "docs" / "url" types land in a follow-on slice.
const (
	SourceTypeGit  = "git"
	SourceTypeDocs = "docs"
	SourceTypeURL  = "url"
)

// SourceEntry is the request-side source descriptor. Mirrors
// semsource/config/source.go SourceEntry; we expose a narrow subset
// (the fields slice-1's git path needs) and leave the rest unmarshaled
// when SemSource adds new fields. Field tags match upstream verbatim.
type SourceEntry struct {
	// Type identifies the handler. Slice 1 always sets "git".
	Type string `json:"type"`

	// URL is the remote endpoint for git and url source types.
	URL string `json:"url,omitempty"`

	// Branch specifies the git branch to track. Defaults to "main"
	// when the tool caller omits it.
	Branch string `json:"branch,omitempty"`

	// Paths is a list of filesystem paths for docs and config sources.
	// Unused for git but present so future docs/cfgfile slices can
	// build the same SourceEntry shape.
	Paths []string `json:"paths,omitempty"`

	// URLs is a list of HTTP/S URLs for url sources. Same rationale
	// as Paths — present for forward compatibility.
	URLs []string `json:"urls,omitempty"`

	// PollInterval controls how often poll-based sources check origin.
	// Semsource defaults if empty: git=60s, url=300s.
	PollInterval string `json:"poll_interval,omitempty"`

	// Watch enables continuous file-system or network watching.
	Watch bool `json:"watch,omitempty"`
}

// Provenance carries caller-supplied attribution. SemSource v1 logs
// only Actor at INFO; OnBehalfOf and TraceID are dropped on the floor
// but should still be sent so future SemSource versions can persist
// them without a wire change. Per ADR-030 (semteams) the ctx-lifted
// X-User-Id flows into OnBehalfOf — Actor identifies the agent that
// originated the call (e.g., "semteams.researcher").
type Provenance struct {
	Actor      string `json:"actor,omitempty"`
	OnBehalfOf string `json:"on_behalf_of,omitempty"`
	TraceID    string `json:"trace_id,omitempty"`
}

// AddRequest is the body sent to graph.ingest.add.{namespace}. The
// upstream payload uses `omitzero` on Provenance for the same
// "nested struct field" reason — we mirror that for wire-compatible
// marshaling (omitzero fires only when every Provenance field is
// zero; in practice the executor always sets Actor, so the field
// always serializes — that matches upstream behaviour for any caller
// stamping Actor).
type AddRequest struct {
	Source     SourceEntry `json:"source"`
	Provenance Provenance  `json:"provenance,omitzero"`
}

// AddedComponent describes one component spawned by an add. A flat
// source produces one; a "repo" meta-source produces multiple. For
// slice 1's git path expect Components to have length 1.
type AddedComponent struct {
	InstanceName string `json:"instance_name"`
	FactoryName  string `json:"factory_name"`
	SourceType   string `json:"source_type"`
	Created      bool   `json:"created"`
}

// IngestErrorCode is a typed error code returned in reply payloads.
// Mirrors semsource/processor/source-manifest/payload_ingest.go IngestErrorCode.
type IngestErrorCode string

const (
	// CodeValidationFailed indicates the SourceEntry did not pass
	// type-specific validation. Not retryable.
	CodeValidationFailed IngestErrorCode = "VALIDATION_FAILED"

	// CodeInstanceExists indicates a component with the deterministic
	// instance name already exists with a different config. Idempotent
	// re-submits with matching configs return Created=false and no
	// error instead.
	CodeInstanceExists IngestErrorCode = "INSTANCE_EXISTS"

	// CodeKVWriteFailed indicates the underlying ConfigManager KV write
	// failed. Retryable.
	CodeKVWriteFailed IngestErrorCode = "KV_WRITE_FAILED"

	// CodeUnsupportedType indicates the SourceEntry.Type is not
	// implementable through this API in the current SemSource build.
	CodeUnsupportedType IngestErrorCode = "UNSUPPORTED_TYPE"
)

// IngestError is the typed error envelope embedded in AddReply.
type IngestError struct {
	Code    IngestErrorCode `json:"code"`
	Message string          `json:"message"`
}

// AddReply is the response on the request/reply round-trip. Components
// is empty when Error is non-nil. ReadyWhen describes the condition the
// caller can poll on StatusSubject to know the source is graph-queryable.
type AddReply struct {
	Components    []AddedComponent `json:"components,omitempty"`
	StatusSubject string           `json:"status_subject,omitempty"`
	ReadyWhen     string           `json:"ready_when,omitempty"`
	Error         *IngestError     `json:"error,omitempty"`
	Timestamp     time.Time        `json:"timestamp"`
}
