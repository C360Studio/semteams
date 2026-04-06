# Proposal: Agentic Superpowers via Rules and Flows

**Status**: Draft
**Date**: 2026-04-06
**Author**: Coby Leuschke

## Problem

OpenClaw (247K GitHub stars) is the leading open-source agentic coordination framework in 2026 -- a self-hosted Node.js daemon that coordinates multi-agent teams across 12+ messaging platforms. Both semdragon and semspec ignored the SemStreams rules engine and built their own orchestration. This pattern repeats across the ecosystem: teams assume they need a dedicated agentic framework.

SemStreams already has the primitives to replace OpenClaw entirely. What it lacks is packaging: stock flows, stock rules, and the ultimate differentiator -- agents that can program the rules engine themselves when they hit a gap.

## Proposal

Ship stock agentic flows and rule sets that prove dedicated coordination frameworks are unnecessary, then enable agents to create new rules and flows on demand via two new tool executors and a small KV integration completion.

## Capability Mapping

Every OpenClaw primitive maps to an existing SemStreams equivalent:

| OpenClaw Primitive | SemStreams Equivalent | Status |
|---|---|---|
| Central Gateway (Node.js daemon) | `agentic-dispatch` -- normalizes `UserMessage` from any channel, routes to `agent.task.*` | Exists |
| Agent Routing (`(channel, user, guild) -> agentId`) | Rules with conditions on `channel_type`/`user_id`/`content` -> `publish_agent` action | Exists (needs stock rules) |
| Coordinator + Specialist | Rules watch `agent.complete.>`, match role/outcome, spawn next agent | Exists (`architect-editor.json`) |
| Inter-Agent Communication | Knowledge graph (shared triples), KV Twofer (write = event), `parent_loop_id` hierarchy | Exists |
| Persistent Memory (MEMORY.md + search) | `agentic-memory` -- graph-backed fact extraction, context hydration, LLM-assisted extraction | Exists |
| Skills/Tools (5,400+ marketplace) | `ToolExecutor` + `RegisterTool()`. Currently: GitHub (3), Graph queries (4) | Gap: 7 tools |
| Multi-Channel (12+ platforms) | CLI, WebSocket, A2A, GitHub webhook, HTTP gateway. All normalize to `UserMessage` | Gap: adapters |
| Always-On Daemon | SemStreams IS always-on. KV Watch = event-driven. Rules = continuous evaluation | Exists |
| Session Management (SQLite + vector) | `LoopTracker` per (user, channel), context compaction, graph-backed retrieval | Exists |
| Time-Triggered Tasks (cron) | Rules fire on state change only | Gap: no timer |

**Scorecard**: 7/10 already exist or are better. 3 narrow gaps.

## Where SemStreams Is Fundamentally Better

### Graph-Native Memory vs Flat Files

OpenClaw agents append to `MEMORY.md` and grep for recall. SemStreams agents write semantic triples to a knowledge graph with relationships, inference tiers (BM25 + neural), and community detection. "What decisions were made about authentication?" is a graph traversal, not a text search.

### Declarative Coordination vs Coordinator Agents

OpenClaw requires a coordinator agent (LLM call) to route every task. SemStreams rules fire deterministically on state -- no LLM call to decide "editor or reviewer?" Rules cost zero tokens per routing decision.

### KV Twofer vs Dual-Write Pub/Sub

OpenClaw uses WebSocket messages plus SQLite -- a dual-write problem. In SemStreams, the write IS the event. Single KV write gives state (`.Get`), events (`.Watch`), and history (replay from any revision).

### Cross-Agent Knowledge Without Messaging

In OpenClaw, agents share info through shared file workspaces. In SemStreams, all agents share the knowledge graph. Agent A writes `(service.auth, has_vulnerability, CVE-2026-1234)`. Agent B's rules see this via KV Watch and automatically block deploys. No coordinator. No message relay. The graph mediates.

### Governance by Default

SemStreams has PII detection, prompt injection prevention, content moderation, and rate limiting built into `agentic-governance`. Every agent message passes through the pipeline. OpenClaw relies on community plugins.

### Full Observability

Every agent action produces trajectory entries (token accounting), knowledge graph triples (queryable facts), rule state (RULE_STATE KV), and KV history (replayable). "Why did the deployment agent block this release?" is a graph query.

## The Ultimate Differentiator: Self-Programming Agents

OpenClaw agents consume pre-built skills from a marketplace. SemStreams agents can build new rules and flows on demand when they hit a gap. The system doesn't just coordinate agents -- it lets agents reprogram the coordination layer itself.

### Why This Works

Rules are JSON. Flows are JSON. An LLM can write JSON. The infrastructure for runtime hot-reload already exists:

| Piece | Status | Location |
|-------|--------|----------|
| Rule JSON schema validation | Exists | `processor/rule/config_validation.go` |
| Runtime rule hot-reload | Exists | `processor/rule/runtime_config.go` -- `ApplyConfigUpdate()` |
| KV-based config watching | Exists | `config/manager.go` -- `OnChange("rules.*")` |
| KV config integration | 85% done | `processor/rule/kv_config_integration.go` -- stub at `saveViaConfigManager()` |
| Flow CRUD API | Exists | semstreams-ui -- `POST /flowbuilder/flows`, deploy/start/stop |
| AI flow generation | Exists | semstreams-ui -- `POST /api/ai/generate-flow` (Claude + MCP) |
| Component discovery | Exists | `GET /components/types` returns schemas |
| Flow validation | Exists | `POST /flowbuilder/flows/{id}/validate` |
| Deploy to runtime | Exists | `POST /flowbuilder/deployment/{id}/deploy` writes to NATS KV |

### The Self-Programming Loop

For rules:

```
Agent encounters gap
  -> "I need to monitor for X but no rule exists"
  -> Agent calls create_rule tool with rule JSON
  -> Rule validated via ValidateConfigUpdate()
  -> Rule written to semstreams_config KV bucket
  -> ConfigManager detects change, broadcasts update
  -> Rule processor picks up new rule via ApplyConfigUpdate()
  -> Agent continues -- new capability is live immediately
```

For flows:

```
Agent needs a new pipeline
  -> "I need a research flow to investigate this"
  -> Agent calls manage_flow tool with natural language description
  -> Tool hits AI generation endpoint (same API the UI uses)
  -> Claude generates flow JSON using component catalog
  -> Flow validated, deployed, started
  -> New pipeline is live
```

### The Safety Model

```
Agent proposes rule
  -> Schema validation (operators, types, structure)     <- catches malformed rules
  -> Governance review (content filters, rate limits)     <- catches dangerous rules
  -> Sandbox evaluation (dry-run against test entity)     <- catches logic errors
  -> Human approval (for high-risk patterns)              <- catches everything else
  -> Hot-reload to production                             <- instant activation
```

Schema validation and governance already exist. Sandbox evaluation and approval gates are new but small.

### What's Missing

1. **Complete the KV integration stub** -- `saveViaConfigManager()` in `processor/rule/kv_config_integration.go` returns `ErrInvalidConfig` instead of writing to KV. The method signature and surrounding infrastructure exist.

2. **Wire ConfigManager to Processor** -- Rule processor needs to subscribe to config updates in its `Start()` method. `ApplyConfigUpdate()` works; it just isn't triggered by KV changes yet.

3. **Two new tool executors**:
   - `create_rule` -- Validates rule JSON, writes to config KV, returns validation result
   - `manage_flow` -- Wraps flow builder REST API (create, update, validate, deploy, start, stop)

4. **Governance gate** -- Agent-generated rules pass through governance before activation. Auto-approve low-risk patterns, require human approval for high-risk.

## Stock Flows

Four pre-built flow configurations composing existing components. No new code required.

### Deep Research Flow

**Pattern**: Question -> classifier -> researcher (iterative search + graph queries) -> synthesizer -> answer

**Components**: `agentic-dispatch` -> `agentic-loop` -> `agentic-model` -> `agentic-tools` -> `agentic-memory` -> `graph-ingest` -> `rule-processor`

**Rule set** (`configs/rules/research/`):

| Rule | Trigger | Action |
|------|---------|--------|
| `classify-question` | New task with role=general | Regex-based topic routing to specialist |
| `gather-evidence` | Researcher completes with insufficient evidence | Re-run with broader query |
| `synthesize-answer` | Evidence gathering completes | Spawn synthesizer role |
| `escalate-timeout` | Loop stalls | Notify user with partial results |

Research facts persist as graph triples. Next query on the same topic starts with accumulated knowledge.

### Code Development Flow (Issue-to-PR)

**Already exists**: `configs/github-pr-workflow.json` + `configs/rules/github-pr-workflow/` (6 rules: review-approved, review-rejected-retry, escalate-deadlock, needs-info, issue-rejected, budget-exceeded).

**Stock version**: Generalize with configurable roles chain (qualifier -> architect -> editor -> reviewer) and parameterized GitHub repo targeting.

### Incident Response Flow

**Pattern**: Alert -> triage -> investigation (graph queries for related entities) -> remediation plan -> human approval gate -> execution

**Rule set** (`configs/rules/incident/`):

| Rule | Trigger | Action |
|------|---------|--------|
| `triage-severity` | Alert arrives | Classify severity, route to specialist |
| `investigate-root-cause` | Triage completes | Spawn investigator with graph context |
| `propose-remediation` | Investigation completes | Spawn planner |
| `await-approval` | Remediation proposed | Transition operator -> `awaiting_approval`, notify human |
| `execute-remediation` | Approval signal | Spawn executor with sandboxed tools |
| `escalate-timeout` | No approval within threshold | Escalate |

Investigation agent queries the knowledge graph for related incidents, service dependencies, and recent changes.

### Conversational Assistant Flow

**Pattern**: Multi-channel input -> intent routing via rules -> specialist agents -> graph-backed memory -> response

**Rule set** (`configs/rules/assistant/`):

| Rule | Trigger | Action |
|------|---------|--------|
| `route-by-intent` | Message content matches pattern | Route to specialist agent |
| `memory-consolidation` | Loop completes with 3+ iterations | Trigger fact extraction |
| `escalate-complex` | Specialist fails | Escalate to general with full context |

Conversation history persists as graph triples across sessions.

## Stock Rule Sets

Reusable rule libraries organized by concern:

### Agent Routing (`configs/rules/routing/`)

- `route-by-channel` -- channel_type + content regex -> specialist `publish_agent`
- `route-by-intent` -- content pattern matching -> role-specific agent
- `route-by-user` -- user_id matching -> personalized agent config
- `fallback-to-general` -- no specialist matched -> general agent

### Coordination (`configs/rules/coordination/`)

- `architect-to-editor` -- (exists) role=architect + success -> spawn editor
- `editor-to-reviewer` -- role=editor + success -> spawn reviewer
- `reviewer-approved` -- role=reviewer + approved -> mark complete
- `reviewer-rejected-retry` -- role=reviewer + rejected + retries < max -> re-spawn editor
- `parallel-fan-out` -- task with subtasks -> spawn multiple agents
- `parallel-fan-in` -- all subtask agents complete -> spawn synthesizer

### Escalation (`configs/rules/escalation/`)

- `escalate-on-failure` -- specialist failed -> spawn general with broader context
- `escalate-on-timeout` -- loop timeout -> notify + optionally re-assign
- `escalate-on-deadlock` -- (exists) rejection count >= 3 -> human escalation
- `escalate-on-budget` -- (exists) token budget exceeded -> halt + notify

### Memory (`configs/rules/memory/`)

- `extract-on-completion` -- loop complete + iterations >= 3 -> trigger fact extraction
- `extract-decisions` -- detect decision language patterns -> create decision triples
- `track-tool-usage` -- tool_calls present -> record tool invocation patterns

### Approval (`configs/rules/approval/`)

- `await-human-approval` -- transition to awaiting_approval -> notify user
- `auto-approve-low-risk` -- risk_level=low -> auto-approve
- `require-dual-approval` -- risk_level=critical -> require 2 approvals

## Gap Analysis

### P0: Must Close for Parity

| Gap | Description | Effort |
|-----|-------------|--------|
| Tool breadth | Need `web_search`, `read_url`, `file_read`, `file_write`, `code_execute`, `database_query` | Medium (each is a `ToolExecutor` impl) |
| Cron-ticker component | Input component publishing clock-tick entities at intervals | Small |

### P1: Needed for Self-Programming and Superiority

| Gap | Description | Effort |
|-----|-------------|--------|
| Complete KV config stub | Implement `saveViaConfigManager()` | Small |
| Wire ConfigManager -> Processor | Subscribe to config updates in `Start()` | Small |
| `create_rule` tool executor | Validate + write rule JSON to config KV | Small |
| `manage_flow` tool executor | Wrap flow builder REST API | Small |
| Phase 3 aggregation operator | Cross-entity conditions (count, threshold, majority) | Medium (already planned) |
| `elapsed_since` operator | Time-since-last-update condition | Small |

### P2: Nice-to-Have

| Gap | Description | Effort |
|-----|-------------|--------|
| Slack/Discord adapters | Thin input components normalizing to `UserMessage` | Small per adapter |
| Streaming responses | End-to-end SSE from model to user channel | Medium |
| LLM intent classifier action | `classify_intent` action calling model, writing result to KV | Medium |

### What We Don't Need

- **Coordinator agent** -- rules replace it
- **Session database** -- KV + graph replaces SQLite
- **Custom memory format** -- graph triples > markdown files
- **Plugin framework** -- `ToolExecutor` interface is sufficient
- **Skills marketplace** -- flow configs ARE skills

## Phased Delivery

### Phase 1: Stock Configs (no code)

- 4 stock flow JSON configs (research, code-dev, incident, assistant)
- ~20 stock rule JSON files across 5 categories
- Documentation: "SemStreams Agentic Cookbook"

### Phase 2: Self-Programming Foundation (small code)

- Complete `saveViaConfigManager()` stub (`processor/rule/kv_config_integration.go`)
- Wire ConfigManager updates -> `Processor.ApplyConfigUpdate()` in rule processor `Start()`
- `create_rule` tool executor (validate + write to config KV)
- `manage_flow` tool executor (wraps flow builder REST API)
- Governance gate for agent-generated rules

### Phase 3: Remaining Gaps (medium code)

- Cron-ticker input component
- 5-6 new tool executors (web_search, read_url, file_read, file_write, code_execute, database_query)
- `elapsed_since` rule operator
- Aggregation operator (KV Twofer Phase 3)

### Phase 4: Demos

- Self-programming: agent encounters monitoring gap, writes rule, gap fills live
- AI flow generation: agent describes pipeline in English, flow deploys
- Cross-agent knowledge: two agents share findings through graph without messaging
- Graph memory: agent recalls decisions from weeks ago via graph traversal

## The Pitch

> OpenClaw is a chatbot framework that learned to coordinate.
> SemStreams is a knowledge graph engine where agents build their own superpowers.

1. **Agents share a brain, not a chatroom.** Knowledge graph with inference, not markdown files with grep.
2. **Workflows are JSON, not code.** Declarative rules hot-reload via KV Watch. No redeploy.
3. **Every action is a queryable fact.** Trajectories, decisions, tool calls -- all in the graph.
4. **Agents program themselves.** When an agent hits a gap, it writes a new rule or spins up a new flow. OpenClaw agents consume skills. SemStreams agents create them.

## Key Files

| File | Role |
|------|------|
| `configs/agentic.json` | Reference agentic flow config |
| `configs/github-pr-workflow.json` | Production PR workflow flow |
| `configs/rules/agentic-workflow/architect-editor.json` | Coordinator -> specialist rule |
| `configs/rules/github-pr-workflow/*.json` | 6 workflow rules |
| `processor/rule/actions.go` | 8 action types |
| `processor/rule/expression/evaluator.go` | 22 operators |
| `processor/rule/runtime_config.go` | `ApplyConfigUpdate()` hot-reload |
| `processor/rule/kv_config_integration.go` | KV rule CRUD (stub at `saveViaConfigManager`) |
| `processor/rule/config_validation.go` | Schema validation |
| `processor/rule/kv_writer.go` | `update_kv` action |
| `processor/agentic-loop/loop_manager.go` | 10-state machine |
| `processor/agentic-tools/global.go` | Tool registry |
| `config/manager.go` | Config KV watching |
| `service/component_manager.go` | `WatchConfig` hot-reload |
| semstreams-ui `src/lib/services/flowApi.ts` | Flow CRUD |
| semstreams-ui `src/lib/services/aiApi.ts` | AI flow generation |
| semstreams-ui `src/routes/api/ai/generate-flow/+server.ts` | Claude + MCP endpoint |
