# Execute rules — measure, emit, route

## Step 1 — Read context

Call `read_loop_result` on the propose loop. The reason carries
the hypothesis + diff summary.

The run entity's substitutions give you the command + parser +
best value bar:

- command: `$entity.triple.autoresearch.command`
- metric_parser: `$entity.triple.autoresearch.metric_parser`
- surface: `$entity.triple.autoresearch.surface` (you'll need
  this for revert)
- best.value: `$entity.triple.autoresearch.best.value` (the bar
  to beat)

## Step 2 — Run the measurement command

Run the command in the workspace your coordinator provisioned:

```
bash <command>
```

The chain-scoped `bash` tool routes commands into the per-tenant
devcontainer the coordinator created via `request_sandbox`. No
`docker exec` prefix, no `--workspace-folder` flag, no container
name lookup — the runner reads the chain entity's
`sandbox.attestation.host_workspace_folder` triple and wraps your
command in `devcontainer exec` automatically. You write the command
as if you were sitting in a shell inside the per-tenant container.

The workspace persists across iterations: the diff propose just
applied is on disk, your measurement runs against it, and any
revert step the rules path triggers operates on the same tree. `git
diff` is your inter-iteration channel — you can see what propose
changed before deciding how to interpret the measurement.

Capture stdout (head + tail) AND exit code AND stderr (last
~200 chars if any).

Two failure modes here:

- **`command not found` / sandbox unreachable** (the shell
  errored before running the command): decide(needs_clarification).
  See identity §"Distinguishing failure modes."
- **Command ran, exited cleanly OR non-zero**: proceed to step 3.

## Step 3 — Apply the metric_parser

The parser's instructions are in the run entity (substituted
into your prompt). Apply via bash:

```
# Example: parser says "last line matching 'Total time: ([0-9.]+)s', group 1"
bash echo "<captured stdout>" | grep -oP 'Total time: \K[0-9.]+' | tail -1
```

Capture the parsed numeric value. If the parser produces no
output or non-numeric output, treat it as pass=false (the
measurement effectively failed) — the tool will stamp
outcome=crashed.

## Step 4 — Call emit_autoresearch_measurement

```
emit_autoresearch_measurement(
  value=<the parsed numeric value, or 0.0 if parse failed>,
  pass=<bool — true iff exit code 0 AND parse succeeded>,
  stdout_tail="<last ~200 chars of command stdout>",
  stderr_tail="<last ~200 chars of stderr, or empty>"
)
```

The tool executor (product-shell, not LLM):

1. Reads `autoresearch.best.value` from the run entity.
2. If pass=false → stamps `autoresearch.measurement.outcome=crashed`.
3. Else if value < best.value → stamps outcome=kept AND updates
   run entity's `autoresearch.best.value` + `.best.experiment_id`.
4. Else → stamps outcome=reverted.

After this call returns, your loop entity carries the
`autoresearch.measurement.outcome` triple.

## Step 5 — Apply the outcome via bash

Read your loop entity's outcome via prompt substitution:
`$entity.triple.autoresearch.measurement.outcome`. Three cases:

- **outcome=kept**: the diff stays. No action. The next propose
  iteration sees the new best.value as the bar.
- **outcome=reverted**: the diff did not improve. Revert via:
  ```
  bash git checkout -- <surface globs>
  ```
  Where `<surface globs>` expands the run entity's surface
  triple. This restores the tree to the best-so-far state.
- **outcome=crashed**: same revert as reverted. The pass gate
  broke; restore the tree.

## Step 6 — Terminal

```
decide(action="measured", reason="value=<n> pass=<bool>
outcome=<kept|reverted|crashed> — <one-line measurement summary>")
```

The reason captures the iteration's result. Rule 04a stamps
experiment.completed; rule 05 or 06 routes to next iteration or
stop.

## Iteration budget

Execute is moderate: 4-6 iterations normal (read context, run
command, parse, emit, revert if needed, decide). If the
measurement command itself takes 30+ seconds (heavy test suite),
that's wallclock-heavy but iteration-light from the LLM's
perspective. Don't try to "optimize" the measurement command —
your job is to measure what propose changed, not to game the
command itself.
