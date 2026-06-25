# Delta for Agentic SDD

## ADDED Requirements

### Requirement: Repo Readiness Command
The system SHALL provide a repo-scoped initialization action for dev workspaces without making generic `/init` the
default command for every SemTeams use case.

#### Scenario: Repo init command is explicit
- GIVEN an operator wants to prepare a repository or folder for spec-driven development
- WHEN they invoke `/init-repo <path>` or the equivalent Initialize repo UI action
- THEN the coordinator routes to the repo-readiness category
- AND the UI shows the resolved path and repo-readiness scope before any mutating action runs

#### Scenario: Generic init remains contextual
- GIVEN an operator enters `/init`
- WHEN SemTeams cannot infer a single active initialization domain
- THEN the UI asks the operator to choose a scoped initializer such as repo readiness
- AND no repo inventory or mutation begins until the scope is explicit

### Requirement: Read-Only Repository Inventory
The system SHALL inventory the target repository or folder before proposing harness work.

#### Scenario: Inventory classifies repo shape
- GIVEN a target folder is readable
- WHEN repo readiness initialization runs
- THEN the system records `repo_readiness.*` facts for languages, package managers, test commands, container support,
  known framework markers, and detected product shape
- AND the inventory does not modify repository files

#### Scenario: OSH shaped repo is recognized
- GIVEN the inventory finds Java/Gradle, OpenSensorHub addon structure, MAVSDK or MAVLink markers, and Connected Systems
  API surfaces
- WHEN the classifier evaluates the repository
- THEN the run marks the repo as OSH-shaped or MAVLink/OSH-shaped
- AND it proposes relevant harness families such as OSH module lifecycle, CS API smoke, PX4 SITL/MAVSDK, and plugin
  coverage

### Requirement: Harness Plan As OpenSpec
The system SHALL produce a reviewed OpenSpec harness plan when required proof infrastructure is missing.

#### Scenario: Missing harnesses become a spec
- GIVEN repository inventory finds missing or unproved harness profiles
- WHEN the initializer finishes
- THEN the system creates an OpenSpec change describing harness profiles, smoke commands, evidence expectations, and
  residual risks
- AND the harness plan is available for human review, edit, approval, and export

#### Scenario: Harness build requires approval
- GIVEN the initializer has drafted a harness OpenSpec change
- WHEN no human approval exists
- THEN SemTeams does not create files, install tooling, or run mutating bootstrap commands
- AND future implementation requests remain blocked by proof readiness until readiness records or waivers exist

### Requirement: Repo Init Feeds Proof Readiness
The system SHALL use repo initialization output as proof-readiness input rather than a replacement for proof readiness.

#### Scenario: Draft harness profiles do not release implementation
- GIVEN repo init proposes `proof.harness_profile.*` facts with draft or missing status
- WHEN an implementation request evaluates the run
- THEN `proof_readiness` treats those profiles as not ready
- AND implementation is not released without fresh readiness evidence or a bounded waiver

#### Scenario: Approved harness bootstrap produces reusable readiness
- GIVEN a harness plan has been approved and executed
- WHEN smoke commands pass
- THEN the run emits reusable harness profile facts and run-scoped readiness records
- AND later specs can reuse those records until their freshness window expires
