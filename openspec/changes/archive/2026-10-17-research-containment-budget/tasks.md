## 1. Claim and OpenSpec Proposal

- [x] 1.1 Create dedicated worktree on `gemini/research-containment-272` and author the `research-containment-budget` OpenSpec proposal, design, and spec delta.
- [x] 1.2 Open draft PR claiming `#272` (`Closes #272`).

## 2. Planner Fan-Out Containment (`1..4` Subtopics)

- [x] 2.1 Define canonical `MinSubtopics = 1` and `MaxSubtopics = 4` constants in `cmd/semteams/research`, enforce `1..4` epics in `devviaspec.Plan.Validate()`, and advertise `minItems: 1, maxItems: 4` on `emit_plan`'s `epics` schema.
- [x] 2.2 Implement `cmd/semteams/tools/researchdecide` and wire it via `SkipBuiltins: ["bash", "decide"]` so `decide` advertises `minItems: 1, maxItems: 4` on `subtopics` and rejects missing/empty/over-limit subtopics pre-mutation (`ToolErrorInvalidArgs`, `StopLoop=false`, zero triples written).
- [x] 2.3 Add `coordinator.decision.subtopics` 1..4 element JSON string-array regex defense-in-depth condition to `configs/rules/research/02-plan-to-gather.json` and align `configs/personas/fragments/researcher-research-plan/` documentation.
- [x] 2.4 Add unit and contract tests covering `N=1`, `N=4`, `N=5` (and `N=15`), missing/empty/malformed subtopics, `emit_plan` bypass attempts, same-loop correction, and unaffected non-gather `decide` calls.

## 3. Cron Cooldown, Inflight Guard, Kill Switch & Recovery Qualification

- [x] 3.1 Add contract and NATS integration tests qualifying cron `cooldown` + scheduler `inflight` guard and the Pattern-B `rule.ConfigManager` `enabled: false` kill switch (`enable → fire → disable → no further fire` without restart, plus running-work non-cancellation).
- [x] 3.2 Add behavioral verification demonstrating that rule `05-reviewer-rejected-retry`'s `max_iterations: 3` is scoped per reviewer entity across fresh loop identities, documenting upstream `semstreams#383`, `semstreams#1005`, and `semengine#24`.

## 4. Verification, Review, and Archive

- [x] 4.1 Run all local verification gates (`task lint`, `task test:race`, `task test:integration`, `go build ./...`, `task schema:generate`, `task schema:check-changes`, `task openspec:validate`, `task openspec:queue-test`, `task publish:verify-test`).
- [x] 4.2 Run `go-reviewer` pass and resolve any findings.
- [x] 4.3 Archive `research-containment-budget` into `openspec/specs/research-containment/spec.md` and validate clean OpenSpec queue.



