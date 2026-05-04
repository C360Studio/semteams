// Package families implements the protocol-family registry — the
// sidecar-side, language-agnostic half of the (family × runtime)
// matrix that backs verification commitments (R3.7.2.d, ADR-033
// §addendum 2026-05-04).
//
// # What's a protocol family
//
// A protocol family describes the WIRE-LEVEL contract a sidecar
// speaks: its schema vocabulary (e.g. protobuf message types), its
// transport (e.g. TCP), its message-framing rules. ONE family is
// shared across MANY concrete sidecars: meshtasticd-3.5,
// meshtasticd-4.0, and any other Meshtastic-protocol harness all
// implement `tcp.binary-protobuf.v1`.
//
// The family is the substance-enforcement layer: the schema validator
// for a family is what stops the architect from writing vague
// "something happens" smoke contracts. The validator looks at a
// proposed contract, walks the catalog entry's declared values
// (allowed protobuf message types, exposed ports, etc.), and rejects
// any reference that doesn't match. Architect can scope WITHIN the
// catalog's declared surface but cannot reach OUTSIDE it.
//
// # Why families are framework-shipped, not chain-authored
//
// ADR-033 §addendum 2026-05-04 captured the load-bearing claim: when
// `harness-via-spec` (R3.7.4) lets the chain author harness catalog
// entries, the chain inherits the family's substance enforcement
// from upstream. The family schema CANNOT be chain-authored — that
// would put the substance-enforcer in the same author chain as the
// thing being verified, restoring Goodhart. Families ship in this
// package; harness catalog entries reference them by name and are
// constrained to what the family permits.
//
// # Slice posture
//
// R3.7.2.d ships:
//   - The Registry primitive (concurrent-safe Register/Get/List).
//   - ONE family: tcp.binary-protobuf.v1, registered with a
//     Validate-able shape but stub validator (concrete contract
//     validation lands in R3.7.2.f when the architect persona pins
//     the smoke_contract field structure).
//
// Future slices add families additively — kafka.topic.v1,
// http.rest.v1, websocket.json.v1 — each shipped as its own subpkg
// under cmd/semteams/verification/families/<name>/. Adding a new
// family is a pure addition; no existing family / runtime / harness
// changes.
//
// # Registry pattern
//
// Mirrors upstream payloadregistry: Pattern-A boot-registry, no KV
// backing (families are framework code, not operator-curated state).
// Wired at boot via families.RegisterAll on a fresh registry; the
// shared registry is then dependency-injected into the harness
// catalog manager / schema gate.
package families
