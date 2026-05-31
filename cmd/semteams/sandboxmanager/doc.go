// Package sandboxmanager implements Layer 2 of the three-layer
// sandbox model per ADR-043: it resolves a typed
// SandboxRequirements (capability contract Coordinator emits) into
// a canonical Profile, runs admission checks, materializes the
// devcontainer via @devcontainers/cli, runs preflight probes, and
// emits an Attestation triple set the Coordinator routes on.
//
// The Coordinator never reads container internals (image,
// privileges, devcontainer features). It reads
// `sandbox.attestation.*` triples — ready/degraded/failed +
// verified capabilities — and routes. Those triples are
// operational state, not domain truth: they live in a separate
// namespace from sandboxfleet's `sandbox.tenant.*` long-lived
// registry, and they age out per-profile TTL.
//
// Three responsibilities, in dependency order:
//
//  1. SandboxRequirements + Validate + Hash. Typed capability
//     contract: languages, tools, services, network, secrets,
//     mounts, privileges, verification probes. Field shapes are
//     domain-shaped (what the work needs) — NOT container-shaped
//     (how to wire it up). Validation pulls forward the PR 3.3
//     reviewer-pass invariants where applicable (probe-command
//     whitespace trim, identifier-shape secret/probe names).
//
//  2. Profile catalog + MatchProfile. Three hand-authored canonical
//     profiles ship under .devcontainer/<name>/devcontainer.json
//     and a Go-side record carries the metadata the matcher needs
//     (advertised capabilities, allowed privileges/network, TTL).
//     MatchProfile scores each profile against the requirements and
//     returns the best fit or a structured no-match error listing
//     unmet capabilities.
//
//  3. Admission + Attestation + Request. AdmissionCheck applies the
//     6 gates from ADR-043 §Decision (privileges, network, mounts,
//     secrets, image_digest). Attest renders probe results into a
//     stable Attestation. Request orchestrates match → admission →
//     up → probes → attest → triple-stamp via a Runner interface
//     (production: shell-out to @devcontainers/cli; tests: fake).
//
// Framework-alignment review (per
// cmd/semteams/tools/README.md "Discipline" section and
// CLAUDE.md "Product-Shell-Tool Discipline"): no upstream
// sandbox/devcontainer primitive exists or is planned in
// semstreams (verified beta.86, 2026-05-29). The package is
// defensible product-shell-local. Realization layer is the
// multi-vendor containers.dev spec and the @devcontainers/cli
// reference implementation — shell-out, not library import — so
// our dependency posture is "pinned external binary," not "Go
// module dep." Migration target: graduate to upstream if a sibling
// product needs the same three-layer model.
//
// Threat model: appropriate for operator-controlled hosts running
// operator-owned workloads. NOT appropriate for adversarial code
// analysis. Admission gates
// (docker-socket, privileged, public network, host-path mounts)
// are first-class because they shape the blast radius; they
// default-deny and route through the admission-reviewer approval
// seam (existing tool-approval pattern) when a request needs an
// exception.
package sandboxmanager
