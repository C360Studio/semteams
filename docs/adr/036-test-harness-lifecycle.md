# ADR-036: Test-Harness Lifecycle and Verification Machinery

## Status

**Proposed (2026-05-06).** Pulls test-harness machinery out of
ADR-033 (which keeps the coordinator-as-decision-authority decision)
and adds the lifecycle primitive smoke #7 surfaced as missing.

## Why this exists

Smoke #7 run-3 (2026-05-05) reached `dev-via-spec-qa-reviewer` for
the first time on real LLM. Wedge-report green. Chain converged in
8m 18s. Zero SAP coercions. **The result was theatre.** The builder
produced:

- `MeshPacket.java` — its own comment says *"Hand-crafted stub for
  Meshtastic MeshPacket. In production this would be generated
  [from protobuf]"*
- `IDataProviderModule.java` — *"Stub interface representing the
  [OSH SPI contract]"*
- `MeshtasticTcpClient.java` — wires the two stubs together
- `MeshtasticDataProviderInitTest.java` — tests against the
  builder's own stubs
- `mvn test` → 12/12 surefire passing

The architect's check named `runtime: process-local-testcontainer`
and `test_harness: meshtasticd-3.x`. The catalog has the meshtasticd
manifest. **No rule, tool, or sandbox primitive starts the test_harness
before the builder runs tests.** The chain has every piece of the
orchestration meta-game (personas, allowlists, rule cascades, evidence
gate) but no engine.

ADR-033 §1–§2 named test-harnesses as a curated platform asset and
called smoke contracts "runtime-executed" — but the runtime that
executes them was deferred. This ADR ships the runtime.

## Decision

**The test_harness lifecycle is owned by the test framework, not by
SemTeams.** The chain emits a manifest; standard test-framework
tooling (Testcontainers, testcontainers-go, pytest-docker, etc.)
manages start/stop within the test process's lifecycle.

Specifically:

### D1. The architect emits a manifest reference per check

When a check declares `runtime: process-local-testcontainer`, it
also names a `test_harness: <catalog-id>`. The catalog entry
(`cmd/semteams/testharness/`, formerly `harness/`) carries the
docker image, exposed ports, env, healthcheck, and bootstrap
metadata. The catalog is platform-curated and audited; architects
reference entries by ID, do not invent.

### D2. `bootstrap_workspace` seeds the manifest into the worktree

When the builder boots its workspace, `bootstrap_workspace` reads
the per-check manifest references from the architect's emitted
spec sidecar (`<slug>.checks.json` per the post-rename name; was
`<slug>.commitments.json`) and writes a workspace-local manifest
file:

```
<workspace>/.test-harness/manifest.json
```

Format:

```json
{
  "version": "v1",
  "harnesses": [
    {
      "id": "meshtasticd-3.x",
      "image": "ghcr.io/meshtastic/meshtasticd:3.0",
      "ports": {"mqtt": 1883, "api": 4403},
      "env": {"MESHTASTICD_LISTEN": "0.0.0.0"},
      "healthcheck": {"port": "mqtt", "timeout_s": 30}
    }
  ]
}
```

### D3. The builder uses standard test-framework idiom; SemTeams ships no harness runner

Java/Maven: builder writes JUnit tests using Testcontainers's
`@Container GenericContainer<?>` against image/ports from
`.test-harness/manifest.json`. Testcontainers manages start, port
exposure, log capture, and teardown within the test process. Same
pattern for Go (`testcontainers-go`), Python (`pytest-docker`),
Node (`testcontainers-node`).

**SemTeams does not ship a `test_harness up` / `test_harness down`
CLI, a sandbox-level pre-test hook, or a cross-loop coordination
primitive.** The test framework already knows how to do this; we
lean on it. (`feedback_fewer_rich_tools` discipline.)

For runtimes the architect names that the test framework can't
manage (e.g. `runtime: external-sidecar`), the architect's check
must specify a non-framework lifecycle path — that's its job. The
default path is the test-framework path.

### D4. Evidence checks read what the test framework produced

`evidence.surefire_passing_count` reads `target/surefire-reports/`.
That directory contains XML from real Testcontainers-driven
integration tests if the builder did its job; the same XML if
the builder wrote stubs-against-stubs. The evidence gate doesn't
distinguish — it counts. **What distinguishes is whether the
architect's check is graded against integration points the
builder cannot fake.**

Per ADR-033 §addendum 2026-05-04 (anti-Goodhart guards): the
catalog manifest must specify a network endpoint (image port) the
builder cannot stub without re-implementing protocol semantics.
A Meshtastic packet capture in the manifest's `fixtures/`
directory is bytes the builder did not author — `surefire_passing_count`
on tests that parse those bytes against a running meshtasticd is
substance evidence, not ceremony.

Catalog hardening (manifest fixtures, healthcheck assertions,
non-stub-able protocol semantics) is the durable Goodhart guard.
The runtime decision (this ADR) makes those guards reachable.

## Vocabulary

ADR-036 standardises post-rename names. Historical references to
the old vocabulary are preserved in source ADRs and smoke
findings.

| Old | New | Reason |
|---|---|---|
| `verification_commitment` | `check` | "commitment" is a vow metaphor; data is just a check |
| `approach` (enum value) | `runtime` | "approach" implies methodology; value names where + how it runs |
| `convention` (filepath/template_id) | `ref` | "convention" implies cultural pattern; data is a pointer |
| `seed_requirements` | `tasks` | "seed" prefix is metaphor that doesn't pay rent |
| `harness` | `test_harness` | Disambiguates from agentic harness / Stanford Meta-Harness / upstream polars eval harness |
| `evidence`, `integration_point`, `artifact` | unchanged | Earn their rent |

## What this ADR explicitly does NOT decide

- **Multi-arc dependency management.** Sequential / gated / re-spawn
  posture stays in ADR-033 §5. ADR-036 is a single-arc concern
  (one builder loop, one harness lifecycle).
- **Coordinator-as-decision-authority.** Stays in ADR-033 §3. The
  coordinator decides *whether* to escalate; it does not start
  containers.
- **Verification-runner / browser-flow.** ADR-034 owns
  `act`-driven workflow YAML for greenfield browser-flow. This ADR
  is `runtime: process-local-testcontainer` only; other runtimes
  compose with it via the same manifest seam.
- **The Approach → Runtime enum rename.** Mechanical work; lands
  in the same Slice 2 PR as the rest of the rename.
- **Catalog content.** The meshtasticd-3.x entry's image, ports,
  fixtures live in `cmd/semteams/testharness/catalog/` and are
  iterated independently of this ADR.

## Phasing

### Phase 1 — Manifest seam (small)

- Extend `cmd/semteams/tools/emitspecartifact/` to write
  `<slug>.checks.json` (name change from `commitments.json`) plus
  resolve `test_harness` references through the catalog and embed
  the resolved manifest data.
- Extend `cmd/semteams/tools/bootstrapworkspace/` to read the
  sidecar, project the harness manifests into
  `.test-harness/manifest.json` in the worktree.

### Phase 2 — Java/Testcontainers wiring

- Builder persona fragment (`30-test-harness.md` or similar) names
  the `.test-harness/manifest.json` contract and shows a
  Testcontainers idiom for Java that reads it.
- Architect persona fragment for `runtime: process-local-testcontainer`
  + `test_harness:` reference is updated to expect Testcontainers
  on the builder side.

### Phase 3 — First substantive smoke

- Re-run smoke #7 with manifest plumbing live + Testcontainers idiom
  in the builder persona. Expected: builder writes a test that
  publishes via real Mosquitto (port from manifest), asserts driver
  emits OSH observation, mvn test runs against meshtasticd
  container started by Testcontainers.
- Evidence checker `surefire_passing_count` against tests that
  exercised real protocol semantics is the first substance
  evidence the chain has produced.

### Phase 4 — Non-Java runtimes

- Add `testcontainers-go`, `pytest-docker`, etc. patterns as
  builder persona fragments when each language target lands.
- Manifest format is language-agnostic; the persona-side idiom
  varies.

## Consequences

### Positive

- **Lifecycle owned by mature ecosystem tooling.** Testcontainers
  has years of production usage. We don't build a parallel runner.
- **Catalog hardening is the substance lever.** Once the manifest
  carries non-stub-able fixtures, the chain produces verifiable
  artifacts even when the builder is naïve.
- **Per-language idiom stays at the persona layer.** No SemTeams Go
  code needs to know Java vs Go vs Python — manifests are JSON.
- **Composes with ADR-034.** Browser-flow / `act`-driven verification
  uses its own runner; `runtime: process-local-testcontainer` uses
  this. Both share the catalog vocabulary.

### Negative

- **The test framework is now part of the chain's correctness
  surface.** A Testcontainers bug or Maven plugin issue surfaces
  as a builder failure. Acceptable — we'd inherit the same risk
  with a SemTeams-specific runner, plus the SemTeams-specific bugs.
- **Persona burden is real.** The builder has to know
  Testcontainers idiom for the language at hand. Mitigated by
  shipping a per-language persona fragment as part of each
  language's enablement.
- **Catalog manifests must include non-stub-able fixtures.** Without
  this, a builder can still stub-against-itself and pass.
  Catalog-curation discipline (ADR-033 §1) becomes load-bearing.

### Neutral

- **No new product-shell tools.** `bootstrap_workspace` extension
  is a behaviour change to an existing tool, not a new tool.
- **No new framework primitives.** Manifest seam is product-shell;
  the upstream framework is unchanged.
- **No new payload-registry types.** `dev_via_spec.artifact` widens
  to carry resolved harness manifest data; no new payload.

## Relationship to prior ADRs

- **ADR-029 (Product-Shell Wiring)**: this slice is product-shell —
  emit-extension, bootstrap-extension, persona content. No new
  framework wiring.
- **ADR-031 (Research Flow)**: orthogonal. Research stabilises an
  artifact; dev-via-spec consumes it. ADR-036 is downstream of both.
- **ADR-032 (Sandbox)**: sandbox provides the workspace + DinD/DooD
  capability. ADR-036 doesn't extend the sandbox primitive — the
  test framework drives docker via the existing socket bind mount.
- **ADR-033 (Coordinator + Multi-Arc)**: ADR-033 §1–§2 (test-harness
  as curated asset; smoke contracts runtime-executed) move HERE
  in the slim. §3, §5, §6 stay in ADR-033 (coordinator authority,
  multi-arc dependency).
- **ADR-034 (Verification-Runner)**: complementary. ADR-034 owns
  `act`-driven greenfield browser-flow; ADR-036 owns
  Testcontainers-driven `process-local-testcontainer`. Same
  catalog, different runtimes.
- **ADR-035 (Dev-via-Spec Arc, NEW)**: ADR-035 codifies the chain
  shape (planner → reviewer → challenger → architect → builder →
  qa-reviewer); ADR-036 is what makes the builder's tests
  meaningful.

## References

- `~/.claude/projects/-Users-coby-Code-c360-semteams/memory/project_strategic_pivot_2026_05_06.md`
  — strategic pivot capturing why this ADR exists.
- `/tmp/smoke7-run3/findings.md` — the smoke that exposed the
  missing primitive.
- `/tmp/smoke7-run3/trajectory-a8b59f7b-637b-44f2-86fe-597f791d0b28.json`
  — builder trajectory showing stubs-against-stubs construction.
- Testcontainers Java: <https://java.testcontainers.org/>
- testcontainers-go: <https://golang.testcontainers.org/>
- ADR-033 §1, §2, §addendum 2026-05-04 #2 — test-harness catalog
  decisions ADR-036 inherits.
