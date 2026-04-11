# Journey Fixtures

Deterministic mock-llm scripts for the journey Playwright specs under
`ui/e2e/agentic/`. Each fixture file pins the LLM responses the agent
will see during a journey run, so the same sequence of tool calls and
loop state transitions happens every time.

## Format

Each fixture is a YAML file named identically to its journey spec slug.
For example: `tool-approval-gate.yaml` pairs with
`ui/e2e/agentic/tool-approval-gate.spec.ts`.

Fixtures are loaded by:

- The existing mock-llm container used in `task e2e:agentic` (Go observer tests)
- The Playwright journey specs under `ui/e2e/agentic/*.spec.ts` (browser tests)

Both layers read the same fixture so backend and browser observers agree
on what the agent "will do" during the run.

## Contents

See `docs/journeys/README.md` for the full journey index and the
Playwright-first rationale.

Initial fixture set (planned, land with Phase C.2 / C.3):

- `tool-approval-gate.yaml` — tracer for the human-in-the-loop approval flow
- `real-time-activity-stream.yaml` — tracer for SSE `loop_update` rendering
