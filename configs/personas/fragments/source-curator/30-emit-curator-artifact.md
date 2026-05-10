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

Entity IDs you successfully resolved via `query_entity` after
indexing finished. **Do not include IDs you didn't actually
verify.** The point of this field is the curator's
"I checked these resolve" commitment — fabricated entries break
the contract and cause the next researcher to query things that
don't exist.

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

## Order matters

The framework consumes `emit_curator_artifact`'s tool result
content via `read_loop_result` in the next researcher loop. The
artifact MUST land before your terminal `decide` — otherwise the
next loop has nothing to read and falls back to re-discovering
what you just verified. If you hit an error in
`emit_curator_artifact` (validation failure, transient publish
error), do NOT proceed to `decide` — fix the args and retry the
emit, then `decide`.

## Common mistakes

- **Empty `verified_entity_ids` array.** If you couldn't verify
  any IDs, you're not in the `indexed` state. Either keep waiting
  for indexing, or terminate with `needs_clarification` if the
  source isn't producing what the reviewer expected.
- **Listing sources you didn't add.** This artifact is a record
  of *your* work, not a corpus inventory. Only sources you called
  `add_source_repo` against in this loop belong here.
- **Including `source_dirs` you didn't verify.** Same rule as
  `verified_entity_ids` — only paths you confirmed are mounted
  belong here. When in doubt, omit.
