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

// Canonical EvidenceRecord.Kind values for web, semsource, and github citations.
// github_read sets Kind to the resource kind ("issue", "pull_request",
// "release", "commit", "workflow_run"); web_search stamps EvidenceKindSnippet
// ("snippet") so search-engine result snippets are never conflated with
// retrieved document bodies ("body").
const (
	EvidenceKindSnippet = "snippet"
	EvidenceKindBody    = "body"
	EvidenceKindEntity  = "entity"
)

// MinQuoteLength is the minimum rune length (after whitespace normalization)
// required for an EvidenceRecord.Quote cited on an Artifact claim. Prevents
// trivial 1-character quotes ("a", "the", "=") that trivially match any tool
// output (#271 M1).
const MinQuoteLength = 12

// trackingQueryParams lists query parameters stripped when normalizing URLs
// to canonical origin identities (#271). Only unambiguous ad/analytics tracking
// parameters are stripped (plus any "utm_*" prefix). Parameters that can select
// distinct resources or versions — such as "ref" (GitHub git ref), "s" (search
// query), "id", "page", "version", or "tag" — are preserved so distinct
// resources never collapse onto the same origin_id.
var trackingQueryParams = map[string]bool{
	"fbclid": true,
	"gclid":  true,
	"mc_cid": true,
	"mc_eid": true,
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

// ValidateArtifactCitation validates an EvidenceRecord cited on an Artifact
// claim (Actor or IntegrationPoint). In addition to Validate(), it requires a
// non-empty verbatim Quote of at least MinQuoteLength characters (after
// whitespace normalization), a non-empty ToolResultRef, and a positive Revision
// not exceeding maxRevision (when maxRevision >= 1).
func (r EvidenceRecord) ValidateArtifactCitation(maxRevision int) error {
	if err := r.Validate(); err != nil {
		return err
	}
	normQuote := strings.Join(strings.Fields(r.Quote), " ")
	if normQuote == "" {
		return fmt.Errorf("evidence.quote required on artifact citations")
	}
	if len([]rune(normQuote)) < MinQuoteLength {
		return fmt.Errorf("evidence.quote must be at least %d characters after whitespace normalization (got %d in %q)",
			MinQuoteLength, len([]rune(normQuote)), r.Quote)
	}
	if strings.TrimSpace(r.ToolResultRef) == "" {
		return fmt.Errorf("evidence.tool_result_ref required on artifact citations")
	}
	if r.Revision < 1 {
		return fmt.Errorf("evidence.revision must be >= 1, got %d", r.Revision)
	}
	if maxRevision >= 1 && r.Revision > maxRevision {
		return fmt.Errorf("evidence.revision (%d) cannot exceed artifact revision (%d)", r.Revision, maxRevision)
	}
	return nil
}

// NormalizeOriginID derives the canonical origin identity for a locator under
// the given channel (#271, #273):
//   - github: normalizes "github://<Owner>/<Repo>/<resource...>" or
//     "https://github.com/<Owner>/<Repo>/<resource...>" by lowercasing the
//     scheme and "<owner>/<repo>" while preserving the resource path and any
//     non-tracking query parameters (such as ?ref=v1.0.0) so distinct issues,
//     PRs, releases, commits, workflow runs, and ref-qualified paths remain
//     distinct origins.
//   - web: parses http/https URLs, lowercases scheme and host, removes URL
//     fragments, strips tracking query parameters (utm_*, fbclid, gclid,
//     mc_cid, mc_eid), and trims a redundant trailing slash on non-root paths.
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

	// Strip URL fragment first.
	if hashIdx := strings.IndexByte(rest, '#'); hashIdx >= 0 {
		rest = rest[:hashIdx]
	}

	var rawQuery string
	if qIdx := strings.IndexByte(rest, '?'); qIdx >= 0 {
		rawQuery = rest[qIdx+1:]
		rest = rest[:qIdx]
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

	canonical := fmt.Sprintf("github://%s/%s/%s", owner, repo, resource)
	if rawQuery != "" {
		if q, err := url.ParseQuery(rawQuery); err == nil {
			stripTrackingQueryParams(q)
			if encoded := q.Encode(); encoded != "" {
				canonical += "?" + encoded
			}
		}
	}
	return canonical, nil
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
		stripTrackingQueryParams(q)
		u.RawQuery = q.Encode()
	}

	if len(u.Path) > 1 {
		u.Path = strings.TrimRight(u.Path, "/")
	}
	u.RawPath = ""
	return u.String(), nil
}

func stripTrackingQueryParams(q url.Values) {
	for k := range q {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, "utm_") || trackingQueryParams[lk] {
			q.Del(k)
		}
	}
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
