# Output contract

1. Call `read_loop_result` on the prior dev-via-spec-challenger
   loop ID (`prior_loop_id` in your task properties). The
   challenger's `decide(accept)` reason field summarises the plan
   it accepted — actor citations, integration references, and the
   epic decomposition the challenger probed. R3.3 of ADR-031 ships
   without rule-engine support for cross-entity property
   passthrough, so the original planner content and the upstream
   research artifact are not directly reachable from your loop.
   Synthesise the final list against what the challenger
   summarised.
2. Synthesise the final epic-shaped seed_requirements list. Each
   final entry maps to:
   - At least one epic the challenger's accept reason cites (for
     scope grounding).
   - At least one actor the challenger's accept reason cites (for
     actor grounding).
   - At least one integration boundary the challenger's accept
     reason cites OR an explicit "internal" note (for boundary
     grounding).
3. Terminate with a single `decide` call:

   ```
   decide(action="seed_requirements_emitted",
          reason="<the final epic-shaped seed requirements list,
                  one bullet per entry, each citing the planner
                  epic + artifact actor + integration boundary
                  it derives from>")
   ```

The `reason` field is the terminal artifact. Format as:

```
Final seed requirements:

SR1 — <one-sentence requirement>
  derives from: planner epic E<N>
  actors: <one or more from artifact>
  integration: <integration_points reference, or "internal: <rationale>">

SR2 — ...
```

Termination is the `decide` call itself — no completion message.
No rule fires on `decide(action="seed_requirements_emitted")`;
your decision is the terminal of the dev-via-spec arc.

Do not invent new actors. Do not invent new integration points.
Every grounding citation must point to an existing artifact /
plan element.

If a planner epic has no actor or integration grounding the
challenger cited (a scope item the challenger accepted but did
not specifically defend), emit the SR but flag it:

```
SR<N> — <requirement>
  derives from: planner epic E<M>
  actors: (none cited by challenger — flagged: scope item without
           visible grounding chain)
  integration: (none cited by challenger — flagged)
```

The flag is honest evidence; downstream consumers see exactly
where the grounding chain breaks. Better to ship an honest flag
than to invent grounding the upstream chain did not provide.
