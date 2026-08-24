# Tasks

## 1. Coordinator Contract

- [x] 1.1 Keep the coordinator as the single conversational and dispatch front door.
- [x] 1.2 Restrict category dispatch to `research` and `autoresearch`, with `respond_direct` and `ask_user` as terminal
  front-door actions.
- [x] 1.3 Treat research and optimization prefixes as intent hints that still require contract validation.
- [x] 1.4 Answer spec-authoring and implementation asks honestly through `respond_direct`; never spawn parked teams.

## 2. Evidence And Reconciliation

- [x] 2.1 Cover direct response, clarification, research, autoresearch, and parked-team honesty in the routing matrix.
- [x] 2.2 Reconcile proposal, design, tasks, and the additive spec delta with ADR-058/059 current wiring.
- [x] 2.3 Run strict OpenSpec validation and the focused process/contract checks on the branch.
