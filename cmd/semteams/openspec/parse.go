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
