# Design: Repo Readiness Initialization

## Technical Approach
Use SemStreams flow-native graph facts and SemTeams category packs to initialize a repository for future work. The
initializer reads repository structure and tool metadata, writes a repo-readiness model, and drafts harness OpenSpec
artifacts. It does not write implementation code or bypass approval gates.

## Architecture Decisions

### Decision: Repo init is scoped, not generic
The command surface should prefer `/init-repo <path>` or a visible "Initialize repo" UI action for dev repositories.
Generic `/init` is too broad for SemTeams because future categories may initialize research corpora, monitoring targets,
decision workspaces, or other non-dev domains. `/init` may become a contextual alias only if the UI can show the resolved
target and require confirmation before work starts.

### Decision: Inventory precedes harness building
The first pass is read-only inventory. It identifies language/toolchain shape, test commands, container/devcontainer
support, standards surfaces, and missing proof dependencies. Harness creation is a later approved action driven by a
reviewed OpenSpec change.

### Decision: Harness plans are OpenSpec artifacts
The initializer emits a harness plan as an OpenSpec change so humans can review, edit, export, or hand it to another
implementation tool. The plan names dependencies, harness profiles, smoke commands, evidence freshness, and residual
risks before any implementation specs are released.

### Decision: OSH-shaped repos are the dogfood target
The first hard-domain target is an OpenSensorHub-shaped Java/Gradle repo or source folder. Candidate harness profiles
include:

- `java.gradle.build`
- `osh.module.lifecycle`
- `csapi.conformance.smoke`
- `mavlink.px4-sitl.mavsdk`
- `plugin.coverage.matrix`

These profiles should start as draft or missing until a harness-bootstrap run proves the smoke commands and emits fresh
readiness records.

### Decision: Initialization feeds proof readiness
Repo init produces inputs for `proof_readiness`; it does not replace that gate. A future implementation request still
requires fresh readiness records or bounded waivers before `dev_from_task` can release code work.

## Data Flow
User selects repo/folder -> `/init-repo` or Initialize repo UI action -> inventory tools -> `repo_readiness.*` facts ->
candidate `proof.*` facts -> harness OpenSpec change -> human review -> optional harness-bootstrap execution -> readiness
records -> future `create_change` / `implement-spec` runs.

## File Changes
- `configs/rules/repo-readiness/` (new post-MVP category pack)
- `configs/personas/fragments/coordinator/` (teach `repo_readiness` routing and `/init-repo`)
- `configs/personas/fragments/repo-readiness/` (inventory and harness-plan guidance)
- `cmd/semteams/tools/` (inventory and harness-plan helpers only if no upstream primitive exists)
- `ui/src/lib/components/board/` and command palette surfaces (Initialize repo action and readiness summary)
