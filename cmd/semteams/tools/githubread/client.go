// Package githubread implements the product-local read-only github_read tool
// for the program-report gather phase (#273, ADR-060 §5 + addendum 2026-10-14).
package githubread

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/c360studio/semstreams/agentic"
)

const (
	// DefaultAPIBase is the default GitHub REST API v3 base URL.
	DefaultAPIBase = "https://api.github.com"

	defaultHTTPTimeout  = 15 * time.Second
	maxResponseBodySize = 1 << 20 // 1 MiB cap per API response
	userAgent           = "semteams-github-read/1.0"
	apiVersionHeader    = "2022-11-28"
)

// ListOpts bounds a single repository activity read.
type ListOpts struct {
	State string    // "open", "closed", or "all" (for issues and pull requests)
	Limit int       // max items to return per kind (1..100)
	Since time.Time // optional watermark hint for endpoints that support ?since=
}

// Issue represents a GitHub issue read from GET /repos/{owner}/{repo}/issues.
type Issue struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body,omitempty"`
	State     string    `json:"state"`
	Labels    []string  `json:"labels,omitempty"`
	Assignees []string  `json:"assignees,omitempty"`
	Milestone string    `json:"milestone,omitempty"`
	Author    string    `json:"author,omitempty"`
	HTMLURL   string    `json:"html_url"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PullRequest represents a GitHub pull request read from GET /repos/{owner}/{repo}/pulls.
type PullRequest struct {
	Number    int        `json:"number"`
	Title     string     `json:"title"`
	Body      string     `json:"body,omitempty"`
	State     string     `json:"state"`
	Draft     bool       `json:"draft"`
	MergedAt  *time.Time `json:"merged_at,omitempty"`
	Author    string     `json:"author,omitempty"`
	HeadRef   string     `json:"head_ref,omitempty"`
	HeadSHA   string     `json:"head_sha,omitempty"`
	BaseRef   string     `json:"base_ref,omitempty"`
	Labels    []string   `json:"labels,omitempty"`
	HTMLURL   string     `json:"html_url"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// Release represents a GitHub release read from GET /repos/{owner}/{repo}/releases.
type Release struct {
	ID          int64     `json:"id"`
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name,omitempty"`
	Body        string    `json:"body,omitempty"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	Author      string    `json:"author,omitempty"`
	HTMLURL     string    `json:"html_url"`
	CreatedAt   time.Time `json:"created_at"`
	PublishedAt time.Time `json:"published_at"`
}

// Commit represents a GitHub commit read from GET /repos/{owner}/{repo}/commits.
type Commit struct {
	SHA         string    `json:"sha"`
	Message     string    `json:"message"`
	Author      string    `json:"author,omitempty"`
	HTMLURL     string    `json:"html_url"`
	CommittedAt time.Time `json:"committed_at"`
}

// WorkflowRun represents a GitHub Actions workflow run read from GET /repos/{owner}/{repo}/actions/runs.
type WorkflowRun struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	HeadBranch string    `json:"head_branch,omitempty"`
	HeadSHA    string    `json:"head_sha,omitempty"`
	Event      string    `json:"event,omitempty"`
	Status     string    `json:"status"`
	Conclusion string    `json:"conclusion,omitempty"`
	HTMLURL    string    `json:"html_url"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Client is the strictly read-only GitHub API surface consumed by Executor.
type Client interface {
	ListIssues(ctx context.Context, owner, repo string, opts ListOpts) ([]Issue, error)
	ListPullRequests(ctx context.Context, owner, repo string, opts ListOpts) ([]PullRequest, error)
	ListReleases(ctx context.Context, owner, repo string, opts ListOpts) ([]Release, error)
	ListCommits(ctx context.Context, owner, repo string, opts ListOpts) ([]Commit, error)
	ListWorkflowRuns(ctx context.Context, owner, repo string, opts ListOpts) ([]WorkflowRun, error)
}

// APIError records a non-2xx HTTP status from the GitHub API.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("github api status %d: %s", e.StatusCode, e.Message)
}

// ClassifyError maps a Client error to an agentic.ToolErrorKind.
func ClassifyError(err error) agentic.ToolErrorKind {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden:
			return agentic.ToolErrorPermission
		case apiErr.StatusCode == http.StatusNotFound:
			return agentic.ToolErrorNotFound
		case apiErr.StatusCode == http.StatusUnprocessableEntity || apiErr.StatusCode == http.StatusBadRequest:
			return agentic.ToolErrorInvalidArgs
		default:
			return agentic.ToolErrorNetwork
		}
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return agentic.ToolErrorTimeout
	}
	return agentic.ToolErrorNetwork
}

// HTTPClient implements Client over the GitHub REST API v3 using GET requests only.
type HTTPClient struct {
	token      string
	apiBase    string
	httpClient *http.Client
}

// NewHTTPClient constructs a read-only GitHub REST client.
func NewHTTPClient(token, apiBase string, timeout time.Duration) *HTTPClient {
	base := strings.TrimRight(strings.TrimSpace(apiBase), "/")
	if base == "" {
		base = DefaultAPIBase
	}
	if timeout <= 0 {
		timeout = defaultHTTPTimeout
	}
	return &HTTPClient{
		token:      strings.TrimSpace(token),
		apiBase:    base,
		httpClient: &http.Client{Timeout: timeout},
	}
}

func (c *HTTPClient) getJSON(ctx context.Context, path string, query url.Values, dst any) error {
	endpoint := c.apiBase + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("build request %s: %w", path, err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", apiVersionHeader)
	req.Header.Set("User-Agent", userAgent)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
	if err != nil {
		return fmt.Errorf("read response %s: %w", path, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(body))
		var ghErr struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(body, &ghErr) == nil && ghErr.Message != "" {
			msg = ghErr.Message
		}
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return &APIError{StatusCode: resp.StatusCode, Message: msg}
	}

	if err := json.Unmarshal(body, dst); err != nil {
		return fmt.Errorf("decode response %s: %w", path, err)
	}
	return nil
}

// ListIssues reads repository issues from GET /repos/{owner}/{repo}/issues,
// excluding pull requests returned on the same endpoint.
func (c *HTTPClient) ListIssues(ctx context.Context, owner, repo string, opts ListOpts) ([]Issue, error) {
	q := url.Values{}
	state := opts.State
	if state == "" {
		state = "all"
	}
	q.Set("state", state)
	q.Set("sort", "updated")
	q.Set("direction", "desc")
	limit := normalizeLimit(opts.Limit)
	// Request extra items when filtering out PRs on the /issues endpoint.
	perPage := limit * 2
	if perPage > 100 {
		perPage = 100
	}
	q.Set("per_page", strconv.Itoa(perPage))
	if !opts.Since.IsZero() {
		q.Set("since", opts.Since.UTC().Format(time.RFC3339))
	}

	var raw []struct {
		Number  int    `json:"number"`
		Title   string `json:"title"`
		Body    string `json:"body"`
		State   string `json:"state"`
		HTMLURL string `json:"html_url"`
		User    struct {
			Login string `json:"login"`
		} `json:"user"`
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
		Assignees []struct {
			Login string `json:"login"`
		} `json:"assignees"`
		Milestone *struct {
			Title string `json:"title"`
		} `json:"milestone"`
		PullRequest *json.RawMessage `json:"pull_request,omitempty"`
		CreatedAt   time.Time        `json:"created_at"`
		UpdatedAt   time.Time        `json:"updated_at"`
	}
	path := fmt.Sprintf("/repos/%s/%s/issues", url.PathEscape(owner), url.PathEscape(repo))
	if err := c.getJSON(ctx, path, q, &raw); err != nil {
		return nil, err
	}

	out := make([]Issue, 0, len(raw))
	for _, item := range raw {
		if item.PullRequest != nil {
			continue
		}
		labels := make([]string, 0, len(item.Labels))
		for _, l := range item.Labels {
			if l.Name != "" {
				labels = append(labels, l.Name)
			}
		}
		assignees := make([]string, 0, len(item.Assignees))
		for _, a := range item.Assignees {
			if a.Login != "" {
				assignees = append(assignees, a.Login)
			}
		}
		var milestone string
		if item.Milestone != nil {
			milestone = item.Milestone.Title
		}
		out = append(out, Issue{
			Number:    item.Number,
			Title:     item.Title,
			Body:      item.Body,
			State:     item.State,
			Labels:    labels,
			Assignees: assignees,
			Milestone: milestone,
			Author:    item.User.Login,
			HTMLURL:   item.HTMLURL,
			CreatedAt: item.CreatedAt.UTC(),
			UpdatedAt: item.UpdatedAt.UTC(),
		})
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// ListPullRequests reads pull requests from GET /repos/{owner}/{repo}/pulls.
func (c *HTTPClient) ListPullRequests(ctx context.Context, owner, repo string, opts ListOpts) ([]PullRequest, error) {
	q := url.Values{}
	state := opts.State
	if state == "" {
		state = "all"
	}
	q.Set("state", state)
	q.Set("sort", "updated")
	q.Set("direction", "desc")
	limit := normalizeLimit(opts.Limit)
	q.Set("per_page", strconv.Itoa(limit))

	var raw []struct {
		Number   int        `json:"number"`
		Title    string     `json:"title"`
		Body     string     `json:"body"`
		State    string     `json:"state"`
		Draft    bool       `json:"draft"`
		MergedAt *time.Time `json:"merged_at"`
		HTMLURL  string     `json:"html_url"`
		User     struct {
			Login string `json:"login"`
		} `json:"user"`
		Head struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		} `json:"head"`
		Base struct {
			Ref string `json:"ref"`
		} `json:"base"`
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	path := fmt.Sprintf("/repos/%s/%s/pulls", url.PathEscape(owner), url.PathEscape(repo))
	if err := c.getJSON(ctx, path, q, &raw); err != nil {
		return nil, err
	}

	out := make([]PullRequest, 0, len(raw))
	for _, item := range raw {
		labels := make([]string, 0, len(item.Labels))
		for _, l := range item.Labels {
			if l.Name != "" {
				labels = append(labels, l.Name)
			}
		}
		var mergedAt *time.Time
		if item.MergedAt != nil && !item.MergedAt.IsZero() {
			t := item.MergedAt.UTC()
			mergedAt = &t
		}
		out = append(out, PullRequest{
			Number:    item.Number,
			Title:     item.Title,
			Body:      item.Body,
			State:     item.State,
			Draft:     item.Draft,
			MergedAt:  mergedAt,
			Author:    item.User.Login,
			HeadRef:   item.Head.Ref,
			HeadSHA:   item.Head.SHA,
			BaseRef:   item.Base.Ref,
			Labels:    labels,
			HTMLURL:   item.HTMLURL,
			CreatedAt: item.CreatedAt.UTC(),
			UpdatedAt: item.UpdatedAt.UTC(),
		})
	}
	return out, nil
}

// ListReleases reads releases from GET /repos/{owner}/{repo}/releases.
func (c *HTTPClient) ListReleases(ctx context.Context, owner, repo string, opts ListOpts) ([]Release, error) {
	q := url.Values{}
	q.Set("per_page", strconv.Itoa(normalizeLimit(opts.Limit)))

	var raw []struct {
		ID         int64  `json:"id"`
		TagName    string `json:"tag_name"`
		Name       string `json:"name"`
		Body       string `json:"body"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
		HTMLURL    string `json:"html_url"`
		Author     struct {
			Login string `json:"login"`
		} `json:"author"`
		CreatedAt   time.Time `json:"created_at"`
		PublishedAt time.Time `json:"published_at"`
	}
	path := fmt.Sprintf("/repos/%s/%s/releases", url.PathEscape(owner), url.PathEscape(repo))
	if err := c.getJSON(ctx, path, q, &raw); err != nil {
		return nil, err
	}

	out := make([]Release, 0, len(raw))
	for _, item := range raw {
		out = append(out, Release{
			ID:          item.ID,
			TagName:     item.TagName,
			Name:        item.Name,
			Body:        item.Body,
			Draft:       item.Draft,
			Prerelease:  item.Prerelease,
			Author:      item.Author.Login,
			HTMLURL:     item.HTMLURL,
			CreatedAt:   item.CreatedAt.UTC(),
			PublishedAt: item.PublishedAt.UTC(),
		})
	}
	return out, nil
}

// ListCommits reads commits from GET /repos/{owner}/{repo}/commits.
func (c *HTTPClient) ListCommits(ctx context.Context, owner, repo string, opts ListOpts) ([]Commit, error) {
	q := url.Values{}
	q.Set("per_page", strconv.Itoa(normalizeLimit(opts.Limit)))
	if !opts.Since.IsZero() {
		q.Set("since", opts.Since.UTC().Format(time.RFC3339))
	}

	var raw []struct {
		SHA     string `json:"sha"`
		HTMLURL string `json:"html_url"`
		Commit  struct {
			Message string `json:"message"`
			Author  struct {
				Name string    `json:"name"`
				Date time.Time `json:"date"`
			} `json:"author"`
			Committer struct {
				Date time.Time `json:"date"`
			} `json:"committer"`
		} `json:"commit"`
		Author *struct {
			Login string `json:"login"`
		} `json:"author"`
	}
	path := fmt.Sprintf("/repos/%s/%s/commits", url.PathEscape(owner), url.PathEscape(repo))
	if err := c.getJSON(ctx, path, q, &raw); err != nil {
		return nil, err
	}

	out := make([]Commit, 0, len(raw))
	for _, item := range raw {
		author := item.Commit.Author.Name
		if item.Author != nil && item.Author.Login != "" {
			author = item.Author.Login
		}
		committedAt := item.Commit.Committer.Date
		if committedAt.IsZero() {
			committedAt = item.Commit.Author.Date
		}
		out = append(out, Commit{
			SHA:         item.SHA,
			Message:     item.Commit.Message,
			Author:      author,
			HTMLURL:     item.HTMLURL,
			CommittedAt: committedAt.UTC(),
		})
	}
	return out, nil
}

// ListWorkflowRuns reads CI workflow runs from GET /repos/{owner}/{repo}/actions/runs.
func (c *HTTPClient) ListWorkflowRuns(ctx context.Context, owner, repo string, opts ListOpts) ([]WorkflowRun, error) {
	q := url.Values{}
	q.Set("per_page", strconv.Itoa(normalizeLimit(opts.Limit)))

	var raw struct {
		WorkflowRuns []struct {
			ID         int64     `json:"id"`
			Name       string    `json:"name"`
			HeadBranch string    `json:"head_branch"`
			HeadSHA    string    `json:"head_sha"`
			Event      string    `json:"event"`
			Status     string    `json:"status"`
			Conclusion string    `json:"conclusion"`
			HTMLURL    string    `json:"html_url"`
			CreatedAt  time.Time `json:"created_at"`
			UpdatedAt  time.Time `json:"updated_at"`
		} `json:"workflow_runs"`
	}
	path := fmt.Sprintf("/repos/%s/%s/actions/runs", url.PathEscape(owner), url.PathEscape(repo))
	if err := c.getJSON(ctx, path, q, &raw); err != nil {
		return nil, err
	}

	out := make([]WorkflowRun, 0, len(raw.WorkflowRuns))
	for _, item := range raw.WorkflowRuns {
		out = append(out, WorkflowRun{
			ID:         item.ID,
			Name:       item.Name,
			HeadBranch: item.HeadBranch,
			HeadSHA:    item.HeadSHA,
			Event:      item.Event,
			Status:     item.Status,
			Conclusion: item.Conclusion,
			HTMLURL:    item.HTMLURL,
			CreatedAt:  item.CreatedAt.UTC(),
			UpdatedAt:  item.UpdatedAt.UTC(),
		})
	}
	return out, nil
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > 100 {
		return 100
	}
	return limit
}
