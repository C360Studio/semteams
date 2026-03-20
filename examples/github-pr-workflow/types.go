package githubprworkflow

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/c360studio/semstreams/message"
)

// Workflow constants.
const (
	// MaxReviewCycles is the maximum number of reviewer rejection/retry loops
	// before the workflow escalates to human intervention.
	MaxReviewCycles = 3

	// WorkflowTimeout is the maximum wall-clock duration for any single
	// execution before the engine marks it as timed out.
	WorkflowTimeout = 30 * time.Minute

	// DefaultTokenBudget is the maximum tokens (in+out) per workflow execution.
	DefaultTokenBudget = 500_000

	// DefaultMaxConcurrentWorkflows limits parallel active workflows.
	DefaultMaxConcurrentWorkflows = 3

	// DefaultHourlyTokenCeiling is the global hourly token limit across all workflows.
	DefaultHourlyTokenCeiling = 2_000_000

	// DefaultIssueCooldown is the minimum time between processing new issues.
	DefaultIssueCooldown = 60 * time.Second
)

// Phase constants represent the distinct states an issue-to-PR execution
// can occupy. They are written as workflow.phase triples and drive all
// rule conditions.
const (
	PhaseQualified        = "qualified"
	PhaseRejected         = "rejected"
	PhaseNotABug          = "not_a_bug"
	PhaseWontFix          = "wont_fix"
	PhaseNeedsInfo        = "needs_info"
	PhaseAwaitingInfo     = "awaiting_info"
	PhaseDevComplete      = "dev_complete"
	PhaseDeveloping       = "developing"
	PhaseApproved         = "approved"
	PhaseChangesRequested = "changes_requested"
	PhaseEscalated        = "escalated"
)

// PhaseIsActive returns true if the phase indicates an active workflow.
func PhaseIsActive(phase string) bool {
	switch phase {
	case PhaseQualified, PhaseDeveloping, PhaseDevComplete,
		PhaseChangesRequested, PhaseNeedsInfo, PhaseAwaitingInfo:
		return true
	}
	return false
}

// GitHubIssueWebhookEvent is a minimal representation of the GitHub webhook
// payload for issue events.
type GitHubIssueWebhookEvent struct {
	Action string      `json:"action"`
	Issue  IssueDetail `json:"issue"`
	Repo   RepoDetail  `json:"repository"`
}

// IssueDetail carries the issue fields required to bootstrap an execution.
type IssueDetail struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
}

// RepoDetail carries the repository owner and name.
type RepoDetail struct {
	Name  string      `json:"name"`
	Owner OwnerDetail `json:"owner"`
}

// OwnerDetail carries the repository owner login.
type OwnerDetail struct {
	Login string `json:"login"`
}

// WorkflowCompletionPayload summarises the outcome of a completed
// issue-to-PR execution for downstream consumers.
type WorkflowCompletionPayload struct {
	ExecutionID         string   `json:"execution_id"`
	IssueNumber         int      `json:"issue_number"`
	RepoOwner           string   `json:"repo_owner"`
	RepoName            string   `json:"repo_name"`
	PRNumber            int      `json:"pr_number"`
	PRUrl               string   `json:"pr_url"`
	FilesChanged        []string `json:"files_changed"`
	DevelopmentAttempts int      `json:"development_attempts"`
	ReviewRejections    int      `json:"review_rejections"`
	TotalTokensIn       int      `json:"total_tokens_in"`
	TotalTokensOut      int      `json:"total_tokens_out"`
}

// Schema implements message.Payload.
func (p *WorkflowCompletionPayload) Schema() message.Type {
	return message.Type{Domain: "github", Category: "workflow_complete", Version: "v1"}
}

// Validate implements message.Payload.
func (p *WorkflowCompletionPayload) Validate() error {
	if p.ExecutionID == "" {
		return fmt.Errorf("execution_id required")
	}
	return nil
}

// MarshalJSON implements message.Payload.
func (p *WorkflowCompletionPayload) MarshalJSON() ([]byte, error) {
	type Alias WorkflowCompletionPayload
	return json.Marshal((*Alias)(p))
}

// UnmarshalJSON implements message.Payload.
func (p *WorkflowCompletionPayload) UnmarshalJSON(data []byte) error {
	type Alias WorkflowCompletionPayload
	return json.Unmarshal(data, (*Alias)(p))
}
