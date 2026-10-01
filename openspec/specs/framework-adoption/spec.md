# Framework adoption specification

## Purpose

Define evidence-backed framework adoption and native autoresearch return contracts while keeping SemTeams
qualification, SemSource readiness and a future SemEngine consumer decision independent.

## Requirements

### Requirement: Frozen framework adoption preserves qualified product behavior

SemTeams SHALL pin the owner-selected SemStreams module and qualify its live product composition using
recorded baseline evidence and isolated infrastructure before asserting compatibility.

#### Scenario: Exact frozen dependency

- **GIVEN** the migration target is SHA `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`
- **WHEN** the product dependency is resolved
- **THEN** it resolves to `v1.0.0-beta.162.0.20260930150212-8b99efe9c66a` without a local replacement

#### Scenario: Honest qualification and approval boundaries

- **GIVEN** research and autoresearch are the live product packs
- **WHEN** compatibility is qualified
- **THEN** routing, fan-out/join/review, terminal and cancellation behavior, approval and sandbox boundaries,
  trajectory access, and graph/UI behavior have explicit observed outcomes or named qualification gaps
- **AND** unavailable evidence bodies are not represented as restored artifact rendering
- **AND** approval policy remains fail-closed and sandbox attestation is not bypassed

#### Scenario: Independent ecosystem adoption

- **GIVEN** SemSource dogfooding and a future SemEngine switch have separate readiness contracts
- **WHEN** this SemStreams upgrade is reviewed
- **THEN** dependency measurements inform SemEngine without automatically expanding its first release
- **AND** SemSource-backed dogfooding stays held until SemSource is ready

### Requirement: Autoresearch returns use the run's authoritative origin

SemTeams SHALL deliver approved results, descended clarification and non-budgeted failure reports through the
existing run origin using native typed coordinator ancestry, without moving iteration-state ownership or requiring
SemStreams changes.

#### Scenario: Approved result preserves current run association

- **GIVEN** an autoresearch run completed after reviewer approval
- **AND** its origin coordinator also carries an older inherited run anchor
- **WHEN** the category return rules publish the result coordinator
- **THEN** native parent and run identifiers name the current run's origin loop
- **AND** the run's existing origin and terminal phase remain unchanged
- **AND** typed dispatch delivers the reply to that origin's user channel

#### Scenario: Return authorization and failure accounting

- **GIVEN** a category source emits a return fact
- **WHEN** the run is evaluated for forwarding
- **THEN** exactly one source and one canonical same-authority, same-instance origin are required
- **AND** approved, clarification and failure facts require completed, executing and failed phases respectively
- **AND** a cancelled run does not qualify
- **AND** a budgeted execute failure consumes its existing iteration budget without producing a run-failure reply

#### Scenario: Existing state and replay semantics remain explicit

- **GIVEN** the run owns its cap, journal, empirical best and stop state
- **WHEN** result handoff or persisted rule-state replay occurs
- **THEN** it does not rearm the iteration driver or duplicate a persisted return action
- **AND** configured caps above three remain governed by the run cap
- **AND** no transactional publication guarantee is asserted across a publication/state-persistence crash
