# Reading the curator artifact (when present)

You may be spawned in one of three shapes. The shape is signalled
by the `recovery` field in your task properties:

- **No `recovery` field** — first-pass research. You're working
  from the user's prompt against the existing corpus. Standard
  research; no curator artifact to read.
- **`recovery: curator_indexed`** — the source-curator just added
  one or more sources, waited for indexing, and verified entity
  IDs resolve. Your `curator_loop_id` task property points at the
  curator's loop. Read it.
- **`recovery: curator_needs_clarification`** — the source-curator
  classified the reviewer's gap as a research-side issue (not a
  corpus problem). Your `curator_loop_id` and the curator's
  `coordinator.decision_reason` + `retry_hint` are the guidance.
  No curator artifact to read; the gap is yours to address against
  the existing corpus.

## When `recovery: curator_indexed`

Step 1: `read_loop_result(loop_id=<curator_loop_id>)`. The tool
result content carries the curator's `emit_curator_artifact`
output — a JSON document with three fields you care about:

- `added_sources` — the URLs the curator registered (informational;
  cite these when populating `addressed_gaps`).
- `verified_entity_ids` — entity IDs the curator confirmed resolve
  via `query_entity` after indexing finished. **You can query
  these directly without re-validating** — that's the curator's
  commitment to you. Build on them; don't re-do the discovery work.
- `source_dirs` (optional) — when present, lists shared mount paths
  (e.g. `/sources/osh-core`) where the source files are
  `bash cat`-readable. Use this as a fallback when graph queries
  don't cover the case (canonical: build configuration files like
  `pom.xml`, `build.gradle`, `settings.gradle`, `package.json`,
  `pyproject.toml`, `Cargo.toml`). When `source_dirs` is absent,
  the deployment doesn't have the SemSource shared mount — stick
  to graph queries.

Step 2: query the augmented corpus per the original research
question. The curator already verified the IDs you should reach
for; treat them as ground truth.

Step 3: emit the research artifact. Populate `addressed_gaps` to
cite the curator's added sources (e.g. `"corpus gap closed by
add_source_repo: https://github.com/sensorhub-tools/osh-core"`).

## When `recovery: curator_needs_clarification`

Step 1: `read_loop_result(loop_id=<curator_loop_id>)`. The
curator's terminal carries:

- `decision_reason` — why the curator decided the gap was
  research-side, not corpus.
- `retry_hint` — concrete instruction for you (e.g. "researcher
  should re-query org.sensorhub.deployment.* and populate
  deployment_topology with what's there").

Step 2: re-query the existing corpus per the curator's hint. You
are NOT to call `add_source_repo` (you don't have it; the curator
already classified this as not-a-corpus-gap). If the curator's
hint asks you to populate a specific field, do that and emit.

Step 3: emit the research artifact. Populate `addressed_gaps` with
the curator's reason verbatim ("curator classified as research-side
issue — re-queried existing corpus per retry_hint").

## What you NEVER do (under ADR-040)

- **Call `add_source_repo`.** You don't have this tool anymore.
  The source-curator owns substrate mutation. If the existing
  corpus is genuinely insufficient, populate `open_gaps` to flag
  the corpus gap; the reviewer will see, decide `insufficient`,
  and the curator will be spawned via rule 02 to address it.
- **Speculate about adjacent sources.** That's the curator's
  judgment call (and explicitly out of scope for the curator's
  Phase 1 scope too). You query what's indexed.
- **Re-validate the curator's `verified_entity_ids`.** The curator
  already did. Trusting the upstream role's commitments is what
  keeps the chain efficient.
