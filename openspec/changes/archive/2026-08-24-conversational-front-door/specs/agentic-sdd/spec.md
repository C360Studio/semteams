# Delta for Agentic SDD

## ADDED Requirements

### Requirement: Conversational Front Door

The system SHALL let users converse with the coordinator before a live specialist team is dispatched.

#### Scenario: Product question answers directly

- GIVEN a user asks what SemTeams can do or how to start
- WHEN the coordinator evaluates the message
- THEN it emits `decide(action="respond_direct")` with a user-facing answer
- AND no specialist team is spawned

#### Scenario: Vague idea is refined before dispatch

- GIVEN a user describes an idea that is not shaped for research or measurable optimization
- WHEN a short answer or one clarifying question would establish the next step
- THEN the coordinator emits `respond_direct` with guidance or `ask_user` with one concrete question
- AND it does not dispatch a category from topical resemblance alone

#### Scenario: Research request routes to the live team

- GIVEN a user clearly asks for evidence gathering, comparison, or synthesis
- WHEN the coordinator validates the request
- THEN it emits `decide(action="research")`
- AND the research category pack owns planning, gathering, synthesis, and review

#### Scenario: Autoresearch request preserves admission and measurement gates

- GIVEN a user supplies a scalar metric, repeatable measurement command, direction, bounded mutation surface, and target
- WHEN sandbox admission returns a ready attestation
- THEN the coordinator may emit `decide(action="autoresearch")`
- AND the hint does not bypass sandbox admission or empirical keep/revert rules

#### Scenario: Parked-team ask receives an honest answer

- GIVEN a user asks for spec authoring or software implementation
- WHEN the coordinator evaluates the message
- THEN it emits `respond_direct` explaining that the requested team is not wired in this deployment
- AND it does not emit `create_change`, `proof_readiness`, `dev_from_task`, or `dev_via_test`
