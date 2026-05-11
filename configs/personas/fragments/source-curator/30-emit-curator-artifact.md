# Emit the curator artifact

When you took the corpus-gap path (added sources, indexing
finished, entity IDs verified), call `emit_curator_artifact`
**before** your terminal `decide(action="indexed")`. The artifact
is your structured handoff to the next researcher; `decide` is
just the framework signal.

## Tool shape

```json
{
  "tool": "emit_curator_artifact",
  "args": {
    "added_sources": [
      {
        "url": "https://github.com/sensorhub-tools/osh-core",
        "namespace": "osh-core",
        "covers": "OSH IDriver interface, IPersistentDriver, BaseSensorModule, build.gradle / settings.gradle for module wiring"
      }
    ],
    "verified_entity_ids": [
      "osh-core.org.sensorhub.api.module.IModule",
      "osh-core.org.sensorhub.api.module.IModuleProvider",
      "osh-core.org.vast.swe.SWEHelper"
    ],
    "source_dirs": [
      {
        "namespace": "osh-core",
        "mount_path": "/sources/osh-core",
        "covers": "OSH module/driver interfaces, gradle build wiring"
      }
    ]
  }
}
```

## Field rules

### `added_sources` (required, ≥1 entry)

One entry per `add_source_repo` call you made in this loop.

- `url` — the same URL you passed to `add_source_repo`.
- `namespace` — the SemSource namespace. Same value you passed
  (or the deployment default if you let the tool resolve it).
- `covers` — one or two phrases naming the symbols, packages,
  or files this source contributes. Examples:
  - `"OSH IDriver interface, IPersistentDriver, BaseSensorModule"`
  - `"meshtasticd CLI flags + systemd unit + protobuf message
     definitions"`
  - `"Spring Data JPA repository annotations"`

  Keep it concrete. The next researcher uses this to decide
  whether to query this source first.

### `verified_entity_ids` (required, ≥1 entry)

Entity IDs you successfully resolved via `query_entity` **in this
loop**. The contract is hard:

- Each ID listed here MUST come from a `query_entity` call that
  returned a successful result during this curator loop.
- No exceptions. No rationalization. No "indexing is slow, let me
  proceed with the IDs I expected." That's fabrication, full stop.
- If `query_entity` hasn't returned a successful result for ANY
  ID yet, you don't have an `indexed` artifact to emit. Use
  `decide(action="needs_clarification", reason="indexing in
  progress; <N> add_source_repo calls succeeded but query_entity
  has not yet resolved post-indexing")` instead. The next
  researcher gets a useful hint; the chain stays honest.

The point of this field is the curator's "I checked these
resolve" commitment to the next researcher. Fabricated entries
silently break the chain — the researcher queries IDs that don't
exist, gets empty results, emits an artifact that passes
structural validation but the reviewer rejects on substance
(generic actors, no concrete interfaces named, wide open_gaps),
the cycle repeats. Worse than `needs_clarification` because the
failure mode is harder to diagnose — everyone "succeeded"
mechanically; only the substance is wrong.

Pick representative entities, not exhaustive ones. 3-10 IDs per
added source is typical: the entity the reviewer cited, plus a
few obviously-load-bearing siblings. The researcher will discover
the rest via the graph's relationship edges.

### `source_dirs` (optional)

Include only when the deployment uses the SemSource shared
source-dir mount (you'll know because the operator's deployment
docs will say so, or you'll see a mount_path env var in scope).
When you can't tell, omit the field — the researcher falls
back to graph-only.

- `namespace` — same as the entry in `added_sources`.
- `mount_path` — the container-absolute path where the source
  directory is mounted (typically `/sources/<namespace>`).
- `covers` — short phrase describing what's in this directory
  that's worth a `bash cat` (build configs, top-level READMEs,
  test fixture files — things the graph doesn't decompose
  cleanly).

The mount is read-only; the researcher can `bash cat` files
under this path but cannot modify them.

## Order of operations

The next loop (researcher / reviewer) reads YOUR loop's terminal
content via `read_loop_result`, which returns the `content` text
of your final assistant message — NOT the args of any tool call.
For decide-terminated curator loops, that means the artifact has
to ride in your final message's prose alongside the decide tool
call.

Two steps in order:

1. **Call `emit_curator_artifact`** with the structured args
   above. This publishes the typed payload + mints marker triples
   on your loop entity (audit trail, ops-observer surface).

2. **In your final assistant message, include the artifact JSON
   verbatim AS PROSE alongside the `decide(action="indexed", ...)`
   tool call.** The text content of this message becomes the
   loop's `Result` field that `read_loop_result(curator_loop_id)`
   returns. Without the prose, the next researcher's read returns
   empty/just-the-decide-action — not the `verified_entity_ids` it
   needs to query.

   Concretely, your final message should look like:
   ```
   <prose>
   Curator artifact for next researcher:
   ```json
   { "added_sources": [...],
     "verified_entity_ids": [...],
     "source_dirs": [...] }
   ```
   </prose>
   <tool_call>decide(action="indexed", reason="...")</tool_call>
   ```

   Anthropic / OpenAI chat formats both allow a content block AND
   a tool_call in the same assistant message. The content becomes
   the loop's Result; the tool call sets coordinator.next_action
   for rule_02b to fire.

If `emit_curator_artifact` errors (validation failure, transient
publish error), do NOT proceed to step 2 — fix the args and
retry the emit first.

## Common mistakes

- **Calling decide without the artifact JSON in the message
  prose.** `read_loop_result` returns the loop's Result field,
  which is your final assistant message's `content` text — NOT
  the decide tool call's args. If your final message is just
  `<tool_call>decide(...)</tool_call>` with no prose, the next
  researcher's read returns nothing useful and they can't
  query your verified_entity_ids. The reviewer correctly
  diagnoses this as "the curator returned a bare status
  message instead of an emit_curator_artifact payload" and
  rejects the downstream researcher's artifact. Recovery loop
  with no substance — pure waste. Always emit the artifact
  JSON as prose in the same message as decide.
- **Fabricating `verified_entity_ids` to keep the chain moving.**
  Observed pattern: query_entity polling fails, indexing seems
  slow, LLM reasons "let me list the IDs I expected and proceed
  to indexed." This is fabrication and downstream queries will
  fail. The contract is hard — only IDs from successful
  query_entity calls in this loop. If you can't get any to
  resolve, the right action is
  `decide(action="needs_clarification", reason="indexing in
  progress; query_entity has not yet resolved post-indexing")`.
  The next researcher then knows to either retry the curator
  with a longer wait OR query the existing corpus.
- **Empty `verified_entity_ids` array.** If you couldn't verify
  any IDs, you're not in the `indexed` state. Use
  `needs_clarification` per above.
- **Listing sources you didn't add.** This artifact is a record
  of *your* work, not a corpus inventory. Only sources you called
  `add_source_repo` against in this loop belong here.
- **Including `source_dirs` you didn't verify.** Same rule as
  `verified_entity_ids` — only paths you confirmed are mounted
  belong here. When in doubt, omit.
