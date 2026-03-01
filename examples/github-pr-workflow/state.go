package githubprworkflow

import "github.com/c360studio/semstreams/processor/reactive"

// IssueToPRState tracks the execution state of the issue-to-PR workflow.
// It embeds ExecutionState to participate in the reactive engine's KV
// watch and async callback mechanisms.
type IssueToPRState struct {
	reactive.ExecutionState

	// Issue context
	IssueNumber int    `json:"issue_number"`
	IssueTitle  string `json:"issue_title"`
	IssueBody   string `json:"issue_body"`
	RepoOwner   string `json:"repo_owner"`
	RepoName    string `json:"repo_name"`

	// Qualifier output
	QualifierVerdict    string  `json:"qualifier_verdict"`
	QualifierConfidence float64 `json:"qualifier_confidence"`
	Severity            string  `json:"severity"`

	// Developer output
	BranchName   string   `json:"branch_name"`
	PRNumber     int      `json:"pr_number"`
	PRUrl        string   `json:"pr_url"`
	FilesChanged []string `json:"files_changed"`

	// Reviewer output
	ReviewVerdict  string `json:"review_verdict"`
	ReviewFeedback string `json:"review_feedback"`

	// Adversarial tracking
	ReviewRejections    int  `json:"review_rejections"`
	DevelopmentAttempts int  `json:"development_attempts"`
	EscalatedToHuman    bool `json:"escalated_to_human"`

	// Cost tracking
	TotalTokensIn  int `json:"total_tokens_in"`
	TotalTokensOut int `json:"total_tokens_out"`
}

// GetExecutionState implements reactive.StateAccessor to avoid reflection overhead.
func (s *IssueToPRState) GetExecutionState() *reactive.ExecutionState {
	return &s.ExecutionState
}
