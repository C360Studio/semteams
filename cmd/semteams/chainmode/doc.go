// Package chainmode stamps the chain.mode triple on the chain entity
// when a coordinator loop terminates with a delegate_* action.
//
// Slice 1b of the coordinator redesign (see
// docs/proposals/coordinator-redesign.md + memory anchor
// project_coordinator_slice1b_pickup). chain.mode classifies the chain
// at dispatch into one of:
//
//   - "research_only"  — set when coordinator emits
//     decide(action=delegate_research). The chain runs plan → gather →
//     synthesize and terminates at reviewer-research; no architect /
//     reviewer-spec / builder / reviewer-qa phases.
//   - "dev_via_spec"   — set when coordinator emits
//     decide(action=delegate_dev_chain). The chain runs the full
//     plan → gather → synthesize → architect → reviewer-spec →
//     builder → reviewer-qa arc.
//
// The triple is the structural authority used by the phase validator
// (cmd/semteams/phasevalidator) to gate the synthesize → architect
// transition. A persona-only fork at synthesize is unreliable under
// real LLM (Slice 1 smoke evidence: 2 of 3 synthesize loops chose
// decide(action=architect) regardless of the spawn prompt's framing).
// Stamping the mode at the coordinator's terminal — the only
// observable instant where the user's intent is classified — gives the
// validator a deterministic read it can refuse.
//
// Design decisions (from the Slice 1b handoff memo):
//
//  1. chain.mode lives on the chain entity, not the loop entity.
//     Downstream loops walk parent → chain via chain.Resolver. The
//     coordinator's loop is the chain root, so chain.mode and
//     chain.dispatched.at land together on the chain entity at
//     coordinator-terminal time.
//
//  2. The stamp is performed by a CompletionHandler, not a rule action.
//     publish_agent properties don't propagate to triples on the
//     spawned loop (per the framing_note in
//     configs/rules/coordinator/01-delegate-research.json) and
//     add_triple stamps to the firing entity (coordinator loop),
//     not the chain entity. A subscriber is the canonical pattern for
//     chain-entity writes (mirror chain.DispatchedStamper +
//     chain.NeedsReviewStamper).
//
//  3. The handler is fail-soft on resolver / entity-read errors. The
//     coordinator's terminal already published; missing chain.mode
//     leaves the downstream chain in pre-1b behaviour (persona-judgment
//     fork at synthesize) rather than dropping the user's request.
//
// Future evolution: when the coordinator gains a third routing class
// (e.g. "review_existing_artifact" — Slice 3 ops feedback consumption),
// add the mode-token here and extend the validator's gate table. The
// stamper itself is action-agnostic — it reads coordinator.next_action
// and maps to the mode via a single switch.
package chainmode
