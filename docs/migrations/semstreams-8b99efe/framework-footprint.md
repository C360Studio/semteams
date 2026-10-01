# Measured SemTeams framework footprint

Matched method-2 reports measure baseline `ce22c961d30014c463a09f8f8a2a90044ee1a1cf` / SemStreams
`v1.0.0-beta.160` and the final migration Go source pinned to
`v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`, SHA `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`.
These are consumer evidence for a later approved SemEngine contract. They do not expand its first release.
SemTeams' SemStreams upgrade and SemSource readiness remain independent decisions.

## Current method and artifacts

- [Baseline method-2 report](framework-footprint-beta160-v2.json)
- [Frozen method-2 report](framework-footprint-frozen-v2.json)
- [Measurement script](../../../scripts/measure-framework-footprint.go)
- [Configured surfaces](configured-surfaces.json)
- [Exact resolved module/source identity](evidence/runtime/frozen-module.json)

The script loads syntax and `go/types` via `golang.org/x/tools/go/packages`, on macOS arm64 with Go 1.26.4.
It keys each exported framework declaration by package, framework module-relative declaration file/line/column and
text. This keeps same-named, same-typed fields on different structs distinct. Every referenced baseline and target
declaration has a nonzero module-relative source position. References include consumer file/line/column and production
or test source classification. `call:true` means a syntactic call-expression target; it includes type conversions and
does not identify later calls through function values. These are declaration/reference counts, not API-call counts.

`direct_imports` records consumer importer, imported path and source position, separating production and `_test.go`
files. It excludes generated test mains. `packages` is the selected compiler package-ID closure, including standard
library and test variants; `framework_packages` is the unique framework package-path subset. `modules` contains
resolved path/version pairs, including the main module. `module_graph` separately records every `go mod graph`
requirement edge, including versions not selected by the compiler closure. Neither report has a module-graph error.

Scopes are `runtime` (`./cmd/semteams`, no tests), `all_with_tests` (`./...`, tests enabled),
`all_with_integration_tests` (adds `integration`), and `all_with_parked_tests` (adds `parked_packs`). The first three
scopes compile without reported errors at both pins. Platform/build tags change these sets. This is compiler closure,
not linker reachability, binary size or an exhaustive dynamic registration/capability census.

The baseline Go source/module were preserved before adaptation. Later E2E fixture/spec corrections in the baseline
directory are separately hashed browser inputs and do not change the measured Go source. The directory as a whole
is therefore not an immutable archive. Target measurements include the revision-fenced approval projection and its
final tests. Further Go changes require a new measurement; documentation/browser-only changes do not change this census.

## Matched method-2 counts

| Scope | Measure | beta.160 | Frozen target |
|---|---|---:|---:|
| runtime | exported declarations referenced | 296 | 326 |
| runtime | defining framework packages referenced | 31 | 29 |
| runtime | framework package closure | 95 | 100 |
| runtime | all package IDs | 577 | 576 |
| runtime | resolved module pairs | 68 | 65 |
| runtime | direct source import locations, all dependencies | 481 | 485 |
| all with tests | exported declarations referenced | 421 | 467 |
| all with tests | defining framework packages referenced | 31 | 31 |
| all with tests | framework package closure | 95 | 100 |
| all with tests | all package IDs | 665 | 662 |
| all with tests | resolved module pairs | 72 | 69 |
| all with tests | direct source import locations, all dependencies | 1096 | 1149 |
| integration + tests | exported declarations referenced | 424 | 513 |
| integration + tests | defining framework packages referenced | 31 | 31 |
| integration + tests | framework package closure | 95 | 100 |
| integration + tests | all package IDs | 668 | 665 |
| integration + tests | resolved module pairs | 72 | 69 |
| integration + tests | direct source import locations, all dependencies | 1128 | 1223 |
| module requirement graph | directed edges | 724 | 718 |

Direct framework import locations (one import statement per consumer file) are runtime production 133 → 136;
all-source production 143 → 146, default test 97 → 121, and integration-tag test 100 → 141.
The JSON supplies the exact edges and reference locations rather than inferring requirements from these totals.

## Parked build context

Both `parked_packs` contexts reproduce the same compile failure in
`test/contract/create_change_fixture_test.go:72`: `recordingPublisher` lacks `Append` for
`agentictools.TriplePublisher`. The reported 421 → 467 declarations, 665 → 662 package IDs and 72 → 69 modules
are **partial diagnostic sets**, not successful compiled obligations. Neither the tag nor any parked product pack
was enabled in runtime configuration, and the donor test defect was not repaired to make measurement pass.

## Consumer interpretation

The full framework registrar pulls in 100 framework packages at the target, although the shipped bootstrap declares
nine component factories. [Configured surfaces](configured-surfaces.json) separately records those factories,
services, rules/projection groups, tool allowlists and port declarations for production and mock configs. It is a
configuration inventory, not runtime liveness or an exhaustive dynamic-registration census. The
[contract inventory](contract-inventory.md) covers semantic and HTTP/GraphQL/NATS boundaries that Go symbol use cannot.

Runtime closure removed: `engine`, `flowstore`, `flowtemplate`, `internal/builtinprojection`.

Runtime closure added: `composition`, `internal/agentterminal`, `internal/deliverylane`, `internal/lifecyclecleanup`, `internal/logforwarderpolicy`, `internal/looptoken`, `pkg/projection/contract`, `processor/agentic-loop/internal/loopbucket`, `processor/agentic-loop/internal/looprequest`.

The product consumes lifecycle/configuration, public projections and graph mutation, agent/run/message types, rules,
NATS, registries/personas/payloads, tools, logging/metrics and vocabulary. Compile closure includes registrar-selected
and parked-source dependencies beyond active features. Every SemEngine inclusion needs its own approved consumer
need; compiled dependencies are not permission to copy retired facilities or widen its first release. SemSource-backed
dogfooding stays held until SemSource is ready.

## Superseded method-1 history

[Method-1 baseline](framework-footprint-beta160.json) and
[method-1 intermediate target](framework-footprint-frozen.json) are retained as development evidence, superseded by
method 2. Their `ObjectString`-only keys merged fields across structs. Baseline counts 277/385/388 became
296/421/424 solely from correcting the method. The old target also predates the approval projection, so comparing its
247/352/393 counts directly with final method 2 would mix method and implementation changes. Use only the matched
method-2 table above for the current framework comparison.

## Reproduction

Run the same script from each consumer directory; it creates no infrastructure or source edits:

```bash
# cwd: /tmp/semteams-migration-8b99efe/baseline-source
go run /Users/coby/.codex/worktrees/semstreams-frozen-migration/semteams/scripts/measure-framework-footprint.go \
  > /tmp/semteams-beta160-framework-footprint-v2.json

# cwd: /Users/coby/.codex/worktrees/semstreams-frozen-migration/semteams
go run scripts/measure-framework-footprint.go > /tmp/semteams-frozen-framework-footprint-v2.json
```

Inspect every scope's `errors` and `module_graph_error`; a zero script exit does not mean a tagged context compiled.
The reports retain the parked failure as data while providing complete successful selected contexts separately.
