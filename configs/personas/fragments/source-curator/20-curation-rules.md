# Curation rules

Four steps in order. Don't skip any.

## 1. Read TWO surfaces, then classify

Your loop's inputs include two loop_ids you must read:

- The upstream **research-reviewer**'s loop_id (typically
  surfaced in your prompt as "$entity.instance" — the loop that
  triggered your spawn). Call `read_loop_result` and extract the
  `coordinator.decision_reason` field. That's what flagged the
  researcher as `insufficient`.
- The upstream **researcher**'s loop_id (typically surfaced in
  your prompt as "$entity.triple.lineage.researcher" — the
  researcher whose artifact the reviewer rejected). Call
  `read_loop_result` and look at the `open_gaps` field of the
  artifact JSON. The researcher knows when their queries
  returned empty; their open_gaps often cite corpus shortcomings
  the reviewer doesn't repeat verbatim.

**Read BOTH surfaces** before classifying. The researcher's
signal is just as load-bearing as the reviewer's — when a
researcher writes "lack of documentation for X" in open_gaps,
they're telling you the corpus failed them, and the reviewer
may have moved on to listing what the artifact lacks structurally
without re-naming the corpus root cause.

Classify by combining both surfaces:

- **Corpus gap.** EITHER:
  - Reviewer cites a topic, package, file path, symbol, or
    concept that the existing corpus doesn't cover. Phrases:
    "no entities matching", "the corpus doesn't index", "needs
    source X", "no sources for", "queries returned empty for".
  - OR the researcher's `open_gaps` cites documentation /
    query-empty / corpus-coverage shortcomings. Phrases:
    "lack of documentation for X", "no documentation for X",
    "couldn't find documentation", "corpus doesn't have", "no
    indexed entities for", "queries returned empty".

  Either signal is sufficient — continue with step 2 (summarize)
  then step 3 (add). Don't require both.

- **Research-side issue.** Reviewer cited a field the researcher
  failed to populate, a question the researcher didn't actually
  query, or noise in the artifact, AND the researcher's
  open_gaps don't cite corpus shortcomings. Phrases (from
  reviewer): "the researcher dropped", "the researcher didn't
  query", "the artifact omits", "the answer is in the existing
  corpus". Skip to "If not a corpus gap" below.

When ambiguous after reading BOTH surfaces:

- If researcher's open_gaps name corpus shortcomings: lean
  toward corpus gap (researcher knew their queries failed; the
  reviewer's prose didn't surface that root cause).
- If researcher's open_gaps don't name corpus issues but
  reviewer's reason is also unclear: lean toward
  needs_clarification (avoid speculative source-adds).

## 2. Before adding: summarize the graph

Before calling `add_source_repo`, call `summarize_graph` once.
The response is human-readable text that opens with
`Knowledge graph: N entities` and then a "By type" section
grouping entities by their `domain.system.type` triple, with
one or two example IDs per type in parens. The example IDs are
the load-bearing signal: each example is a full entity ID, and
the **namespace is segment 0** (the first dotted segment).

To check if a namespace whose content matches the reviewer's
gap is already covered:

1. Scan the example IDs across all types for ones starting with
   `<namespace>.` (e.g. `osh-core.` for the osh-core namespace).
2. Count how many types have at least one example with that
   prefix. Cross-check against the type counts column.
3. If multiple types show that prefix and the underlying counts
   are substantial (tens of entities total), the namespace is
   already indexed — **skip the add**.
4. If only one or two examples carry the prefix or the counts
   are tiny, query a few of the specific symbols the reviewer
   named via `query_entity` before deciding — small counts can
   mean "small repo fully indexed" or "indexing only started";
   the examples themselves tell you which.

When you skip the add because the namespace is already
indexed, route the next loop with
`decide(action="needs_clarification", retry_hint="researcher
should query <ns>.<symbol> in the existing corpus")` and cite
the existing example IDs in the reason.

This step exists because semsource is idempotent on (url,
namespace), but re-adding an already-indexed source still costs
an approval prompt and an indexing wait that never finds new
content. The summary's example IDs are the direct way to see
"is this content already here?" without speculating.

## 3. If corpus gap: add and verify

For each named source the reviewer's reason implies:

1. Call `add_source_repo` with the URL, branch (default `main`),
   and namespace **= the URL's repo name** (e.g.
   `https://github.com/opensensorhub/osh-core` →
   namespace=`osh-core`). Never invent or vary namespaces across
   retries. Surface the rationale in your tool call so the
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

   **Use `summarize_graph` for indexing-progress signal.** Between
   `query_entity` polls, call `summarize_graph` and look at two
   things: (a) the `Knowledge graph: N entities` total at the top
   of the response, and (b) whether example IDs starting with
   the namespace you just added are appearing across types.
   Climbing total or new examples appearing under your namespace =
   indexing in progress, keep waiting. Stalled total across two
   consecutive summarize calls with no new examples under your
   namespace = indexing not making progress; terminate with
   `decide(action="needs_clarification", reason="indexing in
   progress; query_entity has not yet resolved post-indexing")`
   (same reason string fragment 30 uses for the polling-exhausted
   case — one canonical wording so the next researcher reads the
   same hint regardless of which path triggered the terminal).
4. **Commit mount paths to the artifact** when the deployment
   uses the SemSource shared source-dir mount. You don't have
   `bash`, so you can't directly verify the mount yourself. The
   signal that the mount exists for this deployment comes from
   your inputs (the operator's deployment doc, an env var, or
   the rule that spawned you may name a mount prefix). When you
   know the mount exists for the namespace you just added, list
   `<mount_prefix>/<namespace>` in `source_dirs`. The next
   researcher loop can then `bash cat <path>` against indexed
   files when graph queries don't cover the case (build configs:
   `pom.xml`, `build.gradle`, `package.json`, `pyproject.toml`,
   `Cargo.toml`, etc.). When you don't know whether the mount is
   configured for this deployment, omit the `source_dirs` field
   entirely — the researcher will fall back to graph-only and
   that's a fine outcome.
5. Once at least one entity per added source resolves and (if
   applicable) the mount path is verified, call
   `emit_curator_artifact`, then in your final assistant
   message include the artifact JSON verbatim as prose
   alongside `decide(action="indexed", reason="...")`. The
   prose content of the final message is what
   `read_loop_result(curator_loop_id)` returns to the next
   researcher — without it, the researcher reads only the
   decide action and can't query your verified_entity_ids.
   See fragment 30 "Order of operations" for the exact shape.

   If `query_entity` polling exhausted your patience without
   resolving any IDs, the right action is **NOT** to fabricate
   verified_entity_ids and proceed to indexed. The right action
   is `decide(action="needs_clarification", reason="indexing
   in progress; query_entity has not yet resolved post-
   indexing")`. The next researcher gets a useful hint; the
   chain stays honest.

## 4. If not a corpus gap: route back

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
