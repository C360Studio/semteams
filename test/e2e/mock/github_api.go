package mock

import (
	"encoding/json"
	"net/http"
	"strings"
)

type mockGHUser struct {
	Login string `json:"login"`
}

type mockGHLabel struct {
	Name string `json:"name"`
}

type mockGHMilestone struct {
	Title string `json:"title"`
}

type mockGHIssue struct {
	Number    int              `json:"number"`
	Title     string           `json:"title"`
	Body      string           `json:"body"`
	State     string           `json:"state"`
	HTMLURL   string           `json:"html_url"`
	User      mockGHUser       `json:"user"`
	Labels    []mockGHLabel    `json:"labels,omitempty"`
	Assignees []mockGHUser     `json:"assignees,omitempty"`
	Milestone *mockGHMilestone `json:"milestone,omitempty"`
	CreatedAt string           `json:"created_at"`
	UpdatedAt string           `json:"updated_at"`
}

type mockGHPullRef struct {
	Ref string `json:"ref"`
	SHA string `json:"sha,omitempty"`
}

type mockGHPull struct {
	Number    int           `json:"number"`
	Title     string        `json:"title"`
	Body      string        `json:"body"`
	State     string        `json:"state"`
	Draft     bool          `json:"draft"`
	MergedAt  *string       `json:"merged_at,omitempty"`
	HTMLURL   string        `json:"html_url"`
	User      mockGHUser    `json:"user"`
	Head      mockGHPullRef `json:"head"`
	Base      mockGHPullRef `json:"base"`
	Labels    []mockGHLabel `json:"labels,omitempty"`
	CreatedAt string        `json:"created_at"`
	UpdatedAt string        `json:"updated_at"`
}

type mockGHRelease struct {
	ID          int64      `json:"id"`
	TagName     string     `json:"tag_name"`
	Name        string     `json:"name"`
	Body        string     `json:"body"`
	Draft       bool       `json:"draft"`
	Prerelease  bool       `json:"prerelease"`
	HTMLURL     string     `json:"html_url"`
	Author      mockGHUser `json:"author"`
	CreatedAt   string     `json:"created_at"`
	PublishedAt string     `json:"published_at"`
}

type mockGHCommitAuthor struct {
	Name string `json:"name,omitempty"`
	Date string `json:"date"`
}

type mockGHCommitBody struct {
	Message   string             `json:"message"`
	Author    mockGHCommitAuthor `json:"author"`
	Committer mockGHCommitAuthor `json:"committer"`
}

type mockGHCommit struct {
	SHA     string           `json:"sha"`
	HTMLURL string           `json:"html_url"`
	Commit  mockGHCommitBody `json:"commit"`
	Author  *mockGHUser      `json:"author,omitempty"`
}

type mockGHWorkflowRun struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	HeadBranch string `json:"head_branch"`
	HeadSHA    string `json:"head_sha"`
	Event      string `json:"event"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	HTMLURL    string `json:"html_url"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

type mockGHWorkflowRunsEnvelope struct {
	WorkflowRuns []mockGHWorkflowRun `json:"workflow_runs"`
}

// handleGitHubRepos serves deterministic, read-only GitHub REST API v3 responses
// for the configured portfolio repositories (C360Studio/semteams,
// C360Studio/semengine, C360Studio/semstreams) so Program Pulse (/pulse) and
// github_read E2E/contract journeys run without external network calls (#267).
func (s *OpenAIServer) handleGitHubRepos(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "Method Not Allowed"})
		return
	}

	trimmed := strings.TrimPrefix(r.URL.Path, "/repos/")
	parts := strings.Split(strings.Trim(trimmed, "/"), "/")
	if len(parts) < 3 {
		writeGitHubNotFound(w)
		return
	}

	repoKey := strings.ToLower(parts[0] + "/" + parts[1])
	surface := strings.Join(parts[2:], "/")

	data, ok := lookupMockGitHubSurface(repoKey, surface, r.URL.Query().Get("state"))
	if !ok {
		writeGitHubNotFound(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(data)
}

func writeGitHubNotFound(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Not Found"})
}

func lookupMockGitHubSurface(repoKey, surface, stateFilter string) (any, bool) {
	switch repoKey {
	case "c360studio/semteams":
		return lookupSemTeamsSurface(surface, stateFilter)
	case "c360studio/semengine":
		return lookupSemEngineSurface(surface, stateFilter)
	case "c360studio/semstreams":
		return lookupSemStreamsSurface(surface, stateFilter)
	default:
		return nil, false
	}
}

func lookupSemTeamsSurface(surface, stateFilter string) (any, bool) {
	switch surface {
	case "issues":
		issues := []mockGHIssue{
			{
				Number:    267,
				Title:     "program-report: deliver a read-only Program Pulse walking skeleton",
				Body:      "Deliver a read-only Program Pulse walking skeleton backed by github_read.",
				State:     "open",
				HTMLURL:   "https://github.com/C360Studio/semteams/issues/267",
				User:      mockGHUser{Login: "coby"},
				Labels:    []mockGHLabel{{Name: "area:product"}, {Name: "horizon:pre-v1"}},
				Assignees: []mockGHUser{{Login: "coby"}},
				Milestone: &mockGHMilestone{Title: "v0.2.0"},
				CreatedAt: "2026-10-14T12:00:00Z",
				UpdatedAt: "2026-10-17T08:00:00Z",
			},
			{
				Number:    272,
				Title:     "research: enforce containment and cadence budgets",
				Body:      "Enforce per-spawn loop_max_iterations and 1..4 planner fan-out ceiling.",
				State:     "closed",
				HTMLURL:   "https://github.com/C360Studio/semteams/issues/272",
				User:      mockGHUser{Login: "coby"},
				Labels:    []mockGHLabel{{Name: "area:agent"}},
				Milestone: &mockGHMilestone{Title: "v0.2.0"},
				CreatedAt: "2026-10-14T12:00:00Z",
				UpdatedAt: "2026-10-17T07:30:00Z",
			},
		}
		return filterMockIssues(issues, stateFilter), true
	case "pulls":
		mergedAt := "2026-10-17T07:30:00Z"
		pulls := []mockGHPull{
			{
				Number:    336,
				Title:     "feat(research): enforce 1..4 planner fan-out ceiling and containment budgets",
				Body:      "Closes #272 and unblocks #267 for v0.2.0.",
				State:     "closed",
				Draft:     false,
				MergedAt:  &mergedAt,
				HTMLURL:   "https://github.com/C360Studio/semteams/pull/336",
				User:      mockGHUser{Login: "coby"},
				Head:      mockGHPullRef{Ref: "gemini/research-containment-272", SHA: "6777381e"},
				Base:      mockGHPullRef{Ref: "main"},
				Labels:    []mockGHLabel{{Name: "area:agent"}},
				CreatedAt: "2026-10-17T06:00:00Z",
				UpdatedAt: "2026-10-17T07:30:00Z",
			},
		}
		return filterMockPulls(pulls, stateFilter), true
	case "releases":
		return []mockGHRelease{
			{
				ID:          1001,
				TagName:     "v0.1.3",
				Name:        "v0.1.3",
				Body:        "Baseline pre-v0.2.0 release with github_read and /work.",
				Draft:       false,
				Prerelease:  false,
				HTMLURL:     "https://github.com/C360Studio/semteams/releases/tag/v0.1.3",
				Author:      mockGHUser{Login: "coby"},
				CreatedAt:   "2026-10-15T12:00:00Z",
				PublishedAt: "2026-10-15T12:05:00Z",
			},
		}, true
	case "commits":
		return []mockGHCommit{
			{
				SHA:     "6777381e",
				HTMLURL: "https://github.com/C360Studio/semteams/commit/6777381e",
				Author:  &mockGHUser{Login: "coby"},
				Commit: mockGHCommitBody{
					Message:   "feat(research): enforce 1..4 planner fan-out ceiling (#336)",
					Author:    mockGHCommitAuthor{Name: "coby", Date: "2026-10-17T07:30:00Z"},
					Committer: mockGHCommitAuthor{Date: "2026-10-17T07:30:00Z"},
				},
			},
		}, true
	case "actions/runs":
		return mockGHWorkflowRunsEnvelope{
			WorkflowRuns: []mockGHWorkflowRun{
				{
					ID:         9001,
					Name:       "Repository CI",
					HeadBranch: "main",
					HeadSHA:    "6777381e",
					Event:      "push",
					Status:     "completed",
					Conclusion: "success",
					HTMLURL:    "https://github.com/C360Studio/semteams/actions/runs/9001",
					CreatedAt:  "2026-10-17T07:31:00Z",
					UpdatedAt:  "2026-10-17T07:35:00Z",
				},
			},
		}, true
	default:
		return nil, false
	}
}

func lookupSemEngineSurface(surface, stateFilter string) (any, bool) {
	switch surface {
	case "issues":
		issues := []mockGHIssue{
			{
				Number:    24,
				Title:     "durable-execution: generic attempt, effect ledger, and replay contract",
				Body:      "Define generic attempt/effect/replay durability contract for SemTeams runtime adoption.",
				State:     "open",
				HTMLURL:   "https://github.com/C360Studio/semengine/issues/24",
				User:      mockGHUser{Login: "coby"},
				Labels:    []mockGHLabel{{Name: "area:durability"}},
				CreatedAt: "2026-10-10T12:00:00Z",
				UpdatedAt: "2026-10-16T18:00:00Z",
			},
		}
		return filterMockIssues(issues, stateFilter), true
	case "pulls":
		return []mockGHPull{}, true
	case "releases":
		return []mockGHRelease{}, true
	case "commits":
		return []mockGHCommit{
			{
				SHA:     "a1b2c3d4",
				HTMLURL: "https://github.com/C360Studio/semengine/commit/a1b2c3d4",
				Author:  &mockGHUser{Login: "coby"},
				Commit: mockGHCommitBody{
					Message:   "docs(adr): align substrate durability boundary with SemTeams ADR-061",
					Author:    mockGHCommitAuthor{Name: "coby", Date: "2026-10-16T17:00:00Z"},
					Committer: mockGHCommitAuthor{Date: "2026-10-16T17:00:00Z"},
				},
			},
		}, true
	case "actions/runs":
		return mockGHWorkflowRunsEnvelope{
			WorkflowRuns: []mockGHWorkflowRun{
				{
					ID:         9002,
					Name:       "CI",
					HeadBranch: "main",
					HeadSHA:    "a1b2c3d4",
					Event:      "push",
					Status:     "completed",
					Conclusion: "success",
					HTMLURL:    "https://github.com/C360Studio/semengine/actions/runs/9002",
					CreatedAt:  "2026-10-16T17:01:00Z",
					UpdatedAt:  "2026-10-16T17:04:00Z",
				},
			},
		}, true
	default:
		return nil, false
	}
}

func lookupSemStreamsSurface(surface, stateFilter string) (any, bool) {
	switch surface {
	case "issues":
		return []mockGHIssue{}, true
	case "pulls":
		return []mockGHPull{}, true
	case "releases":
		return []mockGHRelease{
			{
				ID:          1002,
				TagName:     "v1.0.0-beta.162",
				Name:        "v1.0.0-beta.162",
				Body:        "Frozen SemStreams dependency baseline 8b99efe9c66a.",
				Draft:       false,
				Prerelease:  true,
				HTMLURL:     "https://github.com/C360Studio/semstreams/releases/tag/v1.0.0-beta.162",
				Author:      mockGHUser{Login: "coby"},
				CreatedAt:   "2026-09-30T15:00:00Z",
				PublishedAt: "2026-09-30T15:02:12Z",
			},
		}, true
	case "commits":
		return []mockGHCommit{
			{
				SHA:     "8b99efe9",
				HTMLURL: "https://github.com/C360Studio/semstreams/commit/8b99efe9",
				Author:  &mockGHUser{Login: "coby"},
				Commit: mockGHCommitBody{
					Message:   "release: v1.0.0-beta.162 frozen baseline",
					Author:    mockGHCommitAuthor{Name: "coby", Date: "2026-09-30T15:02:12Z"},
					Committer: mockGHCommitAuthor{Date: "2026-09-30T15:02:12Z"},
				},
			},
		}, true
	case "actions/runs":
		return mockGHWorkflowRunsEnvelope{
			WorkflowRuns: []mockGHWorkflowRun{
				{
					ID:         9003,
					Name:       "CI",
					HeadBranch: "main",
					HeadSHA:    "8b99efe9",
					Event:      "push",
					Status:     "completed",
					Conclusion: "success",
					HTMLURL:    "https://github.com/C360Studio/semstreams/actions/runs/9003",
					CreatedAt:  "2026-09-30T15:03:00Z",
					UpdatedAt:  "2026-09-30T15:06:00Z",
				},
			},
		}, true
	default:
		return nil, false
	}
}

func filterMockIssues(items []mockGHIssue, state string) []mockGHIssue {
	state = strings.ToLower(strings.TrimSpace(state))
	if state == "" || state == "all" {
		return items
	}
	out := make([]mockGHIssue, 0, len(items))
	for _, it := range items {
		if it.State == state {
			out = append(out, it)
		}
	}
	return out
}

func filterMockPulls(items []mockGHPull, state string) []mockGHPull {
	state = strings.ToLower(strings.TrimSpace(state))
	if state == "" || state == "all" {
		return items
	}
	out := make([]mockGHPull, 0, len(items))
	for _, pr := range items {
		if pr.State == state {
			out = append(out, pr)
		}
	}
	return out
}
