package emitartifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/types"
	agvocab "github.com/c360studio/semstreams/vocabulary/agentic"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semteams/cmd/semteams/chain"
	"github.com/c360studio/semteams/cmd/semteams/research"
	"github.com/c360studio/semteams/cmd/semteams/runanchor"
)

const (
	// defaultAgentContentBucket is the default JetStream ObjectStore bucket
	// where agentic-loop persists full trajectory evidence bodies when no
	// component-level storage_buckets mapping overrides StorageInstance.
	defaultAgentContentBucket = "AGENT_CONTENT"

	// predicateRunEntityID is the canonical 3-segment triple predicate stamped
	// on loop entities pointing at their enclosing run entity ID
	// (org.platform.chain.agent.execution.<run_id> via agentic.ChainExecutionEntityID).
	predicateRunEntityID = agvocab.LoopRunEntityID

	// predicateLoopRun is the canonical 3-segment triple predicate stamped on
	// loop entities holding the bare run root loop ID.
	predicateLoopRun = agvocab.LoopRun

	// predicateLegacyRunID is the legacy 2-segment read-only fallback triple
	// predicate (never emitted).
	predicateLegacyRunID = "agent.run"

	// Research lineage and revision predicates used to scope bare tool_result_ref
	// lookups to the current run's plan, gather, and prior synthesize loops
	// without scanning the entire AGENT_TRAJECTORIES bucket (#271 M2, M4).
	predicateLineagePlanLoop        = "agent.lineage.researcher-plan"
	predicateLineagePlanLoopEntity  = "agent.lineage.plan-loop-entity-id"
	predicateLineagePriorSynthesize = "agent.lineage.researcher"
	predicateGatherCompletedLoop    = "research.gather.completed-subtopic"
	predicatePlanRevision           = "research.plan.revision"
	predicateArtifactRevision       = "research.artifact.revision"
)

// DerivationError classifies a derivation failure into an agentic.ToolErrorKind
// so Executor.Execute can distinguish model-correctable citation errors
// (ToolErrorInvalidArgs) from transient NATS/KV/ObjectStore outages
// (ToolErrorNetwork) and corrupt trajectory state (ToolErrorInternal).
type DerivationError struct {
	Kind agentic.ToolErrorKind
	Err  error
}

func (e *DerivationError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *DerivationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// ErrorKindOf extracts the ToolErrorKind from a DerivationError, defaulting to
// ToolErrorInvalidArgs for unclassified errors.
func ErrorKindOf(err error) agentic.ToolErrorKind {
	var de *DerivationError
	if errors.As(err, &de) && de != nil && de.Kind != "" {
		return de.Kind
	}
	return agentic.ToolErrorInvalidArgs
}

func newInvalidArgsErr(format string, args ...any) error {
	return &DerivationError{
		Kind: agentic.ToolErrorInvalidArgs,
		Err:  fmt.Errorf(format, args...),
	}
}

func newNetworkErr(format string, args ...any) error {
	return &DerivationError{
		Kind: agentic.ToolErrorNetwork,
		Err:  fmt.Errorf(format, args...),
	}
}

func newInternalErr(format string, args ...any) error {
	return &DerivationError{
		Kind: agentic.ToolErrorInternal,
		Err:  fmt.Errorf(format, args...),
	}
}

func wrapWithContext(prefix string, err error) error {
	kind := ErrorKindOf(err)
	return &DerivationError{
		Kind: kind,
		Err:  fmt.Errorf("%s: %w", prefix, err),
	}
}

// ResolvedToolResult holds the resolved output, arguments, status, timestamps,
// and loop lineage of a prior tool execution referenced by an
// EvidenceRecord.ToolResultRef.
type ResolvedToolResult struct {
	Ref               string
	CallID            string
	ExecutionID       string
	LoopID            string
	ToolName          string
	Status            string
	Error             string
	ErrorKind         string
	Content           string
	DispatchArguments map[string]any
	ObservedAt        time.Time
	ProducerRevision  int
}

// ToolResultResolver resolves a tool_result_ref (a ToolCall.ID,
// ToolCall.ExecutionID, or "<loop_id>/<call_id>") within the scope of the
// calling tool execution.
//
// Implementations MUST verify that the resolved tool execution belongs to the
// same research run as call (either same LoopID or same run anchor entity ID)
// and return an error if the reference does not exist or belongs to a
// different run.
type ToolResultResolver interface {
	ResolveToolResult(ctx context.Context, call agentic.ToolCall, ref string) (*ResolvedToolResult, error)
}

func isNilResolver(r ToolResultResolver) bool {
	if r == nil {
		return true
	}
	v := reflect.ValueOf(r)
	return v.Kind() == reflect.Ptr && v.IsNil()
}

// VerifyDerivation mechanically verifies every EvidenceRecord cited across an
// Artifact's Actors and IntegrationPoints against prior tool results from the
// same research run:
//  1. tool_result_ref resolves to a real completed retrieval tool execution in
//     the current research run (same loop or same run anchor across plan,
//     gather, synthesize, and revision-2+ retry loops).
//  2. The resolved tool is an allowed retrieval tool for the citation's
//     channel, succeeded without error, and did not merely echo the cited
//     quote from its own dispatch arguments.
//  3. The cited locator/origin_id and the verbatim quote co-occur within the
//     same retrieved record, search-result block, or JSON item, with URL/token
//     boundary matching.
//  4. Server-authoritative fields (retrieved_at from trajectory ObservedAt,
//     kind for web_search snippets, and revision from the producing loop) are
//     stamped or verified on the EvidenceRecord in place.
func VerifyDerivation(ctx context.Context, resolver ToolResultResolver, call agentic.ToolCall, a *research.Artifact) error {
	_, err := verifyDerivationWithLoops(ctx, resolver, call, a)
	return err
}

func verifyDerivationWithLoops(ctx context.Context, resolver ToolResultResolver, call agentic.ToolCall, a *research.Artifact) ([]string, error) {
	if isNilResolver(resolver) || a == nil {
		return nil, nil
	}

	cache := make(map[string]*ResolvedToolResult)
	var citedRefs []string
	var producerLoops []string
	seenLoops := make(map[string]bool)

	resolve := func(ref string) (*ResolvedToolResult, error) {
		if cached, ok := cache[ref]; ok {
			return cached, nil
		}
		res, err := resolver.ResolveToolResult(ctx, call, ref)
		if err != nil {
			return nil, err
		}
		if res == nil {
			return nil, newInvalidArgsErr("tool_result_ref %q resolved to nil result", ref)
		}
		cache[ref] = res
		citedRefs = append(citedRefs, ref)
		if l := strings.TrimSpace(res.LoopID); l != "" && !seenLoops[l] {
			seenLoops[l] = true
			producerLoops = append(producerLoops, l)
		}
		return res, nil
	}

	for i := range a.Actors {
		actor := &a.Actors[i]
		for j := range actor.Evidence {
			ev := &actor.Evidence[j]
			res, err := resolve(ev.ToolResultRef)
			if err != nil {
				return nil, wrapWithContext(fmt.Sprintf("actors[%d] (%s) evidence[%d]", i, actor.Name, j), err)
			}
			if err := verifyCitationAgainstResult(ev, res); err != nil {
				return nil, wrapWithContext(fmt.Sprintf("actors[%d] (%s) evidence[%d]", i, actor.Name, j), err)
			}
		}
	}

	for i := range a.IntegrationPoints {
		ip := &a.IntegrationPoints[i]
		for j := range ip.Evidence {
			ev := &ip.Evidence[j]
			res, err := resolve(ev.ToolResultRef)
			if err != nil {
				return nil, wrapWithContext(fmt.Sprintf("integration_points[%d] (%s -> %s) evidence[%d]", i, ip.From, ip.To, j), err)
			}
			if err := verifyCitationAgainstResult(ev, res); err != nil {
				return nil, wrapWithContext(fmt.Sprintf("integration_points[%d] (%s -> %s) evidence[%d]", i, ip.From, ip.To, j), err)
			}
		}
	}

	if err := verifyPartialRepoFailureDisclosure(citedRefs, cache, a.OpenGaps); err != nil {
		return nil, err
	}

	return producerLoops, nil
}

// verifyPartialRepoFailureDisclosure enforces that any partial repository
// failures recorded in cited github_read tool results are explicitly disclosed
// in artifact.OpenGaps (#268).
func verifyPartialRepoFailureDisclosure(citedRefs []string, cache map[string]*ResolvedToolResult, openGaps []string) error {
	for _, ref := range citedRefs {
		res := cache[ref]
		if res == nil || strings.TrimSpace(res.Content) == "" {
			continue
		}
		var env evidenceRecordsEnvelope
		if err := json.Unmarshal([]byte(res.Content), &env); err != nil || len(env.Failures) == 0 {
			continue
		}
		for _, failure := range env.Failures {
			repo := strings.TrimSpace(failure.Repository)
			if repo == "" {
				continue
			}
			if !openGapsDiscloseRepository(openGaps, repo) {
				return newInvalidArgsErr(
					"tool_result_ref %q reported partial repository failure for %q (%s: %s), which must be disclosed in open_gaps",
					ref, repo, failure.Kind, failure.Error,
				)
			}
		}
	}
	return nil
}

func openGapsDiscloseRepository(openGaps []string, repoSlug string) bool {
	want := strings.ToLower(strings.TrimSpace(repoSlug))
	if want == "" {
		return true
	}
	for _, gap := range openGaps {
		if strings.Contains(strings.ToLower(gap), want) {
			return true
		}
	}
	return false
}

// evidenceRecordsEnvelope matches structured tool outputs like github_read
// that return {"records": [...], "failures": [...]}.
type evidenceRecordsEnvelope struct {
	Records  []research.EvidenceRecord `json:"records"`
	Failures []struct {
		Repository string `json:"repository"`
		Kind       string `json:"kind"`
		Error      string `json:"error"`
	} `json:"failures"`
}

func verifyCitationAgainstResult(ev *research.EvidenceRecord, res *ResolvedToolResult) error {
	if err := checkToolResultSucceeded(ev, res); err != nil {
		return err
	}
	if err := checkAllowedRetrievalTool(ev, res); err != nil {
		return err
	}
	if quoteEchoedInDispatchArgs(res.DispatchArguments, ev.Quote) {
		return newInvalidArgsErr(
			"quote %q appears in tool_result_ref %q dispatch arguments (query/command echo is not retrieved evidence)",
			ev.Quote, ev.ToolResultRef)
	}

	// Verify or stamp revision against the producing loop's revision (#271 M4).
	if res.ProducerRevision > 0 {
		if ev.Revision == 0 {
			ev.Revision = res.ProducerRevision
		} else if ev.Revision != res.ProducerRevision {
			return newInvalidArgsErr(
				"evidence.revision (%d) does not match producing loop %q revision (%d) for tool_result_ref %q",
				ev.Revision, res.LoopID, res.ProducerRevision, ev.ToolResultRef)
		}
	}

	// Stamp server-authoritative retrieved_at from trajectory ObservedAt when present (#271 m1).
	if !res.ObservedAt.IsZero() {
		ev.RetrievedAt = res.ObservedAt.UTC()
	}

	// 1. Structured record envelope (e.g., github_read): match by OriginID or
	// Locator against a returned record, then verify Quote against that
	// record's RawBody.
	var env evidenceRecordsEnvelope
	if err := json.Unmarshal([]byte(res.Content), &env); err == nil && len(env.Records) > 0 {
		var matched []*research.EvidenceRecord
		for idx := range env.Records {
			rec := &env.Records[idx]
			if (ev.OriginID != "" && rec.OriginID == ev.OriginID) ||
				(ev.Locator != "" && rec.Locator == ev.Locator) {
				matched = append(matched, rec)
			}
		}
		if len(matched) == 0 {
			return newInvalidArgsErr(
				"tool_result_ref %q does not contain record for locator %q or origin_id %q",
				ev.ToolResultRef, ev.Locator, ev.OriginID)
		}
		for _, rec := range matched {
			body := rec.RawBody
			if body == "" {
				body = rec.Quote
			}
			if strings.Contains(body, ev.Quote) {
				if rec.Kind != "" {
					if ev.Kind != "" && ev.Kind != rec.Kind {
						return newInvalidArgsErr(
							"evidence.kind %q does not match record kind %q in tool_result_ref %q",
							ev.Kind, rec.Kind, ev.ToolResultRef)
					}
					ev.Kind = rec.Kind
				}
				if res.ObservedAt.IsZero() && !rec.RetrievedAt.IsZero() {
					ev.RetrievedAt = rec.RetrievedAt.UTC()
				}
				return nil
			}
		}
		return newInvalidArgsErr(
			"quote %q is not a verbatim substring of tool_result_ref %q record %q",
			ev.Quote, ev.ToolResultRef, matched[0].OriginID)
	}

	// 2. web_search output: citations from web_search are search-engine
	// snippets, never full retrieved bodies. Require both locator/origin_id
	// and quote to appear in the SAME numbered [N] result block (ignoring the
	// "web_search results for: <query>" header).
	toolName := strings.ToLower(strings.TrimSpace(res.ToolName))
	if toolName == "web_search" {
		if ev.Kind != "" && ev.Kind != research.EvidenceKindSnippet {
			return newInvalidArgsErr(
				"web_search citations only provide search-result snippets (kind=%q), cannot claim kind=%q",
				research.EvidenceKindSnippet, ev.Kind)
		}
		ev.Kind = research.EvidenceKindSnippet
		return verifyWebSearchBlockCitation(ev, res)
	}

	// Stamp default Kind for non-web_search tools when omitted.
	if ev.Kind == "" {
		switch ev.Channel {
		case research.ChannelSemSource:
			ev.Kind = research.EvidenceKindEntity
		default:
			ev.Kind = research.EvidenceKindBody
		}
	}

	// 3. Multi-item JSON outputs (arrays or object envelopes with an array of
	// items/entities/results): require locator/origin_id and quote to co-occur
	// in the SAME item element (#271 M1).
	if items, ok := extractJSONMultiItems(res.Content); ok {
		return verifyJSONMultiItemCitation(ev, items)
	}

	// 4. Single-resource output (http_request, bash curl/wget/gh, single-entity
	// graph read): verify bounded locator/origin_id and verbatim quote.
	if !singleResultContainsLocatorOrOrigin(res, ev.Channel, ev.Locator, ev.OriginID) {
		return newInvalidArgsErr(
			"tool_result_ref %q does not reference locator %q or origin_id %q",
			ev.ToolResultRef, ev.Locator, ev.OriginID)
	}
	if !contentContainsVerbatimQuote(res.Content, ev.Quote) {
		return newInvalidArgsErr(
			"quote %q is not a verbatim substring of tool_result_ref %q output",
			ev.Quote, ev.ToolResultRef)
	}
	return nil
}

func checkToolResultSucceeded(ev *research.EvidenceRecord, res *ResolvedToolResult) error {
	if strings.EqualFold(strings.TrimSpace(res.Status), string(agentic.TrajectoryStatusFailed)) ||
		strings.TrimSpace(res.Error) != "" ||
		strings.TrimSpace(res.ErrorKind) != "" {
		return newInvalidArgsErr(
			"tool_result_ref %q (tool %q) failed and cannot be cited as evidence",
			ev.ToolResultRef, res.ToolName)
	}
	return nil
}

// checkAllowedRetrievalTool enforces that only genuine external-retrieval tools
// may be cited as evidence sources (#271 C1). Self-authored or state-reading
// tools (read_loop_result, scratchpad, decide, emit_*, write_todos, or bash
// echo/cat/printf) are rejected.
func checkAllowedRetrievalTool(ev *research.EvidenceRecord, res *ResolvedToolResult) error {
	tool := strings.ToLower(strings.TrimSpace(res.ToolName))
	if tool == "" {
		return newInvalidArgsErr(
			"tool_result_ref %q has no recorded tool name and cannot be verified as a retrieval tool",
			ev.ToolResultRef)
	}

	switch ev.Channel {
	case research.ChannelGitHub:
		switch tool {
		case "github_read", "http_request":
			return nil
		case "bash":
			if isBashFetchCommand(res.DispatchArguments, true) {
				return nil
			}
			return newInvalidArgsErr(
				"tool_result_ref %q (bash) cannot be cited for channel %q unless command invokes curl, wget, or gh",
				ev.ToolResultRef, ev.Channel)
		default:
			return newInvalidArgsErr(
				"tool_result_ref %q came from tool %q, which is not an allowed retrieval tool for channel %q",
				ev.ToolResultRef, res.ToolName, ev.Channel)
		}

	case research.ChannelWeb:
		switch tool {
		case "web_search", "http_request":
			return nil
		case "bash":
			if isBashFetchCommand(res.DispatchArguments, false) {
				return nil
			}
			return newInvalidArgsErr(
				"tool_result_ref %q (bash) cannot be cited for channel %q unless command invokes curl or wget",
				ev.ToolResultRef, ev.Channel)
		default:
			return newInvalidArgsErr(
				"tool_result_ref %q came from tool %q, which is not an allowed retrieval tool for channel %q",
				ev.ToolResultRef, res.ToolName, ev.Channel)
		}

	case research.ChannelSemSource:
		switch tool {
		case "graph_query", "graph_search", "query_entity", "query_relationships", "semsource_read":
			return nil
		default:
			return newInvalidArgsErr(
				"tool_result_ref %q came from tool %q, which is not an allowed retrieval tool for channel %q",
				ev.ToolResultRef, res.ToolName, ev.Channel)
		}

	default:
		return newInvalidArgsErr("unsupported evidence channel %q", ev.Channel)
	}
}

var allowedBashPipeFilters = map[string]bool{
	"head":  true,
	"tail":  true,
	"sed":   true,
	"grep":  true,
	"egrep": true,
	"fgrep": true,
	"jq":    true,
	"cut":   true,
	"tr":    true,
	"sort":  true,
	"uniq":  true,
	"wc":    true,
}

// isBashFetchCommand reports whether a bash tool call's command argument is a
// network retrieval command (each command list entry starts with curl, wget, or
// — when allowGH is true — gh, optionally piped into read-only stream filters
// like head/tail/sed/grep/jq) and never invokes local-file or self-echo
// commands (echo, printf, cat, tee, python, sh, etc.).
func isBashFetchCommand(args map[string]any, allowGH bool) bool {
	if len(args) == 0 {
		return false
	}
	cmd, _ := args["command"].(string)
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return false
	}
	// Reject subshell command substitution (`...` or $(...)) so a fetch command
	// cannot smuggle an echo/cat side-channel.
	if strings.Contains(cmd, "`") || strings.Contains(cmd, "$(") {
		return false
	}

	pipelines := splitShellOutOfQuotes(cmd, false)
	foundFetch := false
	for _, pipeline := range pipelines {
		pipeline = strings.TrimSpace(pipeline)
		if pipeline == "" {
			continue
		}
		stages := splitShellOutOfQuotes(pipeline, true)
		stageFetched := false
		for i, stage := range stages {
			base := extractShellCommandBase(stage)
			if base == "" {
				return false
			}
			if i == 0 {
				switch base {
				case "curl", "wget":
					stageFetched = true
					foundFetch = true
				case "gh":
					if !allowGH {
						return false
					}
					stageFetched = true
					foundFetch = true
				default:
					return false
				}
				continue
			}
			if !stageFetched || !allowedBashPipeFilters[base] {
				return false
			}
		}
	}
	return foundFetch
}

func extractShellCommandBase(stage string) string {
	fields := strings.Fields(strings.TrimSpace(stage))
	for len(fields) > 1 && strings.Contains(fields[0], "=") && !strings.HasPrefix(fields[0], "-") {
		fields = fields[1:]
	}
	if len(fields) == 0 {
		return ""
	}
	base := fields[0]
	if idx := strings.LastIndexByte(base, '/'); idx >= 0 {
		base = base[idx+1:]
	}
	return base
}

// splitShellOutOfQuotes splits s either on command-list separators (;, \n, &&,
// ||, &) when splitPipe is false, or on single pipe (|) when splitPipe is true,
// ignoring characters inside single or double quotes.
func splitShellOutOfQuotes(s string, splitPipe bool) []string {
	var out []string
	var cur strings.Builder
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == '\\' && quote == '"' && i+1 < len(s) {
				cur.WriteByte(c)
				i++
				cur.WriteByte(s[i])
				continue
			}
			if c == quote {
				quote = 0
			}
			cur.WriteByte(c)
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			cur.WriteByte(c)
			continue
		}
		if splitPipe {
			if c == '|' && (i+1 >= len(s) || s[i+1] != '|') {
				out = append(out, cur.String())
				cur.Reset()
				continue
			}
			cur.WriteByte(c)
			continue
		}
		if c == ';' || c == '\n' {
			out = append(out, cur.String())
			cur.Reset()
			continue
		}
		if c == '&' {
			if i+1 < len(s) && s[i+1] == '&' {
				i++
			}
			out = append(out, cur.String())
			cur.Reset()
			continue
		}
		if c == '|' && i+1 < len(s) && s[i+1] == '|' {
			i++
			out = append(out, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteByte(c)
	}
	out = append(out, cur.String())
	return out
}

func quoteEchoedInDispatchArgs(args map[string]any, quote string) bool {
	if len(args) == 0 {
		return false
	}
	normQuote := normalizeWhitespace(quote)
	if normQuote == "" {
		return false
	}
	return valueContainsNormalizedQuote(args, quote, normQuote)
}

func valueContainsNormalizedQuote(v any, rawQuote, normQuote string) bool {
	switch typed := v.(type) {
	case string:
		if rawQuote != "" && strings.Contains(typed, rawQuote) {
			return true
		}
		if normQuote != "" && strings.Contains(normalizeWhitespace(typed), normQuote) {
			return true
		}
	case map[string]any:
		for _, val := range typed {
			if valueContainsNormalizedQuote(val, rawQuote, normQuote) {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if valueContainsNormalizedQuote(item, rawQuote, normQuote) {
				return true
			}
		}
	}
	return false
}

func normalizeWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

var webSearchBlockStartRe = regexp.MustCompile(`(?m)^\[\d+\]\s+`)

// parseWebSearchBlocks splits formatted web_search output into individual
// numbered result blocks (`[1] ...`, `[2] ...`), discarding any header text
// before `[1]` (such as `[stub] web_search results for: <query>`).
func parseWebSearchBlocks(content string) []string {
	locs := webSearchBlockStartRe.FindAllStringIndex(content, -1)
	if len(locs) == 0 {
		return nil
	}
	blocks := make([]string, 0, len(locs))
	for i, loc := range locs {
		start := loc[0]
		end := len(content)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		blocks = append(blocks, content[start:end])
	}
	return blocks
}

func verifyWebSearchBlockCitation(ev *research.EvidenceRecord, res *ResolvedToolResult) error {
	blocks := parseWebSearchBlocks(res.Content)
	if len(blocks) == 0 {
		return newInvalidArgsErr(
			"tool_result_ref %q (web_search) contains no numbered result blocks",
			ev.ToolResultRef)
	}

	locatorMatchedBlock := false
	for _, block := range blocks {
		if textContainsLocatorOrOrigin(block, ev.Channel, ev.Locator, ev.OriginID) {
			locatorMatchedBlock = true
			if strings.Contains(block, ev.Quote) {
				return nil
			}
		}
	}
	if !locatorMatchedBlock {
		return newInvalidArgsErr(
			"tool_result_ref %q (web_search) has no result block matching locator %q or origin_id %q",
			ev.ToolResultRef, ev.Locator, ev.OriginID)
	}
	return newInvalidArgsErr(
		"quote %q does not appear in the web_search result block for locator %q in tool_result_ref %q",
		ev.Quote, ev.Locator, ev.ToolResultRef)
}

func extractJSONMultiItems(rawJSON string) ([]any, bool) {
	trimmed := strings.TrimSpace(rawJSON)
	if len(trimmed) == 0 || (trimmed[0] != '[' && trimmed[0] != '{') {
		return nil, false
	}
	var decoded any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return nil, false
	}
	switch typed := decoded.(type) {
	case []any:
		if len(typed) > 0 {
			return typed, true
		}
	case map[string]any:
		for _, key := range []string{"entities", "results", "items", "nodes", "data", "edges"} {
			if arr, ok := typed[key].([]any); ok && len(arr) > 0 {
				return arr, true
			}
		}
	}
	return nil, false
}

func verifyJSONMultiItemCitation(ev *research.EvidenceRecord, items []any) error {
	locatorMatchedItem := false
	for _, item := range items {
		if valueContainsLocatorOrOrigin(item, ev.Channel, ev.Locator, ev.OriginID) {
			locatorMatchedItem = true
			if valueContainsSubstring(item, ev.Quote) {
				return nil
			}
		}
	}
	if !locatorMatchedItem {
		return newInvalidArgsErr(
			"tool_result_ref %q has no result item matching locator %q or origin_id %q",
			ev.ToolResultRef, ev.Locator, ev.OriginID)
	}
	return newInvalidArgsErr(
		"quote %q does not appear in the result item for locator %q in tool_result_ref %q",
		ev.Quote, ev.Locator, ev.ToolResultRef)
}

var allowedLocatorDispatchArgKeys = []string{
	"url",
	"uri",
	"endpoint",
	"path",
	"command",
	"entity_id",
	"id",
	"locator",
}

func singleResultContainsLocatorOrOrigin(res *ResolvedToolResult, channel research.EvidenceChannel, locator, originID string) bool {
	if textContainsLocatorOrOrigin(res.Content, channel, locator, originID) {
		return true
	}
	var decoded any
	if err := json.Unmarshal([]byte(res.Content), &decoded); err == nil {
		if valueContainsLocatorOrOrigin(decoded, channel, locator, originID) {
			return true
		}
	}
	for _, key := range allowedLocatorDispatchArgKeys {
		if val, ok := res.DispatchArguments[key]; ok {
			if valueContainsLocatorOrOrigin(val, channel, locator, originID) {
				return true
			}
		}
	}
	return false
}

func valueContainsLocatorOrOrigin(v any, channel research.EvidenceChannel, locator, originID string) bool {
	switch typed := v.(type) {
	case string:
		return textContainsLocatorOrOrigin(typed, channel, locator, originID)
	case map[string]any:
		for _, val := range typed {
			if valueContainsLocatorOrOrigin(val, channel, locator, originID) {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if valueContainsLocatorOrOrigin(item, channel, locator, originID) {
				return true
			}
		}
	}
	return false
}

var urlTokenRe = regexp.MustCompile(`(?:https?://|github://|semsource://)[^\s"'<>()\[\]{}]+`)

// textContainsLocatorOrOrigin checks whether text contains locator or originID
// on strict URL/token boundaries (#271 M1), or contains a URL token whose
// canonical NormalizeOriginID equals originID.
func textContainsLocatorOrOrigin(text string, channel research.EvidenceChannel, locator, originID string) bool {
	if text == "" {
		return false
	}
	candidates := []string{locator, originID}
	if channel == research.ChannelSemSource {
		for _, c := range []string{locator, originID} {
			if strings.HasPrefix(strings.ToLower(c), "semsource://") {
				candidates = append(candidates, c[len("semsource://"):])
			}
		}
	}
	for _, cand := range candidates {
		cand = strings.TrimSpace(cand)
		if cand == "" {
			continue
		}
		if containsURLOrTokenBounded(text, cand) {
			return true
		}
	}

	// Also check if any URL token in text normalizes to originID under channel.
	if originID != "" {
		for _, rawToken := range urlTokenRe.FindAllString(text, -1) {
			tok := strings.TrimRight(rawToken, ".,;:!?'\")]}>")
			if norm, err := research.NormalizeOriginID(channel, tok); err == nil && norm == originID {
				return true
			}
		}
	}
	return false
}

// containsURLOrTokenBounded reports whether needle appears in haystack with
// non-URL-continuation boundaries on both sides, so "https://example.com/a"
// never matches "https://example.com/a-other" or "https://example.com/a/b".
func containsURLOrTokenBounded(haystack, needle string) bool {
	if haystack == "" || needle == "" {
		return false
	}
	searchFrom := 0
	for {
		idx := strings.Index(haystack[searchFrom:], needle)
		if idx < 0 {
			return false
		}
		start := searchFrom + idx
		end := start + len(needle)

		leftOK := start == 0 || !isURLTokenContinuationByte(haystack[start-1])
		rightOK := end == len(haystack) || !isURLTokenContinuationByte(haystack[end])
		if leftOK && rightOK {
			return true
		}
		searchFrom = start + 1
		if searchFrom >= len(haystack) {
			return false
		}
	}
}

func isURLTokenContinuationByte(b byte) bool {
	if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') {
		return true
	}
	switch b {
	case '/', '?', '&', '=', '#', '-', '_', ':', '%', '@', '+', '~':
		return true
	case '.':
		// A period followed by whitespace/end is sentence punctuation, handled
		// by urlTokenRe; for raw substring boundary checks, treat '.' as a
		// continuation character so "https://example.com/a" cannot match
		// "https://example.com/a.html".
		return true
	}
	return false
}

func contentContainsVerbatimQuote(content, quote string) bool {
	if quote == "" {
		return false
	}
	if strings.Contains(content, quote) {
		return true
	}
	return jsonStringsContain(content, quote)
}

func jsonStringsContain(rawJSON, needle string) bool {
	if needle == "" || len(rawJSON) == 0 {
		return false
	}
	var decoded any
	if err := json.Unmarshal([]byte(rawJSON), &decoded); err != nil {
		return false
	}
	return valueContainsSubstring(decoded, needle)
}

// valueContainsSubstring checks only JSON string values (never JSON object
// keys — #271 m5).
func valueContainsSubstring(v any, needle string) bool {
	switch typed := v.(type) {
	case string:
		return strings.Contains(typed, needle)
	case map[string]any:
		for _, val := range typed {
			if valueContainsSubstring(val, needle) {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if valueContainsSubstring(item, needle) {
				return true
			}
		}
	}
	return false
}

// trajectoryKVStore abstracts prefix-filtered access to the AGENT_TRAJECTORIES
// JetStream KeyValue bucket (#271 M2, M6). Full unfiltered bucket scans are
// intentionally omitted from this interface.
type trajectoryKVStore interface {
	Get(ctx context.Context, key string) ([]byte, error)
	FilteredKeys(ctx context.Context, prefixFilter string) ([]string, error)
}

// byteObjectStore abstracts a JetStream ObjectStore bucket for reading
// trajectory evidence bodies.
type byteObjectStore interface {
	GetBytes(ctx context.Context, key string) ([]byte, error)
}

type natsTrajectoryKVAdapter struct {
	client     *natsclient.Client
	bucket     jetstream.KeyValue
	bucketName string
}

func (a natsTrajectoryKVAdapter) Get(ctx context.Context, key string) ([]byte, error) {
	entry, err := a.bucket.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	return entry.Value(), nil
}

func (a natsTrajectoryKVAdapter) FilteredKeys(ctx context.Context, prefixFilter string) ([]string, error) {
	if strings.TrimSpace(prefixFilter) == "" {
		return nil, fmt.Errorf("trajectory FilteredKeys requires a non-empty loop prefix filter")
	}
	return natsclient.FilteredKeys(ctx, a.bucket, prefixFilter)
}

type jetStreamObjectStoreAdapter struct {
	store jetstream.ObjectStore
}

func (a jetStreamObjectStoreAdapter) GetBytes(ctx context.Context, key string) ([]byte, error) {
	return a.store.GetBytes(ctx, key)
}

// NATSToolResultResolverOption configures optional dependencies on
// NATSToolResultResolver.
type NATSToolResultResolverOption func(*NATSToolResultResolver)

// WithStorageBuckets maps logical trajectory StorageInstance names (from
// TrajectoryFactV1.Evidence.StorageInstance, e.g. "agent-content-store") to
// their configured JetStream ObjectStore bucket names (e.g. "AGENT_CONTENT").
func WithStorageBuckets(buckets map[string]string) NATSToolResultResolverOption {
	return func(r *NATSToolResultResolver) {
		if len(buckets) == 0 {
			return
		}
		r.storageBuckets = make(map[string]string, len(buckets))
		for k, v := range buckets {
			if k != "" && v != "" {
				r.storageBuckets[k] = v
			}
		}
	}
}

// NATSToolResultResolver resolves tool_result_ref citations against the
// runtime's AGENT_TRAJECTORIES KV bucket (using per-loop TrajectoryFactPrefix
// filters) and the ObjectStore bucket named by TrajectoryFactV1.Evidence,
// verifying run lineage via chain.EntityTripleReader (#271 M2, M5, M6).
type NATSToolResultResolver struct {
	natsClient     *natsclient.Client
	entityReader   chain.EntityTripleReader
	platform       types.PlatformMeta
	storageBuckets map[string]string

	// Optional test overrides; when nil, opened lazily from natsClient.
	trajectoriesKV trajectoryKVStore
	contentStore   byteObjectStore
	contentStores  map[string]byteObjectStore
}

// NewNATSToolResultResolver constructs a NATSToolResultResolver backed by
// natsClient and entityReader. Returns nil if natsClient is nil and no test
// stores are injected.
func NewNATSToolResultResolver(
	natsClient *natsclient.Client,
	entityReader chain.EntityTripleReader,
	platform types.PlatformMeta,
	opts ...NATSToolResultResolverOption,
) *NATSToolResultResolver {
	if natsClient == nil {
		return nil
	}
	r := &NATSToolResultResolver{
		natsClient:   natsClient,
		entityReader: entityReader,
		platform:     platform,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(r)
		}
	}
	return r
}

type trajectoryToolCompletionBody struct {
	Result            agentic.ToolResult `json:"result"`
	DispatchArguments map[string]any     `json:"dispatch_arguments"`
}

// ResolveToolResult resolves ref within call's research run.
func (r *NATSToolResultResolver) ResolveToolResult(ctx context.Context, call agentic.ToolCall, ref string) (*ResolvedToolResult, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, newInvalidArgsErr("tool_result_ref is required")
	}

	rawHintLoopID, targetID := splitToolResultRef(ref)
	hintLoopID := r.normalizeLoopID(rawHintLoopID)

	callerRunID, callerRunEntityID, err := r.resolveCallerRun(ctx, call)
	if err != nil {
		return nil, err
	}

	// When an explicit <loop_id>/<call_id> hint is provided, verify that the
	// hinted loop belongs to the current research run BEFORE reading its
	// trajectory prefix, or surface a foreign-run error immediately.
	var loopIDsToScan []string
	if hintLoopID != "" {
		sameRun, prodRun, err := r.belongsToCallerRun(ctx, call.LoopID, callerRunID, callerRunEntityID, hintLoopID)
		if err != nil {
			return nil, err
		}
		if !sameRun {
			if prodRun != "" {
				return nil, newInvalidArgsErr(
					"tool_result_ref %q (loop %q, run %q) does not belong to current research run %q",
					ref, hintLoopID, prodRun, firstNonEmpty(callerRunEntityID, callerRunID, call.LoopID))
			}
			return nil, newInvalidArgsErr(
				"tool_result_ref %q (loop %q) does not belong to current research run %q",
				ref, hintLoopID, firstNonEmpty(callerRunEntityID, callerRunID, call.LoopID))
		}
		loopIDsToScan = []string{hintLoopID}
	} else {
		var err error
		loopIDsToScan, err = r.discoverRunLoopIDs(ctx, call, callerRunID)
		if err != nil {
			return nil, err
		}
	}

	candidates, err := r.collectTrajectoryCandidatesForLoops(ctx, ref, targetID, loopIDsToScan)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, newInvalidArgsErr("tool_result_ref %q not found in current research run trajectories", ref)
	}

	var foreignLoopID string
	var foreignRun string
	for _, cand := range candidates {
		sameRun, prodRun, err := r.belongsToCallerRun(ctx, call.LoopID, callerRunID, callerRunEntityID, cand.LoopID)
		if err != nil {
			return nil, err
		}
		if sameRun {
			rev, err := r.resolveProducerLoopRevision(ctx, cand.LoopID)
			if err != nil {
				return nil, err
			}
			cand.ProducerRevision = rev
			return cand, nil
		}
		foreignLoopID = cand.LoopID
		foreignRun = prodRun
	}

	if foreignRun != "" {
		return nil, newInvalidArgsErr(
			"tool_result_ref %q (loop %q, run %q) does not belong to current research run %q",
			ref, foreignLoopID, foreignRun, firstNonEmpty(callerRunEntityID, callerRunID, call.LoopID))
	}
	return nil, newInvalidArgsErr(
		"tool_result_ref %q (loop %q) does not belong to current research run %q",
		ref, foreignLoopID, firstNonEmpty(callerRunEntityID, callerRunID, call.LoopID))
}

func splitToolResultRef(ref string) (hintLoopID, targetID string) {
	if idx := strings.IndexByte(ref, '/'); idx > 0 && idx < len(ref)-1 {
		return strings.TrimSpace(ref[:idx]), strings.TrimSpace(ref[idx+1:])
	}
	return "", ref
}

// normalizeLoopID extracts the bare loop UUID/token when given a 6-part
// canonical loop entity ID (org.platform.agentic-loop.agent.execution.<loop_id>).
func (r *NATSToolResultResolver) normalizeLoopID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	if strings.Count(id, ".") >= 5 {
		if idx := strings.LastIndexByte(id, '.'); idx >= 0 && idx < len(id)-1 {
			return id[idx+1:]
		}
	}
	return id
}

func (r *NATSToolResultResolver) toLoopEntityID(idOrEntityID string) string {
	idOrEntityID = strings.TrimSpace(idOrEntityID)
	if idOrEntityID == "" {
		return ""
	}
	if strings.Count(idOrEntityID, ".") >= 5 {
		return idOrEntityID
	}
	return agentic.LoopExecutionEntityID(r.platform.Org, r.platform.Platform, idOrEntityID)
}

func (r *NATSToolResultResolver) resolveCallerRun(ctx context.Context, call agentic.ToolCall) (runID, runEntityID string, err error) {
	runID, runEntityID = runanchor.Anchor(call, r.platform.Org, r.platform.Platform)
	if (runID == "" || runEntityID == "") && call.LoopID != "" && r.entityReader != nil {
		loopEntityID := r.toLoopEntityID(call.LoopID)
		triples, readErr := r.entityReader.ReadEntity(ctx, loopEntityID)
		if readErr != nil {
			return "", "", newNetworkErr("read caller loop entity %q: %w", loopEntityID, readErr)
		}
		if triples != nil {
			if runEntityID == "" {
				runEntityID, _ = triples[predicateRunEntityID].(string)
			}
			if runID == "" {
				runID = extractRunIDFromTriples(triples)
			}
		}
	}
	return runID, runEntityID, nil
}

func extractRunIDFromTriples(triples map[string]any) string {
	if s, ok := triples[predicateLoopRun].(string); ok && s != "" {
		return s
	}
	if s, ok := triples[predicateLegacyRunID].(string); ok && s != "" {
		return s
	}
	return ""
}

func (r *NATSToolResultResolver) belongsToCallerRun(
	ctx context.Context,
	callerLoopID, callerRunID, callerRunEntityID, producerLoopID string,
) (bool, string, error) {
	producerLoopID = r.normalizeLoopID(producerLoopID)
	callerLoopID = r.normalizeLoopID(callerLoopID)
	if producerLoopID != "" && producerLoopID == callerLoopID {
		return true, callerRunEntityID, nil
	}
	if producerLoopID == "" || r.entityReader == nil {
		return false, "", nil
	}
	prodLoopEntityID := r.toLoopEntityID(producerLoopID)
	triples, err := r.entityReader.ReadEntity(ctx, prodLoopEntityID)
	if err != nil {
		return false, "", newNetworkErr("read producer loop entity %q: %w", prodLoopEntityID, err)
	}
	if len(triples) == 0 {
		return false, "", nil
	}
	prodRunEntityID, _ := triples[predicateRunEntityID].(string)
	prodRunID := extractRunIDFromTriples(triples)

	if callerRunEntityID != "" && prodRunEntityID == callerRunEntityID {
		return true, prodRunEntityID, nil
	}
	if callerRunID != "" && prodRunID == callerRunID {
		return true, prodRunID, nil
	}
	return false, firstNonEmpty(prodRunEntityID, prodRunID), nil
}

// discoverRunLoopIDs collects the loop IDs belonging to the current research
// run (the calling synthesize loop, its plan loop(s), all gather subtopic
// loops recorded under research.gather.completed-subtopic on those plan loops,
// and any prior-revision synthesize/plan loops linked via
// agent.lineage.researcher) so bare <call_id> references can be resolved via
// per-loop TrajectoryFactPrefix queries without scanning the full bucket (#271 M2).
func (r *NATSToolResultResolver) discoverRunLoopIDs(ctx context.Context, call agentic.ToolCall, callerRunID string) ([]string, error) {
	state := newRunLoopDiscovery(r)
	state.addLoop(call.LoopID)
	state.addLoop(callerRunID)
	state.seedFromToolCall(call)

	if r.entityReader == nil {
		return state.loopIDs, nil
	}
	if err := r.drainSynthesizeQueue(ctx, state); err != nil {
		return nil, err
	}
	for len(state.planQueue) > 0 {
		planID := state.planQueue[0]
		state.planQueue = state.planQueue[1:]
		if err := r.discoverFromPlanLoop(ctx, planID, state); err != nil {
			return nil, err
		}
		if err := r.drainSynthesizeQueue(ctx, state); err != nil {
			return nil, err
		}
	}
	return state.loopIDs, nil
}

type runLoopDiscovery struct {
	resolver   *NATSToolResultResolver
	loopIDs    []string
	seen       map[string]bool
	planQueue  []string
	synthQueue []string
	seenPlan   map[string]bool
	seenSynth  map[string]bool
}

func newRunLoopDiscovery(r *NATSToolResultResolver) *runLoopDiscovery {
	return &runLoopDiscovery{
		resolver:  r,
		seen:      make(map[string]bool),
		seenPlan:  make(map[string]bool),
		seenSynth: make(map[string]bool),
	}
}

func (d *runLoopDiscovery) addLoop(raw string) {
	norm := d.resolver.normalizeLoopID(raw)
	if norm == "" || d.seen[norm] {
		return
	}
	d.seen[norm] = true
	d.loopIDs = append(d.loopIDs, norm)
}

func (d *runLoopDiscovery) enqueuePlan(raw string) {
	norm := d.resolver.normalizeLoopID(raw)
	if norm != "" && !d.seenPlan[norm] {
		d.seenPlan[norm] = true
		d.planQueue = append(d.planQueue, norm)
		d.addLoop(norm)
	}
}

func (d *runLoopDiscovery) enqueueSynth(raw string) {
	norm := d.resolver.normalizeLoopID(raw)
	if norm != "" && !d.seenSynth[norm] {
		d.seenSynth[norm] = true
		d.synthQueue = append(d.synthQueue, norm)
		d.addLoop(norm)
	}
}

func (d *runLoopDiscovery) seedFromToolCall(call agentic.ToolCall) {
	if call.LoopID != "" {
		d.enqueueSynth(call.LoopID)
	}
	if call.Metadata == nil {
		return
	}
	rel, ok := call.Metadata["related_loops"].(map[string]any)
	if !ok {
		return
	}
	for k, v := range rel {
		s, ok := v.(string)
		if !ok {
			continue
		}
		if strings.Contains(k, "plan") {
			d.enqueuePlan(s)
		} else {
			d.enqueueSynth(s)
		}
	}
}

func (r *NATSToolResultResolver) drainSynthesizeQueue(ctx context.Context, state *runLoopDiscovery) error {
	for len(state.synthQueue) > 0 {
		synthID := state.synthQueue[0]
		state.synthQueue = state.synthQueue[1:]
		triples, err := r.entityReader.ReadEntity(ctx, r.toLoopEntityID(synthID))
		if err != nil {
			return newNetworkErr("read synthesize loop entity %q: %w", synthID, err)
		}
		if p, ok := triples[predicateLineagePlanLoop].(string); ok {
			state.enqueuePlan(p)
		}
		if p, ok := triples[predicateLineagePlanLoopEntity].(string); ok {
			state.enqueuePlan(p)
		}
		if prior, ok := triples[predicateLineagePriorSynthesize].(string); ok {
			state.enqueueSynth(prior)
		}
	}
	return nil
}

func (r *NATSToolResultResolver) discoverFromPlanLoop(ctx context.Context, planID string, state *runLoopDiscovery) error {
	planEntityID := r.toLoopEntityID(planID)
	if multiReader, ok := r.entityReader.(chain.MultiEntityTripleReader); ok {
		allTriples, err := multiReader.ReadEntityAll(ctx, planEntityID)
		if err != nil {
			return newNetworkErr("read plan loop entity %q: %w", planEntityID, err)
		}
		for _, obj := range allTriples[predicateGatherCompletedLoop] {
			if s, ok := obj.(string); ok {
				state.addLoop(s)
			}
		}
		for _, obj := range allTriples[predicateLineagePriorSynthesize] {
			if s, ok := obj.(string); ok {
				state.enqueueSynth(s)
			}
		}
		return nil
	}

	triples, err := r.entityReader.ReadEntity(ctx, planEntityID)
	if err != nil {
		return newNetworkErr("read plan loop entity %q: %w", planEntityID, err)
	}
	switch v := triples[predicateGatherCompletedLoop].(type) {
	case string:
		state.addLoop(v)
	case []string:
		for _, s := range v {
			state.addLoop(s)
		}
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok {
				state.addLoop(s)
			}
		}
	}
	if prior, ok := triples[predicateLineagePriorSynthesize].(string); ok {
		state.enqueueSynth(prior)
	}
	return nil
}

// resolveProducerLoopRevision determines which research revision (1, 2, ...)
// produced the given loop's tool results by reading research.plan.revision or
// research.artifact.revision on the loop entity or its parent plan loop entity.
func (r *NATSToolResultResolver) resolveProducerLoopRevision(ctx context.Context, loopID string) (int, error) {
	if loopID == "" || r.entityReader == nil {
		return 0, nil
	}
	triples, err := r.entityReader.ReadEntity(ctx, r.toLoopEntityID(loopID))
	if err != nil {
		return 0, newNetworkErr("read loop entity %q for revision: %w", loopID, err)
	}
	if rev := parsePositiveInt(triples[predicatePlanRevision]); rev > 0 {
		return rev, nil
	}
	if rev := parsePositiveInt(triples[predicateArtifactRevision]); rev > 0 {
		return rev, nil
	}
	for _, pred := range []string{predicateLineagePlanLoopEntity, predicateLineagePlanLoop} {
		if parent, ok := triples[pred].(string); ok && parent != "" {
			pt, err := r.entityReader.ReadEntity(ctx, r.toLoopEntityID(parent))
			if err != nil {
				return 0, newNetworkErr("read parent plan loop entity %q for revision: %w", parent, err)
			}
			if rev := parsePositiveInt(pt[predicatePlanRevision]); rev > 0 {
				return rev, nil
			}
		}
	}
	return 0, nil
}

func parsePositiveInt(v any) int {
	switch typed := v.(type) {
	case int:
		if typed > 0 {
			return typed
		}
	case int64:
		if typed > 0 {
			return int(typed)
		}
	case float64:
		if typed > 0 {
			return int(typed)
		}
	case json.Number:
		if n, err := typed.Int64(); err == nil && n > 0 {
			return int(n)
		}
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil && n > 0 {
			return n
		}
	}
	return 0
}

func (r *NATSToolResultResolver) collectTrajectoryCandidatesForLoops(
	ctx context.Context,
	rawRef, targetID string,
	loopIDs []string,
) ([]*ResolvedToolResult, error) {
	trajKV, err := r.getTrajectoryKV(ctx)
	if err != nil {
		return nil, err
	}
	if trajKV == nil {
		return nil, newNetworkErr("trajectory KV store unavailable")
	}

	var out []*ResolvedToolResult
	seen := make(map[string]bool)
	for _, loopID := range loopIDs {
		loopID = r.normalizeLoopID(loopID)
		if loopID == "" {
			continue
		}
		filter := agentic.TrajectoryFactPrefix(loopID) + ">"
		keys, err := trajKV.FilteredKeys(ctx, filter)
		if err != nil {
			if errors.Is(err, jetstream.ErrNoKeysFound) {
				continue
			}
			return nil, newNetworkErr("list trajectory keys for loop %q: %w", loopID, err)
		}
		for _, k := range keys {
			data, getErr := trajKV.Get(ctx, k)
			if getErr != nil {
				if errors.Is(getErr, jetstream.ErrKeyNotFound) {
					continue
				}
				return nil, newNetworkErr("get trajectory fact %q: %w", k, getErr)
			}
			cand, matched, decErr := r.decodeTrajectoryCandidate(ctx, loopID, rawRef, targetID, data)
			if decErr != nil {
				return nil, decErr
			}
			if !matched || cand == nil {
				continue
			}
			dedupKey := cand.LoopID + "|" + cand.ExecutionID + "|" + cand.CallID
			if !seen[dedupKey] {
				seen[dedupKey] = true
				out = append(out, cand)
			}
		}
	}
	return out, nil
}

func (r *NATSToolResultResolver) decodeTrajectoryCandidate(
	ctx context.Context,
	loopID, rawRef, targetID string,
	data []byte,
) (*ResolvedToolResult, bool, error) {
	var fact agentic.TrajectoryFactV1
	if err := json.Unmarshal(data, &fact); err != nil {
		return nil, false, newInternalErr("decode trajectory fact JSON: %w", err)
	}
	if fact.Kind != agentic.TrajectoryKindToolCompleted || fact.Evidence == nil {
		return nil, false, nil
	}

	// Quick filter before fetching the ObjectStore body when SourceCorrelation
	// is populated and does not match targetID/rawRef (still allowing body
	// inspection if SourceCorrelation is empty).
	if fact.SourceCorrelation != "" &&
		fact.SourceCorrelation != targetID &&
		fact.SourceCorrelation != rawRef {
		return nil, false, nil
	}

	contentStore, err := r.getContentStore(ctx, fact.Evidence.StorageInstance)
	if err != nil {
		return nil, false, err
	}
	if contentStore == nil {
		return nil, false, newNetworkErr("trajectory content store unavailable for instance %q", fact.Evidence.StorageInstance)
	}

	bodyBytes, err := contentStore.GetBytes(ctx, fact.Evidence.Key)
	if err != nil {
		return nil, false, newNetworkErr("get trajectory evidence %q from %q: %w",
			fact.Evidence.Key, fact.Evidence.StorageInstance, err)
	}

	// Verify SHA-256 digest when recorded on the trajectory fact (#271 m5).
	if strings.TrimSpace(fact.EvidenceDigest) != "" {
		sum := sha256.Sum256(bodyBytes)
		gotDigest := hex.EncodeToString(sum[:])
		if !strings.EqualFold(gotDigest, strings.TrimSpace(fact.EvidenceDigest)) {
			return nil, false, newInternalErr(
				"trajectory evidence digest mismatch for key %q: got %s, want %s",
				fact.Evidence.Key, gotDigest, fact.EvidenceDigest)
		}
	}

	var env agentic.TrajectoryEvidenceV1
	if err := json.Unmarshal(bodyBytes, &env); err != nil {
		return nil, false, newInternalErr("decode trajectory evidence envelope %q: %w", fact.Evidence.Key, err)
	}
	if len(env.Body) == 0 {
		return nil, false, newInternalErr("trajectory evidence envelope %q has empty body", fact.Evidence.Key)
	}
	var body trajectoryToolCompletionBody
	if err := json.Unmarshal(env.Body, &body); err != nil {
		return nil, false, newInternalErr("decode trajectory tool completion body %q: %w", fact.Evidence.Key, err)
	}

	cand := &ResolvedToolResult{
		Ref:               rawRef,
		CallID:            firstNonEmpty(body.Result.CallID, fact.SourceCorrelation),
		ExecutionID:       body.Result.ExecutionID,
		LoopID:            firstNonEmpty(body.Result.LoopID, loopID),
		ToolName:          firstNonEmpty(body.Result.Name, fact.ToolPreview),
		Status:            string(fact.Status),
		Error:             body.Result.Error,
		ErrorKind:         string(body.Result.ErrorKind),
		Content:           body.Result.Content,
		DispatchArguments: body.DispatchArguments,
		ObservedAt:        fact.ObservedAt,
	}
	if !matchesTargetID(cand, rawRef, targetID) {
		return nil, false, nil
	}
	return cand, true, nil
}

func matchesTargetID(cand *ResolvedToolResult, rawRef, targetID string) bool {
	for _, want := range []string{targetID, rawRef} {
		if want == "" {
			continue
		}
		if cand.CallID == want || cand.ExecutionID == want {
			return true
		}
	}
	return false
}

func (r *NATSToolResultResolver) getTrajectoryKV(ctx context.Context) (trajectoryKVStore, error) {
	if r.trajectoriesKV != nil {
		return r.trajectoriesKV, nil
	}
	if r.natsClient == nil {
		return nil, newNetworkErr("nats client unavailable for trajectory KV bucket %q", agentic.TrajectoryBucketName)
	}
	trajBucket, err := r.natsClient.GetKeyValueBucket(ctx, agentic.TrajectoryBucketName)
	if err != nil || trajBucket == nil {
		return nil, newNetworkErr("open trajectory KV bucket %q: %w", agentic.TrajectoryBucketName, err)
	}
	return natsTrajectoryKVAdapter{
		client:     r.natsClient,
		bucket:     trajBucket,
		bucketName: agentic.TrajectoryBucketName,
	}, nil
}

func (r *NATSToolResultResolver) resolveContentBucketName(storageInstance string) string {
	storageInstance = strings.TrimSpace(storageInstance)
	if storageInstance != "" && len(r.storageBuckets) > 0 {
		if mapped, ok := r.storageBuckets[storageInstance]; ok && mapped != "" {
			return mapped
		}
	}
	if storageInstance != "" {
		return storageInstance
	}
	return defaultAgentContentBucket
}

func (r *NATSToolResultResolver) getContentStore(ctx context.Context, storageInstance string) (byteObjectStore, error) {
	bucketName := r.resolveContentBucketName(storageInstance)
	if len(r.contentStores) > 0 {
		if st, ok := r.contentStores[bucketName]; ok && st != nil {
			return st, nil
		}
		if st, ok := r.contentStores[storageInstance]; ok && st != nil {
			return st, nil
		}
	}
	if r.contentStore != nil {
		return r.contentStore, nil
	}
	if r.natsClient == nil {
		return nil, newNetworkErr("nats client unavailable for trajectory ObjectStore %q", bucketName)
	}
	js, err := r.natsClient.JetStream()
	if err != nil || js == nil {
		return nil, newNetworkErr("jetstream unavailable for trajectory ObjectStore %q: %w", bucketName, err)
	}
	objStore, err := js.ObjectStore(ctx, bucketName)
	if err != nil && bucketName != defaultAgentContentBucket {
		// Fallback to AGENT_CONTENT when StorageInstance holds an unmapped logical
		// component name (e.g. "agent-content-store") and no explicit mapping was
		// provided.
		if fallbackStore, fbErr := js.ObjectStore(ctx, defaultAgentContentBucket); fbErr == nil && fallbackStore != nil {
			return jetStreamObjectStoreAdapter{store: fallbackStore}, nil
		}
	}
	if err != nil || objStore == nil {
		return nil, newNetworkErr("open trajectory ObjectStore %q (instance %q): %w", bucketName, storageInstance, err)
	}
	return jetStreamObjectStoreAdapter{store: objStore}, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
