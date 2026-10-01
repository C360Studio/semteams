// Package approvalpause reflects tool approvals in the product-owned run phase.
// It consumes the framework's pending and response events, resolves the loop's
// run anchor, and reconciles three product predicates on that existing run:
// pending and answered JSON pair sets, plus their derived outstanding count.
// The exact authority revision fences every update, so separate subscribers and
// late responses cannot lose a newer gate. Lifecycle rules retain phase ownership.
//
// Approval facts survive subscriber recreation and remain historical after a
// terminal transition. They never authorize a tool: the framework's current loop
// pending_approval.execution_id remains authoritative for approval admission.
// Core-NATS delivery is still best effort; this subscriber adds no replay worker.
//
// Framework alignment and the public projection-client migration posture are
// documented in ADR-029's 2026-10-01 approval projection addendum. Fresh graph
// storage is required across the migration; legacy loop-reference markers are
// rejected rather than interpreted through a compatibility alias.
package approvalpause
