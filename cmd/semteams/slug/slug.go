// Package slug centralises the YYYY-MM-DD-prefixed lower-kebab-case
// slug derivation that every cmd/semteams/tools/emit* tool uses to
// name its rendered markdown artifact under docs/<arc>/<slug>.md.
//
// Background: ADR-038 PR C iteratively shipped emit_research_artifact
// (PR C Phase C1) and emit_plan (C2); the pre-existing
// emit_dev_via_spec_artifact already had its own variant. (PR C Phase
// C3's emit_consensus was retired in ADR-041 Slice 2D-5 alongside the
// challenger role.) Two of the surviving three converged on the
// identical 3-arg shape with a loopID-based fallback when the persona
// omits a title; spec_artifact requires a non-empty title via persona
// contract and detects degenerate input via a trailing-dash check on
// the slug.
//
// This package replaces those duplicates with a single
// DeriveDated function that supports both modes via the
// fallbackPrefix parameter — empty means "no fallback, signal
// degenerate via trailing dash" (spec_artifact's contract); non-empty
// means "fall back to <prefix>-<loopID[:8]> or <prefix>" (the other
// three).
package slug

import (
	"regexp"
	"strings"
	"time"
)

// nonAlnum matches runs of non-[a-z0-9] characters for slug
// normalisation. Lower-kebab-case discipline: callers lowercase the
// title before applying this regex, which collapses runs of
// non-ASCII-alphanumeric characters to a single "-".
var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// StemFromPath extracts the chain-stable slug stem from a rendered
// artifact path. Strips the directory, the .md extension, and the
// trailing "-<kindSuffix>" if present; returns empty when the input
// is empty or when stripping yields nothing.
//
// Example:
//
//	StemFromPath("docs/research/2026-05-09-osh-meshtastic-driver-research.md", "research")
//	  → "2026-05-09-osh-meshtastic-driver"
//
// Used by ResearchMilestoneStamper to derive the chain.slug.stem
// predicate at first-emit time so downstream emit-tools can compose
// "<stem>-plan", "<stem>-consensus", "<stem>-implementation" without
// re-deriving the stem from a persona-supplied title (which drifts —
// smoke #8 run-5 D2). Inputs MAY arrive as relative or absolute paths
// per the SEMTEAMS_*_DIR contract; both are handled by basename
// extraction.
func StemFromPath(path, kindSuffix string) string {
	if path == "" {
		return ""
	}
	base := path
	if i := strings.LastIndexAny(base, "/\\"); i >= 0 {
		base = base[i+1:]
	}
	base = strings.TrimSuffix(base, ".md")
	if kindSuffix != "" {
		base = strings.TrimSuffix(base, "-"+kindSuffix)
	}
	return base
}

// DeriveDated returns a date-prefixed lower-kebab-case slug suitable
// for naming a markdown artifact under docs/<arc>/<slug>.md. The
// returned shape is "<YYYY-MM-DD>-<slug>" where the date is t in UTC
// and the slug is built from `title` (preferred) or from
// fallbackPrefix when title reduces to empty after normalisation.
//
// Behavior matrix:
//
//   - title non-empty AND has ASCII-alphanumeric content
//     → "<YYYY-MM-DD>-<title-as-kebab-case>"
//
//   - title empty (or all-non-ASCII) AND loopID non-empty AND
//     fallbackPrefix non-empty
//     → "<YYYY-MM-DD>-<fallbackPrefix>-<loopID[:8]>"
//
//   - title empty (or all-non-ASCII) AND fallbackPrefix non-empty
//     (loopID empty or unused)
//     → "<YYYY-MM-DD>-<fallbackPrefix>"
//
//   - title empty (or all-non-ASCII) AND fallbackPrefix empty
//     → "<YYYY-MM-DD>-" (trailing dash; callers can HasSuffix("-")
//     check to detect degenerate input — emit_dev_via_spec_artifact
//     uses this signal to reject titles with no ASCII content per
//     its persona contract)
//
// Title is lowercased and whitespace-trimmed before the regex
// normalisation. Same input → same output (deterministic by t and
// the input strings); good for filename-stable re-emissions on
// retry.
func DeriveDated(title, loopID, fallbackPrefix string, t time.Time) string {
	source := strings.ToLower(strings.TrimSpace(title))
	if source == "" {
		if fallbackPrefix != "" && loopID != "" {
			short := loopID
			if len(short) > 8 {
				short = short[:8]
			}
			source = fallbackPrefix + "-" + short
		} else if fallbackPrefix != "" {
			source = fallbackPrefix
		}
	}
	s := nonAlnum.ReplaceAllString(source, "-")
	s = strings.Trim(s, "-")
	// Final defensive fallback: if normalisation reduced the slug to
	// empty AND a prefix was supplied, use the prefix verbatim. When
	// no prefix is supplied (spec-artifact's contract path), leave
	// the slug empty so the caller's HasSuffix("-") check catches the
	// degenerate input.
	if s == "" && fallbackPrefix != "" {
		s = fallbackPrefix
	}
	return t.Format("2006-01-02") + "-" + s
}
