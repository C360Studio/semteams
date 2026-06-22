// Package renderopenspec implements the SemTeams-local render_openspec tool — the
// read/render half of the OpenSpec tool pair (ADR-057 OQ1; the emit_change
// counterpart writes). It reads the change.<slug>.* triples a create_change arc
// stamped on the run entity, reconstructs the OpenSpec change via the pure
// openspec.ChangeFromFacts, and renders it to a single OpenSpec-markdown document
// via openspec.RenderChangeFolder.
//
// Primary consumer (P2): the create-change wake-up coordinator (configs/rules/
// create-change/03) calls it to deliver the ACTUAL reviewed spec to the user, not
// just a prose summary. UC-1 extension (deferred): also write the rendered folder
// into the sandbox workspace for a PR — that mode needs a sandbox FS writer
// (mirror emit_autoresearch_artifact) and a PR flow that does not exist in P2.
//
// Thin graph adapter: all format logic lives in the dep-free openspec package
// (extraction-ready per ADR-057 §Framework-alignment 5a); this tool only resolves
// the run entity, reads its triples, and maps them to the openspec.Fact list the
// pure reconstructor consumes.
//
// Framework-alignment review: read-shaped, like upstream read_loop_result /
// read_entity / query_entities — but those return raw graph shapes, not a
// rendered OpenSpec projection, so none substitutes. Migration target is the same
// anticipated read_artifact suite (ADR-028 §What's not built here, verified absent
// as of semstreams beta.114) as emit_change's. See cmd/semteams/tools/README.md.
package renderopenspec

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/types"

	"github.com/c360studio/semteams/cmd/semteams/openspec"
	"github.com/c360studio/semteams/cmd/semteams/runanchor"
)

// ToolName is the LLM-facing tool name.
const ToolName = "render_openspec"

// changePredicatePrefix is the predicate namespace emit_change stamps under.
const changePredicatePrefix = "change."

// EntityReader reads a graph entity's triples into a predicate→object map.
// *chain.NATSEntityReader satisfies it; tests inject fakes.
type EntityReader interface {
	ReadEntity(ctx context.Context, entityID string) (map[string]any, error)
}

// Executor implements agentic.ToolExecutor for render_openspec.
type Executor struct {
	reader   EntityReader
	platform types.PlatformMeta
	logger   *slog.Logger
}

// NewExecutor constructs an Executor. reader must be non-nil.
func NewExecutor(reader EntityReader, platform types.PlatformMeta, logger *slog.Logger) *Executor {
	if reader == nil {
		panic("renderopenspec.NewExecutor: reader must not be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Executor{reader: reader, platform: platform, logger: logger}
}

type renderArgs struct {
	// Slug is optional: when empty, the tool renders the single change present
	// on the run entity (erroring if zero or more than one).
	Slug string `json:"slug"`
}

// Execute resolves the run entity, reads its change.<slug>.* triples, and
// returns the rendered OpenSpec change-folder markdown.
func (e *Executor) Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
	if call.Name != ToolName {
		return errResult(call, agentic.ToolErrorNotFound, "unknown tool: %s", call.Name)
	}

	args, err := parseArgs(call.Arguments)
	if err != nil {
		return errResult(call, agentic.ToolErrorInvalidArgs, "%v", err)
	}

	// Resolve via the dispatch-stamped run anchor — NOT related_loops like
	// emit_change. emit_change runs in the AUTHOR loop, which rule 01 pins with
	// related_loops["create-change-run"]; render_openspec runs in the WAKE-UP
	// coordinator, which carries no such pin but DOES carry the run anchor on its
	// ToolCall.Metadata (it inherits agent.run from the run chain, so dispatch
	// stamps agent.run_id/agent.run_entity_id on every tool call it makes).
	_, runEntityID := runanchor.Anchor(call, e.platform.Org, e.platform.Platform)
	if runEntityID == "" {
		return errResult(call, agentic.ToolErrorInvalidArgs,
			"render_openspec: no run anchor on the call — this tool reads a change off the run entity, so it must be called from within a create_change run")
	}

	triples, err := e.reader.ReadEntity(ctx, runEntityID)
	if err != nil {
		return errResult(call, agentic.ToolErrorNetwork, "read run entity %s: %v", runEntityID, err)
	}

	slug, err := resolveSlug(triples, args.Slug)
	if err != nil {
		return errResult(call, agentic.ToolErrorInvalidArgs, "%v", err)
	}

	change := openspec.ChangeFromFacts(slug, changeFacts(triples, slug))
	markdown := openspec.RenderChangeFolder(change)

	e.logger.Info("render_openspec rendered",
		slog.String("run_entity_id", runEntityID),
		slog.String("slug", slug),
		slog.Int("bytes", len(markdown)))

	return agentic.ToolResult{
		CallID:   call.ID,
		Name:     call.Name,
		Content:  markdown,
		Metadata: map[string]any{"run_entity_id": runEntityID, "slug": slug},
	}, nil
}

func parseArgs(raw map[string]any) (renderArgs, error) {
	var a renderArgs
	if raw == nil {
		return a, nil // slug optional — empty triggers auto-discovery
	}
	buf, err := json.Marshal(raw)
	if err != nil {
		return a, fmt.Errorf("marshal arguments: %w", err)
	}
	dec := json.NewDecoder(strings.NewReader(string(buf)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil {
		return a, fmt.Errorf("unmarshal arguments: %w", err)
	}
	a.Slug = strings.TrimSpace(a.Slug)
	return a, nil
}

// resolveSlug returns the change slug to render. When want is set it verifies the
// run entity carries that change; otherwise it auto-discovers the single change
// present (erroring on zero or ambiguous-multiple — the create_change arc is
// single-emit, so >1 means an unexpected state the caller must disambiguate).
func resolveSlug(triples map[string]any, want string) (string, error) {
	present := changeSlugs(triples)
	if want != "" {
		if !present[want] {
			return "", fmt.Errorf("render_openspec: no change %q on the run entity (present: %s)", want, slugList(present))
		}
		return want, nil
	}
	switch len(present) {
	case 0:
		return "", fmt.Errorf("render_openspec: no change.* triples on the run entity — nothing to render (was emit_change called?)")
	case 1:
		return slugList(present), nil
	default:
		return "", fmt.Errorf("render_openspec: %d changes on the run entity (%s) — pass slug to pick one", len(present), slugList(present))
	}
}

// changeSlugs extracts the distinct <slug> from every change.<slug>.* predicate.
func changeSlugs(triples map[string]any) map[string]bool {
	out := map[string]bool{}
	for pred := range triples {
		rest, ok := strings.CutPrefix(pred, changePredicatePrefix)
		if !ok {
			continue
		}
		slug, _, ok := strings.Cut(rest, ".")
		if ok && slug != "" {
			out[slug] = true
		}
	}
	return out
}

// slugList renders a slug set as a sorted comma list (for messages + the
// single-element happy path).
func slugList(set map[string]bool) string {
	xs := make([]string, 0, len(set))
	for s := range set {
		xs = append(xs, s)
	}
	sort.Strings(xs)
	return strings.Join(xs, ", ")
}

// changeFacts maps the run entity's change.<slug>.* triples to the openspec.Fact
// list ChangeFromFacts consumes. ChangeFromFacts ignores predicates it does not
// model (the writer-only status / acceptance_command / §D6 task fields), so
// passing the full change.<slug>.* set is safe — only the markdown-representable
// content is reconstructed.
func changeFacts(triples map[string]any, slug string) []openspec.Fact {
	prefix := changePredicatePrefix + slug + "."
	out := make([]openspec.Fact, 0, len(triples))
	for pred, val := range triples {
		if !strings.HasPrefix(pred, prefix) {
			continue
		}
		out = append(out, openspec.Fact{Predicate: pred, Object: stringify(val)})
	}
	return out
}

// stringify renders a graph object as the string Object the openspec decoders
// expect. emit_change stamps every change.* object as a string, so this is
// identity in the steady state; the type switch is defensive against a future
// encoder returning typed numbers/bools.
func stringify(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", s)
	}
}

func errResult(call agentic.ToolCall, kind agentic.ToolErrorKind, format string, args ...any) (agentic.ToolResult, error) {
	return agentic.ToolResult{
		CallID:    call.ID,
		Name:      call.Name,
		Error:     fmt.Sprintf(format, args...),
		ErrorKind: kind,
	}, nil
}
