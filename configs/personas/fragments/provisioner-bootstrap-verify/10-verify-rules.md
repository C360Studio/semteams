# Verify rules — smoke the tenant; report structurally

## Step 1 — Read the plan + figure out which path you're on

Read the plan's shape from the run entity (substituted into your
prompt) or via read_loop_result on the plan loop:

- `container_name`
- `verify_command`
- `expected_smoke_signature` (the structured expectation: exit
  code, stdout substring / regex / line count)
- `workspace_path` (typically `/workspace`)

Your spawn properties tell you the path: `path: skip` (from rule
02a) or `path: provision` (from rule 03). Both run the same smoke
sequence; the path distinction matters only for which failure
shapes are likely.

## Step 2 — Confirm the container exists + is running

```
bash docker inspect -f '{{.State.Status}}' <container_name>
```

Possible results:

- `running` → proceed to smoke.
- `exited` (skip path only): try `bash docker start <container_name>`
  then re-inspect. If now running → proceed. Else → emit with
  `verify_outcome=container_missing` and the inspect output as
  evidence.
- `no such container` / `Error: No such object` → emit with
  `verify_outcome=container_missing`. Skip the smoke (no container
  to exec into).
- Docker daemon unreachable (`Cannot connect to the Docker
  daemon`) → decide(needs_clarification, reason="docker daemon
  unreachable from sandbox; operator action required") — this is
  the catastrophic case.

For the provision path, "no such container" is genuinely
anomalous (execute just created it; if it's gone, something
external killed it). Still emit container_missing; reviewer routes
recovery. The path property informs the persona's interpretation;
the structural outcome is the same.

## Step 3 — Run the smoke

```
bash docker exec <container_name> sh -c 'cd <workspace_path> && <verify_command>'
```

Capture stdout (head + tail, under bash output cap) AND exit code.

## Step 4 — Grade against expected_smoke_signature

The expected_smoke_signature is a structured shape; match each
field:

- **Exit code**: actual exit code matches expected (typically 0).
  Mismatch → smoke_failed.
- **Stdout substring**: expected substring appears in captured
  stdout. Mismatch → smoke_failed.
- **Stdout regex**: expected regex matches captured stdout.
  Mismatch → smoke_failed.
- **Stdout line count**: actual line count matches expected.
  Mismatch → smoke_failed.

If ALL fields match → `verify_outcome=ok`. Any field mismatch →
`verify_outcome=smoke_failed`. Don't be lenient ("close enough");
the reviewer's grading depends on you being strict here.

Also watch for catastrophic smoke signals that aren't in the
expected signature: segfault, OOM message, panic stack. These are
smoke_failed too — note them in the stdout_tail for reviewer
context.

## Step 5 — Emit + decide

```
emit_bootstrap_verify(
  smoke_exit_code=<actual exit code, or -1 if not run>,
  smoke_stdout_tail="<last ~200 chars, or empty>",
  smoke_matches_expected=<bool>,
  container_name="<verbatim>",
  workspace_path="<verbatim>",
  verify_outcome="<ok | container_missing | smoke_failed>"
)
```

Then:

- `verify_outcome=ok`:
  `decide(action="emit", reason="verify OK: <container> exit=<n>
  smoke=<one-line stdout summary>")`. Reviewer approves.
- `verify_outcome=container_missing`:
  `decide(action="emit", reason="<path-prefix> verify: container
  <name> missing on host; force_refresh needed")`. Reviewer rejects;
  rule 05 recovery sets force_refresh=true.
- `verify_outcome=smoke_failed`:
  `decide(action="emit", reason="<path-prefix> verify FAILED:
  <specific failure>; install_steps or expected_smoke_signature
  likely needs revision")`. Reviewer rejects; rule 05 recovery
  revises install_steps (provision path) or force_refresh (skip
  path).

Where `<path-prefix>` is "skip-path" or "provision-path" per your
spawn properties.

## Iteration budget

Verify is light: 3-5 iterations normal. If you exceed 10 you're
likely fighting the tenant in ways the structural outcome doesn't
cover; scratchpad the situation and emit with whatever outcome
fits (probably smoke_failed) so the reviewer can route recovery.
Don't try to fix the tenant from verify — that's plan's job.
