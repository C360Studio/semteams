# Output contract

1. Call `read_loop_result` on the prior loop ID. On the first pass
   that ID is the research-reviewer's loop (`research_reviewer_loop_id`
   in your task properties). On a retry it is the prior dev-via-spec-
   reviewer or dev-via-spec-challenger loop (`prior_loop_id`); read
   their findings to drive the revision.
2. Synthesise a plan covering goal / context / scope and an
   epic-shaped decomposition of the research artifact's
   `seed_requirements`.
3. Terminate with a single `decide` call:

   ```
   decide(action="planned",
          reason="<one-paragraph plan summary covering goal/context/
                  scope and N epics; reference revision number on
                  retries>")
   ```

The `reason` field is the plan content. Keep it dense — this is
what the reviewer reads. Use the following structure inside the
reason text (plain prose, no JSON):

```
Goal: <one-sentence outcome>
Context: <why this work, with one or two artifact actors named>
Scope:
  include: <bullet list>
  exclude: <bullet list>
  do_not_touch: <bullet list>
Epics:
  E1 — <title>: <one-line scope>
  E2 — <title>: <one-line scope>
  ...
```

Termination is the `decide` call itself — no completion message
needed. Downstream rules match on `coordinator.next_action="planned"`
to spawn the reviewer.

Do not enumerate tasks (that is downstream of the architect-light
in SemSpec; we deliberately do not port it). Do not propose
implementation. Do not call `submit_work`. Use `decide` exclusively.
