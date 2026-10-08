# Development-pack Admission Specification

## Purpose

Define source-backed admission for absorbing SemDev into the shared SemTeams agent runtime. Preserve repository-artifact
authority, runtime evidence authority and explicit verification/approval limits without asserting pack activation or
implemented runtime behavior.

## Requirements

### Requirement: Source-backed development-pack admission

A SemTeams development-pack absorption design SHALL identify exact donor, consumer and framework revisions, trace the
committed issue-to-PR workflow and its source/test authorities, and distinguish implemented behavior from accepted
but unimplemented decisions, unmerged work and target requirements. It SHALL classify development policy, shared
agent runtime and generic engine mechanics before admitting an implementation boundary. This requirement governs
admission and does not assert that SemTeams has an active development pack.

#### Scenario: Donor work is in progress

- **GIVEN** a pinned committed donor baseline and separate draft or dirty-checkout work
- **WHEN** the absorption design states a guarantee
- **THEN** it SHALL identify whether source and tests at the baseline establish that guarantee
- **AND** accepted design status or unmerged implementation SHALL NOT be represented as shipped donor or SemTeams behavior

#### Scenario: A shared boundary prevents independent transfer

- **GIVEN** a proposed pack transfer that depends on shared types, sentinels, generic policy or an unsettled engine contract
- **WHEN** its implementation boundary is evaluated
- **THEN** it SHALL preserve one upstream authority and record the integration requirement or hold
- **AND** it SHALL NOT introduce lookalike contracts, duplicate policy or an invented API to force early extraction

### Requirement: Explicit development-workflow guarantees and limits

Development-pack admission SHALL preserve the issue → OpenSpec → approval → implementation → measurement/review →
clean-room verification → PR workflow. The design SHALL map harness-owned outcomes, actual and declared fact writers,
authoritative artifact revisions, human decisions, independent verification, bounded execution and governed external
effects to concrete source/tests or explicit unmet admission requirements. It SHALL require success and refusal evidence
before those properties are claimed for an absorbed pack.

#### Scenario: Validation and human approval refer to different authorities

- **GIVEN** a donor check that the current artifact revision matches a CLI-validated revision
- **WHEN** the design evaluates human authorization of that artifact
- **THEN** it SHALL separately prove approval binding to the revision reviewed and stale-decision rejection, or name the gap
- **AND** validation freshness alone SHALL NOT be treated as human approval of that revision

#### Scenario: Fact writers are declared but enforcement is not established

- **GIVEN** caller-declared fact-writer contracts and actual write paths
- **WHEN** admission evaluates outcome authority
- **THEN** it SHALL distinguish writer declarations and conformance checks from enforced ownership
- **AND** model claims SHALL NOT be accepted as harness-generated measurement or verification facts

#### Scenario: Supported execution or verification is narrower than the target workflow

- **GIVEN** an implementation limit such as a single-task projector or a verification rejection
- **WHEN** the donor workflow is mapped into an absorption design
- **THEN** the design SHALL preserve the refusal and its evidence or require a separately reviewed behavioral change
- **AND** it SHALL NOT claim multi-task execution, independent verification or successful external effects from workflow labels alone

### Requirement: Artifact and execution authority remain distinct

Development-pack admission SHALL require human-editable OpenSpec artifacts to retain repository authority, with
validation and approval bound to an identified artifact revision. Runtime task projections and execution facts SHALL
retain their own canonical authority, producer and attempt/revision references. A donor's graph-authored source model
SHALL be recorded as an adaptation requirement rather than carried as a second mutable authoring authority.

#### Scenario: A person edits the actual OpenSpec artifact

- **GIVEN** an artifact revision with prior validation, approval or projected work
- **WHEN** the real repository artifact changes
- **THEN** admission evidence SHALL prove validation and human approval bind to the revision used by subsequent execution
- **AND** stale approval or execution eligibility SHALL be refused or explicitly invalidated
- **AND** a display copy or mutable graph copy SHALL NOT silently replace the repository artifact as authoring authority

#### Scenario: Decisions and harness evidence have runtime authority

- **GIVEN** an authorized conversation or UI decision and harness-produced execution evidence
- **WHEN** the design maps their source of truth
- **THEN** it SHALL preserve actor or producer identity, attempt/artifact revision, decision consequence and evidence references
- **AND** it SHALL NOT treat model prose as a harness outcome or require all runtime facts to become repository artifacts

#### Scenario: A write outcome or verification input is uncertain

- **GIVEN** a requested artifact or external write, or a change to a verification command, environment, indirect script or report
- **WHEN** admission evaluates controls and proof
- **THEN** it SHALL require visible accepted, applied, rejected or unknown outcomes rather than treating acknowledgment as completion
- **AND** it SHALL require evidence that approval and verification remain bound to their authoritative inputs
- **AND** committed-artifact isolation alone SHALL NOT be claimed as proof of checker authority or exactly-once external effects

#### Scenario: Authorized approval and rejection compete

- **GIVEN** concurrent authorized approval and rejection of the same work
- **WHEN** admission evaluates decision handling and its human-visible account
- **THEN** it SHALL require evidence for the competing command outcomes and the actual decision governing execution
- **AND** it SHALL expose unresolved or unknown outcomes rather than infer an atomic winner from sequential replay or opposite-decision refusal
- **AND** single-valued replacement alone SHALL NOT be represented as deterministic concurrent-decision ordering
