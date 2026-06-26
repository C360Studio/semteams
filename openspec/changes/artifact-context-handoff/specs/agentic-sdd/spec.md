# Delta for Agentic SDD

## ADDED Requirements

### Requirement: Artifact Context Handoff
The system SHALL let users reuse emitted artifacts as context for any supported coordinator-routed team without assuming
the next workflow.

#### Scenario: Artifact can be attached to a follow-up prompt
- GIVEN a user is viewing a rendered emitted artifact
- WHEN they choose to use the artifact with a team
- THEN the chat input is seeded with the selected public team command
- AND the artifact is attached as context visible to the user before send

#### Scenario: Handoff remains team-neutral
- GIVEN the artifact is a research artifact
- WHEN the UI offers follow-up actions
- THEN it offers multiple public team commands such as research, create-change, optimize, and dev-via-test
- AND it does not imply that research must feed spec authoring

#### Scenario: Descendant artifact remains reachable
- GIVEN a top-level coordinator task has descendant team loops
- AND a descendant loop emitted a reusable artifact
- WHEN the user opens the top-level task detail panel
- THEN the descendant loop is reachable from the task's sub-task list
- AND the emitted artifact can be expanded and used as context

#### Scenario: Artifact context is sent through coordinator
- GIVEN an artifact context is attached to the chat input
- WHEN the user sends a prompt
- THEN the outbound coordinator message includes the user's editable prompt and the rendered artifact context
- AND the coordinator still validates the requested route before dispatching a specialist team

#### Scenario: User can drop context
- GIVEN an artifact context is attached to the chat input
- WHEN the user clears the context chip
- THEN the artifact is not included in the outbound message
- AND the typed prompt remains available for editing
