// Package bootstrapworkspace implements the SemTeams-local
// bootstrap_workspace tool. The builder calls this tool
// exactly once at iteration 1 to create its sandbox worktree and seed
// the rendered spec markdown as SPEC.md in the workspace root.
//
// # Why this is a product-shell tool, not a rule action
//
// ADR-032 §R3.6.2.d originally proposed doing this work via an
// `http_request` rule action that would POST /worktree + PUT /file
// before publish_agent fires. semstreams beta.39 does not ship an
// http_request rule action, and there is a chicken-and-egg in
// publish_agent's design: publish_agent generates the spawned loop's
// task_id internally (`fmt.Sprintf("rule-%s-%d", entityID,
// time.Now().UnixNano())`) at action-execution time, so the rule
// cannot pre-create a worktree at the new loop's task_id without
// either:
//
//   - upstream adding `http_request` AND a substitution variable
//     for the not-yet-published task_id, or
//   - the dispatcher exposing a per-role pre-loop hook the product
//     shell could install.
//
// Neither shipped in beta.39. Adding both upstream is heavy
// framework work that blocks every product wanting rule-driven HTTP
// side effects. We chose the cheaper structural fit: a product-shell
// tool the builder persona calls as its first iteration, reading
// the spec path from the rule-substituted prompt (which carries
// $entity.triple.dev_via_spec.artifact.path on the architect's
// loop entity).
//
// # Why bash cannot subsume this
//
// Per Coby's "fewer rich tools" principle (feedback memory
// 2026-05-03), net-new product-shell tools must clear a
// "bash-genuinely-insufficient" bar. Bash runs *inside* the sandbox
// workspace — it can't:
//
//   - Reach the host filesystem to read the rendered spec markdown
//     at SEMTEAMS_DEVVIASPEC_ARTIFACT_DIR/<slug>.md
//   - POST /worktree to create the workspace itself (sandbox.Exec
//     requires an existing worktree per the integration_test.go
//     contract)
//   - PUT /file from outside the sandbox network without the
//     SANDBOX_URL credential and HTTP client
//
// All three operations need host-side execution before bash inside
// the sandbox is even reachable. bootstrap_workspace closes that
// gap; bash takes over from iteration 2 onward.
//
// # Migration target
//
// Stays product-local until ONE of these lands upstream:
//
//   - `http_request` rule action + a `${rule.spawned_task}`
//     substitution that resolves to publish_agent's generated
//     task_id within the same on_enter sequence.
//   - A dispatcher pre-loop hook the product shell can install for
//     the builder role.
//
// When either lands, this package can be deleted: the rule does
// the seed inline and the builder's first iteration becomes its
// real first iteration. ADR-032 addendum captures the decision.
//
// # Shape
//
// Single tool, single arg:
//
//	bootstrap_workspace(spec_path: "docs/specs/<slug>.md")
//
// The arg comes from the rule's prompt template, which substitutes
// $entity.triple.dev_via_spec.artifact.path on the architect's loop
// entity. Path traversal is rejected; spec_path must resolve under
// SEMTEAMS_DEVVIASPEC_ARTIFACT_DIR (or the package default
// "docs/specs"). Worktree create is idempotent on the sandbox side
// (returns "exists" status) so a retry is safe.
package bootstrapworkspace
