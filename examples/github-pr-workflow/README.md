# GitHub Issue-to-PR Workflow

An adversarial automation workflow that watches a GitHub issue queue, qualifies issues, develops fixes, and reviews PRs before human involvement.

## Architecture

```
GitHub Webhook ──> github-webhook (input)
                        |
                        v
                   NATS JetStream (GITHUB stream)
                        |
         +--------------+--------------+
         v              v              v
   Qualifier       Developer       Reviewer
   (agentic-loop)  (agentic-loop)  (agentic-loop)
         |              |              |
         +------+-------+------+------+
                v              v
          Reactive Workflow    GitHub Tools
          (coordination)       (executors)
                |
                v
          Knowledge Graph
```

## Three Adversarial Agents

**Qualifier** (`qualifier` role) — Skeptical triage. Challenges every issue for reproducibility, clarity, and duplicates. Only qualified issues proceed to development.

**Developer** (`developer` role) — Disciplined implementer. Explores the codebase, plans minimal changes, writes tests, pushes a PR. Must satisfy the Reviewer.

**Reviewer** (`reviewer` role) — Critical code reviewer. Checks correctness, edge cases, test adequacy, and pattern compliance. Can reject PRs back to Developer.

## Phase Flow

```
issue_received -> qualifying -> qualified -> developing -> dev_complete -> reviewing -> approved -> complete
                      |                          ^                            |
                      +-> rejected/needs_info    +---- changes_requested -----+
                                                 (max 3 review cycles, then escalate to human)
```

## Prerequisites

- NATS server with JetStream enabled
- `GITHUB_TOKEN` environment variable (personal access token with `repo` scope)
- `GITHUB_WEBHOOK_SECRET` environment variable (webhook HMAC secret)
- An LLM endpoint (OpenAI-compatible)

## Setup

1. Configure a GitHub webhook pointing to `http://your-host:8090/github/webhook`
2. Set the webhook secret and content type to `application/json`
3. Subscribe to events: Issues, Pull requests, Pull request reviews, Issue comments
4. Set environment variables:

```bash
export GITHUB_TOKEN="ghp_..."
export GITHUB_WEBHOOK_SECRET="your-webhook-secret"
export OPENAI_API_KEY="sk-..."  # or configure model_registry in the config
```

5. Run with the flow config:

```bash
go run ./cmd/semstreams -config configs/github-pr-workflow.json
```

## NATS Topology

| Stream | Subjects | Purpose |
|--------|----------|---------|
| GITHUB | `github.event.>`, `github.action.>`, `github.workflow.>` | Webhook events and workflow lifecycle |
| AGENT | `agent.>`, `tool.>` | Agent task dispatch, model calls, tool execution |

| KV Bucket | Purpose |
|-----------|---------|
| `GITHUB_ISSUE_PR_STATE` | Workflow execution state |
| `AGENT_LOOPS` | Agent loop state |
| `AGENT_TRAJECTORIES` | Agent execution trajectories |
| `ENTITY_STATES` | Knowledge graph entities |

## Adversarial Tension

The quality funnel works through opposing pressures:

- **Qualifier vs Issues**: Incentivized to reject. Vague, duplicate, or invalid issues are filtered out.
- **Reviewer vs Developer**: Incentivized to find problems. Forces the Developer to produce correct, well-tested code.
- **Escalation**: After 3 review cycles without convergence, the workflow escalates to a human with a full history summary.

## Graph Entities

Issues, PRs, and reviews become graph entities, accumulating knowledge over time:

| Entity | ID Pattern | Example |
|--------|------------|---------|
| Issue | `{org}.github.repo.{repo}.issue.{n}` | `c360.github.repo.semstreams.issue.42` |
| PR | `{org}.github.repo.{repo}.pr.{n}` | `c360.github.repo.semstreams.pr.87` |
| Review | `{org}.github.repo.{repo}.review.{id}` | `c360.github.repo.semstreams.review.rv-001` |

Relationship triples connect them: `pr.fixes -> issue`, `review.targets -> pr`, `pr.modifies -> file`.
