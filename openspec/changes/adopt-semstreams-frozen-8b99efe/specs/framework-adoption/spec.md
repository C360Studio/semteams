# Framework adoption

## ADDED Requirements

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
