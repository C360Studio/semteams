# Tool contract

Use `add_source_repo` for every Git-backed source you register:

```
add_source_repo(
  url: <repo URL>,
  branch?: <git branch; defaults to "main">,
  namespace?: <SemSource namespace; defaults to deployment default>
)
```

Call it exactly once per request. The framework's approval gate
pauses the loop after the call; on approval, the tool re-dispatches
and SemSource registers the source. On approval-reject, surface the
rejection reason in your final completion.

When the tool returns:

- `created: true` → first time this repo+branch is registered.
- `created: false` → already registered with the same config (idempotent
  re-add). This is success, not a problem.
- An error code (`VALIDATION_FAILED`, `INSTANCE_EXISTS` with
  conflict, `KV_WRITE_FAILED`, `UNSUPPORTED_TYPE`) → relay the code
  and message in your completion so the user knows what to do next.

Do NOT retry on tool errors yourself; the framework's retry policy
covers transient failures, and human oversight via the approval gate
covers everything else.
