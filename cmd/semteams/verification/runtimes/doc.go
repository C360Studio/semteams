// Package runtimes implements the test-runtime registry — the
// driver-side, per-language half of the (family × runtime) matrix
// that backs verification commitments (R3.7.2.d, ADR-033 §addendum
// 2026-05-04).
//
// # What's a runtime
//
// A runtime describes how the chain's code under test exercises a
// sidecar in its OWN language ecosystem. The runtime knows:
//
//   - The invocation command the builder runs to execute tests
//     (e.g. `mvn verify -P<harness-profile>`, `go test ./...`,
//     `npx playwright test`).
//   - The per-family templates that turn a smoke contract into
//     real test code in this language. Each (family × runtime)
//     cell of the matrix is one template owned by the runtime.
//
// A meshtasticd-3.x catalog entry can be exercised by a Java driver
// (java-junit-testcontainers runtime), a Go driver (go-testing-net
// runtime, future), or a Node.js driver (node-vitest-supertest,
// future) — same sidecar, different test code rendered from the
// runtime's per-family template.
//
// # Why runtimes are framework-shipped
//
// Same load-bearing claim as families: runtime templates can NOT
// be chain-authored. If `harness-via-spec` (R3.7.4) writes a new
// catalog entry, it inherits the runtime's template substance from
// upstream. Chain authors entries; framework supplies templates.
// The Goodhart vector (architect writes vague contract → builder
// writes assertTrue(true) JUnit) closes only when the test code is
// template-rendered, not LLM-authored.
//
// # Slice posture
//
// R3.7.2.d ships:
//   - Registry primitive (concurrent-safe Register/Get/List).
//   - ONE runtime: java-junit-testcontainers, registered with
//     metadata (invocation command) but EMPTY templates map.
//     Concrete templates land in R3.7.2.f when the architect
//     persona pins the smoke_contract shape.
//
// Future runtimes are pure additions:
//   - go-testing-net (Go testcontainers-go pattern)
//   - node-vitest-supertest (Node test runner against HTTP/TCP)
//   - python-pytest-asyncio (Python async test runner)
//   - playwright-typescript (browser-flow runtime, R3.8+)
//
// Each ships as cmd/semteams/verification/runtimes/<name>/ with its
// own package, registration function, and per-family template files.
package runtimes
