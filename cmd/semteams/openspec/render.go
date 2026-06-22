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
		b.WriteString("\n### Requirement: ")
		b.WriteString(r.Name)
		b.WriteString("\n")
		if r.Statement != "" {
			b.WriteString(r.Statement)
			b.WriteString("\n")
		}
		for _, sc := range r.Scenarios {
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
	}

	return b.String()
}
