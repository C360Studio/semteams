## ADDED Requirements

### Requirement: Pinned rule-family consumer evidence

SemTeams rule-family consumer contributions SHALL preserve exact source revisions, portable input fixtures and
independently stated expected values. They SHALL distinguish existing configuration from supplemental supported-contract
cases, identify actual exercised public code paths and retain the limits of each evidence level. This requirement
qualifies contribution evidence; it does not assert that a future engine/family split or an inactive pack is implemented.

#### Scenario: Existing and supplemental cases have different product significance

- **GIVEN** current SemTeams rule configurations and supplemental forced-function or response-format cases
- **WHEN** the contribution describes their provenance and behavior
- **THEN** it SHALL identify the original source/action selector and live/support/parked status, or label the case supplemental
- **AND** it SHALL NOT infer a current product dependency or activation from a supported supplemental input

#### Scenario: Fields survive decoding and the actual public executor boundary

- **GIVEN** independently expected tool-choice mode/function name and response-format type/name/strict/nested schema values
- **WHEN** frozen public decoding and action execution are exercised
- **THEN** evidence SHALL assert the literal decoded values and the canonical TaskMessage fields at the executor output boundary
- **AND** it SHALL distinguish decoder-only assertions from actual executor emission
- **AND** it SHALL NOT claim agent-loop/provider behavior or future family implementation from that bounded result

#### Scenario: Run inheritance and retry behavior have held or distinct owners

- **GIVEN** run-inheritance anchors and retry examples selected from pinned consumer source
- **WHEN** the contribution inventories their dependency on E3
- **THEN** it SHALL identify each relevant reading symbol and classify core operation retry, agent-loop behavior or pack policy
- **AND** retained E3 fixtures or source inspection SHALL NOT be represented as executed future inheritance, recovery or full-corpus qualification
