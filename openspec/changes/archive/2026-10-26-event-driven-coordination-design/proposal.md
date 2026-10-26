# Design event-driven coordination for delegated managers

## Why

[Issue #335](https://github.com/C360Studio/semteams/issues/335), claimed by [PR #358](https://github.com/C360Studio/semteams/pull/358), asks how a Program Manager can delegate bounded work to durable Project Managers without turning inter-agent communication into freeform chat. Repository activity also needs to trigger work when the SemTeams deployment cannot expose a public webhook endpoint. The design must distinguish GitHub or Gitea work records from internal runtime events and preserve the SemTeams agent-runtime ownership boundary in [ADR-061](../../../../docs/adr/061-agent-runtime-ownership-and-observable-work.md).

## What changes

- Record the target architecture in ADR-064: GitHub or Gitea remains the authoritative repository work surface; an outbound poller can discover changes where inbound webhooks are unavailable. Normalize observed changes into typed, provenance-backed events for rule-pack triggers. Webhooks are an optional adapter where deployment permits them, with the same downstream contract.
- Define the design boundary for inter-manager requests: explicit program and project coordinates, typed intent and evidence references, bounded fan-out and pending work, one claimant per request, and visible terminal outcomes. SemTeams owns the agent-specific contract and governance; SemEngine owns shared transport, graph, rule, and generic durability. The design must state which claim/replay guarantees need later substrate qualification; it does not choose a graph inbox or queue.
- Plan one later polling proof using one configured GitHub repository and one existing issue. After baseline, a fixture changes that issue's title or body and `updated_at`; the next read discovers and stages one scoped logical observation. A downstream observation/result is persisted through a proven boundary before the cursor commits; only then does an unchanged repeat produce none. Pre-commit retries may replay the same source identity. The proof must refuse out-of-scope input, preserve the successful cursor on incomplete or failed fetch, and treat prompt-like prose and synthetic secrets as untrusted data. It does not require Gitea implementation, a model call, a task response, or a durable execution guarantee. SemSource remains a recommended enrichment source when indexing is needed, not a prerequisite for the first proof.
- Add a design-admission contract to OpenSpec that requires explicit scope validation, metadata-first notices, fenced retrieval of untrusted request or repository text, provenance outside the fence, and typed secret-redaction labels before persistence. The implementation slice must qualify these behaviors and their failure paths before multi-manager activation.
- Align the product roadmap with this planned step, keeping shipped behavior and target capabilities distinct.

## Non-goals

- No runtime, rule-pack, JSON, API, or UI implementation in this change; no activation of parked development packs. The first later proof does not implement Gitea or delegated-manager execution.
- No new freeform agent chat bus, Mattermost dependency, second execution ledger, or workflow engine.
- No requirement for a publicly reachable webhook receiver, SemSource indexing, or a frontier harness in the first polling proof.
- No claim that SemEngine durability or a delegated Project Manager is already shipped. This design does not close #335; the implementation and qualification remain future work.

## Impact

This docs-only change adds ADR-064, a proposed OpenSpec design contract and task plan, and a product-roadmap link. It does not change current SemTeams behavior or grant agents new write authority.
