# Agent-contract extraction design

## Why

[Issue #284](https://github.com/C360Studio/semteams/issues/284) owns the next design after
[foundation PR #283](https://github.com/C360Studio/semteams/pull/283). SemTeams must move agent-runtime ownership
without giving frozen upstream processors separately defined lookalike Go payloads or copying engine-owned mechanics.
A candidate package family is not yet an approved extraction cut.

## What changes

- Measure root `agentic`, `vocabulary/agentic` and `internal/looptoken` at frozen SemStreams `8b99efe9`, including
  production/test closure, private dependencies, reverse consumers, concrete types and shared sentinel authority.
- Record the smallest supportable cut, transition alternatives, source/test/licensing provenance and proving checks.
- Map engine dependencies and generic recovery/durability to the admitted or planned SemEngine contracts; preserve
  unresolved integration requirements without inventing APIs.
- Define extraction admission requirements and obtain independent architecture approval of the design.

[Draft PR #286](https://github.com/C360Studio/semteams/pull/286) owns the design claim.
It is stacked on #283; both PRs remain separate review units. No release milestone membership is implied.

## Non-goals

No runtime copying, import rewrites, dependency replacements, SemEngine cutover, generic execution journal, SemDev
activation, parked-pack revival, UI changes, paid runs or retained-state conversion. Research and autoresearch remain
the only live product-facing packs. The artifact describes an extraction design, not shipped migration behavior.

## Impact

The deliverable is measured evidence, a reviewed architecture decision, conservative OpenSpec requirements and
reproducible analysis tooling if needed. Existing runtime, configs, schemas and dependency pins remain unchanged.
Follow-up implementation scope and holds belong in GitHub issues. Issue closure still requires `CONFIRM-CLOSE`.
