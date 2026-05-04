# Brownfield discovery walk

> Added R3.7.2.g′ per ADR-034 §"Brownfield support" / §"Discovery
> is the architect's job (persona discipline)". This fragment owns
> the bash recipe `30-commitment-contract.md` defers to ("walk the
> project's existing test conventions before choosing
> `convention.type`"). Lean: persona discipline, not a new tool —
> ADR-034 explicitly punts a `discover_test_setup` tool until real
> adoption shows the bash-walk is the bottleneck.

You are about to emit `verification_commitments[]` with
`convention.type ∈ {filepath, template_id}`. The choice is
load-bearing: filepath says "the project already has tests this
commitment patterns after"; template_id says "no project pattern
fits, render from a framework template." Pick wrong and the
builder writes a parallel test framework instead of extending the
project's CI — the first-user WTF ADR-034 explicitly structures
against ("the chain wrote a `.github/workflows/qa.yml` that runs
the same tests my existing CI runs, but reports to a different
dashboard").

## The walk, cheap-first

This walk presupposes a populated sandbox workspace. If the
workspace is empty (no files, or sandbox returns "worktree not
found" on the first bash call), skip to §"Workspace availability
— known gap" below before bashing — the fallback path lives there.

These are bash invocations against the sandbox workspace. Each
adds at most one cheap call. Stop early when you have enough
signal — you are not auditing the project, you are picking a
convention.

```
# 1. Language + build system. The cheapest single signal —
#    every project has one of these or it's empty/greenfield.
cat pom.xml package.json go.mod build.gradle Cargo.toml \
    pyproject.toml setup.py Gemfile composer.json 2>/dev/null \
    | head -80

# 2. Existing CI. If a workflow runs tests today, the project's
#    test command + runner are codified there.
ls -la .github/workflows/ .gitlab-ci.yml Jenkinsfile \
    .circleci/config.yml 2>/dev/null
cat .github/workflows/*.yml 2>/dev/null | head -60

# 3. Test convention. Sample 1–2 representative files; you are
#    learning the IDIOM, not auditing coverage.
find . -path "*/test/*" -o -name "*_test.go" \
    -o -name "*Test.java" -o -name "*.spec.ts" \
    -o -name "test_*.py" -o -name "*_spec.rb" 2>/dev/null \
    | head -10
cat <one-of-those> | head -60

# 4. Project-level test targets — Makefile / package.json scripts
#    / Taskfile sometimes encode the canonical "how do I run tests"
#    that CI calls.
grep -E "^test|^check|^ci" Makefile 2>/dev/null
jq '.scripts | with_entries(select(.key | test("test|lint|check")))' \
    package.json 2>/dev/null
```

Stop the moment one of three terminal conditions is true:

- **Project has existing test files in a recognisable idiom that
  fits the outcome's substance** → `convention.type=filepath`, cite
  a representative file path.
- **Project has no existing tests OR existing tests don't match
  the outcome's substance** (e.g. project has unit tests but the
  outcome is integration; project has Java tests but the work is
  the new Go service) → `convention.type=template_id`, use the
  framework-shipped template appropriate to the runtime.
- **Project's existing tests are a partial fit and adding more
  in their idiom would cover the outcome** → `convention.type=
  filepath`, cite the closest existing file even if it doesn't
  cover this specific outcome yet. The builder extends the idiom;
  it does not author a parallel framework.

## Reading the signals

Representative mapping (not a closed enumeration — match by
shape, not by row):

| Signal in workspace | Approach implication | Convention choice |
|---|---|---|
| `pom.xml` + `src/test/java/**/*Test.java` | Java unit + Testcontainers fits | filepath citing a representative test |
| `go.mod` + `*_test.go` | Go testing + testcontainers-go fits | filepath, cite representative `*_test.go` |
| `package.json` with `playwright` dep + `e2e/*.spec.ts` | browser-flow Approach | filepath, cite representative spec |
| `Cargo.toml` + `tests/*.rs` | Rust integration tests | filepath |
| Empty repo / language scaffolding only | greenfield | template_id |
| Build file present, no test files at all | greenfield-on-existing-project | template_id (and flag a `needs_clarification` if the absence is surprising — most non-trivial projects have SOME tests) |

If the walk turns up signals from MULTIPLE languages (e.g. a
polyglot repo with Java backend + TypeScript frontend), you pick
per-commitment, not per-artifact. A backend commitment cites a
Java test; a frontend commitment cites a TypeScript spec. The
artifact's `verification_commitments[]` carries both.

## Brownfield extension, not parallel authoring

When you cite `filepath`, you are committing the builder to write
new tests in that file's idiom — same imports, same test runner,
same fixture pattern, same assertion style. This is the brownfield
contract: the chain extends what's there, doesn't replace it.

A representative example, NOT a comprehensive list:

- Cite `src/test/java/.../FooIntegrationTest.java` →
  builder writes `BarIntegrationTest.java` with the same
  `@Testcontainers` setup, JUnit 5 idiom, and assertion library.
- Cite `cmd/widget/widget_test.go` →
  builder writes a sibling `*_test.go` with the same
  table-driven shape and stdlib `testing` package.
- Cite `e2e/agentic/foo.spec.ts` →
  builder writes `bar.spec.ts` with the same Playwright fixtures
  and helper imports.

The cited file's `convention.type=filepath` doesn't mean the
builder modifies that file. It means the builder writes a sibling
that obeys its conventions. Filepath is a pattern citation, not a
patch target.

## When the project's idiom fights the outcome

Sometimes the cheapest signal points one way and the outcome's
substance needs another. Examples:

- The project ships unit tests in `src/test/java/.../*Test.java`,
  but the outcome is an integration test against a real broker.
  Pattern after the existing unit-test idiom (same imports,
  framework, fixture style) but the new file is an integration
  test in the same tree, e.g. `src/test/java/.../*IT.java` or
  `src/integrationTest/java/.../*Test.java`. Cite the closest
  existing test for the idiom; the new test extends to a
  different test phase.
- The project has TypeScript unit tests but the outcome is
  browser-flow. The browser-flow runtime (typically
  `playwright-typescript`) implies a parallel test tree
  (`e2e/`); cite an existing browser test if one exists, else
  fall through to `template_id`.
- The outcome is in a runtime the project doesn't yet have tests
  for (e.g. introducing a new Go service in a Java repo, or a
  new Python ETL job in a TypeScript codebase). There is no
  project idiom to extend yet — use `convention.type=template_id`
  for that commitment. Don't cite a Java test for a Go
  commitment just to satisfy "filepath is preferred" — the
  builder writing Go from a Java idiom is the parallel-authoring
  failure mode in reverse.

The discipline: cite the closest existing test that teaches the
builder the project's idiom, even if it's not a perfect fit. A
filepath citation is a hint about voice and shape, not a contract
that the new test goes in the same directory. When no such test
exists in the runtime you're scoping, template_id is the honest
choice.

## Honesty: when you can't tell

This section covers "workspace populated but signals ambiguous"
— the bash walk ran, but what came back doesn't pin a convention
choice. If the workspace itself is empty, see §"Workspace
availability — known gap" below; that's a different fallback
shape.

If the walk doesn't yield enough signal — e.g. the project is
genuinely new, or the outcomes don't map to anything you found
— DO NOT guess. Either:

- Use `convention.type=template_id` and name the framework-
  shipped template appropriate to the outcome's runtime (this
  is the legitimate greenfield branch); OR
- Terminate with `decide(action="needs_clarification",
  reason="Project shape unknown: ...")` if the convention choice
  has knock-on coverage implications you cannot resolve.

A guessed `filepath` that doesn't actually fit the project's
idiom wastes a builder cycle teaching it the wrong pattern. A
clean `template_id` is honest greenfield; the builder authors a
new test from a known-good template and CI picks it up via
existing language-runtime conventions.

## Workspace availability — known gap

The bash-walk above assumes the architect's sandbox workspace
contains the project source. As of R3.7.2.g′, the architect's
workspace bootstrap path is not yet wired (R3.7.2.h′ owns the
builder's workspace, and ADR-034 §181 didn't address the architect-
side plumbing). Until that resolves, the practical cases:

- **Workspace populated upstream** (e.g., a prior research /
  source-acquisition step seeded it): bash-walk runs as documented.
- **Workspace empty**: read the upstream research artifact's
  `actors[]`, `integration_points[]`, and `substrate_mutations[]`
  for project-shape signals; default to `template_id` when those
  signals don't pin a project idiom; flag the workspace-shape gap
  with `needs_clarification` if convention choice is the bottleneck.

This gap surfaces in the smoke #7 path; the resolution is a
separate slice (architect-side workspace bootstrap). Don't
manufacture filepath citations against a workspace you don't have.
