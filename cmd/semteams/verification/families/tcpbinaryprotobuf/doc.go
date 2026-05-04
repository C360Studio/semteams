// Package tcpbinaryprotobuf implements the TCP-based binary-protobuf
// protocol family — concrete sidecars speak protobuf-framed messages
// over TCP. First family in the (family × runtime) matrix; concrete
// harnesses (meshtasticd-3.x in R3.7.2.e) reference this family by
// FullName "tcp.binary-protobuf.v1".
//
// # What sidecars implement this
//
// Anything that exposes one or more TCP ports speaking length-
// prefixed (or otherwise framed) binary protobuf messages from a
// declared message-type vocabulary. Examples planned across the
// project lifetime:
//
//   - meshtasticd (R3.7.2.e first concrete entry)
//   - gRPC services with declared .proto files
//   - Custom protobuf-over-TCP wire protocols (NATS-like, etc.)
//
// The family does NOT cover:
//
//   - HTTP/REST APIs (different family — http.rest.v1, future)
//   - WebSocket message streams (websocket.json.v1, future)
//   - Protocol buffers over UDP / Unix sockets (future variant)
//
// # Validate semantics (R3.7.2.d posture: stub)
//
// ValidateContract is currently a stub that accepts any contract
// shape. The concrete validator lands in R3.7.2.f when the architect
// persona pins the smoke_contract field structure. At that point
// the validator will check:
//
//   - Each scenario.given references a message type that's in the
//     harness's declared real_dependencies + protobuf message-type
//     allowlist
//   - Each scenario.given references a TCP port that's in the
//     harness's exposes.tcp list
//   - Each scenario.then describes an output the family supports
//     observing (driver-side output stream contract)
//
// Until then the registry primitive's job is to PROVE the wiring
// works (a family is registered, the catalog can look it up, the
// runtime can ask "is this family registered?"). Substance follows
// in .f.
package tcpbinaryprotobuf
