package research

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// EvidenceChannel identifies the retrieval channel that produced an EvidenceRecord.
type EvidenceChannel string

// Supported EvidenceChannel values (ADR-060 §5, #271, #273).
const (
	ChannelGitHub    EvidenceChannel = "github"
	ChannelWeb       EvidenceChannel = "web"
	ChannelSemSource EvidenceChannel = "semsource"
)

// SourceClass classifies the authority tier of an evidence source (ADR-060 §6, #271).
type SourceClass string

// Supported SourceClass values.
const (
	SourceClassPrimary   SourceClass = "primary"
	SourceClassOfficial  SourceClass = "official"
	SourceClassSecondary SourceClass = "secondary"
	SourceClassCommunity SourceClass = "community"
)

// trackingQueryParams lists query parameters stripped when normalizing web URLs
// to canonical origin identities (#271). Semantic query parameters (e.g. id,
// page, version, tag) are preserved so distinct resources never collapse.
var trackingQueryParams = map[string]bool{
	"fbclid":  true,
	"gclid":   true,
	"mc_cid":  true,
	"mc_eid":  true,
	"ref":     true,
	"ref_src": true,
	"s":       true,
}

// EvidenceRecord is the channel-typed evidence unit shared across retrieval
// adapters (such as github_read in #273) and artifact claim derivation (#271).
//
// Tool results populate RawBody with the deterministic text representation of
// the retrieved object so the emit executor can verify that every cited Quote
// appears verbatim in a tool result from the run's own trajectory without
// calling an LLM.
type EvidenceRecord struct {
	Channel       EvidenceChannel `json:"channel"`
	Locator       string          `json:"locator"`
	OriginID      string          `json:"origin_id"`
	SourceClass   SourceClass     `json:"source_class"`
	RetrievedAt   time.Time       `json:"retrieved_at"`
	RawBody       string          `json:"raw_body,omitempty"`
	Quote         string          `json:"quote,omitempty"`
	ToolResultRef string          `json:"tool_result_ref,omitempty"`
	Revision      int             `json:"revision,omitempty"`
	Kind          string          `json:"kind,omitempty"`
	Repository    string          `json:"repository,omitempty"`
	HTMLURL       string          `json:"html_url,omitempty"`
	UpdatedAt     *time.Time      `json:"updated_at,omitempty"`
}

// Validate checks structural invariants on an EvidenceRecord:
//   - channel is one of github, web, or semsource;
//   - locator is non-empty and normalizes to origin_id;
//   - source_class is one of primary, official, secondary, or community;
//   - retrieved_at is non-zero;
//   - at least one of raw_body or quote is non-empty, and when both are
//     present, quote must appear verbatim inside raw_body.
func (r EvidenceRecord) Validate() error {
	switch r.Channel {
	case ChannelGitHub, ChannelWeb, ChannelSemSource:
	default:
		return fmt.Errorf("evidence.channel must be one of github/web/semsource (got %q)", r.Channel)
	}
	if strings.TrimSpace(r.Locator) == "" {
		return fmt.Errorf("evidence.locator required")
	}
	expectedOrigin, err := NormalizeOriginID(r.Channel, r.Locator)
	if err != nil {
		return fmt.Errorf("evidence.locator invalid: %w", err)
	}
	if strings.TrimSpace(r.OriginID) == "" {
		return fmt.Errorf("evidence.origin_id required")
	}
	if r.OriginID != expectedOrigin {
		return fmt.Errorf("evidence.origin_id %q does not match canonical normalization %q for locator %q", r.OriginID, expectedOrigin, r.Locator)
	}
	switch r.SourceClass {
	case SourceClassPrimary, SourceClassOfficial, SourceClassSecondary, SourceClassCommunity:
	default:
		return fmt.Errorf("evidence.source_class must be one of primary/official/secondary/community (got %q)", r.SourceClass)
	}
	if r.RetrievedAt.IsZero() {
		return fmt.Errorf("evidence.retrieved_at required")
	}
	if strings.TrimSpace(r.RawBody) == "" && strings.TrimSpace(r.Quote) == "" {
		return fmt.Errorf("evidence requires non-empty raw_body or quote")
	}
	if r.RawBody != "" && r.Quote != "" && !strings.Contains(r.RawBody, r.Quote) {
		return fmt.Errorf("evidence.quote does not appear in evidence.raw_body for locator %q", r.Locator)
	}
	return nil
}

// ValidateToolResult validates an EvidenceRecord emitted by a retrieval tool
// (such as github_read), requiring a non-empty RawBody that downstream emit
// derivation checks can match against.
func (r EvidenceRecord) ValidateToolResult() error {
	if err := r.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(r.RawBody) == "" {
		return fmt.Errorf("evidence.raw_body required on retrieval tool results")
	}
	return nil
}

// NormalizeOriginID derives the canonical origin identity for a locator under
// the given channel (#271, #273):
//   - github: normalizes "github://<Owner>/<Repo>/<resource...>" or
//     "https://github.com/<Owner>/<Repo>/<resource...>" by lowercasing the
//     scheme and "<owner>/<repo>" while preserving the resource path so
//     distinct issues, PRs, releases, commits, and workflow runs remain
//     distinct origins.
//   - web: parses http/https URLs, lowercases scheme and host, removes URL
//     fragments, strips tracking query parameters (utm_*, fbclid, gclid, etc.),
//     and trims a redundant trailing slash on non-root paths.
//   - semsource: trims whitespace and normalizes scheme/host casing while
//     preserving the entity or path identifier.
func NormalizeOriginID(channel EvidenceChannel, locator string) (string, error) {
	loc := strings.TrimSpace(locator)
	if loc == "" {
		return "", fmt.Errorf("locator is empty")
	}

	switch channel {
	case ChannelGitHub:
		return normalizeGitHubOrigin(loc)
	case ChannelWeb:
		return normalizeWebOrigin(loc)
	case ChannelSemSource:
		return normalizeSemSourceOrigin(loc)
	default:
		return "", fmt.Errorf("unsupported evidence channel %q", channel)
	}
}

func normalizeGitHubOrigin(loc string) (string, error) {
	var rest string
	switch {
	case strings.HasPrefix(strings.ToLower(loc), "github://"):
		rest = loc[len("github://"):]
	case strings.HasPrefix(strings.ToLower(loc), "https://github.com/"):
		rest = loc[len("https://github.com/"):]
	case strings.HasPrefix(strings.ToLower(loc), "http://github.com/"):
		rest = loc[len("http://github.com/"):]
	default:
		return "", fmt.Errorf("github locator %q must use github://<owner>/<repo>/<resource> or https://github.com/<owner>/<repo>/<resource>", loc)
	}

	if idx := strings.IndexAny(rest, "?#"); idx >= 0 {
		rest = rest[:idx]
	}
	rest = strings.Trim(rest, "/")
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 3 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" || strings.TrimSpace(parts[2]) == "" {
		return "", fmt.Errorf("github locator %q must include owner, repo, and resource path", loc)
	}
	owner := strings.ToLower(strings.TrimSpace(parts[0]))
	repo := strings.ToLower(strings.TrimSpace(parts[1]))
	resource := strings.Trim(parts[2], "/")
	if resource == "" {
		return "", fmt.Errorf("github locator %q has empty resource path", loc)
	}
	return fmt.Sprintf("github://%s/%s/%s", owner, repo, resource), nil
}

func normalizeWebOrigin(loc string) (string, error) {
	u, err := url.Parse(loc)
	if err != nil {
		return "", fmt.Errorf("parse web locator %q: %w", loc, err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("web locator %q must have http or https scheme", loc)
	}
	host := strings.ToLower(u.Host)
	if host == "" {
		return "", fmt.Errorf("web locator %q is missing host", loc)
	}
	u.Scheme = scheme
	u.Host = host
	u.Fragment = ""
	u.RawFragment = ""

	if u.RawQuery != "" {
		q := u.Query()
		for k := range q {
			lk := strings.ToLower(k)
			if strings.HasPrefix(lk, "utm_") || trackingQueryParams[lk] {
				q.Del(k)
			}
		}
		u.RawQuery = q.Encode()
	}

	if len(u.Path) > 1 {
		u.Path = strings.TrimRight(u.Path, "/")
	}
	u.RawPath = ""
	return u.String(), nil
}

func normalizeSemSourceOrigin(loc string) (string, error) {
	if strings.HasPrefix(strings.ToLower(loc), "semsource://") {
		body := strings.Trim(loc[len("semsource://"):], "/")
		if body == "" {
			return "", fmt.Errorf("semsource locator %q has empty resource path", loc)
		}
		return "semsource://" + body, nil
	}
	return "", fmt.Errorf("semsource locator %q must start with semsource://", loc)
}
