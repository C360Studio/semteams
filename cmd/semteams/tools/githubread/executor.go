package githubread

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/c360studio/semstreams/agentic"
	agentictools "github.com/c360studio/semstreams/processor/agentic-tools"

	"github.com/c360studio/semteams/cmd/semteams/portfolio"
	"github.com/c360studio/semteams/cmd/semteams/research"
)

// ToolName is the LLM-facing tool name registered on the ExecutorRegistry.
const ToolName = "github_read"

// ToolCategory is the dedicated agentic-tools category for github_read (#273).
const ToolCategory = agentictools.ToolCategory("github")

// Supported activity kinds read by github_read.
const (
	KindIssues       = "issues"
	KindPullRequests = "pull_requests"
	KindReleases     = "releases"
	KindCommits      = "commits"
	KindCIStatus     = "ci_status"
)

// AllKinds is the default set of GitHub activity kinds queried when the caller omits kinds.
var AllKinds = []string{
	KindIssues,
	KindPullRequests,
	KindReleases,
	KindCommits,
	KindCIStatus,
}

// Config configures the product-local github_read Executor.
type Config struct {
	// Portfolio is the validated operator-authored portfolio allowlist.
	// Required: calls fail closed when nil or empty.
	Portfolio *portfolio.Portfolio

	// Token is the GitHub Personal Access Token or fine-grained token sourced
	// from server configuration (never exposed to the bash environment).
	Token string

	// APIBase overrides DefaultAPIBase (used by unit/contract httptest servers).
	APIBase string

	// RequestTimeout bounds each HTTP API request when Client is constructed by NewExecutor.
	RequestTimeout time.Duration

	// Client optionally overrides the HTTPClient constructed from Token/APIBase.
	Client Client

	// Cursors persists per-(repository, kind) watermarks and version digests.
	// Defaults to an in-memory store when nil.
	Cursors CursorStore

	// Clock supplies the retrieval timestamp stamped on EvidenceRecords.
	// Defaults to time.Now when nil.
	Clock func() time.Time
}

// Result is the structured JSON payload returned in ToolResult.Content.
type Result struct {
	Channel        research.EvidenceChannel     `json:"channel"`
	Repositories   []string                     `json:"repositories"`
	Kinds          []string                     `json:"kinds"`
	NewCount       int                          `json:"new_count"`
	UnchangedCount int                          `json:"unchanged_count"`
	Cursors        map[string]map[string]string `json:"cursors"`
	Records        []research.EvidenceRecord    `json:"records"`
}

// Executor implements agentic.ToolExecutor for the read-only github_read tool.
type Executor struct {
	config  Config
	client  Client
	cursors CursorStore
	clock   func() time.Time
	logger  *slog.Logger
}

// NewExecutor constructs a github_read Executor and registers ToolName under
// ToolCategory ("github") in agentic-tools.
func NewExecutor(cfg Config, logger *slog.Logger) *Executor {
	if logger == nil {
		logger = slog.Default()
	}
	client := cfg.Client
	if client == nil {
		client = NewHTTPClient(cfg.Token, cfg.APIBase, cfg.RequestTimeout)
	}
	cursors := cfg.Cursors
	if cursors == nil {
		cursors = NewMemoryCursorStore()
	}
	clock := cfg.Clock
	if clock == nil {
		clock = time.Now
	}
	agentictools.RegisterToolCategory(ToolName, ToolCategory)
	return &Executor{
		config:  cfg,
		client:  client,
		cursors: cursors,
		clock:   clock,
		logger:  logger,
	}
}

// ListTools returns the LLM-facing schema for github_read.
func (e *Executor) ListTools() []agentic.ToolDefinition {
	return []agentic.ToolDefinition{{
		Name:        ToolName,
		Description: "Read GitHub activity (issues, pull_requests, releases, commits, ci_status) for repositories in the operator-configured portfolio with per-(repository, kind) cursors. Returns channel-typed evidence records (channel=\"github\", locator, origin_id, source_class=\"primary\", retrieved_at, raw_body) that emit_research_artifact can mechanically verify. Read-only; unchanged items already observed in prior runs are suppressed unless include_unchanged is true.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"program": map[string]any{
					"type":        "string",
					"description": "Optional program ID from the configured portfolio to scope the read.",
				},
				"project": map[string]any{
					"type":        "string",
					"description": "Optional project ID from the configured portfolio to scope the read.",
				},
				"repository": map[string]any{
					"type":        "string",
					"description": "Optional \"owner/name\" repository slug from the configured portfolio. Rejected if not in the portfolio.",
				},
				"kinds": map[string]any{
					"type":        "array",
					"description": "Activity kinds to read: issues, pull_requests, releases, commits, ci_status. Defaults to all five when omitted.",
					"items": map[string]any{
						"type": "string",
						"enum": []string{KindIssues, KindPullRequests, KindReleases, KindCommits, KindCIStatus},
					},
				},
				"state": map[string]any{
					"type":        "string",
					"description": "State filter for issues and pull_requests: open, closed, or all (default: all).",
					"enum":        []string{"open", "closed", "all"},
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Maximum items per kind per repository (1..100, default 20).",
				},
				"include_unchanged": map[string]any{
					"type":        "boolean",
					"description": "When true, include items even if their version digest matches the stored cursor watermark. Default false.",
				},
			},
		},
	}}
}

type executeRequest struct {
	repos            []portfolio.Repository
	kinds            []string
	state            string
	limit            int
	includeUnchanged bool
}

func (e *Executor) parseExecuteRequest(args map[string]any) (executeRequest, error) {
	programID, _ := args["program"].(string)
	projectID, _ := args["project"].(string)
	repoRef, _ := args["repository"].(string)

	repos, err := e.config.Portfolio.ResolveRepositories(programID, projectID, repoRef)
	if err != nil {
		return executeRequest{}, err
	}
	if len(repos) == 0 {
		return executeRequest{}, fmt.Errorf("no repositories configured in the requested portfolio scope")
	}

	kinds, err := parseKinds(args["kinds"])
	if err != nil {
		return executeRequest{}, err
	}

	state := "all"
	if rawState, ok := args["state"].(string); ok && strings.TrimSpace(rawState) != "" {
		state = strings.ToLower(strings.TrimSpace(rawState))
		if state != "open" && state != "closed" && state != "all" {
			return executeRequest{}, fmt.Errorf("invalid state %q: must be open, closed, or all", rawState)
		}
	}

	limit, err := parseLimit(args["limit"])
	if err != nil {
		return executeRequest{}, err
	}

	includeUnchanged, _ := args["include_unchanged"].(bool)
	return executeRequest{
		repos:            repos,
		kinds:            kinds,
		state:            state,
		limit:            limit,
		includeUnchanged: includeUnchanged,
	}, nil
}

// Execute reads GitHub state for the requested portfolio scope, filters out
// unchanged items via the CursorStore, and returns validated EvidenceRecords.
func (e *Executor) Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
	if e.config.Portfolio == nil {
		return e.argError(call, "github_read disabled: deployment has no configured portfolio"), nil
	}

	req, err := e.parseExecuteRequest(call.Arguments)
	if err != nil {
		return e.argError(call, err.Error()), nil
	}

	retrievedAt := e.clock().UTC()
	records := make([]research.EvidenceRecord, 0)
	cursorsOut := make(map[string]map[string]string, len(req.repos))
	repoSlugs := make([]string, 0, len(req.repos))
	unchangedCount := 0

	for _, repo := range req.repos {
		repoSlugs = append(repoSlugs, repo.Slug())
		repoKey := portfolio.RepositoryKey(repo.Owner, repo.Name)
		cursorsOut[repoKey] = make(map[string]string, len(req.kinds))

		for _, kind := range req.kinds {
			newRecs, unchanged, watermark, toolErr := e.executeRepoKind(
				ctx, call, repo, repoKey, kind,
				ListOpts{State: req.state, Limit: req.limit},
				req.includeUnchanged, retrievedAt,
			)
			if toolErr != nil {
				return *toolErr, nil
			}
			records = append(records, newRecs...)
			unchangedCount += unchanged
			if watermark != "" {
				cursorsOut[repoKey][kind] = watermark
			}
		}
	}

	res := Result{
		Channel:        research.ChannelGitHub,
		Repositories:   repoSlugs,
		Kinds:          req.kinds,
		NewCount:       len(records),
		UnchangedCount: unchangedCount,
		Cursors:        cursorsOut,
		Records:        records,
	}
	payload, err := json.Marshal(res)
	if err != nil {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     fmt.Sprintf("marshal github_read result: %v", err),
			ErrorKind: agentic.ToolErrorInternal,
		}, nil
	}

	e.logger.Info("github_read completed",
		slog.Int("repositories", len(repoSlugs)),
		slog.Int("new_records", len(records)),
		slog.Int("unchanged_records", unchangedCount))

	return agentic.ToolResult{
		CallID:  call.ID,
		Name:    call.Name,
		Content: string(payload),
		Metadata: map[string]any{
			"channel":         string(research.ChannelGitHub),
			"new_count":       len(records),
			"unchanged_count": unchangedCount,
			"repositories":    repoSlugs,
		},
	}, nil
}

func (e *Executor) executeRepoKind(
	ctx context.Context,
	call agentic.ToolCall,
	repo portfolio.Repository,
	repoKey, kind string,
	opts ListOpts,
	includeUnchanged bool,
	retrievedAt time.Time,
) ([]research.EvidenceRecord, int, string, *agentic.ToolResult) {
	prevCursor, _ := e.cursors.Get(repoKey, kind)
	if prevCursor.Seen == nil {
		prevCursor.Seen = make(map[string]string)
	}
	nextCursor := prevCursor.clone()

	fetched, err := e.fetchKind(ctx, repo, kind, opts, retrievedAt, call.ID)
	if err != nil {
		res := agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     fmt.Sprintf("github_read %s (%s): %v", repo.Slug(), kind, err),
			ErrorKind: ClassifyError(err),
		}
		return nil, 0, "", &res
	}

	records := make([]research.EvidenceRecord, 0, len(fetched))
	unchangedCount := 0
	for _, item := range fetched {
		if err := item.record.ValidateToolResult(); err != nil {
			res := agentic.ToolResult{
				CallID:    call.ID,
				Name:      call.Name,
				Error:     fmt.Sprintf("invalid evidence record for %s: %v", item.record.Locator, err),
				ErrorKind: agentic.ToolErrorInternal,
			}
			return nil, 0, "", &res
		}

		digest := VersionDigest(item.activityAt, item.record.RawBody)
		if !includeUnchanged && prevCursor.Seen[item.record.OriginID] == digest {
			unchangedCount++
			continue
		}

		records = append(records, item.record)
		nextCursor.Seen[item.record.OriginID] = digest
		if item.activityAt.After(nextCursor.Watermark) {
			nextCursor.Watermark = item.activityAt.UTC()
		}
	}

	if err := e.cursors.Put(repoKey, kind, nextCursor); err != nil {
		res := agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     fmt.Sprintf("persist cursor for %s (%s): %v", repo.Slug(), kind, err),
			ErrorKind: agentic.ToolErrorInternal,
		}
		return nil, 0, "", &res
	}

	watermark := ""
	if !nextCursor.Watermark.IsZero() {
		watermark = nextCursor.Watermark.UTC().Format(time.RFC3339Nano)
	}
	return records, unchangedCount, watermark, nil
}

type fetchedRecord struct {
	record     research.EvidenceRecord
	activityAt time.Time
}

func (e *Executor) fetchKind(
	ctx context.Context,
	repo portfolio.Repository,
	kind string,
	opts ListOpts,
	retrievedAt time.Time,
	callID string,
) ([]fetchedRecord, error) {
	switch kind {
	case KindIssues:
		return e.fetchIssues(ctx, repo, opts, retrievedAt, callID)
	case KindPullRequests:
		return e.fetchPullRequests(ctx, repo, opts, retrievedAt, callID)
	case KindReleases:
		return e.fetchReleases(ctx, repo, opts, retrievedAt, callID)
	case KindCommits:
		return e.fetchCommits(ctx, repo, opts, retrievedAt, callID)
	case KindCIStatus:
		return e.fetchWorkflowRuns(ctx, repo, opts, retrievedAt, callID)
	default:
		return nil, fmt.Errorf("unsupported kind %q", kind)
	}
}

func resolveActivityTime(primary, fallback, retrievedAt time.Time) time.Time {
	if !primary.IsZero() {
		return primary.UTC()
	}
	if !fallback.IsZero() {
		return fallback.UTC()
	}
	return retrievedAt.UTC()
}

func (e *Executor) fetchIssues(ctx context.Context, repo portfolio.Repository, opts ListOpts, retrievedAt time.Time, callID string) ([]fetchedRecord, error) {
	items, err := e.client.ListIssues(ctx, repo.Owner, repo.Name, opts)
	if err != nil {
		return nil, err
	}
	out := make([]fetchedRecord, 0, len(items))
	for _, it := range items {
		loc := fmt.Sprintf("github://%s/%s/issues/%d", repo.Owner, repo.Name, it.Number)
		originID, err := research.NormalizeOriginID(research.ChannelGitHub, loc)
		if err != nil {
			return nil, err
		}
		actUTC := resolveActivityTime(it.UpdatedAt, it.CreatedAt, retrievedAt)
		out = append(out, fetchedRecord{
			activityAt: actUTC,
			record: research.EvidenceRecord{
				Channel:       research.ChannelGitHub,
				Locator:       loc,
				OriginID:      originID,
				SourceClass:   research.SourceClassPrimary,
				RetrievedAt:   retrievedAt,
				RawBody:       formatIssueBody(repo, it),
				ToolResultRef: callID,
				Kind:          "issue",
				Repository:    repo.Slug(),
				HTMLURL:       it.HTMLURL,
				UpdatedAt:     &actUTC,
			},
		})
	}
	return out, nil
}

func (e *Executor) fetchPullRequests(ctx context.Context, repo portfolio.Repository, opts ListOpts, retrievedAt time.Time, callID string) ([]fetchedRecord, error) {
	items, err := e.client.ListPullRequests(ctx, repo.Owner, repo.Name, opts)
	if err != nil {
		return nil, err
	}
	out := make([]fetchedRecord, 0, len(items))
	for _, pr := range items {
		loc := fmt.Sprintf("github://%s/%s/pulls/%d", repo.Owner, repo.Name, pr.Number)
		originID, err := research.NormalizeOriginID(research.ChannelGitHub, loc)
		if err != nil {
			return nil, err
		}
		actUTC := resolveActivityTime(pr.UpdatedAt, pr.CreatedAt, retrievedAt)
		out = append(out, fetchedRecord{
			activityAt: actUTC,
			record: research.EvidenceRecord{
				Channel:       research.ChannelGitHub,
				Locator:       loc,
				OriginID:      originID,
				SourceClass:   research.SourceClassPrimary,
				RetrievedAt:   retrievedAt,
				RawBody:       formatPullRequestBody(repo, pr),
				ToolResultRef: callID,
				Kind:          "pull_request",
				Repository:    repo.Slug(),
				HTMLURL:       pr.HTMLURL,
				UpdatedAt:     &actUTC,
			},
		})
	}
	return out, nil
}

func (e *Executor) fetchReleases(ctx context.Context, repo portfolio.Repository, opts ListOpts, retrievedAt time.Time, callID string) ([]fetchedRecord, error) {
	items, err := e.client.ListReleases(ctx, repo.Owner, repo.Name, opts)
	if err != nil {
		return nil, err
	}
	out := make([]fetchedRecord, 0, len(items))
	for _, rel := range items {
		tag := strings.TrimSpace(rel.TagName)
		if tag == "" {
			tag = fmt.Sprintf("id-%d", rel.ID)
		}
		loc := fmt.Sprintf("github://%s/%s/releases/%s", repo.Owner, repo.Name, tag)
		originID, err := research.NormalizeOriginID(research.ChannelGitHub, loc)
		if err != nil {
			return nil, err
		}
		actUTC := resolveActivityTime(rel.PublishedAt, rel.CreatedAt, retrievedAt)
		out = append(out, fetchedRecord{
			activityAt: actUTC,
			record: research.EvidenceRecord{
				Channel:       research.ChannelGitHub,
				Locator:       loc,
				OriginID:      originID,
				SourceClass:   research.SourceClassPrimary,
				RetrievedAt:   retrievedAt,
				RawBody:       formatReleaseBody(repo, rel),
				ToolResultRef: callID,
				Kind:          "release",
				Repository:    repo.Slug(),
				HTMLURL:       rel.HTMLURL,
				UpdatedAt:     &actUTC,
			},
		})
	}
	return out, nil
}

func (e *Executor) fetchCommits(ctx context.Context, repo portfolio.Repository, opts ListOpts, retrievedAt time.Time, callID string) ([]fetchedRecord, error) {
	items, err := e.client.ListCommits(ctx, repo.Owner, repo.Name, opts)
	if err != nil {
		return nil, err
	}
	out := make([]fetchedRecord, 0, len(items))
	for _, c := range items {
		loc := fmt.Sprintf("github://%s/%s/commits/%s", repo.Owner, repo.Name, c.SHA)
		originID, err := research.NormalizeOriginID(research.ChannelGitHub, loc)
		if err != nil {
			return nil, err
		}
		actUTC := resolveActivityTime(c.CommittedAt, time.Time{}, retrievedAt)
		out = append(out, fetchedRecord{
			activityAt: actUTC,
			record: research.EvidenceRecord{
				Channel:       research.ChannelGitHub,
				Locator:       loc,
				OriginID:      originID,
				SourceClass:   research.SourceClassPrimary,
				RetrievedAt:   retrievedAt,
				RawBody:       formatCommitBody(repo, c),
				ToolResultRef: callID,
				Kind:          "commit",
				Repository:    repo.Slug(),
				HTMLURL:       c.HTMLURL,
				UpdatedAt:     &actUTC,
			},
		})
	}
	return out, nil
}

func (e *Executor) fetchWorkflowRuns(ctx context.Context, repo portfolio.Repository, opts ListOpts, retrievedAt time.Time, callID string) ([]fetchedRecord, error) {
	items, err := e.client.ListWorkflowRuns(ctx, repo.Owner, repo.Name, opts)
	if err != nil {
		return nil, err
	}
	out := make([]fetchedRecord, 0, len(items))
	for _, run := range items {
		loc := fmt.Sprintf("github://%s/%s/actions/runs/%d", repo.Owner, repo.Name, run.ID)
		originID, err := research.NormalizeOriginID(research.ChannelGitHub, loc)
		if err != nil {
			return nil, err
		}
		actUTC := resolveActivityTime(run.UpdatedAt, run.CreatedAt, retrievedAt)
		out = append(out, fetchedRecord{
			activityAt: actUTC,
			record: research.EvidenceRecord{
				Channel:       research.ChannelGitHub,
				Locator:       loc,
				OriginID:      originID,
				SourceClass:   research.SourceClassPrimary,
				RetrievedAt:   retrievedAt,
				RawBody:       formatWorkflowRunBody(repo, run),
				ToolResultRef: callID,
				Kind:          "ci_status",
				Repository:    repo.Slug(),
				HTMLURL:       run.HTMLURL,
				UpdatedAt:     &actUTC,
			},
		})
	}
	return out, nil
}

func formatIssueBody(repo portfolio.Repository, it Issue) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Repository: %s\nKind: issue\nNumber: #%d\nState: %s\nTitle: %s\n",
		repo.Slug(), it.Number, it.State, it.Title)
	if it.Author != "" {
		fmt.Fprintf(&b, "Author: %s\n", it.Author)
	}
	if len(it.Assignees) > 0 {
		fmt.Fprintf(&b, "Assignees: %s\n", strings.Join(it.Assignees, ", "))
	}
	if len(it.Labels) > 0 {
		fmt.Fprintf(&b, "Labels: %s\n", strings.Join(it.Labels, ", "))
	}
	if it.Milestone != "" {
		fmt.Fprintf(&b, "Milestone: %s\n", it.Milestone)
	}
	if !it.UpdatedAt.IsZero() {
		fmt.Fprintf(&b, "UpdatedAt: %s\n", it.UpdatedAt.UTC().Format(time.RFC3339))
	}
	if it.HTMLURL != "" {
		fmt.Fprintf(&b, "URL: %s\n", it.HTMLURL)
	}
	if strings.TrimSpace(it.Body) != "" {
		fmt.Fprintf(&b, "Body:\n%s\n", strings.TrimSpace(it.Body))
	}
	return b.String()
}

func formatPullRequestBody(repo portfolio.Repository, pr PullRequest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Repository: %s\nKind: pull_request\nNumber: #%d\nState: %s\nDraft: %t\nTitle: %s\n",
		repo.Slug(), pr.Number, pr.State, pr.Draft, pr.Title)
	if pr.Author != "" {
		fmt.Fprintf(&b, "Author: %s\n", pr.Author)
	}
	if pr.HeadRef != "" || pr.BaseRef != "" {
		fmt.Fprintf(&b, "Branches: %s -> %s\n", pr.HeadRef, pr.BaseRef)
	}
	if pr.MergedAt != nil && !pr.MergedAt.IsZero() {
		fmt.Fprintf(&b, "MergedAt: %s\n", pr.MergedAt.UTC().Format(time.RFC3339))
	}
	if len(pr.Labels) > 0 {
		fmt.Fprintf(&b, "Labels: %s\n", strings.Join(pr.Labels, ", "))
	}
	if !pr.UpdatedAt.IsZero() {
		fmt.Fprintf(&b, "UpdatedAt: %s\n", pr.UpdatedAt.UTC().Format(time.RFC3339))
	}
	if pr.HTMLURL != "" {
		fmt.Fprintf(&b, "URL: %s\n", pr.HTMLURL)
	}
	if strings.TrimSpace(pr.Body) != "" {
		fmt.Fprintf(&b, "Body:\n%s\n", strings.TrimSpace(pr.Body))
	}
	return b.String()
}

func formatReleaseBody(repo portfolio.Repository, rel Release) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Repository: %s\nKind: release\nTag: %s\nName: %s\nDraft: %t\nPrerelease: %t\n",
		repo.Slug(), rel.TagName, rel.Name, rel.Draft, rel.Prerelease)
	if rel.Author != "" {
		fmt.Fprintf(&b, "Author: %s\n", rel.Author)
	}
	if !rel.PublishedAt.IsZero() {
		fmt.Fprintf(&b, "PublishedAt: %s\n", rel.PublishedAt.UTC().Format(time.RFC3339))
	}
	if rel.HTMLURL != "" {
		fmt.Fprintf(&b, "URL: %s\n", rel.HTMLURL)
	}
	if strings.TrimSpace(rel.Body) != "" {
		fmt.Fprintf(&b, "Body:\n%s\n", strings.TrimSpace(rel.Body))
	}
	return b.String()
}

func formatCommitBody(repo portfolio.Repository, c Commit) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Repository: %s\nKind: commit\nSHA: %s\n", repo.Slug(), c.SHA)
	if c.Author != "" {
		fmt.Fprintf(&b, "Author: %s\n", c.Author)
	}
	if !c.CommittedAt.IsZero() {
		fmt.Fprintf(&b, "CommittedAt: %s\n", c.CommittedAt.UTC().Format(time.RFC3339))
	}
	if c.HTMLURL != "" {
		fmt.Fprintf(&b, "URL: %s\n", c.HTMLURL)
	}
	if strings.TrimSpace(c.Message) != "" {
		fmt.Fprintf(&b, "Message:\n%s\n", strings.TrimSpace(c.Message))
	}
	return b.String()
}

func formatWorkflowRunBody(repo portfolio.Repository, run WorkflowRun) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Repository: %s\nKind: ci_status\nRunID: %d\nWorkflow: %s\nStatus: %s\nConclusion: %s\n",
		repo.Slug(), run.ID, run.Name, run.Status, run.Conclusion)
	if run.HeadBranch != "" {
		fmt.Fprintf(&b, "Branch: %s\n", run.HeadBranch)
	}
	if run.HeadSHA != "" {
		fmt.Fprintf(&b, "HeadSHA: %s\n", run.HeadSHA)
	}
	if !run.UpdatedAt.IsZero() {
		fmt.Fprintf(&b, "UpdatedAt: %s\n", run.UpdatedAt.UTC().Format(time.RFC3339))
	}
	if run.HTMLURL != "" {
		fmt.Fprintf(&b, "URL: %s\n", run.HTMLURL)
	}
	return b.String()
}

func parseKinds(raw any) ([]string, error) {
	if raw == nil {
		return slices.Clone(AllKinds), nil
	}
	var items []string
	switch v := raw.(type) {
	case []string:
		items = v
	case []any:
		items = make([]string, 0, len(v))
		for _, elem := range v {
			s, ok := elem.(string)
			if !ok {
				return nil, fmt.Errorf("kinds elements must be strings")
			}
			items = append(items, s)
		}
	default:
		return nil, fmt.Errorf("kinds must be an array of strings")
	}
	if len(items) == 0 {
		return slices.Clone(AllKinds), nil
	}
	out := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, k := range items {
		k = strings.ToLower(strings.TrimSpace(k))
		if !slices.Contains(AllKinds, k) {
			return nil, fmt.Errorf("unsupported kind %q (allowed: %v)", k, AllKinds)
		}
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	return out, nil
}

func parseLimit(raw any) (int, error) {
	if raw == nil {
		return 20, nil
	}
	var n int
	switch v := raw.(type) {
	case float64:
		if float64(int(v)) != v {
			return 0, fmt.Errorf("limit must be an integer")
		}
		n = int(v)
	case int:
		n = v
	case int64:
		n = int(v)
	default:
		return 0, fmt.Errorf("limit must be an integer")
	}
	if n < 1 || n > 100 {
		return 0, fmt.Errorf("limit must be between 1 and 100 (got %d)", n)
	}
	return n, nil
}

func (e *Executor) argError(call agentic.ToolCall, msg string) agentic.ToolResult {
	return agentic.ToolResult{
		CallID:    call.ID,
		Name:      call.Name,
		Error:     msg,
		ErrorKind: agentic.ToolErrorInvalidArgs,
	}
}
