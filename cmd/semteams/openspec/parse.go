package openspec

import (
	"fmt"
	"strings"
)

// scenarioStepKeywords are the recognised Given/When/Then bullet leads.
var scenarioStepKeywords = map[string]bool{
	"GIVEN": true,
	"WHEN":  true,
	"THEN":  true,
	"AND":   true,
}

// bulletMarkers are the CommonMark unordered-list markers a scenario step may
// use. OpenSpec's canonical form is "- ", but real brownfield specs (and LLM
// output) also use "* " and "+ "; all three are recognised on ingest and
// normalised back to "- " on render.
var bulletMarkers = []string{"- ", "* ", "+ "}

// ParseSpec parses a capability spec.md body into a Spec. It is a line-oriented
// state machine over OpenSpec's regular heading structure:
//
//	# <Title>
//	## Purpose
//	## Requirements
//	### Requirement: <Name>
//	<RFC-2119 statement>
//	#### Scenario: <Name>
//	- GIVEN/WHEN/THEN/AND <text>
//
// Ingest is lenient by design (ADR-057 §D2 — the graph is canonical, markdown
// is a lossy projection): ParseSpec never fails, returning a best-effort Spec.
// Content that cannot be placed is dropped; the structural cases are recorded
// in Spec.Warnings rather than lost silently:
//
//   - an unknown "## " section — its prose is skipped (forward-compat);
//   - a "#### Scenario:" before any requirement — dropped, and warned;
//   - prose after a scenario has opened — dropped (it is past the statement slot);
//   - a "### Requirement:" with an empty name — kept as-is.
//
// Strict structural validation (every requirement has a SHALL statement and
// ≥1 scenario) is deliberately NOT done here — it belongs to create_change's
// emit_change tool, which gates authored output, not ingest (ADR-057 §D3).
func ParseSpec(md string) *Spec {
	s := &Spec{}

	var (
		section      string // "purpose" | "requirements" | "" (other/none)
		purposeLines []string
		stmtLines    []string
		curReq       *Requirement
		curScen      *Scenario
	)

	flushScenario := func() {
		if curReq != nil && curScen != nil {
			curReq.Scenarios = append(curReq.Scenarios, *curScen)
		}
		curScen = nil
	}
	flushRequirement := func() {
		flushScenario()
		if curReq != nil {
			if len(stmtLines) > 0 {
				// Newline-join (matching Purpose) preserves the authored line
				// structure of a multi-line statement; render writes it back
				// verbatim, so re-parse is stable.
				curReq.Statement = strings.TrimSpace(strings.Join(stmtLines, "\n"))
			}
			s.Requirements = append(s.Requirements, *curReq)
		}
		curReq = nil
		stmtLines = nil
	}

	for raw := range strings.SplitSeq(md, "\n") {
		line := strings.TrimRight(raw, " \t\r")
		trimmed := strings.TrimSpace(line)

		switch {
		case strings.HasPrefix(line, "#### Scenario:"):
			flushScenario()
			name := strings.TrimSpace(strings.TrimPrefix(line, "#### Scenario:"))
			if curReq == nil {
				s.Warnings = append(s.Warnings,
					fmt.Sprintf("scenario %q before any requirement; dropped", name))
				break
			}
			curScen = &Scenario{Name: name}

		case strings.HasPrefix(line, "### Requirement:"):
			flushRequirement()
			curReq = &Requirement{Name: strings.TrimSpace(strings.TrimPrefix(line, "### Requirement:"))}

		case strings.HasPrefix(line, "## "):
			flushRequirement()
			switch strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, "## "))) {
			case "purpose":
				section = "purpose"
			case "requirements":
				section = "requirements"
			default:
				section = ""
			}

		case strings.HasPrefix(line, "# "):
			s.Title = strings.TrimSpace(strings.TrimPrefix(line, "# "))

		case trimmed == "":
			// blank line — boundaries are heading-driven; ignore

		default:
			if body, ok := bulletBody(trimmed); ok && curScen != nil {
				curScen.Steps = append(curScen.Steps, parseStep(body))
				break
			}
			switch {
			case section == "purpose":
				purposeLines = append(purposeLines, trimmed)
			case curReq != nil && curScen == nil:
				// Prose between a requirement header and its first scenario is
				// the RFC-2119 statement (bullets here are kept as statement
				// body, not steps, since no scenario is open).
				stmtLines = append(stmtLines, trimmed)
			}
		}
	}
	flushRequirement()

	s.Purpose = strings.TrimSpace(strings.Join(purposeLines, "\n"))
	return s
}

// bulletBody returns the text of an unordered-list item (after its
// "- " / "* " / "+ " marker, trimmed) and true, or "", false if the line is
// not a bullet.
func bulletBody(trimmed string) (string, bool) {
	for _, m := range bulletMarkers {
		if rest, ok := strings.CutPrefix(trimmed, m); ok {
			return strings.TrimSpace(rest), true
		}
	}
	return "", false
}

// parseStep splits a scenario bullet (without its leading marker) into a Step.
// A recognised leading keyword (GIVEN/WHEN/THEN/AND, case-insensitive) is
// uppercased into Keyword; otherwise Keyword is empty and the bullet is kept
// verbatim in Text so render reproduces it.
func parseStep(bullet string) Step {
	bullet = strings.TrimSpace(bullet)
	head, rest, _ := strings.Cut(bullet, " ")
	if kw := strings.ToUpper(head); scenarioStepKeywords[kw] {
		// rest is "" for a keyword-only bullet (e.g. "GIVEN"), which renders
		// back as "- GIVEN" — stable.
		return Step{Keyword: kw, Text: strings.TrimSpace(rest)}
	}
	return Step{Text: bullet}
}

// deltaSection is the current "## ADDED/MODIFIED/REMOVED Requirements" section
// while parsing a delta.
type deltaSection int

const (
	sectionNone deltaSection = iota
	sectionAdded
	sectionModified
	sectionRemoved
)

// reqAccum accumulates one "### Requirement:" block during delta parsing,
// before it is dispatched into Delta.Added/Modified/Removed on flush.
type reqAccum struct {
	name       string
	scenarios  []Scenario
	previously string
	rationale  string
}

// ParseDelta parses a change delta spec.md body into a Delta. It reuses the
// requirement/scenario grammar of ParseSpec, adding the three delta sections
// and the "(Previously: ...)" / "(Rationale: ...)" annotations. Like ParseSpec
// it is lenient and infallible (ADR-057 §D2/§D3): a "### Requirement:" outside
// any ADDED/MODIFIED/REMOVED section, or a scenario before any requirement, is
// dropped with a warning.
func ParseDelta(md string) *Delta {
	d := &Delta{}

	var (
		section deltaSection
		cur     *reqAccum
		curScen *Scenario
		stmt    []string
	)

	flushScenario := func() {
		if cur != nil && curScen != nil {
			cur.scenarios = append(cur.scenarios, *curScen)
		}
		curScen = nil
	}
	flushRequirement := func() {
		flushScenario()
		if cur != nil {
			statement := ""
			if len(stmt) > 0 {
				statement = strings.TrimSpace(strings.Join(stmt, "\n"))
			}
			appendDeltaRequirement(d, section, cur, statement)
		}
		cur = nil
		stmt = nil
	}

	for raw := range strings.SplitSeq(md, "\n") {
		line := strings.TrimRight(raw, " \t\r")
		trimmed := strings.TrimSpace(line)

		switch {
		case strings.HasPrefix(line, "#### Scenario:"):
			flushScenario()
			name := strings.TrimSpace(strings.TrimPrefix(line, "#### Scenario:"))
			if cur == nil {
				d.Warnings = append(d.Warnings,
					fmt.Sprintf("scenario %q before any requirement; dropped", name))
				break
			}
			curScen = &Scenario{Name: name}

		case strings.HasPrefix(line, "### Requirement:"):
			flushRequirement()
			cur = &reqAccum{name: strings.TrimSpace(strings.TrimPrefix(line, "### Requirement:"))}

		case strings.HasPrefix(line, "## "):
			flushRequirement()
			section = parseDeltaSection(strings.TrimPrefix(line, "## "))

		case strings.HasPrefix(line, "# "):
			d.Title = strings.TrimSpace(strings.TrimPrefix(line, "# "))

		case trimmed == "":
			// blank line — ignore

		default:
			if cur != nil {
				if v, ok := parenAnnotation(trimmed, "Previously"); ok {
					cur.previously = v
					break
				}
				if v, ok := parenAnnotation(trimmed, "Rationale"); ok {
					cur.rationale = v
					break
				}
			}
			if body, ok := bulletBody(trimmed); ok && curScen != nil {
				curScen.Steps = append(curScen.Steps, parseStep(body))
				break
			}
			if cur != nil && curScen == nil {
				stmt = append(stmt, trimmed)
			}
		}
	}
	flushRequirement()

	return d
}

// appendDeltaRequirement dispatches a completed requirement accumulator into
// the matching Delta section, recording a warning for the lossy REMOVED case
// (content beyond name+rationale) and the orphan (sectionNone) case.
func appendDeltaRequirement(d *Delta, section deltaSection, cur *reqAccum, statement string) {
	req := Requirement{Name: cur.name, Statement: statement, Scenarios: cur.scenarios}
	switch section {
	case sectionAdded:
		d.Added = append(d.Added, req)
	case sectionModified:
		d.Modified = append(d.Modified, ModifiedRequirement{Requirement: req, Previously: cur.previously})
	case sectionRemoved:
		if statement != "" || len(cur.scenarios) > 0 {
			d.Warnings = append(d.Warnings,
				fmt.Sprintf("removed requirement %q carries a statement or scenarios; only name + rationale are kept", cur.name))
		}
		d.Removed = append(d.Removed, RemovedRequirement{Name: cur.name, Rationale: cur.rationale})
	default: // sectionNone
		d.Warnings = append(d.Warnings,
			fmt.Sprintf("requirement %q outside any ADDED/MODIFIED/REMOVED section; dropped", cur.name))
	}
}

// parseDeltaSection maps a "## " heading body to its delta section. The match
// is a case-insensitive prefix (not exact): OpenSpec's canonical heading is
// "## ADDED Requirements", but brownfield/LLM output also writes "## Added" or
// "## ADDED". An unrelated "## Added Notes" would false-positive into the
// section; that is an accepted leniency tradeoff (strict validation is
// emit_change's job, §D3). An unrecognised heading yields sectionNone.
func parseDeltaSection(heading string) deltaSection {
	switch h := strings.ToUpper(strings.TrimSpace(heading)); {
	case strings.HasPrefix(h, "ADDED"):
		return sectionAdded
	case strings.HasPrefix(h, "MODIFIED"):
		return sectionModified
	case strings.HasPrefix(h, "REMOVED"):
		return sectionRemoved
	default:
		return sectionNone
	}
}

// parenAnnotation parses a single-line "(Label: value)" annotation, matching
// the label case-insensitively for lenient ingest (OpenSpec emits the
// canonical capitalized form, but brownfield/LLM output may not). It returns
// the trimmed value and true on a match, else "", false. The closing ")" is
// optional — a value with no terminator is taken to end-of-line — and an
// interior ")" is preserved, since only the final one is trimmed.
func parenAnnotation(trimmed, label string) (string, bool) {
	prefix := "(" + label + ":"
	if len(trimmed) < len(prefix) || !strings.EqualFold(trimmed[:len(prefix)], prefix) {
		return "", false
	}
	inner := strings.TrimSuffix(strings.TrimSpace(trimmed[len(prefix):]), ")")
	return strings.TrimSpace(inner), true
}
