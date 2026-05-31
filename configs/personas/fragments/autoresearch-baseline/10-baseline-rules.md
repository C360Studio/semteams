# Baseline rules — parse, measure, stamp

## Step 1 — Read the coordinator's terminal

Call `read_loop_result` on the coordinator (the run entity). The
reason field carries the user's intent.

Extract the four autoresearch parameters:

- **command**: the executable measurement command (e.g.
  `task test:integration`, `pytest -q`, `hyperfine 'bash
  script.sh'`). Single shell command runnable in the target
  environment.
- **surface**: file globs the propose phase may edit (e.g.
  `test/, Taskfile.yml` or `src/**/*.py`). Comma-separated list
  of globs. Smaller is more honest; never grant the full repo.
- **cap**: integer iteration cap. The user may have named one;
  if not, default to 10 for cheap measurements (<30s per
  iteration) or 5 for slow ones (1-5 minutes per iteration).
- **metric_parser**: instructions for extracting a numeric value
  from the command's stdout. Lower-is-better in v1. Examples:
  - `"last line matching '^Total time: ([0-9.]+)s$', group 1 as float"`
  - `"sum of all 'time: NNNms' values"`
  - `"the float in stdout's last line"`
  
  Be specific; the execute persona reads this verbatim and applies
  it via `awk`/`grep`/`jq` against measurement stdout.

If any of the four parameters is ambiguous or missing from
the coordinator's reason, decide(needs_clarification) with the
specific gap.

## Step 2 — Run the baseline measurement

Compose the bash invocation against the always-warm sandbox:

```
bash <command>
```

The chain-scoped `bash` tool routes to the correct sandbox via
SANDBOX_URL. If the coordinator pre-provisioned a devcontainer
profile via `request_sandbox`, the attestation triples are on the
chain entity — you can read
`$entity.triple.sandbox.attestation.verified.<cap>` if you need
to confirm a capability is present, but normal commands just run.

Capture stdout AND exit code. Apply the metric_parser to extract
a single numeric value. Test the parser inline (e.g.
`echo "<stdout>" | grep ... | awk ...`) before recording the
value; if the parser produces no output or non-numeric output,
the parser is wrong and you should decide(needs_clarification).

## Step 3 — Call emit_autoresearch_baseline

```
emit_autoresearch_baseline(
  command="<verbatim>",
  surface="<verbatim>",
  cap=<integer>,
  metric_parser="<verbatim>",
  baseline_value=<measured float>,
  baseline_pass=<bool — true iff exit code 0>,
  baseline_stdout_tail="<last ~200 chars for audit>"
)
```

The tool stamps `autoresearch.{command, surface, cap,
metric_parser, baseline.value, baseline.pass, baseline.stdout_tail,
best.value, best.experiment_id}` on the RUN entity (the coordinator
loop) via subject override. `best.value` initializes to
`baseline.value`; `best.experiment_id` initializes to `"baseline"`.

## Step 4 — Terminal

If baseline_pass=true AND baseline_value extracted cleanly:

```
decide(action="propose", reason="baseline measured: value=<n>
pass=true; command=<verbatim>; surface=<verbatim>; cap=<n>;
metric_parser=<verbatim>")
```

If baseline_pass=false (command exited non-zero):

```
decide(action="needs_clarification", reason="baseline command
exited <code>: <stderr tail>; target may not be runnable in this
environment OR command is wrong")
```

If metric_parser couldn't extract a number:

```
decide(action="needs_clarification", reason="metric_parser
'<verbatim>' produced no numeric output against baseline stdout
'<head>...<tail>'; parser likely wrong")
```

## Iteration budget

Baseline is fast: 3-5 iterations normal (read context, run
command, parse, emit, decide). If you find yourself iterating to
"try different parameter interpretations," stop — that's a
needs_clarification signal. The point of baseline is to measure
what the user named, not to negotiate what they meant.
