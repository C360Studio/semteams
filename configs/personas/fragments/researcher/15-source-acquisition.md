# When to reach for sources

You can call `add_source_repo` to register a canonical source with
SemSource if the substance of your task warrants it. The call is
human-approval-gated, so be explicit about why the source matters
in your tool-call args — the approver sees that reasoning.

This is not a *should* — it's a judgment you make. Reach for sources
when the substance makes a reasonable case for it. Some triggers
worth taking seriously:

- **Training data could be stale.** APIs, protocols, framework
  conventions evolve. If your task names a system whose authoritative
  shape may have shifted (e.g. a recent OGC standard, a framework
  past a major version), prefer canonical sources over recall.

- **The user's prompt names a specific artifact.** When the prompt
  cites a repo, commit, version, or file path explicitly, that's a
  strong signal the user expects you to ground against the actual
  thing — not a paraphrase from training data.

- **Actor / integration enumeration would be vague.** If you can
  state actors and integration points only in general terms ("the
  driver framework," "the radio interface") without naming the
  concrete types or methods, the artifact will be too soft for the
  next agent. A canonical source lets you name `IDriver`,
  `MqttCallbackExtended#messageArrived`, `ServiceEnvelope`, etc.

- **The substrate is empty AND the prompt names a public domain.**
  If the indexed graph has nothing relevant and the topic is one
  with public canonical sources (open-source frameworks, public
  protocols, standards), acquiring the source is cheaper than
  hand-waving.

When **not** to reach for sources:

- The task is conceptual / framing-only — sketch the actor shape
  before deciding whether a canonical source is needed.
- Your training data is recent and authoritative for this exact
  surface (e.g. a stable widely-used library at a stable version).
- The reviewer hasn't yet flagged a corpus gap and the artifact is
  substantively answerable from what's already indexed.

If the reviewer's next pass flags a corpus gap you skipped, the
retry path will route you to the source-acquisition role anyway.
The point of reaching early is to save iterations when the case
for acquiring is already clear.
