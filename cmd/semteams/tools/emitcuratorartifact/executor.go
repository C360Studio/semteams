// Package emitcuratorartifact implements the SemTeams-local
// emit_curator_artifact tool. The tool is the source-curator role's terminal
// handoff mechanism: it publishes the typed curator.artifact.v1 payload on a
// stable JetStream subject and mints five marker triples on the curator's loop
// entity. The payload is runtime data consumed by the next researcher loop via
// read_loop_result — no markdown render, no output directory. The artifact
// commitment carries newly-indexed source repos, verified entity IDs, and
// (when deployed with the SemSource shared source-dir mount) mount-path
// metadata the researcher can bash-cat against.
//
// Framework-alignment carve-out (CLAUDE.md product-shell-tool discipline):
// the curator concept is SemTeams-specific; semstreams has no notion of a
// curator role. Surveyed upstream at beta.57 — no equivalent primitive
// planned or shipped. No migration target. Documented in ADR-040
// §"`emit_curator_artifact` typed payload".
//
// See docs/adr/040-source-curator-role.md for the design rationale.
package emitcuratorartifact

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	agentictools "github.com/c360studio/semstreams/processor/agentic-tools"
	"github.com/c360studio/semstreams/types"

	"github.com/c360studio/semteams/cmd/semteams/curator"
)

// ToolName is the LLM-facing tool name.
const ToolName = "emit_curator_artifact"

// toolSource tags the triples this tool publishes.
const toolSource = "source-curator-emit-artifact"

// payloadSubjectPrefix is the JetStream subject namespace under which the
// typed Artifact payload is published. The published subject is
// curator.artifact.{loop_id}.
const payloadSubjectPrefix = "curator.artifact"

// Predicate names stamped on the curator's loop entity. Five in total —
// counts + namespaces index + timestamp (no path triple: the artifact is
// runtime data, not a rendered file).
const (
	predicateAddedSourcesCount      = "curator.artifact.added_sources_count"
	predicateVerifiedEntityIDsCount = "curator.artifact.verified_entity_ids_count"
	predicateSourceDirsCount        = "curator.artifact.source_dirs_count"
	predicateNamespaces             = "curator.artifact.namespaces"
	predicateGeneratedAt            = "curator.artifact.generated_at"
)

// PayloadPublisher is the narrow surface for publishing the typed Artifact
// payload onto a stable NATS subject. Same interface shape as emitconsensus
// and emitartifact.
type PayloadPublisher interface {
	Publish(ctx context.Context, subject string, data []byte) error
}

// Executor implements agentic.ToolExecutor for emit_curator_artifact.
type Executor struct {
	publisher   agentictools.TriplePublisher
	natsPublish PayloadPublisher
	platform    types.PlatformMeta
	logger      *slog.Logger
}

// NewExecutor constructs an Executor.
func NewExecutor(publisher agentictools.TriplePublisher, natsPublish PayloadPublisher, platform types.PlatformMeta, logger *slog.Logger) *Executor {
	if logger == nil {
		logger = slog.Default()
	}
	return &Executor{
		publisher:   publisher,
		natsPublish: natsPublish,
		platform:    platform,
		logger:      logger,
	}
}

// ListTools returns the LLM-facing schema. Args mirror the Artifact JSON
// shape directly; loop_id and produced_at are server-supplied.
func (e *Executor) ListTools() []agentic.ToolDefinition {
	addedSourceSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url":       map[string]any{"type": "string", "description": "The same URL passed to add_source_repo."},
			"namespace": map[string]any{"type": "string", "description": "The SemSource namespace for this source."},
			"covers":    map[string]any{"type": "string", "description": "One or two phrases naming the symbols, packages, or files this source contributes. Keep it concrete — the next researcher uses this to decide which source to query first."},
		},
		"required": []string{"url", "namespace", "covers"},
	}
	sourceDirSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"namespace":  map[string]any{"type": "string", "description": "Same namespace as the entry in added_sources."},
			"mount_path": map[string]any{"type": "string", "description": "Container-absolute path where the source directory is mounted (e.g. /sources/osh-core)."},
			"covers":     map[string]any{"type": "string", "description": "Short phrase describing what in this directory is worth a bash cat (build configs, top-level READMEs, test fixture files)."},
		},
		"required": []string{"namespace", "mount_path", "covers"},
	}
	return []agentic.ToolDefinition{{
		Name:        ToolName,
		Description: "Emit the source-curator's terminal handoff artifact. Records the sources you added (add_source_repo), the entity IDs you verified resolve (query_entity), and optionally the shared mount paths available for bash-cat fallback. Call once before decide(action=\"indexed\"). Do NOT call when terminating with needs_clarification — no sources were added, no entity IDs verified, so there is nothing to emit.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"added_sources": map[string]any{
					"type":        "array",
					"items":       addedSourceSchema,
					"description": "One entry per add_source_repo call made in this loop. Required (>= 1). Each entry carries url, namespace, and a short coverage note.",
				},
				"verified_entity_ids": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Entity IDs you successfully resolved via query_entity after indexing finished. Required (>= 1). These are your commitment to the next researcher: 'I checked these resolve; you can query them directly.' Do not include IDs you did not actually verify.",
				},
				"source_dirs": map[string]any{
					"type":        "array",
					"items":       sourceDirSchema,
					"description": "Optional. Include only when the deployment uses the SemSource shared source-dir mount and you verified the directories are accessible. Each entry: namespace, mount_path (container-absolute), covers (what's worth a bash cat). Omit if the deployment has no shared mount.",
				},
			},
			"required": []string{"added_sources", "verified_entity_ids"},
		},
	}}
}

// Execute parses, validates, stamps triples, publishes payload.
func (e *Executor) Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
	if call.Name != ToolName {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     fmt.Sprintf("unknown tool: %s", call.Name),
			ErrorKind: agentic.ToolErrorNotFound,
		}, nil
	}
	if call.LoopID == "" {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     "emit_curator_artifact invoked without a loop_id; cannot resolve calling loop entity",
			ErrorKind: agentic.ToolErrorInternal,
		}, nil
	}

	artifact, err := parseArgsIntoArtifact(call.Arguments, call.LoopID)
	if err != nil {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     err.Error(),
			ErrorKind: agentic.ToolErrorInvalidArgs,
		}, nil
	}
	if err := artifact.Validate(); err != nil {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     fmt.Sprintf("curator artifact validation failed: %v", err),
			ErrorKind: agentic.ToolErrorInvalidArgs,
		}, nil
	}

	now := artifact.ProducedAt
	loopEntityID := agentic.LoopExecutionEntityID(e.platform.Org, e.platform.Platform, call.LoopID)

	// Publish payload first — it's the substantive handoff. Triple-stamp
	// comes after so a publish failure surfaces before any triple writes.
	payloadBytes, err := json.Marshal(artifact)
	if err != nil {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     fmt.Sprintf("marshal curator artifact payload: %v", err),
			ErrorKind: agentic.ToolErrorInternal,
		}, nil
	}

	subject := payloadSubjectPrefix + "." + call.LoopID
	if err := e.natsPublish.Publish(ctx, subject, payloadBytes); err != nil {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     fmt.Sprintf("publish curator artifact payload to %s: %v", subject, err),
			ErrorKind: agentic.ToolErrorNetwork,
		}, nil
	}

	triples := buildTriples(loopEntityID, artifact, now)
	for _, triple := range triples {
		if err := e.publisher.AddTriple(ctx, triple); err != nil {
			return agentic.ToolResult{
				CallID:    call.ID,
				Name:      call.Name,
				Error:     fmt.Sprintf("publish %s triple: %v", triple.Predicate, err),
				ErrorKind: agentic.ToolErrorNetwork,
			}, nil
		}
	}

	e.logger.Info("emit_curator_artifact published",
		slog.String("loop_id", artifact.LoopID),
		slog.Int("added_sources", len(artifact.AddedSources)),
		slog.Int("verified_entity_ids", len(artifact.VerifiedEntityIDs)),
		slog.Int("source_dirs", len(artifact.SourceDirs)),
		slog.String("subject", subject))

	return agentic.ToolResult{
		CallID:  call.ID,
		Name:    call.Name,
		Content: string(payloadBytes),
		Metadata: map[string]any{
			"loop_entity_id":  loopEntityID,
			"payload_subject": subject,
		},
	}, nil
}

// parseArgsIntoArtifact builds a curator.Artifact from LLM args.
// loop_id and produced_at are server-supplied; any LLM-claimed values
// for those fields are stripped (mirrors emitartifact / emitconsensus).
func parseArgsIntoArtifact(raw map[string]any, loopID string) (*curator.Artifact, error) {
	clean := make(map[string]any, len(raw))
	for k, v := range raw {
		if k == "loop_id" || k == "produced_at" {
			continue
		}
		clean[k] = v
	}
	clean["loop_id"] = loopID
	clean["produced_at"] = time.Now().UTC()

	b, err := json.Marshal(clean)
	if err != nil {
		return nil, fmt.Errorf("marshal tool args: %v", err)
	}
	var artifact curator.Artifact
	if err := json.Unmarshal(b, &artifact); err != nil {
		return nil, fmt.Errorf("unmarshal tool args into curator artifact: %v", err)
	}
	return &artifact, nil
}

// buildTriples assembles the five deterministic triples in publish order.
// Entity: the curator's loop entity. Predicates: counts + deduplicated
// sorted namespace index + generated_at timestamp.
//
// Namespace index is derived from added_sources ∪ source_dirs so the ops-
// chain-observer can see "which namespaces did this curator loop affect"
// without deserializing the full payload.
func buildTriples(loopEntityID string, a *curator.Artifact, now time.Time) []message.Triple {
	base := func(pred string, obj any) message.Triple {
		return message.Triple{
			Subject:    loopEntityID,
			Predicate:  pred,
			Object:     obj,
			Source:     toolSource,
			Timestamp:  now,
			Confidence: 1.0,
		}
	}

	// Collect namespaces from both added_sources and source_dirs,
	// deduplicate, and sort for determinism.
	seen := make(map[string]struct{})
	for _, src := range a.AddedSources {
		if src.Namespace != "" {
			seen[src.Namespace] = struct{}{}
		}
	}
	for _, dir := range a.SourceDirs {
		if dir.Namespace != "" {
			seen[dir.Namespace] = struct{}{}
		}
	}
	ns := make([]string, 0, len(seen))
	for k := range seen {
		ns = append(ns, k)
	}
	sort.Strings(ns)

	return []message.Triple{
		base(predicateAddedSourcesCount, fmt.Sprintf("%d", len(a.AddedSources))),
		base(predicateVerifiedEntityIDsCount, fmt.Sprintf("%d", len(a.VerifiedEntityIDs))),
		base(predicateSourceDirsCount, fmt.Sprintf("%d", len(a.SourceDirs))),
		base(predicateNamespaces, strings.Join(ns, ",")),
		base(predicateGeneratedAt, now.UTC().Format(time.RFC3339)),
	}
}
