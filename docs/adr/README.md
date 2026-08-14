# Architecture Decision Records

This index is for maintainers working on architecture or product-shell changes.
The records are listed roughly in build order; the bold ones are the
load-bearing reads for understanding the current architecture.

| ADR | What it decides |
|---|---|
| [023](023-provider-adapters-and-tool-choice.md) | LLM provider adapters and tool-choice handling |
| [**029**](029-product-shell-wiring.md) | How `cmd/semteams/main.go` wires framework primitives |
| [030](030-approval-flow-ui-and-identity.md) | Approval-flow UI + the `X-User-Id` identity seam |
| [031](031-research-flow-and-semspec-handoff.md) | Research-flow ownership + dev-via-spec internal mode. Largely superseded by 042; dev-via-spec arc retired in MVP-7. |
| [032](032-r36-sandbox-design.md) | R3.6 builder sandbox design, precursor to 043 |
| [033](033-harness-anchored-verification-and-coordinator-authority.md) | Harness-anchored verification + coordinator-as-decision-authority |
| [034](034-qa-runner-pattern-adoption.md) | QA-runner pattern, verification-runner pivot |
| [035](035-dev-via-spec-arc.md) | Dev-via-spec arc. Superseded by 042 and retained for archeology. |
| [036](036-test-harness-lifecycle.md) | Test-harness lifecycle |
| [037](037-chain-failure-handling.md) | Chain failure handling + chain-pause semantics |
| [038](038-chain-entity-and-milestone-rendering.md) | Chain entity + milestone rendering |
| [039](039-needs-clarification-recovery.md) | `needs_clarification` recovery routing |
| [040](040-source-curator-role.md) | Source-curator role split |
| [041](041-mvp-role-compression-and-graph-as-substrate.md) | MVP role compression + graph-as-substrate, direct precursor to 042 |
| [**042**](042-coordinator-instantiated-flows-via-templates.md) | Substrate-plus-overlays: one product-shell flow, category-keyed rule packs + persona bundles |
| [**043**](043-devcontainer-as-sandbox-spec.md) | Devcontainer-as-sandbox spec: per-tenant attestation + attestation-aware artifact routing |
| [**044**](044-dev-via-test-pack.md) | Dev-via-test pack: plan-fidelity gate + chain-end integration gate, both bounded reject/retry/approve |
| [053](053-adoption-plan.md) | Adoption plan for proof/readiness work |
| [054](054-test-harness-team-proof-environments-before-code.md) | Test-harness team for proof environments before code |
| [055](055-formal-claim-analysis-for-verification-gates.md) | Formal claim analysis for verification gates |
| [**056**](056-openspec-spec-driven-development-umbrella.md) | OpenSpec-compatible, environment-gated spec-driven development umbrella |
| [**057**](057-openspec-graph-spec-model-and-create-change.md) | Graph-backed OpenSpec model and `create_change` pack |

## Conventions

Architecture decision records follow the standard format: Status, Context,
Decision, Consequences, Alternatives, and Related. Addenda land in place with a
dated heading when they refine scope without overturning the decision.
