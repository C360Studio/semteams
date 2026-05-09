package testharness

import (
	"fmt"
	"strings"
)

// RenderResearcherFragment produces the markdown body of the
// researcher persona fragment that lists currently-registered
// test harnesses. The fragment is upserted into the PERSONAS KV bucket
// at boot under a stable fragment ID, alongside the static
// `40-harness-catalog.md` instructions fragment.
//
// Two cases:
//   - Catalog has entries: render a numbered table-like list with
//     the fields the LLM needs to choose intelligently between
//     entries (name, image, smoke schema, what the test harness verifies).
//   - Catalog is empty: render a short "no test harnesses currently
//     registered" notice. The static instructions fragment tells
//     the LLM to flag `needs_test_harness:` in open_gaps in this case.
//
// The output is plain markdown — no raw JSON, no HTML, no code-
// fence escapes. Small models read prose well; the structured form
// (configs/harnesses.json) is for operators, not the LLM.
func RenderResearcherFragment(catalog []*TestHarness) string {
	var b strings.Builder
	b.WriteString("# Available test harnesses\n\n")

	if len(catalog) == 0 {
		b.WriteString("**No test harnesses are currently registered for this deployment.** The operator's catalog file (`configs/harnesses.json`) is empty.\n\n")
		b.WriteString("If the work this artifact describes needs integration verification against an external system (a real protocol, a real broker, a real upstream service), leave `test_harness` unset and add a single line to `open_gaps` of the form:\n\n")
		b.WriteString("    needs_test_harness: <one-line description of the integration target>\n\n")
		b.WriteString("Examples of well-formed `needs_test_harness` lines:\n\n")
		b.WriteString("- `needs_test_harness: real Meshtastic radio over LoRa for protobuf POSITION_APP packets`\n")
		b.WriteString("- `needs_test_harness: a Kafka cluster with the upstream's exact topic schema`\n")
		b.WriteString("- `needs_test_harness: an OPC-UA server exposing the vendor's address space`\n\n")
		b.WriteString("If the work does NOT need external integration verification (pure unit-testable logic, in-process algorithms), use the explicit escape hatch — leave `test_harness` unset and add a `needs_test_harness: not applicable — <one-line reason>` line to `open_gaps`. The artifact's tool layer requires either `test_harness` OR a `needs_test_harness:`-prefixed line; the \"not applicable\" pattern is honest about why the gap is empty and keeps the structural marker present so downstream tooling can distinguish \"researcher chose to skip\" from \"researcher forgot.\"\n")
		return b.String()
	}

	b.WriteString("This list is rendered from `configs/harnesses.json` at boot — it shows every test harness registered for THIS deployment, which is what your `test_harness` selection on the artifact must reference.\n\n")
	b.WriteString(fmt.Sprintf("**%d test harness(es) registered.** Pick the one whose domain best matches the integration target your work describes.\n\n", len(catalog)))
	for i, h := range catalog {
		b.WriteString(fmt.Sprintf("## %d. `%s`\n\n", i+1, h.Name))
		b.WriteString(fmt.Sprintf("- **What it verifies:** %s\n", h.DomainDescription))
		b.WriteString(fmt.Sprintf("- **Image:** `%s`\n", h.Image))
		b.WriteString(fmt.Sprintf("- **Smoke contract schema:** `%s`\n", h.SmokeContractSchema))
		if len(h.Exposes.TCP) > 0 {
			b.WriteString("- **TCP surfaces:** ")
			for j, p := range h.Exposes.TCP {
				if j > 0 {
					b.WriteString(", ")
				}
				b.WriteString(fmt.Sprintf("port %d (%s)", p.Port, p.Protocol))
			}
			b.WriteString("\n")
		}
		if len(h.RealDependencies) > 0 {
			b.WriteString("- **Real dependencies:** ")
			for j, d := range h.RealDependencies {
				if j > 0 {
					b.WriteString(", ")
				}
				b.WriteString(fmt.Sprintf("%s:%s %s", d.GroupID, d.ArtifactID, d.VersionRange))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("If NONE of the registered test harnesses fits the work this artifact describes, leave `test_harness` unset and add a `needs_test_harness: <description>` line to `open_gaps`. The reviewer (and downstream coordinator routing) treats absence-with-gap as honest scoping; absence-without-gap is a reviewer rejection.\n")
	return b.String()
}
