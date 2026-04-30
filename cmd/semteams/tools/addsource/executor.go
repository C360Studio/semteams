package addsource

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/natsclient"
)

// RepoToolName is the LLM-facing tool name. Listed in
// agentic-tools allowed_tools (and approval_required) per deployment.
const RepoToolName = "add_source_repo"

// defaultRequestTimeout bounds a single add round-trip. SemSource's
// add path writes one KV entry and replies; this is a fast-path call
// in healthy state. Five seconds is conservative against
// ConfigManager KV latency.
const defaultRequestTimeout = 5 * time.Second

// defaultActor is the Provenance.Actor value when the tool wasn't given
// a more specific role override. Identifies the agent shape that
// originated the call rather than the human approver.
const defaultActor = "semteams.researcher"

// defaultBranch is the git branch the tool uses when the LLM omits one.
// Tracks SemSource's own default-branch resolution.
const defaultBranch = "main"

// NATSRequester is the narrow surface RepoExecutor uses to publish
// the request and await the reply. *natsclient.Client satisfies it
// via RequestWithRetry; tests inject a recording fake.
//
// We use RequestWithRetry (not bare Request) per
// semstreams/docs/operations/07-nats-request-retry.md: graph-mutation
// writes go through retry to avoid silent data loss on NATS startup
// races / responder restarts. SemSource's add handler is idempotent
// (deterministic instance names + INSTANCE_EXISTS reply for matching
// configs), so retry-safe at the responder side.
type NATSRequester interface {
	RequestWithRetry(ctx context.Context, subject string, data []byte, timeout time.Duration, retry natsclient.RetryConfig) ([]byte, error)
}

// Config holds the per-deployment policy the executor needs:
// the namespace allowlist (so an LLM cannot add to arbitrary SemSource
// instances) and the default namespace when the LLM omits one.
type Config struct {
	// AllowedNamespaces is the set of SemSource namespaces this
	// deployment may add sources to. An empty list disables the tool
	// entirely (every call returns an error). Operators must
	// explicitly opt in per deployment.
	AllowedNamespaces []string

	// DefaultNamespace is used when the LLM omits the namespace
	// argument. Must be in AllowedNamespaces if set; if unset and
	// the LLM omits the field, the call errors.
	DefaultNamespace string

	// RequestTimeout overrides the default 5s round-trip cap. Zero
	// means use the package default.
	RequestTimeout time.Duration

	// Actor is the Provenance.Actor stamp. Defaults to
	// "semteams.researcher" if empty.
	Actor string
}

// RepoExecutor implements agentic.ToolExecutor for the
// add_source_repo tool. It publishes an AddRequest on
// graph.ingest.add.{namespace} and parses the AddReply. Approval
// gating is the agentic-tools layer's responsibility (config field
// approval_required); this executor assumes it is called only after
// the gate has cleared.
type RepoExecutor struct {
	requester NATSRequester
	config    Config
	logger    *slog.Logger
}

// NewRepoExecutor constructs an executor. The Config is
// validated lazily — the constructor accepts any Config so callers
// can read failures at first-use time rather than at boot when an
// allowlist might still be loading from KV.
func NewRepoExecutor(req NATSRequester, cfg Config, logger *slog.Logger) *RepoExecutor {
	if logger == nil {
		logger = slog.Default()
	}
	return &RepoExecutor{requester: req, config: cfg, logger: logger}
}

// ListTools returns the LLM-facing schema for add_source_repo. Kept
// narrow on purpose — only `url`, `branch`, `namespace` are exposed.
// SourceEntry has many other fields (poll intervals, branch globs,
// max_branches) that are deployment policy; surfacing them to the LLM
// invites confidently-wrong invocations.
func (e *RepoExecutor) ListTools() []agentic.ToolDefinition {
	return []agentic.ToolDefinition{{
		Name:        RepoToolName,
		Description: "Add a public Git repository as a SemSource source so its contents become graph-queryable. The repo is registered with SemSource for the configured namespace; SemSource clones, indexes, and exposes its entities via graph.query.*. Returns once the source is graph-queryable. Approval-gated by default — a human must approve the add before the source registers.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "HTTPS or SSH URL of the Git repository to ingest (e.g. https://github.com/example/repo).",
				},
				"branch": map[string]any{
					"type":        "string",
					"description": "Branch to track. Defaults to \"main\" when omitted.",
				},
				"namespace": map[string]any{
					"type":        "string",
					"description": "SemSource namespace to add the source to. Must be in this deployment's namespace allowlist. Defaults to the deployment's default namespace when omitted.",
				},
			},
			"required": []string{"url"},
		},
	}}
}

// Execute publishes an AddRequest and parses the AddReply. Returns a
// ToolResult with structured Content on success, or with a non-empty
// Error and an appropriate ErrorKind on any failure path.
func (e *RepoExecutor) Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
	rawURL, _ := call.Arguments["url"].(string)
	branch, _ := call.Arguments["branch"].(string)
	namespace, _ := call.Arguments["namespace"].(string)

	if rawURL == "" {
		return e.argError(call, "url is required"), nil
	}
	if err := validateRepoURL(rawURL); err != nil {
		return e.argError(call, err.Error()), nil
	}
	if branch == "" {
		branch = defaultBranch
	}

	ns, err := e.resolveNamespace(namespace)
	if err != nil {
		return e.argError(call, err.Error()), nil
	}

	req := AddRequest{
		Source: SourceEntry{
			Type:   SourceTypeGit,
			URL:    rawURL,
			Branch: branch,
			Watch:  true,
		},
		Provenance: e.buildProvenance(call),
	}

	payload, err := json.Marshal(&req)
	if err != nil {
		return agentic.ToolResult{
			CallID: call.ID,
			Name:   call.Name,
			Error:  fmt.Sprintf("marshal AddRequest: %v", err),
		}, nil
	}

	subject := "graph.ingest.add." + ns
	timeout := e.config.RequestTimeout
	if timeout == 0 {
		timeout = defaultRequestTimeout
	}

	respBytes, err := e.requester.RequestWithRetry(ctx, subject, payload, timeout, natsclient.DefaultRetryConfig())
	if err != nil {
		return agentic.ToolResult{
			CallID: call.ID,
			Name:   call.Name,
			Error:  fmt.Sprintf("nats request to %s failed: %v", subject, err),
		}, nil
	}

	var reply AddReply
	if err := json.Unmarshal(respBytes, &reply); err != nil {
		return agentic.ToolResult{
			CallID: call.ID,
			Name:   call.Name,
			Error:  fmt.Sprintf("decode AddReply: %v", err),
		}, nil
	}

	if reply.Error != nil {
		return agentic.ToolResult{
			CallID: call.ID,
			Name:   call.Name,
			Error:  fmt.Sprintf("%s: %s", reply.Error.Code, reply.Error.Message),
		}, nil
	}

	resultJSON, err := json.Marshal(map[string]any{
		"namespace":      ns,
		"components":     reply.Components,
		"status_subject": reply.StatusSubject,
		"ready_when":     reply.ReadyWhen,
		"timestamp":      reply.Timestamp,
	})
	if err != nil {
		return agentic.ToolResult{
			CallID: call.ID,
			Name:   call.Name,
			Error:  fmt.Sprintf("marshal tool result: %v", err),
		}, nil
	}

	e.logger.Info("add_source_repo published",
		slog.String("namespace", ns),
		slog.String("url", rawURL),
		slog.String("branch", branch),
		slog.Int("components", len(reply.Components)))

	return agentic.ToolResult{
		CallID:  call.ID,
		Name:    call.Name,
		Content: string(resultJSON),
	}, nil
}

// resolveNamespace honours the LLM's choice when supplied (provided it
// is in the allowlist), else falls back to the deployment default.
// Refusing the call when both are absent is intentional — there's no
// safe implicit target.
func (e *RepoExecutor) resolveNamespace(requested string) (string, error) {
	if len(e.config.AllowedNamespaces) == 0 {
		return "", fmt.Errorf("add_source_repo disabled: deployment has no SemSource namespace allowlist")
	}
	if requested != "" {
		if slices.Contains(e.config.AllowedNamespaces, requested) {
			return requested, nil
		}
		return "", fmt.Errorf("namespace %q is not in this deployment's allowlist %v", requested, e.config.AllowedNamespaces)
	}
	if e.config.DefaultNamespace == "" {
		return "", fmt.Errorf("namespace is required: deployment has no default and the call did not specify one")
	}
	if slices.Contains(e.config.AllowedNamespaces, e.config.DefaultNamespace) {
		return e.config.DefaultNamespace, nil
	}
	return "", fmt.Errorf("default namespace %q is not in allowlist %v (deployment misconfiguration)", e.config.DefaultNamespace, e.config.AllowedNamespaces)
}

// buildProvenance stamps the request with attribution. Actor is the
// agent shape; OnBehalfOf is the human identity (lifted by the
// ADR-030 middleware via ToolCall.Metadata or ApprovedBy when the
// call has been through the approval gate); TraceID propagates from
// the call.
func (e *RepoExecutor) buildProvenance(call agentic.ToolCall) Provenance {
	actor := e.config.Actor
	if actor == "" {
		actor = defaultActor
	}
	return Provenance{
		Actor:      actor,
		OnBehalfOf: call.ApprovedBy,
		TraceID:    call.TraceID,
	}
}

// argError wraps an LLM-facing error as a ToolResult. Returning a
// nil error from Execute (alongside this result) keeps the framework
// from flagging it as an executor failure — it's a normal "tool
// reported a user-facing problem" path.
func (e *RepoExecutor) argError(call agentic.ToolCall, msg string) agentic.ToolResult {
	return agentic.ToolResult{
		CallID: call.ID,
		Name:   call.Name,
		Error:  msg,
	}
}

// validateRepoURL is a defense against the LLM passing junk. We only
// accept http/https URLs or git@host:path SSH form; everything else
// gets rejected at the boundary so the SemSource side doesn't have to
// guess what we sent.
//
// Not validated: localhost / RFC1918 hosts. ADR-0003 §"Authorization"
// places the trust boundary at NATS subject permissions, not URL
// content; SemSource clones whatever URL we forward. If a deployment
// ever runs SemTeams against an LLM that might emit private-host URLs,
// add a SEMTEAMS_SEMSOURCE_ALLOW_PRIVATE_HOSTS=false guard here.
// Tracked as a follow-on; not a slice-1 blocker.
func validateRepoURL(s string) error {
	if rest, isSSH := strings.CutPrefix(s, "git@"); isSSH {
		host, path, ok := strings.Cut(rest, ":")
		if !ok || host == "" || path == "" {
			return fmt.Errorf("invalid ssh form: want git@host:path, got %q", s)
		}
		return nil
	}
	u, err := url.Parse(s)
	if err != nil {
		return fmt.Errorf("invalid url: %v", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported scheme %q (only http, https, or git@host:path ssh form)", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("url missing host")
	}
	return nil
}
