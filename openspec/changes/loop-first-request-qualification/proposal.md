# Qualify the loop's first-request boundary

## Why

[Issue #294](https://github.com/C360Studio/semteams/issues/294) closes the current consumer-evidence gap between
rule-family TaskMessage emission (#293) and model handling of a manually authored AgentRequest (#289). This change
starts at SemTeams `a331910ee6ec297f86530c4a1d69ca84001a19f5`, with frozen SemStreams
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`.

## What changes

- Exercise the shipped graph-ingest and teams-loop through product registration, public managers and real local NATS.
- Verify both components started, actual typed loop birth, and the first canonical AgentRequest with correct identity
  and independently expected tool-choice/response-format values.
- Reuse the live versus supplemental fixture distinctions and prove omitted knobs on a separate fresh root do not
  inherit another root's values. Do not infer same-loop cache reset semantics.
- Preserve platform, configuration, streams, subjects and budgets; document and prove reversibility of only the input
  External markers required for fixture-owned publishers.
- Retain bounded readiness/cleanup, meaningful failure/restoration evidence and independent architecture/Go review.

## Non-goals

No runtime/config/dependency change, private helper copy, graph-policy emulation, provider call, tool enforcement,
continuation, recovery, run inheritance, durable-effect, retained-state or future SemEngine qualification. If real
startup needs more components or policy emulation, revisit the bounded plan. SemEngine #77 and held E3 work remain
separate. Merge and issue closure require separate authorization; CONFIRM-CLOSE remains required.

## Impact

One product-wired integration qualification, a compact source/evidence record and an OpenSpec qualification contract.
The running product keeps its frozen substrate and current live packs.
