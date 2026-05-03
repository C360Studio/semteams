# Bash iteration contract

You operate inside a **sandboxed workspace** scoped to your
current loop. The workspace is a real git working tree (the
spawn rule ran `git init -b main` plus an initial empty commit),
so `git status`, `git diff`, `git add`, `git commit` all work.
The toolchain (Java + Maven + Gradle, Go, Node, Python, protoc)
is pre-installed and on PATH.

You will iterate up to `max_iterations` = 8 bash calls
(ADR-032 §15 locks the budget at 8 to accommodate Maven/OSGi
config discovery on the bare-seed). Make each call count.

## Step 1 — orient

On your first iteration, **understand what you have**:

```
bash cat SPEC.md
```

`SPEC.md` is the rendered spec artifact (the architect's
`emit_dev_via_spec_artifact` output) seeded into the workspace
root by the spawn rule. Read it end-to-end before writing
anything. The spec contains the goal, context, named actors,
integration points with directions, and seed requirements. Each
seed requirement enumerates the actors and integration points it
grounds against — that is your decomposition map.

You may also call `read_loop_result` on the architect's loop
(`prior_loop_id` in your task properties) for the structured
metadata (slug, generated_at, counts) if you want to confirm the
artifact identity.

If `SPEC.md` is missing, return immediately with the canonical
boot-failure terminal so operators can recognise the symptom
across runs:

```
builder_decide(
  action: "needs_clarification",
  reason: "SPEC.md not seeded into workspace by spawn rule; no
           input artifact to build from",
  blocking_question: "confirm the dev-via-spec-builder spawn
                      rule (R3.6.2.d) wrote SPEC.md from the
                      prior_loop_id artifact before the loop
                      started"
)
```

Do not attempt to recover by reading other paths or
synthesising a fallback spec — the absence is a wiring failure,
not a content gap.

## Step 2 — plan locally, then execute

Inside the prose part of your message (not in bash), think about
what files you need to produce and in what order. **Do not** turn
this into a multi-paragraph design exercise — the dev-via-spec
chain already designed. State your build plan in 3–6 bullets,
then start writing.

Typical OSH-Java-Maven order:

1. `pom.xml` and the directory layout (`src/main/java/...`,
   `src/test/java/...`).
2. The bnd / OSGi metadata.
3. The driver implementation class(es).
4. The test class(es).
5. Run `mvn compile`, fix compile errors.
6. Run `mvn test`, fix test errors.
7. Loop on (5–6) until tests pass or budget exhausted.

## Step 3 — write files via bash

Use here-docs or `printf`. The here-doc form is reliable across
shells:

```
bash <<'BASH'
mkdir -p src/main/java/com/example/osh
cat > src/main/java/com/example/osh/MyDriver.java <<'EOF'
package com.example.osh;

public class MyDriver {
    // ...
}
EOF
BASH
```

For idempotency on retry: prefer `cat > file` (truncates) over
`cat >> file` (appends). Re-running an iteration that wrote a
file should produce the same final state, not double the
content.

## Step 4 — run the build, then the tests

```
bash mvn -B -ntp compile
bash mvn -B -ntp test
```

`-B` (batch mode) suppresses the interactive download progress;
`-ntp` (no transfer progress) suppresses dependency-download
chatter that bloats your token usage. Both are pure noise
suppression — output stays informative on real failures.

Surefire writes results to `target/surefire-reports/`. After
`mvn test`:

```
bash ls target/surefire-reports/
bash cat target/surefire-reports/*.txt
```

These give you exact `Tests run:` / `Failures:` / `Errors:`
counts you will need to fill in `builder_decide`.

For Go targets the equivalent is:

```
bash go test ./... -v
```

## Step 5 — verify_clean for hands-off zones

If the spec calls out files or directories that must NOT be
modified (e.g. "do not touch the `vendor/` tree", or you've
seeded a baseline you need to preserve for comparison), pass
`read_only_paths` and `verify_clean: true` on the next bash call.
The sandbox refuses commands that would dirty those paths *and*
returns a precondition failure naming the offending file. Use
this whenever:

- The spec marks a directory read-only.
- You are running a refactor that could accidentally touch
  generated code you depend on.

Example:

```
bash(command="mvn -B -ntp test",
     read_only_paths=["src/main/resources/baseline/"],
     verify_clean=true)
```

The default is no `verify_clean`; reach for it when the spec
calls out an immutable region.

## Iteration discipline

- **One purpose per bash call.** Don't compile, test, and reformat
  in one shell command — if any step fails the others are wasted
  and the failure is hard to attribute.
- **Read the actual error, not the framing.** When `mvn` fails,
  the error chain is in stderr. `bash cat
  target/surefire-reports/*Test.txt` gives the exact assertion
  that failed. Don't guess from the high-level "Tests run: 5,
  Failures: 1" line.
- **Stop when the budget is gone.** If you hit iteration 7 and
  tests are still failing, **terminate with
  `tests_failing` and a real `retry_hint`** — not with another
  speculative fix. The retry hint is what the next builder spawn
  reads from your loop result; that's the value you produce on
  failure.
- **Do not git-push. Do not git-tag. Do not edit history.** The
  workspace is per-task; nothing outside the workspace exists
  for you to touch. Commits are fine and useful for `git diff`
  visibility, but they are local-only.

## What you cannot fix in iteration

If the spec describes a target that is impossible-to-build with
the available toolchain (e.g. "use Java 25 features" when the
sandbox has Java 21), do not try to install a different JDK. Do
not reach for the network beyond what Maven/npm/go-mod fetch
through their own settings. **Terminate with
`needs_clarification`** and name the constraint that blocks you.
That feedback is what closes the loop with the architect (or, in
R3.5, the coordinator).
