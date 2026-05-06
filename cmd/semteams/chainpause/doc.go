// Package chainpause implements the v1 chain-pause primitive from ADR-037.
//
// When any agentic loop in a managed arc (dev-via-spec-*, research-arc,
// dispatch researcher) fails before emitting its coordinator.next_action triple,
// no downstream rule fires and the chain dies silently. This package closes that
// gap by:
//
//  1. Subscribing to agent.failed.* events and writing the full ADR-037 §D5
//     audit triple set (chain.paused.cause, classification, error_shape, etc.)
//     onto the failed loop's entity.
//  2. Accepting operator decisions (retry / kill / defer) via an HTTP endpoint
//     and dispatching accordingly.
//
// # Scope (v1 only)
//
// v1 wires only the "operator" decision authority. The validator rejects:
//   - Reserved verbs: apply_fix_and_retry (v2)
//   - Reserved authority values: coordinator (v2), auto (v3)
//
// # Relationship to the rule engine
//
// The rule processor (configs/rules/chain/00-loop-failed-pause.json) fires on
// the KV watch path and stamps a chain.paused marker triple immediately. This
// package's Pauser fires on the NATS agent.failed.* subject in parallel and
// writes the richer §D5 schema. Both write on the same failed-loop entity.
//
// # Migration target
//
// Product-local until a second product (semspec, semdragons) needs the same
// shape. The framework-alignment review (ADR-037 §"Relationship to prior ADRs")
// found no upstream equivalent. If the pattern recurs, propose upstreaming the
// pause-and-decide primitive via the semstreams issue tracker before adding a
// second copy in a sibling product.
package chainpause
