// Package javajunittestcontainers implements the
// Java + JUnit 5 + Testcontainers test runtime — first runtime in the
// (family × runtime) matrix, paired with tcp.binary-protobuf.v1 as
// the first matrix cell when smoke contracts arrive in R3.7.2.f.
//
// # What this runtime does
//
// Drives integration tests for Java drivers against operator-curated
// sidecar harnesses. Builder runs `mvn verify -P<harness-profile>`
// from inside the sandbox; the rendered template lands as a JUnit 5
// test class that:
//
//   - Uses Testcontainers' GenericContainer to wait-for-port the
//     sidecar declared by the harness's compose_profile (lifecycle
//     management is the runtime's job, not the LLM's).
//   - Opens a TCP connection to the harness's exposed port.
//   - Writes the protobuf message types the smoke contract names
//     (drawn from the catalog's declared real_dependencies allowlist).
//   - Asserts the driver's output matches the contract's `then`
//     clause structurally.
//
// The LLM never writes JUnit code directly. The architect pins the
// smoke contract; the runtime's per-family template renders it into
// real JUnit. That removes the assertTrue(true) Goodhart vector
// surfaced by R3.6.2.g.
//
// # Slice posture
//
// R3.7.2.d ships:
//   - The Runtime interface implementation (Name, Description,
//     InvokeCommand metadata).
//   - SupportedFamilies returns []string{} — no templates wired yet.
//   - TemplateFor returns ("", false) for any family.
//
// R3.7.2.f wires the actual template for tcp.binary-protobuf.v1
// after the architect persona pins the smoke_contract structure.
// At that point the matrix cell goes live and ValidateContract +
// TemplateFor return concrete substance.
package javajunittestcontainers
