# ADR-064: Rule-pack coordination and source observation

**Status:** Accepted direction (2026-10-26) for [issue #335](https://github.com/C360Studio/semteams/issues/335) and [draft PR #358](https://github.com/C360Studio/semteams/pull/358). This records design admission for later qualified slices. It does not add a watcher, manager inbox, or runtime guarantee.

**Refines:** [ADR-042](042-coordinator-instantiated-flows-via-templates.md)'s category packs, [ADR-060](060-program-manager-direction-and-research-design.md)'s bounded program-manager loop, and [ADR-061](061-agent-runtime-ownership-and-observable-work.md)'s runtime/substrate ownership and observable work. Their live/parked and read-only MVP boundaries remain.

## Context

SemTeams currently composes a frozen SemStreams runtime. Category rule packs coordinate its live `research` and `autoresearch` journeys; durable Project Managers and inter-manager requests are target capabilities. The product needs to observe repository work and eventually coordinate bounded investigations without requiring every agent to read every event or exchange freeform messages. GitHub/Gitea work records are the human-visible authority for repository work, while runtime events and execution facts have different owners.

Many deployments have no publicly reachable inbound endpoint. A webhook-only design would make those deployments depend on a tunnel or relay. A service-side outbound observation path is therefore part of the target contract. A webhook can reduce detection delay where explicitly configured, but its notification must be reconciled against source state before policy acts. Polling may miss intermediate changes or deleted records; it is not full webhook-history equivalence.

The current product-owned `github_read` reads configured GitHub repositories and tracks per-repository, per-kind watermarks and origin-version digests ([cursor.go:26-48](../../cmd/semteams/tools/githubread/cursor.go), [executor.go:386-461](../../cmd/semteams/tools/githubread/executor.go)). Its run-scoped cursor stages a delta and replays the same pre-run baseline on retries; it promotes the cursor after a downstream research artifact is persisted ([cursor.go:50-66](../../cmd/semteams/tools/githubread/cursor.go), [emitartifact/executor.go:409-419](../../cmd/semteams/tools/emitartifact/executor.go)). That is useful source evidence for a later polling proof, not proof of a general watcher or exactly-once task execution.

## Decision

### Coordinate through bounded pack policy

Category rule packs remain the default policy for deciding when an observation starts or advances a bounded task. The current flow declares NATS transport and JetStream ports for rule processing, agent loops and dispatch ([flow-bootstrap.json:11-69](../../configs/flow-bootstrap.json), [flow-bootstrap.json:271-310](../../configs/flow-bootstrap.json), [flow-bootstrap.json:411-521](../../configs/flow-bootstrap.json), [flow-bootstrap.json:629-674](../../configs/flow-bootstrap.json)). It is the existing internal transport; this ADR does not promise new delivery semantics for it. No general-purpose agent conversation bus is selected. A pack may later request direct peer communication only with an explicit need, typed intent, scope, authority, and stop condition; shared evidence alone does not wake every agent.

For repository work, GitHub or Gitea issues, comments, PRs, reviews, and commits remain durable discussion and artifact records. Source-specific adapters may also observe timers, documents, or incoming messages for teams with no forge. Adapters validate and normalize source observations; pack policy determines permitted follow-up. A forge account is not required for every team. An observed item is evidence, not an instruction or authorization to act.

SemTeams owns agent-specific request shape, loop/dispatch, context, model/tool orchestration, approval/governance, trajectory, and agent rule extensions. SemEngine owns generic transport, graph, mutation, rule evaluation, lifecycle/projection, and durability. Before any new payload, action, KV bucket, or stream, the implementation must complete the framework-alignment review in `cmd/semteams/tools/README.md`. This ADR selects no new queue, graph schema, NATS subject, API, or claim primitive. [SemEngine #24](https://github.com/C360Studio/semengine/issues/24) remains the owner of generic attempt/effect/replay contracts; its pending work does not prevent bounded SemTeams design and qualification against the frozen runtime.

### Observe sources without inbound reachability

A service-side source adapter performs outbound polling when inbound webhooks are unavailable. It shares observations across agent and UI consumers; individual models and browsers do not each poll the same repository. A timer tick is a reason to inspect source state, not a reason to invoke a model. Where deployed reachability and policy permit, an explicitly enabled webhook is a notification accelerator that feeds the same validation and reconciliation path. Neither path independently proves downstream completion.

The later adapter design must define selected source kinds and coverage, stable object/revision identity, incremental queries and pagination, checkpoint timing, overlap deduplication, bounded retries and rate-limit handling, freshness/last-success state, and unavailable or partial state. Failed reads cannot be interpreted as no changes. Source-specific gaps in history and deletion detection must be declared. Gitea support requires its own provider qualification; this ADR does not infer it from GitHub support.

SemSource is recommended when code or documentation indexing is needed. Its supported HTTP/MCP interfaces may enrich evidence; a forge-observation journey must still work without it. A repository clone or code index does not by itself contain issue and review discussion. Local agent execution does not depend on a vendor coding harness; hosted versus local model inference is a separate choice. This does not reopen subscription-backed CLI execution.

### Admit inter-manager work only with scope and authority

Future Program-to-Project-Manager work must name explicit sender and recipient program/project coordinates, originating run, typed intent, evidence references, and a bounded response/outcome contract. Scope is validated against operator-authored portfolio configuration; an unknown or ambiently inferred project fails closed. Admission needs run-wide work, retry, time, and expenditure limits as well as per-loop limits. [#301](https://github.com/C360Studio/semteams/issues/301) owns cost accounting/policy and [#298](https://github.com/C360Studio/semteams/issues/298) reactive policy; neither an observed threshold nor a per-loop ceiling alone caps concurrent in-flight spending. Model routing remains capability-based under #301, without silent quality downgrades.

A candidate request lifecycle is `pending → claimed → completed | cancelled`, with one successful claimant, bounded pending depth, explicit cancellation rules, and inspectable outcomes. These are admission properties, not a selected graph inbox or compare-and-set implementation. The implementation must map single-claim and recovery behavior to qualified substrate contracts and state the failure window it can actually prove. An accepted message, graph revision, or cursor advance does not prove an external effect occurred exactly once. Human approvals remain revision-bound governance, not text conventions in a chat message.

Cross-project request bodies and external issue/PR/comment prose are untrusted input. A manager first receives a bounded metadata notice with verified coordinates and evidence references. Body retrieval is an explicit second step; it places escaped source text inside an untrusted-input fence and keeps authenticated provenance outside that fence. Sanitization before persistence uses typed markers such as `[REDACTED:github_token]` or `[REDACTED:bearer_token]`, without retaining the secret in trajectories or messages. The later implementation must qualify these controls and refusal paths before multi-manager activation.

### Bound the first implementation proof

The first candidate after this decision is one configured GitHub repository and one existing issue. A baseline read establishes source progress. A fixture then changes that issue's title or body and `updated_at`; the next read discovers and stages one logical observation with stable issue/revision identity and explicit project scope. The downstream observation/result must be persisted through a selected, proven boundary before the source cursor commits. Only a later unchanged poll may suppress that revision. A retry before commit can replay the same identity under the current `github_read` contract; it does not mean execution occurred once. The proof must refuse an out-of-scope repository or project, prevent a failed or incomplete fetch from advancing the successful cursor or being reported as empty, and show that prompt-like issue prose stays data and synthetic secret text is redacted before any persisted result. Fixture-backed HTTP can establish this without a model call, delegated task response, SemSource, or inbound access. The implementation claim must assess `github_read` reuse and qualify its own storage/commit boundary before promising restart behavior. It must not label this proof Gitea support, durable task execution, or exactly-once outside effects.

## Consequences

- Rule packs govern when source observations become agent work; provenance, scope, authority, and bounded outcomes make those decisions inspectable.
- Outbound-only deployments have a planned observation path. Webhooks and non-forge adapters can be added without changing pack policy, after each source's coverage and failure behavior is qualified.
- The first proof is intentionally narrow. Subsequent claims can separately qualify pagination, outages, webhook overlap, other forge objects, non-forge events, inter-manager lifecycle, and cost admission.
- ADR-061/062/063 remain the runtime extraction and pack-absorption boundaries. This design does not activate parked packs, grant write authority, or replace GitHub/OpenSpec as repository artifact authorities.

## Alternatives considered

- **Use GitHub/Gitea webhooks only.** This excludes deployments without an inbound route; outbound polling is required as a supported target mode.
- **Use forge comments as the entire runtime bus.** Human-visible records are authoritative for repository work, but internal execution events and non-forge sources need typed runtime handling.
- **Add a freeform peer chat service or generic inbox now.** The need is bounded, policy-driven handoff. A chat bus or speculative durable queue would duplicate or bypass current ownership and leave authority ambiguous.
- **Require SemSource for every observation.** Indexing enriches code/document evidence, while issue and review state still belongs to a forge adapter.

## Related

- [Program-manager product direction](../product/program-manager.md) and [roadmap](../ROADMAP.md)
- [Issue #335](https://github.com/C360Studio/semteams/issues/335) and [OpenSpec design admission](../../openspec/changes/event-driven-coordination-design/design.md)
- [GitHub webhook reachability](https://docs.github.com/en/webhooks/testing-and-troubleshooting-webhooks/troubleshooting-webhooks) and [REST polling guidance](https://docs.github.com/en/rest/using-the-rest-api/best-practices-for-using-the-rest-api)
