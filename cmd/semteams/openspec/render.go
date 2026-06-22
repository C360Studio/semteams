package openspec

import "strings"

// RenderSpec renders a Spec back to OpenSpec capability spec.md markdown. The
// output is canonical (one blank line between blocks, "- " bullets); it is not
// guaranteed to be byte-identical to arbitrary input, because the round-trip
// contract is semantic stability — parse(RenderSpec(parse(x))) equals parse(x)
// — not byte stability (ADR-057 §D2). Spec.Warnings is diagnostic-only and is
// not rendered. RenderSpec panics on a nil Spec.
func RenderSpec(s *Spec) string {
	var b strings.Builder

	b.WriteString("# ")
	b.WriteString(s.Title)
	b.WriteString("\n")

	if s.Purpose != "" {
		b.WriteString("\n## Purpose\n")
		b.WriteString(s.Purpose)
		b.WriteString("\n")
	}

	b.WriteString("\n## Requirements\n")
	for _, r := range s.Requirements {
		renderRequirement(&b, r, "", "")
	}

	return b.String()
}

// RenderDelta renders a Delta back to OpenSpec change delta spec.md markdown, in
// canonical section order (ADDED, MODIFIED, REMOVED); empty sections are
// omitted. Same semantic-stability contract as RenderSpec. Panics on a nil
// Delta.
func RenderDelta(d *Delta) string {
	var b strings.Builder

	b.WriteString("# ")
	b.WriteString(d.Title)
	b.WriteString("\n")

	if len(d.Added) > 0 {
		b.WriteString("\n## ADDED Requirements\n")
		for _, r := range d.Added {
			renderRequirement(&b, r, "", "")
		}
	}
	if len(d.Modified) > 0 {
		b.WriteString("\n## MODIFIED Requirements\n")
		for _, m := range d.Modified {
			renderRequirement(&b, m.Requirement, m.Previously, "")
		}
	}
	if len(d.Removed) > 0 {
		b.WriteString("\n## REMOVED Requirements\n")
		for _, r := range d.Removed {
			renderRequirement(&b, Requirement{Name: r.Name}, "", r.Rationale)
		}
	}

	return b.String()
}

// renderRequirement writes one "### Requirement:" block: name, optional
// statement, optional "(Previously:)" / "(Rationale:)" annotations (delta
// MODIFIED/REMOVED only — empty otherwise), then scenarios.
func renderRequirement(b *strings.Builder, r Requirement, previously, rationale string) {
	b.WriteString("\n### Requirement: ")
	b.WriteString(r.Name)
	b.WriteString("\n")
	if r.Statement != "" {
		b.WriteString(r.Statement)
		b.WriteString("\n")
	}
	if previously != "" {
		b.WriteString("(Previously: ")
		b.WriteString(previously)
		b.WriteString(")\n")
	}
	if rationale != "" {
		b.WriteString("(Rationale: ")
		b.WriteString(rationale)
		b.WriteString(")\n")
	}
	for _, sc := range r.Scenarios {
		renderScenario(b, sc)
	}
}

// renderScenario writes one "#### Scenario:" block. Steps are normalised to
// "- " bullets; a step with an empty Keyword is written verbatim from Text.
func renderScenario(b *strings.Builder, sc Scenario) {
	b.WriteString("\n#### Scenario: ")
	b.WriteString(sc.Name)
	b.WriteString("\n")
	for _, st := range sc.Steps {
		b.WriteString("- ")
		if st.Keyword != "" {
			b.WriteString(st.Keyword)
			if st.Text != "" {
				b.WriteString(" ")
				b.WriteString(st.Text)
			}
		} else {
			b.WriteString(st.Text)
		}
		b.WriteString("\n")
	}
}
