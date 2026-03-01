package githubwebhook

import "time"

// WebhookEvent is the common header present in every GitHub webhook event.
// Specific event types embed this struct and add their own domain-specific fields.
type WebhookEvent struct {
	// EventType is the value of the X-GitHub-Event header (e.g. "issues").
	EventType string `json:"event_type"`

	// Action is the fine-grained sub-action within the event type
	// (e.g. "opened", "edited", "closed").
	Action string `json:"action"`

	// Repository identifies the repository that generated the event.
	Repository Repository `json:"repository"`

	// Sender is the GitHub login of the user who triggered the event.
	Sender string `json:"sender"`

	// ReceivedAt is the UTC timestamp at which this process received the event.
	ReceivedAt time.Time `json:"received_at"`
}

// Repository holds the identifying fields of a GitHub repository.
type Repository struct {
	// Owner is the organisation or user that owns the repository.
	Owner string `json:"owner"`

	// Name is the repository name without the owner prefix.
	Name string `json:"name"`

	// FullName is the canonical "owner/name" identifier.
	FullName string `json:"full_name"`
}

// IssueEvent represents a GitHub "issues" webhook event.
type IssueEvent struct {
	WebhookEvent
	Issue IssuePayload `json:"issue"`
}

// IssuePayload contains the fields extracted from a GitHub issue object.
type IssuePayload struct {
	// Number is the repository-scoped issue number.
	Number int `json:"number"`

	// Title is the issue title.
	Title string `json:"title"`

	// Body is the issue body text (may be empty).
	Body string `json:"body"`

	// State is the issue state: "open" or "closed".
	State string `json:"state"`

	// Labels is the list of label names applied to the issue.
	Labels []string `json:"labels"`

	// Author is the GitHub login of the issue author.
	Author string `json:"author"`

	// HTMLURL is the canonical browser URL for the issue.
	HTMLURL string `json:"html_url"`
}

// PREvent represents a GitHub "pull_request" webhook event.
type PREvent struct {
	WebhookEvent
	PullRequest PRPayload `json:"pull_request"`
}

// PRPayload contains the fields extracted from a GitHub pull-request object.
type PRPayload struct {
	// Number is the repository-scoped pull-request number.
	Number int `json:"number"`

	// Title is the pull-request title.
	Title string `json:"title"`

	// Body is the pull-request description (may be empty).
	Body string `json:"body"`

	// State is the PR state: "open", "closed", or "merged".
	State string `json:"state"`

	// Head is the name of the source branch.
	Head string `json:"head"`

	// Base is the name of the target branch.
	Base string `json:"base"`

	// HTMLURL is the canonical browser URL for the pull request.
	HTMLURL string `json:"html_url"`
}

// ReviewEvent represents a GitHub "pull_request_review" webhook event.
type ReviewEvent struct {
	WebhookEvent
	Review      ReviewPayload `json:"review"`
	PullRequest PRPayload     `json:"pull_request"`
}

// ReviewPayload contains the fields extracted from a GitHub pull-request review.
type ReviewPayload struct {
	// State is the review outcome: "approved", "changes_requested", or "commented".
	State string `json:"state"`

	// Body is the review summary comment (may be empty).
	Body string `json:"body"`
}

// CommentEvent represents a GitHub "issue_comment" webhook event.
type CommentEvent struct {
	WebhookEvent
	Comment CommentPayload `json:"comment"`
}

// CommentPayload contains the fields extracted from a GitHub issue comment.
type CommentPayload struct {
	// Body is the comment text.
	Body string `json:"body"`

	// Author is the GitHub login of the comment author.
	Author string `json:"author"`
}
