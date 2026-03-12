// Package githubprworkflow provides Graphable entity types for the GitHub
// issue-to-PR automation workflow.
//
// These entities model GitHub artifacts (issues, pull requests, reviews) as
// first-class graph nodes, enabling the knowledge graph to reason about
// repository activity and code change history.
//
// Note: GitHubIssueEntity, GitHubPREntity, and GitHubReviewEntity are
// published by the github-webhook input component when it ingests events from
// GitHub. The pr-workflow-spawner component in this package does not
// instantiate these entities directly — it writes workflow-phase triples via
// the graph mutation API instead.
package githubprworkflow

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/c360studio/semstreams/message"
)

// Domain and version constants for GitHub entity message types.
const (
	domainGitHub         = "github"
	categoryIssueEntity  = "issue_entity"
	categoryPREntity     = "pr_entity"
	categoryReviewEntity = "review_entity"
	schemaVersion        = "v1"
	sourceGitHubWorkflow = "github-workflow"
)

// GitHubIssueEntity represents a GitHub issue as a graph node.
// It implements the Graphable interface to expose issue facts as semantic triples.
type GitHubIssueEntity struct {
	// Org is the GitHub organization or user that owns the repository.
	Org string `json:"org"`
	// Repo is the repository name within the organization.
	Repo string `json:"repo"`
	// Number is the issue number within the repository.
	Number int `json:"number"`
	// Title is the issue title.
	Title string `json:"title"`
	// Body is the issue body text.
	Body string `json:"body"`
	// Labels contains any labels applied to the issue.
	Labels []string `json:"labels,omitempty"`
	// State is the current issue state (open, closed).
	State string `json:"state"`
	// Author is the GitHub username of the issue author.
	Author string `json:"author"`
	// Severity is an optional triage severity classification (e.g., critical, high, medium, low).
	Severity string `json:"severity,omitempty"`
	// Complexity is an optional effort estimation (e.g., small, medium, large).
	Complexity string `json:"complexity,omitempty"`
}

// EntityID returns the 6-part federated identifier for this issue.
// Format: {org}.github.repo.{repo}.issue.{number}
func (e *GitHubIssueEntity) EntityID() string {
	return fmt.Sprintf("%s.github.repo.%s.issue.%d", e.Org, e.Repo, e.Number)
}

// Triples returns semantic facts about this GitHub issue.
// Core facts (title, state, author) are always included; optional fields
// (severity, complexity, labels) are only included when set.
func (e *GitHubIssueEntity) Triples() []message.Triple {
	id := e.EntityID()
	now := time.Now()

	triples := []message.Triple{
		{Subject: id, Predicate: "github.issue.title", Object: e.Title, Source: sourceGitHubWorkflow, Timestamp: now, Confidence: 1.0},
		{Subject: id, Predicate: "github.issue.state", Object: e.State, Source: sourceGitHubWorkflow, Timestamp: now, Confidence: 1.0},
		{Subject: id, Predicate: "github.issue.author", Object: e.Author, Source: sourceGitHubWorkflow, Timestamp: now, Confidence: 1.0},
	}

	if e.Severity != "" {
		triples = append(triples, message.Triple{
			Subject: id, Predicate: "github.issue.severity", Object: e.Severity,
			Source: sourceGitHubWorkflow, Timestamp: now, Confidence: 1.0,
		})
	}

	if e.Complexity != "" {
		triples = append(triples, message.Triple{
			Subject: id, Predicate: "github.issue.complexity", Object: e.Complexity,
			Source: sourceGitHubWorkflow, Timestamp: now, Confidence: 1.0,
		})
	}

	for _, label := range e.Labels {
		triples = append(triples, message.Triple{
			Subject: id, Predicate: "github.issue.label", Object: label,
			Source: sourceGitHubWorkflow, Timestamp: now, Confidence: 1.0,
		})
	}

	return triples
}

// Schema returns the message type for GitHubIssueEntity payloads.
func (e *GitHubIssueEntity) Schema() message.Type {
	return message.Type{Domain: domainGitHub, Category: categoryIssueEntity, Version: schemaVersion}
}

// Validate checks that the issue entity has all required fields.
func (e *GitHubIssueEntity) Validate() error {
	if e.Org == "" {
		return fmt.Errorf("org is required")
	}
	if e.Repo == "" {
		return fmt.Errorf("repo is required")
	}
	if e.Number <= 0 {
		return fmt.Errorf("number must be positive")
	}
	return nil
}

// MarshalJSON implements json.Marshaler using the alias pattern to avoid recursion.
func (e *GitHubIssueEntity) MarshalJSON() ([]byte, error) {
	type Alias GitHubIssueEntity
	return json.Marshal((*Alias)(e))
}

// UnmarshalJSON implements json.Unmarshaler using the alias pattern to avoid recursion.
func (e *GitHubIssueEntity) UnmarshalJSON(data []byte) error {
	type Alias GitHubIssueEntity
	return json.Unmarshal(data, (*Alias)(e))
}

// GitHubPREntity represents a GitHub pull request as a graph node.
// It implements the Graphable interface and records the relationship to the
// issue it fixes as a triple linking the two entities by their IDs.
type GitHubPREntity struct {
	// Org is the GitHub organization or user that owns the repository.
	Org string `json:"org"`
	// Repo is the repository name within the organization.
	Repo string `json:"repo"`
	// Number is the pull request number within the repository.
	Number int `json:"number"`
	// Title is the pull request title.
	Title string `json:"title"`
	// Body is the pull request description.
	Body string `json:"body"`
	// Head is the source branch name.
	Head string `json:"head"`
	// Base is the target branch name.
	Base string `json:"base"`
	// State is the current PR state (open, closed, merged).
	State string `json:"state"`
	// IssueNumber is the number of the issue this PR addresses.
	IssueNumber int `json:"issue_number,omitempty"`
	// FilesChanged lists the repository-relative paths of modified files.
	FilesChanged []string `json:"files_changed,omitempty"`
}

// EntityID returns the 6-part federated identifier for this pull request.
// Format: {org}.github.repo.{repo}.pr.{number}
func (e *GitHubPREntity) EntityID() string {
	return fmt.Sprintf("%s.github.repo.%s.pr.%d", e.Org, e.Repo, e.Number)
}

// Triples returns semantic facts about this pull request.
// When IssueNumber is set, a "github.pr.fixes" triple is added that
// references the linked issue entity by its ID, enabling graph traversal
// from PR to issue.
func (e *GitHubPREntity) Triples() []message.Triple {
	id := e.EntityID()
	now := time.Now()

	triples := []message.Triple{
		{Subject: id, Predicate: "github.pr.title", Object: e.Title, Source: sourceGitHubWorkflow, Timestamp: now, Confidence: 1.0},
		{Subject: id, Predicate: "github.pr.state", Object: e.State, Source: sourceGitHubWorkflow, Timestamp: now, Confidence: 1.0},
		{Subject: id, Predicate: "github.pr.head", Object: e.Head, Source: sourceGitHubWorkflow, Timestamp: now, Confidence: 1.0},
		{Subject: id, Predicate: "github.pr.base", Object: e.Base, Source: sourceGitHubWorkflow, Timestamp: now, Confidence: 1.0},
	}

	// Relationship triple linking this PR to the issue it fixes.
	// The object is the issue's entity ID, enabling graph traversal.
	if e.IssueNumber > 0 {
		issueID := fmt.Sprintf("%s.github.repo.%s.issue.%d", e.Org, e.Repo, e.IssueNumber)
		triples = append(triples, message.Triple{
			Subject: id, Predicate: "github.pr.fixes", Object: issueID,
			Source: sourceGitHubWorkflow, Timestamp: now, Confidence: 1.0,
		})
	}

	for _, f := range e.FilesChanged {
		triples = append(triples, message.Triple{
			Subject: id, Predicate: "github.pr.file", Object: f,
			Source: sourceGitHubWorkflow, Timestamp: now, Confidence: 1.0,
		})
	}

	return triples
}

// Schema returns the message type for GitHubPREntity payloads.
func (e *GitHubPREntity) Schema() message.Type {
	return message.Type{Domain: domainGitHub, Category: categoryPREntity, Version: schemaVersion}
}

// Validate checks that the pull request entity has all required fields.
func (e *GitHubPREntity) Validate() error {
	if e.Org == "" {
		return fmt.Errorf("org is required")
	}
	if e.Repo == "" {
		return fmt.Errorf("repo is required")
	}
	if e.Number <= 0 {
		return fmt.Errorf("number must be positive")
	}
	return nil
}

// MarshalJSON implements json.Marshaler using the alias pattern to avoid recursion.
func (e *GitHubPREntity) MarshalJSON() ([]byte, error) {
	type Alias GitHubPREntity
	return json.Marshal((*Alias)(e))
}

// UnmarshalJSON implements json.Unmarshaler using the alias pattern to avoid recursion.
func (e *GitHubPREntity) UnmarshalJSON(data []byte) error {
	type Alias GitHubPREntity
	return json.Unmarshal(data, (*Alias)(e))
}

// GitHubReviewEntity represents a code review conducted by an agent on a pull request.
// It implements the Graphable interface and records the relationship to the reviewed
// PR as a "github.review.targets" triple.
type GitHubReviewEntity struct {
	// Org is the GitHub organization or user that owns the repository.
	Org string `json:"org"`
	// Repo is the repository name within the organization.
	Repo string `json:"repo"`
	// ID is a unique identifier for this review (e.g., UUID or sequential ID).
	ID string `json:"id"`
	// PRNumber is the pull request number that this review targets.
	PRNumber int `json:"pr_number"`
	// Verdict is the review outcome: approve, request_changes, or reject.
	Verdict string `json:"verdict"`
	// Issues is the count of issues found during review.
	Issues int `json:"issues"`
	// Agent is the role of the agent that performed this review (e.g., "reviewer").
	Agent string `json:"agent"`
}

// EntityID returns the 6-part federated identifier for this review.
// Format: {org}.github.repo.{repo}.review.{id}
func (e *GitHubReviewEntity) EntityID() string {
	return fmt.Sprintf("%s.github.repo.%s.review.%s", e.Org, e.Repo, e.ID)
}

// Triples returns semantic facts about this review.
// A "github.review.targets" triple is always added when PRNumber is set,
// linking this review to its pull request entity by ID.
func (e *GitHubReviewEntity) Triples() []message.Triple {
	id := e.EntityID()
	now := time.Now()

	triples := []message.Triple{
		{Subject: id, Predicate: "github.review.verdict", Object: e.Verdict, Source: sourceGitHubWorkflow, Timestamp: now, Confidence: 1.0},
		{Subject: id, Predicate: "github.review.issues", Object: e.Issues, Source: sourceGitHubWorkflow, Timestamp: now, Confidence: 1.0},
		{Subject: id, Predicate: "github.review.agent", Object: e.Agent, Source: sourceGitHubWorkflow, Timestamp: now, Confidence: 1.0},
	}

	// Relationship triple linking this review to the PR it targets.
	// The object is the PR's entity ID, enabling graph traversal from review to PR.
	if e.PRNumber > 0 {
		prID := fmt.Sprintf("%s.github.repo.%s.pr.%d", e.Org, e.Repo, e.PRNumber)
		triples = append(triples, message.Triple{
			Subject: id, Predicate: "github.review.targets", Object: prID,
			Source: sourceGitHubWorkflow, Timestamp: now, Confidence: 1.0,
		})
	}

	return triples
}

// Schema returns the message type for GitHubReviewEntity payloads.
func (e *GitHubReviewEntity) Schema() message.Type {
	return message.Type{Domain: domainGitHub, Category: categoryReviewEntity, Version: schemaVersion}
}

// Validate checks that the review entity has all required fields.
func (e *GitHubReviewEntity) Validate() error {
	if e.Org == "" {
		return fmt.Errorf("org is required")
	}
	if e.Repo == "" {
		return fmt.Errorf("repo is required")
	}
	if e.ID == "" {
		return fmt.Errorf("id is required")
	}
	return nil
}

// MarshalJSON implements json.Marshaler using the alias pattern to avoid recursion.
func (e *GitHubReviewEntity) MarshalJSON() ([]byte, error) {
	type Alias GitHubReviewEntity
	return json.Marshal((*Alias)(e))
}

// UnmarshalJSON implements json.Unmarshaler using the alias pattern to avoid recursion.
func (e *GitHubReviewEntity) UnmarshalJSON(data []byte) error {
	type Alias GitHubReviewEntity
	return json.Unmarshal(data, (*Alias)(e))
}
