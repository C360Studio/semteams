# Agent-runtime foundation qualification

Issue [#282](https://github.com/C360Studio/semteams/issues/282), draft PR
[#283](https://github.com/C360Studio/semteams/pull/283), qualified 2026-10-03.
The parent is SemTeams `d5ee63252e987f75885b110b3ba02844ab04dedd` (PR #281), with
SemStreams frozen at `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`.

## Result and scope

The product-owned catalog preserves the frozen framework's 27 factories and seven
payload owner families. Runtime bootstrap, schema/OpenAPI generation and live
contract tests use that catalog. Product payload overlays remain in bootstrap.
Factory metadata, concrete implementations, port declarations and payload contracts
are checked against the frozen aggregate registrars, which remain test oracles only.
Registration failures identify the owner and abort bootstrap; callers discard partial
registries. Service and vocabulary registration are unchanged.

`go.mod`, `go.sum`, configs, fixtures, UI source, generated schemas and OpenAPI are
identical to the parent. This qualifies a registration boundary, not an extracted
runtime, reduced dependency closure, SemEngine cutover or development-pack activation.
The target ownership and observable-work commitments are in
[ADR-061](../../adr/061-agent-runtime-ownership-and-observable-work.md).

## Checks

Local Go is `go1.26.4 darwin/arm64`; Node is `v26.0.0`, with locked UI dependencies.
These required commands passed against the final runtime source:

- `task lint` (the same six pre-existing warnings; zero errors)
- `task test:race`
- `task test:integration` (Docker-backed, race enabled, sequential packages)
- `go build ./...`
- `task schema:generate` and `task schema:check-changes`
- `task test:browser-runner`
- `task openspec:validate` and `task openspec:queue-test` (26 reporter fixtures)
- In `ui`: `OPENAPI_SPEC_PATH=../specs/openapi.v3.yaml npm run generate-types:check`

The first type-generation invocation omitted `OPENAPI_SPEC_PATH` and failed with
`Can't parse empty schema`. The corrected invocation passed without changing generated
source. Both logs are retained; this was an invocation error, not a rerun of a flaky
product test. After archive/spec synchronization, strict validation passed all four specifications and
the queue-reporter checks passed all 26 fixtures.
Hosted CI is reported on the PR for its actual commit, not asserted by an OpenSpec task.

Focused regression evidence is retained in [regression-evidence.txt](regression-evidence.txt):
removing `agentic-loop` fails exact component parity; removing `graph/inference` fails
payload parity and populated hierarchy decoding. Both mutations were restored before
full checks and image construction. The Python source guard first failed to detect an
`internal/runtimecatalog` edit, then passed after adding `internal` to its snapshot;
that regression now runs in the Go CI lane.

Eight valid framework payload samples cover all seven owners. Five product payloads
have actual BaseMessage decoding checks. The sixth product registration,
`semsource.status.v1`, remains constructible as `*semsource.StatusPayload`, but lacks
`MarshalJSON`/`UnmarshalJSON` and therefore the `message.Payload` interface. Its inherited
decoding gap is **not** qualified here. [Issue #285](https://github.com/C360Studio/semteams/issues/285)
tracks repair versus retirement under the approved service boundary; SemSource dogfood
holds remain in force.

## Browser qualification

Six representative mock-only scenarios passed seven tests, with zero skips, retries,
unexpected failures or flaky results:

| Scenario | Tests | Boundary exercised |
|---|---:|---|
| `autoresearch` | 1 | Active empirical workflow and payload/tool composition |
| `coordinator-routing-matrix` | 1 | Product dispatch/routing |
| `admin-flows-inventory` | 2 | Advertised component/flow inventory |
| `approval-boundary-approve` | 1 | Human approval and continuation |
| `approval-boundary-cancel` | 1 | Human cancellation boundary |
| `research-fanout` | 1 | Research planning, fan-out and convergence |

The backend was rebuilt from this worktree with Go 1.26.4 using the production Docker
stage. Unchanged mock, sandbox and UI images from the frozen baseline were reused;
fixture and UI source mounts point at this worktree. NATS is `2.14.4-alpine`. No paid
model or search credentials were supplied. Project `stmigfoundation283`, local port
33283, fresh volumes per scenario and project-scoped cleanup isolate the run. Final
inspection found zero project containers, volumes or networks.

The runner actively sampled test progress, backend logs, readiness and message-logger
state at 25-second intervals. The source hash before and after every scenario and the
whole matrix is identical:
`a4f4668222c4b8dfa1de3b1236b7f9b97d1a3b417ec851cc0c6da460c85e0043`.
The backend build's shared source hashes match that browser snapshot. The image identity,
per-scenario config/fixture/runner hashes, actual Playwright counts and gate exit codes
are in [qualification.json](qualification.json); the full observed file map is in
[source-manifest.json](source-manifest.json). Raw local logs and final-state snapshots
remain at `/tmp/semteams-agent-runtime-foundation/browser`.

This targeted run complements the historical full frozen-baseline qualification in
[PR #281's evidence](../semstreams-8b99efe/README.md); those earlier runs are not claimed
as branch runs. Fresh-state mock runs do not prove retained-state migration, paid-model
behavior, future pack contracts or external-effect durability.

## Reproduction

From a clean checkout of this PR, install the locked UI dependencies with `npm ci` in
`ui`, run the checks above, and build the production backend:

```sh
docker build --target production --build-arg GO_VERSION=1.26.4 \
  --build-arg VERSION=foundation-283 --build-arg COMMIT_SHA=worktree-283 \
  -f docker/Dockerfile -t stmigfoundation283-backend:foundation .
```

The recorded build used a temporary slim context containing `go.mod`, `go.sum`, `cmd`,
`internal`, `configs`, `.devcontainer` and `docker`; exact inputs and the command are
recorded in the evidence. Reuse or build the mock/sandbox/UI images from the frozen
baseline's Dockerfiles. Copy [compose.json](compose.json) outside the checkout and
replace its historical worktree bind-source prefix with the current checkout. Create
a writable isolated tenant directory and substitute its path in both the sandbox
mount and backend environment. Keep paid credentials blank. If the recorded project
or port is occupied, use a new `stmig`-prefixed project and change every explicit
volume/network name and port consistently; never take over another run's resources.

```sh
python3 scripts/qualify-migration-browser.py --source "$PWD" \
  --compose /tmp/foundation-compose.json --project stmigfoundation283 \
  --output /tmp/foundation-browser --port 33283 \
  --names coordinator-routing-matrix,research-fanout,autoresearch,approval-boundary-approve,approval-boundary-cancel,admin-flows-inventory
```

## Review and next step

Independent architecture review passed before implementation; final architecture/docs
review passed. Independent Go review passed with no unresolved findings. It covers the explicit catalog, runtime/generator
consumers, payload behavior, failure propagation, Python source guard and CI wiring.
The reviewed source manifest hash is `5c23e7a5d094f81229025ffa809d3c04c77c6735727b198b7be8f32da55f3791`
across 20 implementation/test/CI files. No UI implementation or generated UI contract changed, so a frontend implementation
review was not required. Independent final review of the archive/spec synchronization and consolidated evidence
also passed with no findings before the final content commit.

[Issue #284](https://github.com/C360Studio/semteams/issues/284) owns the next design-only
slice: prove the agentic contract family's production/test dependency closure and plan
one authoritative concrete payload type family before copying code. Generic engine
mechanics and unresolved agent rule-action extension APIs remain upstream dependencies.
