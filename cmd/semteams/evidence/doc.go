// Package evidence implements the post-build evidence gate the
// dev-via-spec dvs-reviewer (R3.7.2.j′) consults to grade an
// architect's verification commitments. Each architect commitment
// (verification.Commitment, R3.7.2.a) carries an Evidence []EvidenceRule
// slice; this package owns the executor that dispatches each rule's
// Kind to a concrete Checker against a populated Context
// (workspace, runner output paths) and reports per-rule + aggregate
// results.
//
// # Why a registry, not a closed switch
//
// New evidence-rule kinds will land per language ecosystem (Maven,
// Go, npm, pytest, .NET). Operators may also wire deployment-specific
// kinds (e.g. a fixture-file checker that asserts a project-curated
// JSON fixture is well-formed). A registry keeps the dispatch shape
// uniform — Register at boot, Run at gate time — and lets each
// language's checkers live in its own file or sub-package without a
// central enum the dispatcher walks.
//
// # Closed-fail on unknown kinds
//
// Per the comment in cmd/semteams/verification/commitment.go on
// EvidenceRule, unknown kinds are fail-closed at gate time, NOT at
// validate time. This package's Run dispatcher returns
// ResultUnknownKind for any rule whose Kind is not registered; the
// reviewer treats that as a rejection just as a failed assertion
// would be. The validate-time pattern stays structural-only (kind
// matches ^[a-z][a-z0-9_]*$).
//
// # Why not a tool the LLM calls
//
// The evidence gate is post-build infrastructure, not LLM-facing.
// The dvs-reviewer LLM reads the *aggregated result* (a summary of
// pass/fail/unknown counts plus per-rule detail), but does not
// invoke checkers itself. Coby's "fewer rich tools" principle
// (memory 2026-05-03) and ADR-034's "structural checks beat LLM
// judgment" memory both point the same way: when a constraint can
// be enforced structurally, do it structurally.
//
// # Migration target
//
// Stays product-local until a 2nd product (semspec, semdragons)
// needs the same shape. ADR-034 §"What's deferred" notes that
// upstream is unlikely to ship this primitive — the patterns are
// per-language and per-toolchain, not framework-shaped.
//
// # First-shipped checker kinds (R3.7.2.i′)
//
//   - test_file_exists: workspace-relative path exists. Universal.
//   - test_uses_build_tag: a Go *_test.go file declares a specific
//     `//go:build <tag>` constraint. The narrowest Goodhart-resistant
//     check for an integration-tagged Go test.
//   - surefire_passing_count: Maven surefire reports under
//     target/surefire-reports/ show ≥N passing tests, optionally
//     filtered by Java class. The smoke #7 (OSH-Meshtastic, Java +
//     Maven + Testcontainers) load-bearing checker.
//
// Adding a new kind: write a new Checker, register from init or a
// shared bootstrap, add a unit test against testdata fixtures.
package evidence
