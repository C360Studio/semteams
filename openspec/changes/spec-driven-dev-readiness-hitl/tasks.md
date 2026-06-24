# Tasks

## 1. Spec Artifact And Review
- [x] 1.1 Pin the `create_change` proposal, delta, task, and graph-only execution-field contracts.
- [x] 1.2 Add ingest and render round-trip coverage for a repo-root `openspec/` tree.
- [x] 1.3 Add UI spec review with edit, approve, reject, and request-revision actions.
- [x] 1.4 Add export for the OpenSpec change folder and rendered single-document projection.
- [x] 1.5 Add slash-command shortcuts that invoke the same governed actions as visible UI controls.

## 2. Proof Readiness
- [x] 2.1 Define the claim, proof dependency, harness profile, readiness record, evidence, and waiver fact model.
- [x] 2.2 Implement a deterministic proof-readiness analyzer that emits `formal_claims.*` findings onto the run entity.
- [x] 2.3 Route missing proof dependencies to test-harness and passed readiness to implementation.
- [x] 2.4 Add UI cards for proof dependencies, readiness records, evidence freshness, and waivers.

## 3. Dev From Task
- [x] 3.1 Project approved `change.<slug>.task.*` facts into `plan.task.*` plus the chain-level acceptance command.
- [x] 3.2 Dispatch one ready task at a time through the existing Ralph execution loop.
- [x] 3.3 Preserve CBG as the final acceptance gate and surface rejected gates to the coordinator and UI.
- [x] 3.4 Enforce the definition-of-done authority stack in coordinator, Ralph, CBG, and projection contracts.

## 4. Run Health UI
- [x] 4.1 Build run health from graph facts, lifecycle, active loops, approvals, proof findings, and evidence freshness.
- [x] 4.2 Display run health as working, waiting, blocked, failing, or complete with the current gate and next action.
- [x] 4.3 Keep raw trajectories, logs, and graph triples as drill-down evidence behind the summary.
- [x] 4.4 Add Prometheus metric freshness, component pressure, queue depth, latency, and error-rate evidence to run health.

## 5. Hard-Scenario Vertical
- [x] 5.1 Use a deterministic MAVLink-shaped fixture to validate proof-first routing before implementation.
- [x] 5.2 Produce or reject a reusable harness profile before feature code is released.
- [x] 5.3 Demonstrate that missing PX4/MAVSDK readiness blocks implementation unless a human waiver is recorded.

## 6. Real LLM And E2E Validation
- [ ] 6.1 Add a Gemini-backed real-LLM smoke path for model-dependent routing and prompt behavior.
- [x] 6.2 Add Playwright e2e coverage for spec review, export, run health, and approval/wait states.
- [x] 6.3 Capture model ID, provider, journey evidence, and artifact output in the e2e report.
- [ ] 6.4 Add e2e coverage that autoresearch refuses vague/non-scalar goals and preserves metric guardrails.
