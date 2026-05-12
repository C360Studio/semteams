# Checklist — OSH driver framework actor types

For the bounded R1 prompt — *"identify the actor types in OSH's
driver framework, given OSH core repo is already indexed in our
test SemSource"* — the artifact must include at minimum:

**Actors (3 required, more allowed):**

- **OSH driver framework actor.** Names what an OSH driver is and
  what interface(s) it must implement. A bare name without role
  text counts as missing.
- **OGC Connected Systems endpoints actor.** Names which CS
  endpoints (observations, system descriptions, control streams)
  the driver exposes or consumes.
- **Meshtastic radio actor.** What the radio interface looks like
  on the wire — packets, channels, identifiers.

**Integration points (2 required, more allowed):**

- At least one actor pair involving the OSH driver and the OGC CS
  endpoints, with direction.
- At least one actor pair involving Meshtastic radio and the OSH
  driver, with direction.

**Tasks (1 required, decomposable granularity):**

- At least one task of "implement X interface backed by Y so that
  Z" shape. Aspirational tasks ("build a driver") are insufficient
  — must be decomposable.

If any required item is absent, the artifact is `insufficient`.
List the specific item(s) missing in your `decide` reason.
