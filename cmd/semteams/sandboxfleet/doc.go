// Package sandboxfleet implements the multi-tenant sandbox fleet
// substrate per ADR-042 §addendum 2026-05-29 §A.
//
// The package is the product-shell foundation under the
// sandbox-bootstrap category pack and the autoresearch category pack:
// the bootstrap pack provisions/reuses tenant containers keyed by a
// stable target signature, and autoresearch arcs reach into the same
// fleet for sustained-state measurement.
//
// Three responsibilities, in dependency order:
//
//  1. TargetSignature + Canonicalize + Hash. The deterministic
//     Go canonicalizer is the cache key for tenant reuse. Two users
//     describing the same target two different ways must produce the
//     same Hash (no waste); materially-different targets must produce
//     distinct Hashes (no silent collision). LLM-prose normalization
//     cannot guarantee either — prose normalization is
//     non-deterministic by construction. Per ADR-042 §addendum review
//     H2 fix: typed canonicalizer in Go, not in the plan persona.
//
//  2. TenantRegistry. Entity-namespaced state store under the canonical
//     6-part entity ID {org}.{platform}.sandbox.tenant.execution.{sig}
//     — sibling to {org}.{platform}.agent.agentic-loop.execution.{loop}
//     and {org}.{platform}.agent.chain.execution.{chain}. State
//     transitions: nil → provisioning → ready_running ↔ ready_stopped,
//     with stale + evicting terminals. The registry is read by
//     query_sandbox_tenant and mutated by the emit_bootstrap_* tools.
//     Entity-namespace storage (ADR-042 §F-5 Option B) reuses the
//     graph substrate; tenant history threads with chain history.
//
//  3. (Deferred to a follow-up PR.) A Go-level Docker SDK wrapper.
//     The ADR-042 §addendum §A names Provision/Exec primitives, but
//     all Docker work invoked by the sandbox-bootstrap rule pack
//     today flows through the chain-scoped bash tool driven by the
//     provisioner-bootstrap-{plan,execute,verify} personas: each
//     persona issues `docker run`, `docker exec`, etc. via bash, and
//     the tenant container's docker socket is mounted in for
//     testcontainers-using targets. No tool shipping in PR 1
//     requires a Go-level Docker call. The wrapper lands when a
//     real substrate-side caller surfaces (programmatic auto-GC, a
//     future Go-driven defense-in-depth check in
//     emit_bootstrap_committed, etc.). Document the migration path
//     in an addendum at that point.
//
// Framework-alignment review (per cmd/semteams/tools/README.md and
// CLAUDE.md "Product-Shell-Tool Discipline"): no upstream
// "tenant container fleet" primitive exists or is planned in
// semstreams (verified beta.86, 2026-05-29). The package is
// defensible product-shell-local. Migration target: graduate to
// upstream if semspec / semdragons gain similar multi-tenant
// containerization needs.
//
// Threat model: this substrate is appropriate for substrate-driven
// measurement against operator-owned repos on an
// operator-controlled host. It is NOT appropriate for adversarial
// code analysis: LLM-authored install_steps have docker-CLI surface
// inside the tenant (the socket is mounted), and compromised
// upstream packages execute install scripts with the docker socket
// in scope. Per ADR-042 §addendum §A H1 threat model — operators
// should only point this at targets they would run on their own
// laptop.
package sandboxfleet
