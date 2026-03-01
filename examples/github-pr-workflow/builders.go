package githubprworkflow

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/processor/reactive"
)

// GitHubIssueWebhookEvent is a minimal representation of the GitHub webhook
// payload for issue events. Only the fields required by the workflow rules
// are modelled here; the full payload is not needed for routing.
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

// buildQualifierTask creates a TaskMessage for the qualifier agent from the
// triggering GitHub issue webhook event. The qualifier decides whether the
// issue is actionable and returns a structured verdict.
func buildQualifierTask(ctx *reactive.RuleContext) (message.Payload, error) {
	event, ok := ctx.Message.(*GitHubIssueWebhookEvent)
	if !ok {
		return nil, fmt.Errorf("buildQualifierTask: expected *GitHubIssueWebhookEvent, got %T", ctx.Message)
	}

	es := reactive.ExtractExecutionState(ctx.State)
	taskID := fmt.Sprintf("qualifier-%s", es.ID)

	prompt := fmt.Sprintf(
		"You are an expert software engineer triaging GitHub issues.\n\n"+
			"Repository: %s/%s\n"+
			"Issue #%d: %s\n\n"+
			"Body:\n%s\n\n"+
			"Evaluate this issue and respond with a JSON object containing:\n"+
			"  - verdict: one of [qualified, rejected, not_a_bug, wont_fix, needs_info]\n"+
			"  - confidence: float between 0.0 and 1.0\n"+
			"  - severity: one of [critical, high, medium, low]\n"+
			"  - reasoning: brief explanation of your verdict\n\n"+
			"Only respond with the JSON object, no other text.",
		event.Repo.Owner.Login, event.Repo.Name,
		event.Issue.Number, event.Issue.Title,
		event.Issue.Body,
	)

	return &agentic.TaskMessage{
		TaskID:       taskID,
		Role:         agentic.RoleQualifier,
		Model:        "claude-opus-4-6",
		Prompt:       prompt,
		WorkflowSlug: "github-issue-to-pr",
		WorkflowStep: "qualify",
	}, nil
}

// handleQualifierResult processes the qualifier agent's LoopCompletedEvent
// and updates the execution state with the verdict, confidence, and severity.
func handleQualifierResult(ctx *reactive.RuleContext, result any) error {
	s, ok := ctx.State.(*IssueToPRState)
	if !ok {
		return fmt.Errorf("handleQualifierResult: expected *IssueToPRState, got %T", ctx.State)
	}

	event, ok := result.(*agentic.LoopCompletedEvent)
	if !ok {
		return fmt.Errorf("handleQualifierResult: expected *LoopCompletedEvent, got %T", result)
	}

	// Parse the structured JSON verdict from the agent's result text.
	var verdict struct {
		Verdict    string  `json:"verdict"`
		Confidence float64 `json:"confidence"`
		Severity   string  `json:"severity"`
	}
	if err := json.Unmarshal([]byte(event.Result), &verdict); err != nil {
		// Treat unparseable results as needing more information rather than
		// failing the workflow outright — the qualifier may have emitted prose.
		s.Phase = PhaseNeedsInfo
		s.QualifierVerdict = "parse_error"
		return nil
	}

	s.QualifierVerdict = verdict.Verdict
	s.QualifierConfidence = verdict.Confidence
	s.Severity = verdict.Severity
	s.Phase = verdict.Verdict // phase mirrors verdict (qualified, rejected, etc.)

	// Accumulate token usage for budget tracking
	s.TotalTokensIn += event.TokensIn
	s.TotalTokensOut += event.TokensOut

	return nil
}

// buildDeveloperTask creates a TaskMessage for the developer agent. On the
// first attempt it includes the qualifier verdict; on retries it also
// includes the reviewer's change requests so the developer can iterate.
func buildDeveloperTask(ctx *reactive.RuleContext) (message.Payload, error) {
	s, ok := ctx.State.(*IssueToPRState)
	if !ok {
		return nil, fmt.Errorf("buildDeveloperTask: expected *IssueToPRState, got %T", ctx.State)
	}

	taskID := fmt.Sprintf("developer-%s-attempt%d", s.ID, s.DevelopmentAttempts+1)

	var sb strings.Builder
	sb.WriteString("You are an expert software engineer.\n\n")
	sb.WriteString(fmt.Sprintf("Repository: %s/%s\n", s.RepoOwner, s.RepoName))
	sb.WriteString(fmt.Sprintf("Issue #%d: %s\n\n", s.IssueNumber, s.IssueTitle))
	sb.WriteString(fmt.Sprintf("Issue body:\n%s\n\n", s.IssueBody))
	sb.WriteString(fmt.Sprintf("Qualifier verdict: %s (confidence %.2f, severity %s)\n\n",
		s.QualifierVerdict, s.QualifierConfidence, s.Severity))

	if s.ReviewFeedback != "" {
		sb.WriteString("Previous review feedback that must be addressed:\n")
		sb.WriteString(s.ReviewFeedback)
		sb.WriteString("\n\n")
	}

	sb.WriteString("Implement a fix for this issue. Create a feature branch, commit your changes, " +
		"and open a pull request. Respond with a JSON object containing:\n" +
		"  - branch_name: the branch you created\n" +
		"  - pr_number: the pull request number\n" +
		"  - pr_url: the URL of the pull request\n" +
		"  - files_changed: list of file paths modified\n\n" +
		"Only respond with the JSON object, no other text.")

	return &agentic.TaskMessage{
		TaskID:       taskID,
		Role:         agentic.RoleDeveloper,
		Model:        "claude-opus-4-6",
		Prompt:       sb.String(),
		WorkflowSlug: "github-issue-to-pr",
		WorkflowStep: "develop",
	}, nil
}

// handleDeveloperResult processes the developer agent's LoopCompletedEvent
// and updates the execution state with the branch name and PR details.
func handleDeveloperResult(ctx *reactive.RuleContext, result any) error {
	s, ok := ctx.State.(*IssueToPRState)
	if !ok {
		return fmt.Errorf("handleDeveloperResult: expected *IssueToPRState, got %T", ctx.State)
	}

	event, ok := result.(*agentic.LoopCompletedEvent)
	if !ok {
		return fmt.Errorf("handleDeveloperResult: expected *LoopCompletedEvent, got %T", result)
	}

	var output struct {
		BranchName   string   `json:"branch_name"`
		PRNumber     int      `json:"pr_number"`
		PRUrl        string   `json:"pr_url"`
		FilesChanged []string `json:"files_changed"`
	}
	if err := json.Unmarshal([]byte(event.Result), &output); err != nil {
		return fmt.Errorf("handleDeveloperResult: parse agent output: %w", err)
	}

	s.BranchName = output.BranchName
	s.PRNumber = output.PRNumber
	s.PRUrl = output.PRUrl
	s.FilesChanged = output.FilesChanged
	s.Phase = PhaseDevComplete

	// Accumulate token usage for budget tracking
	s.TotalTokensIn += event.TokensIn
	s.TotalTokensOut += event.TokensOut

	return nil
}

// buildReviewerTask creates a TaskMessage for the reviewer agent. It includes
// the PR details and the full list of changed files so the reviewer can
// perform an adversarial code review.
func buildReviewerTask(ctx *reactive.RuleContext) (message.Payload, error) {
	s, ok := ctx.State.(*IssueToPRState)
	if !ok {
		return nil, fmt.Errorf("buildReviewerTask: expected *IssueToPRState, got %T", ctx.State)
	}

	taskID := fmt.Sprintf("reviewer-%s-cycle%d", s.ID, s.ReviewRejections+1)

	prompt := fmt.Sprintf(
		"You are a senior software engineer performing an adversarial code review.\n\n"+
			"Repository: %s/%s\n"+
			"Issue #%d: %s\n"+
			"Pull Request #%d: %s\n"+
			"Files changed: %s\n\n"+
			"Review the pull request thoroughly. Respond with a JSON object containing:\n"+
			"  - verdict: one of [approved, request_changes]\n"+
			"  - feedback: detailed review comments (required if verdict is request_changes)\n\n"+
			"Only respond with the JSON object, no other text.",
		s.RepoOwner, s.RepoName,
		s.IssueNumber, s.IssueTitle,
		s.PRNumber, s.PRUrl,
		strings.Join(s.FilesChanged, ", "),
	)

	return &agentic.TaskMessage{
		TaskID:       taskID,
		Role:         agentic.RoleReviewer,
		Model:        "claude-opus-4-6",
		Prompt:       prompt,
		WorkflowSlug: "github-issue-to-pr",
		WorkflowStep: "review",
	}, nil
}

// handleReviewerResult processes the reviewer agent's LoopCompletedEvent and
// advances the execution state based on the review verdict.
//
// An "approved" verdict sets phase to PhaseApproved, triggering rule 4.
// A "request_changes" verdict increments the rejection counter and sets
// phase to PhaseChangesRequested, which triggers either rule 5 or rule 6
// depending on whether the retry budget is exhausted.
func handleReviewerResult(ctx *reactive.RuleContext, result any) error {
	s, ok := ctx.State.(*IssueToPRState)
	if !ok {
		return fmt.Errorf("handleReviewerResult: expected *IssueToPRState, got %T", ctx.State)
	}

	event, ok := result.(*agentic.LoopCompletedEvent)
	if !ok {
		return fmt.Errorf("handleReviewerResult: expected *LoopCompletedEvent, got %T", result)
	}

	var review struct {
		Verdict  string `json:"verdict"`
		Feedback string `json:"feedback"`
	}
	if err := json.Unmarshal([]byte(event.Result), &review); err != nil {
		return fmt.Errorf("handleReviewerResult: parse agent output: %w", err)
	}

	s.ReviewVerdict = review.Verdict
	s.ReviewFeedback = review.Feedback

	// Accumulate token usage for budget tracking
	s.TotalTokensIn += event.TokensIn
	s.TotalTokensOut += event.TokensOut

	switch review.Verdict {
	case "approved":
		s.Phase = PhaseApproved
	case "request_changes", "reject":
		s.Phase = PhaseChangesRequested
		s.ReviewRejections++
	default:
		// Treat unknown verdicts as a request for changes to be safe.
		s.Phase = PhaseChangesRequested
		s.ReviewRejections++
	}

	return nil
}

// buildCompletionPayload assembles the WorkflowCompletionPayload that is
// published to "github.workflow.complete" when the PR is approved.
func buildCompletionPayload(ctx *reactive.RuleContext) (message.Payload, error) {
	s, ok := ctx.State.(*IssueToPRState)
	if !ok {
		return nil, fmt.Errorf("buildCompletionPayload: expected *IssueToPRState, got %T", ctx.State)
	}

	es := reactive.ExtractExecutionState(ctx.State)
	executionID := ""
	if es != nil {
		executionID = es.ID
	}

	return &WorkflowCompletionPayload{
		ExecutionID:         executionID,
		IssueNumber:         s.IssueNumber,
		RepoOwner:           s.RepoOwner,
		RepoName:            s.RepoName,
		PRNumber:            s.PRNumber,
		PRUrl:               s.PRUrl,
		FilesChanged:        s.FilesChanged,
		DevelopmentAttempts: s.DevelopmentAttempts,
		ReviewRejections:    s.ReviewRejections,
		TotalTokensIn:       s.TotalTokensIn,
		TotalTokensOut:      s.TotalTokensOut,
	}, nil
}
