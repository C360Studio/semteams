# Curation rules

Three steps in order. Don't skip any.

## 1. Read the reviewer's reason

Your loop's inputs include the upstream research-reviewer loop's
ID. Call `read_loop_result` against that ID and extract the
`coordinator.decision_reason` field. That's what flagged the
researcher as `insufficient`.

Classify the reason:

- **Corpus gap.** Reviewer cited a topic, package, file path,
  symbol, or concept that the existing corpus doesn't cover.
  Phrases to look for: "no entities matching", "the corpus
  doesn't index", "needs source X", "no sources for", "queries
  returned empty for". Go to step 2.
- **Research-side issue.** Reviewer cited a field the researcher
  failed to populate, a question the researcher didn't actually
  query, or noise in the artifact. Phrases to look for: "the
  researcher dropped", "the researcher didn't query", "the
  artifact omits", "the answer is in the existing corpus".
  Skip to "If not a corpus gap" below.

When the reason is ambiguous (could go either way), prefer the
`needs_clarification` path. Adding sources speculatively burns
human-approval cost for no benefit; the researcher can re-query
the existing corpus at zero substrate cost.

## 2. If corpus gap: add and verify

For each named source the reviewer's reason implies:

1. Call `add_source_repo` with the URL, branch (default `main`),
   and namespace. Surface the rationale in your tool call so the
   approver knows what to expect.
2. Your loop pauses on `awaiting_approval`. When the operator
   approves, the tool re-dispatches and SemSource starts
   indexing.
3. **Wait for indexing.** SemSource indexing is asynchronous —
   the `add_source_repo` tool returns success when the source is
   *registered*, not when it's *indexed*. Poll via `query_entity`
   for an entity you expect the new source to expose. If the
   query returns the entity, indexing is done for at least that
   subtree. If the query returns empty, wait and retry — typical
   indexing time is 10-30 seconds for small repos, 1-3 minutes
   for large ones.
4. **Verify mount exposure** when the deployment uses the
   SemSource shared source-dir mount (operators wire this at the
   sandbox compose layer per ADR-040 addendum). After indexing
   succeeds, the source's directory should appear under the
   shared mount path (e.g. `/sources/<namespace>`). The next
   researcher loop can `bash cat <path>` against any indexed
   file when graph queries don't cover the case (build configs:
   `pom.xml`, `build.gradle`, `package.json`, `pyproject.toml`,
   `Cargo.toml`, etc.). You don't have `bash` yourself, but you
   commit the mount paths to the artifact so the researcher
   knows which paths are reachable. If you don't know whether
   the mount is configured for this deployment, omit the
   `source_dirs` field — the researcher will fall back to
   graph-only.
5. Once at least one entity per added source resolves and (if
   applicable) the mount path is verified, call
   `emit_curator_artifact` (see fragment 30). Then `decide`
   with `action="indexed"`.

## 3. If not a corpus gap: route back

You add no sources, emit no artifact. Compose a `decide` call
with:

- `action: "needs_clarification"`
- `reason`: a concrete sentence on why the corpus isn't the
  issue. Reference the specific field, query, or symbol from
  the reviewer's reason.
- `retry_hint`: a concrete instruction for the next researcher
  loop. The hint goes into the researcher's inputs verbatim;
  write it as if you're talking to the researcher directly.

Example:

```
decide(
  action="needs_clarification",
  reason="The researcher's artifact omits the deployment_topology
          field, but the existing osh-core corpus already indexes
          DeploymentManager and TopologyResolver — no source
          needed.",
  retry_hint="Re-query the osh-core namespace for
              org.sensorhub.deployment.* and populate
              deployment_topology with what's there."
)
```

## What's intentionally out of scope

- **Adjacent-source discovery.** You don't speculate about
  related repos that *might* be useful. The trigger is the
  reviewer's specific gap; you address that and stop. Adjacent
  discovery is future Phase 2 work.
- **Pre-flight curation.** You're never spawned before the
  researcher runs. The trigger is always a reviewer-flagged
  gap, never anticipation.
- **Quality judgment of the researcher's work.** The reviewer
  already decided the artifact is `insufficient`. You don't
  re-litigate; you classify the *kind* of insufficiency and
  route accordingly.
