package githubprworkflow_test

import (
	"fmt"
	"testing"

	githubprworkflow "github.com/c360studio/semstreams/examples/github-pr-workflow"
)

func TestGitHubIssueEntity_EntityID(t *testing.T) {
	tests := []struct {
		name   string
		entity githubprworkflow.GitHubIssueEntity
		wantID string
	}{
		{
			name: "standard issue",
			entity: githubprworkflow.GitHubIssueEntity{
				Org: "acme", Repo: "webapp", Number: 42,
				Title: "Bug fix", State: "open", Author: "alice",
			},
			wantID: "acme.github.repo.webapp.issue.42",
		},
		{
			name: "single digit number",
			entity: githubprworkflow.GitHubIssueEntity{
				Org: "myorg", Repo: "myrepo", Number: 1,
				Title: "Init", State: "open", Author: "bob",
			},
			wantID: "myorg.github.repo.myrepo.issue.1",
		},
		{
			name: "large issue number",
			entity: githubprworkflow.GitHubIssueEntity{
				Org: "bigco", Repo: "platform", Number: 9999,
				Title: "Refactor", State: "closed", Author: "carol",
			},
			wantID: "bigco.github.repo.platform.issue.9999",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.entity.EntityID()
			if got != tt.wantID {
				t.Errorf("EntityID() = %q, want %q", got, tt.wantID)
			}
		})
	}
}

func TestGitHubIssueEntity_Triples_CorePredicates(t *testing.T) {
	entity := githubprworkflow.GitHubIssueEntity{
		Org: "acme", Repo: "webapp", Number: 42,
		Title: "Fix login bug", State: "open", Author: "alice",
	}

	triples := entity.Triples()

	// Core predicates must always be present.
	wantPredicates := []string{
		"github.issue.title",
		"github.issue.state",
		"github.issue.author",
	}

	predicateSet := make(map[string]any)
	for _, tr := range triples {
		predicateSet[tr.Predicate] = tr.Object
	}

	for _, pred := range wantPredicates {
		if _, ok := predicateSet[pred]; !ok {
			t.Errorf("missing expected predicate %q in triples", pred)
		}
	}

	// Verify subject is correct entity ID on every triple.
	wantSubject := entity.EntityID()
	for _, tr := range triples {
		if tr.Subject != wantSubject {
			t.Errorf("triple subject = %q, want %q (predicate: %s)", tr.Subject, wantSubject, tr.Predicate)
		}
	}
}

func TestGitHubIssueEntity_Triples_OptionalFields(t *testing.T) {
	t.Run("severity and complexity absent when empty", func(t *testing.T) {
		entity := githubprworkflow.GitHubIssueEntity{
			Org: "acme", Repo: "webapp", Number: 1,
			Title: "T", State: "open", Author: "a",
		}
		for _, tr := range entity.Triples() {
			if tr.Predicate == "github.issue.severity" {
				t.Error("severity triple should not appear when Severity is empty")
			}
			if tr.Predicate == "github.issue.complexity" {
				t.Error("complexity triple should not appear when Complexity is empty")
			}
		}
	})

	t.Run("severity triple present when set", func(t *testing.T) {
		entity := githubprworkflow.GitHubIssueEntity{
			Org: "acme", Repo: "webapp", Number: 2,
			Title: "T", State: "open", Author: "a",
			Severity: "critical",
		}
		found := false
		for _, tr := range entity.Triples() {
			if tr.Predicate == "github.issue.severity" {
				found = true
				if tr.Object != "critical" {
					t.Errorf("severity object = %v, want %q", tr.Object, "critical")
				}
			}
		}
		if !found {
			t.Error("expected github.issue.severity triple when Severity is set")
		}
	})

	t.Run("complexity triple present when set", func(t *testing.T) {
		entity := githubprworkflow.GitHubIssueEntity{
			Org: "acme", Repo: "webapp", Number: 3,
			Title: "T", State: "open", Author: "a",
			Complexity: "large",
		}
		found := false
		for _, tr := range entity.Triples() {
			if tr.Predicate == "github.issue.complexity" {
				found = true
				if tr.Object != "large" {
					t.Errorf("complexity object = %v, want %q", tr.Object, "large")
				}
			}
		}
		if !found {
			t.Error("expected github.issue.complexity triple when Complexity is set")
		}
	})

	t.Run("label triples produced for each label", func(t *testing.T) {
		entity := githubprworkflow.GitHubIssueEntity{
			Org: "acme", Repo: "webapp", Number: 4,
			Title: "T", State: "open", Author: "a",
			Labels: []string{"bug", "priority:high", "needs-triage"},
		}
		var labelObjects []any
		for _, tr := range entity.Triples() {
			if tr.Predicate == "github.issue.label" {
				labelObjects = append(labelObjects, tr.Object)
			}
		}
		if len(labelObjects) != 3 {
			t.Errorf("got %d label triples, want 3", len(labelObjects))
		}
		wantLabels := map[any]bool{"bug": true, "priority:high": true, "needs-triage": true}
		for _, obj := range labelObjects {
			if !wantLabels[obj] {
				t.Errorf("unexpected label object: %v", obj)
			}
		}
	})

	t.Run("no label triples when Labels is empty", func(t *testing.T) {
		entity := githubprworkflow.GitHubIssueEntity{
			Org: "acme", Repo: "webapp", Number: 5,
			Title: "T", State: "open", Author: "a",
		}
		for _, tr := range entity.Triples() {
			if tr.Predicate == "github.issue.label" {
				t.Error("label triple should not appear when Labels is empty")
			}
		}
	})
}

func TestGitHubPREntity_EntityID(t *testing.T) {
	tests := []struct {
		name   string
		entity githubprworkflow.GitHubPREntity
		wantID string
	}{
		{
			name: "standard PR",
			entity: githubprworkflow.GitHubPREntity{
				Org: "acme", Repo: "webapp", Number: 101,
				Title: "Fix login", State: "open", Head: "fix/login", Base: "main",
			},
			wantID: "acme.github.repo.webapp.pr.101",
		},
		{
			name: "merged PR",
			entity: githubprworkflow.GitHubPREntity{
				Org: "bigco", Repo: "platform", Number: 7,
				Title: "Feature", State: "merged", Head: "feat/x", Base: "main",
			},
			wantID: "bigco.github.repo.platform.pr.7",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.entity.EntityID()
			if got != tt.wantID {
				t.Errorf("EntityID() = %q, want %q", got, tt.wantID)
			}
		})
	}
}

func TestGitHubPREntity_Triples_CorePredicates(t *testing.T) {
	entity := githubprworkflow.GitHubPREntity{
		Org: "acme", Repo: "webapp", Number: 101,
		Title: "Fix login", State: "open", Head: "fix/login", Base: "main",
	}

	triples := entity.Triples()

	wantPredicates := []string{
		"github.pr.title",
		"github.pr.state",
		"github.pr.head",
		"github.pr.base",
	}

	predicateSet := make(map[string]bool)
	for _, tr := range triples {
		predicateSet[tr.Predicate] = true
	}

	for _, pred := range wantPredicates {
		if !predicateSet[pred] {
			t.Errorf("missing expected predicate %q in triples", pred)
		}
	}
}

func TestGitHubPREntity_Triples_FixesRelationship(t *testing.T) {
	t.Run("fixes triple present when IssueNumber set", func(t *testing.T) {
		entity := githubprworkflow.GitHubPREntity{
			Org: "acme", Repo: "webapp", Number: 101,
			Title: "Fix", State: "open", Head: "fix/login", Base: "main",
			IssueNumber: 42,
		}
		wantIssueID := fmt.Sprintf("%s.github.repo.%s.issue.%d", entity.Org, entity.Repo, entity.IssueNumber)
		found := false
		for _, tr := range entity.Triples() {
			if tr.Predicate == "github.pr.fixes" {
				found = true
				if tr.Object != wantIssueID {
					t.Errorf("fixes object = %v, want %q", tr.Object, wantIssueID)
				}
			}
		}
		if !found {
			t.Error("expected github.pr.fixes triple when IssueNumber is set")
		}
	})

	t.Run("fixes triple absent when IssueNumber is zero", func(t *testing.T) {
		entity := githubprworkflow.GitHubPREntity{
			Org: "acme", Repo: "webapp", Number: 102,
			Title: "Chore", State: "open", Head: "chore/cleanup", Base: "main",
		}
		for _, tr := range entity.Triples() {
			if tr.Predicate == "github.pr.fixes" {
				t.Error("fixes triple should not appear when IssueNumber is zero")
			}
		}
	})
}

func TestGitHubPREntity_Triples_FilesChanged(t *testing.T) {
	t.Run("file triples produced for each file", func(t *testing.T) {
		entity := githubprworkflow.GitHubPREntity{
			Org: "acme", Repo: "webapp", Number: 103,
			Title: "Update", State: "open", Head: "feat/update", Base: "main",
			FilesChanged: []string{"cmd/main.go", "internal/auth/handler.go"},
		}
		var fileObjects []any
		for _, tr := range entity.Triples() {
			if tr.Predicate == "github.pr.file" {
				fileObjects = append(fileObjects, tr.Object)
			}
		}
		if len(fileObjects) != 2 {
			t.Errorf("got %d file triples, want 2", len(fileObjects))
		}
	})

	t.Run("no file triples when FilesChanged is empty", func(t *testing.T) {
		entity := githubprworkflow.GitHubPREntity{
			Org: "acme", Repo: "webapp", Number: 104,
			Title: "Empty", State: "open", Head: "feat/empty", Base: "main",
		}
		for _, tr := range entity.Triples() {
			if tr.Predicate == "github.pr.file" {
				t.Error("file triple should not appear when FilesChanged is empty")
			}
		}
	})
}

func TestGitHubReviewEntity_EntityID(t *testing.T) {
	tests := []struct {
		name   string
		entity githubprworkflow.GitHubReviewEntity
		wantID string
	}{
		{
			name: "standard review",
			entity: githubprworkflow.GitHubReviewEntity{
				Org: "acme", Repo: "webapp", ID: "rev-001",
				PRNumber: 101, Verdict: "approve", Issues: 0, Agent: "reviewer",
			},
			wantID: "acme.github.repo.webapp.review.rev-001",
		},
		{
			name: "review with UUID",
			entity: githubprworkflow.GitHubReviewEntity{
				Org: "bigco", Repo: "core", ID: "550e8400-e29b-41d4-a716-446655440000",
				PRNumber: 7, Verdict: "request_changes", Issues: 3, Agent: "reviewer",
			},
			wantID: "bigco.github.repo.core.review.550e8400-e29b-41d4-a716-446655440000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.entity.EntityID()
			if got != tt.wantID {
				t.Errorf("EntityID() = %q, want %q", got, tt.wantID)
			}
		})
	}
}

func TestGitHubReviewEntity_Triples_CorePredicates(t *testing.T) {
	entity := githubprworkflow.GitHubReviewEntity{
		Org: "acme", Repo: "webapp", ID: "rev-001",
		PRNumber: 101, Verdict: "approve", Issues: 0, Agent: "reviewer",
	}

	triples := entity.Triples()

	wantPredicates := []string{
		"github.review.verdict",
		"github.review.issues",
		"github.review.agent",
	}

	predicateSet := make(map[string]any)
	for _, tr := range triples {
		predicateSet[tr.Predicate] = tr.Object
	}

	for _, pred := range wantPredicates {
		if _, ok := predicateSet[pred]; !ok {
			t.Errorf("missing expected predicate %q in triples", pred)
		}
	}

	// Verify verdict value.
	if predicateSet["github.review.verdict"] != "approve" {
		t.Errorf("verdict = %v, want %q", predicateSet["github.review.verdict"], "approve")
	}
}

func TestGitHubReviewEntity_Triples_TargetsRelationship(t *testing.T) {
	t.Run("targets triple present when PRNumber set", func(t *testing.T) {
		entity := githubprworkflow.GitHubReviewEntity{
			Org: "acme", Repo: "webapp", ID: "rev-002",
			PRNumber: 101, Verdict: "request_changes", Issues: 2, Agent: "reviewer",
		}
		wantPRID := fmt.Sprintf("%s.github.repo.%s.pr.%d", entity.Org, entity.Repo, entity.PRNumber)
		found := false
		for _, tr := range entity.Triples() {
			if tr.Predicate == "github.review.targets" {
				found = true
				if tr.Object != wantPRID {
					t.Errorf("targets object = %v, want %q", tr.Object, wantPRID)
				}
			}
		}
		if !found {
			t.Error("expected github.review.targets triple when PRNumber is set")
		}
	})

	t.Run("targets triple absent when PRNumber is zero", func(t *testing.T) {
		entity := githubprworkflow.GitHubReviewEntity{
			Org: "acme", Repo: "webapp", ID: "rev-003",
			Verdict: "approve", Issues: 0, Agent: "reviewer",
		}
		for _, tr := range entity.Triples() {
			if tr.Predicate == "github.review.targets" {
				t.Error("targets triple should not appear when PRNumber is zero")
			}
		}
	})
}

func TestGitHubReviewEntity_Triples_ConfidenceAndSource(t *testing.T) {
	entity := githubprworkflow.GitHubReviewEntity{
		Org: "acme", Repo: "webapp", ID: "rev-004",
		PRNumber: 101, Verdict: "approve", Issues: 0, Agent: "reviewer",
	}
	for _, tr := range entity.Triples() {
		if tr.Confidence != 1.0 {
			t.Errorf("triple %q confidence = %v, want 1.0", tr.Predicate, tr.Confidence)
		}
		if tr.Source != "github-workflow" {
			t.Errorf("triple %q source = %q, want %q", tr.Predicate, tr.Source, "github-workflow")
		}
	}
}
