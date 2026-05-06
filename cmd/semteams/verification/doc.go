// Package verification implements the verification-check
// primitive — the architect's structured statement of WHAT is
// verified, AGAINST WHAT, with WHAT EVIDENCE, by WHOM (R3.7.2,
// ADR-033 §addendum 2026-05-04).
//
// # Why this package exists
//
// In dev-via-spec, "tests pass" is a Goodhart-vulnerable signal
// when the same actor authors both the implementation and the
// tests (R3.6.2.g surfaced this — chain converged to tests-passing
// against its own mocks, with no real-stack verification). The
// verification-check primitive defeats this by forcing the
// architect to commit, at spec-emit time, to:
//
//   - target:        a natural-language description of WHAT is verified
//   - runtime:       a closed-enum kind that determines lifecycle and
//     substance enforcement (unit / testcontainer /
//     sidecar / browser-flow / static-analysis)
//   - test_harness:  when runtime needs real-stack, names a
//     catalog entry the operator has curated
//   - test_runtime:  when runtime is testcontainer/sidecar/
//     browser-flow, names the language runtime
//     (java-junit-testcontainers, go-testing-net, etc.)
//   - ref:           file path (brownfield — cite an existing test
//     pattern in the repo) OR template id (greenfield
//     — point at a framework-shipped template)
//   - evidence:      []EvidenceRule the post-build evidence gate runs
//     mechanically against the workspace
//
// Each check is a typed payload registered with
// payloadregistry under verification.check.v1. The architect
// emits them on the dev_via_spec.artifact alongside the
// implementation spec. The reviewer judges *coverage adequacy* —
// whether the check SHAPE is enough for this work — while a
// separate mechanical evidence gate checks each rule against the
// workspace.
//
// # Why product-shell-local
//
// The verification-check shape is a SemTeams product-policy
// concern: which runtime kinds we recognise, which evidence-rule
// kinds we ship checkers for, how strict the architect persona's
// emission contract is — all are deployment policy, not framework
// universals. Lives in cmd/semteams/verification/ alongside the
// other product-shell payloads (research, devviaspec, testharness).
// Migration target: extract upstream when a 2nd product needs the
// primitive (likely later than R3.7).
//
// # Slice layering
//
// This file (R3.7.2.a) ships only the typed payload + closed
// enums + structural Validate. Catalog-bound validation (does the
// named test_harness exist? does the runtime support this family?)
// lands in R3.7.2.e alongside the evidence-rule registry.
// Wiring into dev_via_spec.artifact.v2 lands in R3.7.2.b. The
// architect persona's emission contract lands in R3.7.2.f. The
// evidence gate lands in R3.7.2.h.
package verification
