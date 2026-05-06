package evidence

import (
	"fmt"
	"strings"
)

// CheckSummary pairs the architect-cited target prose with the
// gate's per-check results. The qa-reviewer persona (R3.7.2.j′)
// reads one block per CheckSummary in its prompt; the rendering
// helper below produces that exact block shape.
//
// The caller (the future rule action / tool that wires the gate
// into the qa-reviewer's prompt, R3.7.2.k′) is responsible for
// assembling these from the architect's artifact +
// Registry.Run output: one summary per check, in the artifact's
// Checks[] order so the block headings number stably.
//
// Aggregate is intentionally NOT a field — Summarize derives it
// from Results so a caller cannot construct an inconsistent
// CheckSummary (Aggregate.Total = 99 with len(Results) = 3).
// Registry.Run already returns the Aggregate alongside Results;
// the k′ caller discards it when assembling these structs and
// the renderer recomputes from the canonical Results list.
type CheckSummary struct {
	// Index is the 1-based check ordinal, mirroring the SPEC
	// template's `### C{i+1}` heading. Used as the block heading
	// number so the qa-reviewer's reject-reason citations align
	// with the spec's prose.
	Index int

	// Target is the architect's verifiable-claim text from
	// check.Target. Quoted verbatim in the heading so the
	// reviewer can ground its verdict against what was promised.
	Target string

	// Results is the per-rule outcome list from Registry.Run.
	// Order matches check.Evidence; the reviewer reads
	// in-order to grade rule-by-rule.
	Results []Result
}

// Summarize renders one or more check summaries as the prose
// block-list the qa-reviewer's `evidence_summary` task property
// will carry. Format is intentionally markdown-flavoured (## for
// check headings, [status] prefixes on per-rule lines)
// because small LLMs read prose well and the reviewer's evaluation
// contract grounds its rules against this exact shape.
//
// Empty input yields a single line ("(no checks)") so the
// reviewer can still grade — Aggregate.IsEmpty across the whole
// artifact is the R3.7.2.f′ "under-specified architect" condition
// the reviewer rejects on.
//
// Per-Result Detail strings are echoed verbatim; they're already
// in operator-readable form (the checker's responsibility).
func Summarize(checks []CheckSummary) string {
	if len(checks) == 0 {
		return "(no checks)\n"
	}
	var b strings.Builder
	for i, cs := range checks {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(fmt.Sprintf("## Check %d — %s\n\n", cs.Index, cs.Target))
		writeAggregateLine(&b, deriveAggregate(cs.Results))
		b.WriteString("\n")
		writePerRuleLines(&b, cs.Results)
	}
	return b.String()
}

// deriveAggregate counts Status values across results to produce
// the canonical Aggregate. Mirrors Registry.Run's accumulator so
// the renderer's Aggregate matches what the gate actually emitted
// regardless of whether the caller threaded it through.
//
// Out-of-enum Status values land in none of the typed counters,
// so Total may exceed Pass+Fail+UnknownKind+Error. writeAggregateLine
// surfaces that drift via a "(counter mismatch)" headline so a
// future enum extension that doesn't update this counter shows up
// loud rather than silently routing to "NOT all passed."
func deriveAggregate(results []Result) Aggregate {
	a := Aggregate{Total: len(results)}
	for _, r := range results {
		switch r.Status {
		case StatusPass:
			a.Pass++
		case StatusFail:
			a.Fail++
		case StatusUnknownKind:
			a.UnknownKind++
		case StatusError:
			a.Error++
		}
	}
	return a
}

// writeAggregateLine renders the Aggregate as one prose line:
//
//	Aggregate: 3 pass / 1 fail / 0 unknown / 0 error / total 4 — NOT all passed
//
// Headline ("ALL PASSED" vs "NOT all passed" vs "(no rules on this
// check)") encodes the routing-relevant cases. The qa-reviewer
// can scan the headline first and only descend into per-rule when
// the Aggregate is non-trivial.
func writeAggregateLine(b *strings.Builder, agg Aggregate) {
	headline := "NOT all passed"
	switch {
	case agg.IsEmpty():
		headline = "(no rules on this check)"
	case agg.AllPassed():
		headline = "ALL PASSED"
	case agg.Pass+agg.Fail+agg.UnknownKind+agg.Error != agg.Total:
		// Defensive: if Total drifts from the typed counters
		// (a future Status value the renderer didn't learn
		// about, a caller hand-constructing Aggregate
		// inconsistently — which CheckSummary's design
		// already prevents, but this is belt-and-suspenders),
		// surface loudly so the operator routes correctly.
		headline = "(counter mismatch — out-of-enum Status?)"
	}
	b.WriteString(fmt.Sprintf(
		"Aggregate: %d pass / %d fail / %d unknown / %d error / total %d — %s\n",
		agg.Pass, agg.Fail, agg.UnknownKind, agg.Error, agg.Total, headline,
	))
}

// writePerRuleLines renders one line per Result:
//
//   - [pass] test_file_exists
//   - [fail] surefire_passing_count: expected ≥ 3 ..., got 1
//
// Detail is inlined when present; an empty Detail (the typical
// pass case) renders as "- [pass] <kind>" with no trailing colon.
func writePerRuleLines(b *strings.Builder, results []Result) {
	if len(results) == 0 {
		b.WriteString("Per-rule: (none)\n")
		return
	}
	b.WriteString("Per-rule:\n")
	for _, r := range results {
		if r.Detail == "" {
			b.WriteString(fmt.Sprintf("- [%s] %s\n", r.Status, r.Kind))
			continue
		}
		b.WriteString(fmt.Sprintf("- [%s] %s: %s\n", r.Status, r.Kind, r.Detail))
	}
}
