# Delta for Agentic SDD

## ADDED Requirements

### Requirement: Conversational Front Door
The system SHALL let users converse with the coordinator before a specialist team is dispatched.

#### Scenario: Product question answers directly
- GIVEN a user asks what SemTeams can do or how to start
- WHEN the coordinator evaluates the message
- THEN it emits `decide(action="respond_direct")` with a user-facing answer
- AND no specialist team is spawned unless the user makes a team-shaped request

#### Scenario: Vague idea is refined before dispatch
- GIVEN a user describes an idea that is not yet shaped for research, spec authoring, optimization, or implementation
- WHEN a short answer or one clarifying question would help shape the next step
- THEN the coordinator either emits `respond_direct` with guidance and team options or emits `ask_user` with one concrete
  question
- AND it does not dispatch a category merely because the text resembles a supported topic

#### Scenario: Clear team-shaped request still routes
- GIVEN a user clearly asks for evidence research, OpenSpec authoring, scalar optimization, or implementation with tests
- WHEN the coordinator evaluates the message
- THEN it emits the matching category action
- AND the normal sandbox, readiness, approval, and review gates still apply

## MODIFIED Requirements

### Requirement: Command Shortcut Surface
The system SHALL provide contextual slash-command shortcuts for product-level team intent, spec export, implementation
dispatch, run status, evidence review, and approval control without creating a separate workflow control plane.

(Previously: The system SHALL provide contextual slash-command shortcuts for spec export, implementation dispatch, run
status, evidence review, and approval control without creating a separate workflow control plane.)

#### Scenario: Team command is a coordinator-routed hint
- GIVEN a user enters a product-level team command such as `/research`, `/create-change`, `/optimize`, or `/dev-via-test`
- WHEN the message reaches SemTeams
- THEN the coordinator receives the full prompt and validates whether it matches the requested team's contract
- AND the command does not bypass coordinator routing, sandbox checks, proof readiness, approvals, or clarification policy

#### Scenario: Mis-shaped command asks or redirects
- GIVEN a user invokes a team command with a prompt that is missing required facts or belongs to a different team
- WHEN the coordinator evaluates the prompt
- THEN it asks one clarifying question or responds with the better route
- AND no category team is spawned until the request is shaped for that team's contract

#### Scenario: Internal roles remain hidden
- GIVEN a user is using slash commands or action chips
- WHEN the UI lists available team shortcuts
- THEN it shows product-level categories only
- AND it does not expose internal phase roles such as Lisa, Ralph, CBG, or reviewer agents as direct commands
