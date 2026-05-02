# Failure classes (probe taxonomy)

> Port lineage: SemSpec's `configs/error_categories.json`. Light
> port — categories adapted to dev-via-spec planning failures
> rather than SemSpec's pipeline-specific failures. ADR-031
> §addendum 2026-05-02 captures the substance-over-format pivot:
> these are *probe questions*, not rigid format checks.

For each class, ask: *given the reviewer's summary of the approved
plan, would this class of failure block successful execution?*
Polish concerns and minor improvements do **not** count — the bar
is execution-blocking. The reviewer already gated on completeness;
you add the falsification pass, focused on what would actually
break.

## 1. Decomposition too coarse

Would an implementer have to make architectural decisions inside
a single epic? If the plan groups two distinct integration
boundaries (e.g. read-side packet ingestion AND write-side
observation publishing) into one epic, splitting would produce a
better implementation roadmap. Coarse decomposition is a concern
when execution would benefit from clearer boundaries.

Probe: would two engineers working on the same epic in parallel
hit conflicts? If yes, the epic needs splitting.

## 2. Scope creep

Does the plan include work the upstream research artifact does
not motivate? Look for scope items the reviewer's summary cites
that don't trace back to an artifact actor or integration_point.
Creep is a concern when it expands the work beyond what the user
asked for or what the substrate supports.

Probe: would a PM ask "wait, where did *that* come from?" when
reading the plan? If yes, scope creep is a concern.

## 3. Missing failure mode

Does the plan handle the failure cases the integration boundaries
imply? For asynchronous read-side actors (radios, sensors,
network feeds): is disconnect / packet-loss / timeout owned by
some epic? For external write-side actors (APIs, databases,
queues): is back-pressure / rejection / authentication owned?

Probe: if the integration partner went down for an hour, would
the plan-as-described handle it correctly, or would the system
silently corrupt state?

## 4. Unaccounted integration

Does every integration_point the research artifact enumerates
appear somewhere in the plan — either delivered by an epic or
explicitly excluded with rationale? If the artifact named X but
the plan ignored X, that's a concern (either the plan needs to
cover it or the plan needs to say why not).

Probe: enumerate the artifact's integration_points (visible in
the reviewer's summary). For each, find the corresponding scope
or exclusion. Anything left over is the concern.

## 5. Goal/scope incoherence

Does the goal sentence describe a capability the scope actually
delivers? Mismatches: goal claims X but scope only delivers
prerequisites; goal is narrow but scope keeps adding context that
doesn't serve it; context names actors no epic touches. Incoherence
is a concern when implementers would build the wrong thing.

Probe: re-read the goal as if you were the implementer. Would
shipping every epic produce something that satisfies the goal? If
not, name where the gap is.

## 6. Revision-respect (only on retry)

Did the planner address each prior finding visibly, or just
re-assert the prior position? Pure rebuttal without scope change
is a concern. Silent dropping of prior findings is a concern.

Probe: cross-reference your own prior `concerns_raised` reason
and the reviewer's prior `insufficient` reasons. Each prior
concern has visible resolution OR explicit disambiguation OR is
unresolved (still a concern).

## What this is NOT

- Not a code review. The plan has no implementation yet.
- Not an SOP audit. SemSpec's plan-reviewer does that; we don't
  port the SOP layer.
- Not a security review. Production hardening is downstream.
- Not a re-run of the reviewer's completeness checklist. The
  reviewer already approved on substance; you're looking past
  completeness to falsifiable execution risks.
- Not a polish pass. Wording, naming, or formatting suggestions
  are not concerns.

If your concerns reduce to "the plan could be sharper" or "the
plan is incomplete," **accept**. The chain has retry budget
elsewhere; spend yours on actual execution risk.
