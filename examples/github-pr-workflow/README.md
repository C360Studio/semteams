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

## Cost Safety Controls

Built-in guardrails prevent runaway token spend:

| Control | Default | What it does |
|---------|---------|--------------|
| Token budget per execution | 500k tokens | Caps total tokens (in+out) across all agents in one workflow run. Escalates to human when exceeded. |
| Issue cooldown | 60s | Minimum time between processing new issues. Prevents burst processing. |
| Max review cycles | 3 | Caps developer/reviewer back-and-forth. Escalates to human after 3 rejections. |
| Workflow timeout | 30 min | Hard wall-clock limit per execution. |

When the token budget is exceeded mid-workflow, the execution escalates to a human rather than continuing to burn tokens. The completion payload includes `total_tokens_in` and `total_tokens_out` so you can monitor actual spend.

Constants are in `workflow.go` — adjust `DefaultTokenBudget`, `DefaultIssueCooldown`, etc. to taste.

## Testing with a Toy Repo

To try this safely:

1. Create a throwaway GitHub repo (e.g. `yourname/workflow-test`)
2. Set `repo_allowlist` in the config to `["yourname/workflow-test"]` to restrict which repos trigger workflows
3. Configure the webhook on that repo only
4. Open a simple issue like "Add a hello world endpoint" and watch the agents work
5. Monitor token usage in the workflow completion events

## Adversarial Tension

The quality funnel works through opposing pressures:

- **Qualifier vs Issues**: Incentivized to reject. Vague, duplicate, or invalid issues are filtered out.
- **Reviewer vs Developer**: Incentivized to find problems. Forces the Developer to produce correct, well-tested code.
- **Escalation**: After 3 review cycles without convergence, or when the token budget is exceeded, the workflow escalates to a human with a full history summary.

## Graph Entities

Issues, PRs, and reviews become graph entities, accumulating knowledge over time:

| Entity | ID Pattern | Example |
|--------|------------|---------|
| Issue | `{org}.github.repo.{repo}.issue.{n}` | `c360.github.repo.semstreams.issue.42` |
| PR | `{org}.github.repo.{repo}.pr.{n}` | `c360.github.repo.semstreams.pr.87` |
| Review | `{org}.github.repo.{repo}.review.{id}` | `c360.github.repo.semstreams.review.rv-001` |

Relationship triples connect them: `pr.fixes -> issue`, `review.targets -> pr`, `pr.modifies -> file`.
