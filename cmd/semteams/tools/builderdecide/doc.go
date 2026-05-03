// Package builderdecide implements the SemTeams-local builder_decide tool.
// This tool is the dev-via-spec-builder role's terminal action: the builder
// iterates bash calls in its sandbox workspace until it can call
// builder_decide(action="tests_passing"|"tests_failing"|"needs_clarification",
// ...) with the action-specific evidence fields.
//
// Why a separate tool from upstream `decide` (the framework-alignment review):
//
//  1. Upstream `decide` (semstreams beta.39) takes only {action, reason,
//     subtopics, retry_hint} in its parameter schema. The builder terminal
//     contract per ADR-032 §15 requires evidence fields (`tests_run`,
//     `tests_passed`, `tests_failed`, `artifact_summary`, `failure_summary`,
//     `blocking_question`) that are not in upstream's schema.
//
//  2. The framework's ExecutorRegistry has no Unregister/Replace surface, so
//     a wrapping replacement of the canonical `decide` after
//     executors.RegisterBuiltins is not possible without an upstream change.
//
//  3. Per ADR-032 §15 the validator "lives in product-shell decide-handler,
//     not LLM persona." A new tool is the simplest framework-aligned shape.
//     The ADR explicitly authorises this: "Shape: a thin product-shell tool
//     wrapper or a `decide` pre-validator."
//
//  4. Mirrors the canonical agent-terminal-tool emission shape of upstream
//     `decide` (and product-shell siblings `emit_research_artifact`,
//     `emit_dev_via_spec_artifact`): emits coordinator.next_action +
//     coordinator.decision_reason triples on the loop entity for downstream
//     rule routing, returns StopLoop=true with the full args in Content for
//     read_loop_result consumers.
//
// Migration target: when upstream ships a per-role terminal-tool primitive
// or a `decide`-with-extension hook (currently absent in beta.39), revisit.
// Until then, this lives in product code with the migration posture
// recorded in cmd/semteams/tools/README.md and ADR-032's R3.6.2.b addendum.
//
// Action set (closed enum) and required evidence per ADR-032 §15:
//
//   - tests_passing       — tests_run (>0), tests_passed, tests_failed,
//     artifact_summary
//   - tests_failing       — tests_run, tests_failed, failure_summary,
//     retry_hint
//   - needs_clarification — blocking_question (and the always-required
//     reason field doubles as the clarification context;
//     R3.5 routing rule lands additively)
//
// Triples emitted on the calling loop entity:
//
//   - coordinator.next_action          → action          (parity with decide)
//   - coordinator.decision_reason      → reason          (parity with decide)
//   - dev_via_spec.builder.tests_run   → int             (passing|failing)
//   - dev_via_spec.builder.tests_passed   → int          (passing only)
//   - dev_via_spec.builder.tests_failed   → int          (passing|failing)
//   - dev_via_spec.builder.artifact_summary    → string  (passing only)
//   - dev_via_spec.builder.failure_summary     → string  (failing only)
//   - dev_via_spec.builder.retry_hint          → string  (failing only)
//   - dev_via_spec.builder.blocking_question   → string  (needs_clarification only)
//
// Predicates are kept as string constants here (not promoted to a vocabulary
// package) until a second consumer needs them — same convention as
// emitspecartifact's marker predicates.
package builderdecide
