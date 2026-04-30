# Failure classes (probe taxonomy)

> Port lineage: SemSpec's `configs/error_categories.json` (failure-
> class taxonomy used for negative-memory injection). Light port:
> categories adapted to dev-via-spec planning failures rather than
> SemSpec's pipeline-specific failures (parse errors, schema
> violations). ADR-030 (semspec) pairs this taxonomy with the
> review pattern; we adopt the discipline here.

Walk every failure class. For each, ask "is this plan vulnerable
to this class of error?" — and ground the answer in evidence
from the plan and the upstream research artifact.

## 1. Decomposition too coarse

Symptoms:
- An epic covers two distinct integration boundaries.
- An epic's scope spans both read-side and write-side flows.
- An epic title ends with "Build" or "Implement" without a
  named interface or endpoint.

Probe: count the integration_points the epic touches. If > 1
distinct flow direction, the epic is a decomposition candidate.

## 2. Scope creep

Symptoms:
- scope.include item names a file or capability not grounded in
  the research artifact's actors or integration_points.
- scope.include item adds a redesign or refactor that the artifact
  does not motivate.
- scope.include item exceeds the seed_requirements' verb (artifact
  says "expose"; plan says "redesign").

Probe: cross-reference every scope.include item to the artifact.
Items without grounding are creep candidates.

## 3. Missing failure mode

Symptoms:
- Asynchronous integration points without a timeout / disconnect /
  retry consideration in any epic.
- External-system integration without an authentication or
  authorisation concern.
- Stateful flows without a recovery-after-crash concern.

Probe: for each artifact integration point with `direction: read`
from an external actor, check whether any epic owns the failure
case. For each `direction: write`, check whether any epic owns
the back-pressure or rejection case.

## 4. Unaccounted integration

Symptoms:
- Artifact enumerates an integration_points entry; no epic's scope
  includes it; no scope.exclude entry rationalises excluding it.

Probe: walk the artifact's `integration_points`. Each one is
either in some epic's scope, or explicitly excluded with a
rationale, or a concern.

## 5. Goal/scope incoherence

Symptoms:
- Goal sentence implies a capability the scope does not include.
- Scope includes work the goal does not motivate.
- Context cites an actor that no epic touches.

Probe: re-read the goal/context/scope as a coherent narrative. If
the goal would be unfulfilled even after every epic ships,
incoherence is a concern.

## 6. Revision-respect (only on retry)

Symptoms:
- Plan claims to address a prior finding but the revision adds no
  new scope or splits no epic.
- Plan rebuts a prior finding without adding disambiguation.
- Plan addresses some prior findings but silently drops others.

Probe: if this is a retry, cross-reference your own prior
`concerns_raised` reason and the reviewer's prior `insufficient`
reasons. Each prior concern is either addressed (with visible
revision) or open (still a concern).

## What this is NOT

- Not a code review. The plan has no implementation yet.
- Not an SOP audit. SemSpec's plan-reviewer does that; we don't
  port the SOP layer for R3.3.
- Not a security review. Production hardening is downstream of
  the planner's role.
- Not a re-run of the reviewer's completeness checklist. The
  reviewer already approved; you are looking past completeness
  to falsifiable concerns.

If your concerns reduce to "the plan is incomplete," accept and
let the planner ship — completeness was the reviewer's gate, not
yours. Your job is the residual after completeness.
