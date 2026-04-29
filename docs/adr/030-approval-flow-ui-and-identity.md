# ADR-030: Approval-Flow UI and the X-User-Id Identity Seam

## Status

Accepted — 2026-04-29

## Context

Semstreams beta.19 closed the agent-loop side of the
`approval_required` gate: when the agentic-tools `ApprovalFilter`
rejects a tool call with the documented `approval_required: ...`
prefix, the agentic-loop now pauses, snapshots the pending call
on `LoopEntity.PendingApproval`, emits an `ApprovalPendingEvent`
on `agent.approval_pending.<loop_id>`, and resumes only on a
matching `ApprovalResponse` on `agent.approval_response.<loop_id>`.
Before beta.19 the rejection bled back into the LLM round-trip
and the gate was effectively advisory.

Beta.22 added the canonical HTTP entry point for the resolution
side: `POST /loops/{id}/approval` on `agentic-dispatch`, mounted
at `/teams-dispatch/loops/{id}/approval` for semteams. Body shape
`{decision, modified_arguments?, reason?, user_id?}`, decision
∈ approve|reject|modify, status codes 200/400/404/409/500, plus
the `IdentityFromRequest(r, fallbackBody)` helper with
resolution order ctx > body.user_id > "http-user".

Beta.23 added `service.HTTPMiddleware` and
`Manager.UseHTTPMiddleware(...)` so products can plug auth /
panic-recovery / identity middleware into every framework HTTP
route at boot.

The remaining gap was UI-side: TaskDetailPanel had Approve / Reject
buttons that misrouted to `/signal`, agentStore exposed
`awaitingApproval` but no resolution path, and there was no
client publisher for `agent.approval_response.*`. Anything
relying on `approval_required` for human-in-the-loop safety
(rule writes, persona changes, anything in the e2e-agentic
config's `approval_required` allowlist) had no end-to-end story.

## Decision

Three product-shell pieces wire the gap closed without changing
the framework:

1. **`cmd/semteams/middleware.go`** — `xUserIDIdentityMiddleware`
   reads the `X-User-Id` header, sanitises it (trimmed,
   printable-ASCII-only, length ≤ 256), and lifts the value into
   the agentic-dispatch identity ctx via
   `agenticdispatch.WithIdentity`. Wired through
   `manager.UseHTTPMiddleware(productMiddleware()...)` in
   `main.go` step 12, before `runWithSignalHandling`. The
   product-shell middleware seam (a slice of `service.HTTPMiddleware`)
   stays single-source-of-truth in `productMiddleware()` so future
   chain entries (panic recovery, request logging, real auth) plug
   in at one named extension point.

2. **`ui/src/lib/stores/userIdentity.svelte.ts`** — module-level
   `$state`-backed identity store seeded from `localStorage`,
   default `"ui-anonymous"`. Updates persist; the default value
   is never written to storage. The `agentApi.submitApproval`
   client reads from this store to populate the `X-User-Id`
   header on every approval submission. The body's `user_id`
   field is set to the same value as a fallback for deployments
   that skip the middleware.

3. **`ui/src/lib/components/board/PendingApprovalSection.svelte`** —
   renders the gated tool's name + reason + JSON arguments + the
   approve / reject / modify controls. The Modify path opens a
   JSON editor pre-filled with the current arguments, validates
   (must be a JSON object), and POSTs `decision: "modify"` with
   the parsed `modified_arguments`. Mounted from `TaskDetailPanel`
   above the action-buttons row when `state === "awaiting_approval"`
   and `task.primaryLoop.pending_approval` is populated. The
   prior in-header Approve / Reject buttons that misrouted to
   `/signal` are removed; Cancel remains as the
   abandon-the-whole-task escape hatch.

## Identity is forgeable on the wire

`X-User-Id` is a plaintext claim. Anyone with browser access can
change the localStorage value and submit any identity. Per
upstream beta.19's security model, NATS publish on
`tool.execute.>` is the actual safety boundary; the approval flow
is a coordination mechanism, not an authorization mechanism.
ADR-030 Phase 3 upstream tracks the bypass-token threat model
(server-side per-call tokens that the executor consumes, replacing
the wire-trusted `ApprovedBy` field).

What this product shell guarantees today:

- The middleware is the only sanctioned identity-injection point.
  Future contributors adding panic-recovery / request-logging /
  auth middleware extend `productMiddleware()` rather than calling
  `manager.UseHTTPMiddleware` from elsewhere — the chain stays
  inspectable at one site.
- Header sanitisation rejects control characters and over-length
  values at the boundary, so log injection and metric-cardinality
  attacks via crafted headers fail closed.
- The UI's `submitApproval` call always sends both the header and
  the body fallback. A deployment without the middleware still
  resolves the caller via `IdentityFromRequest`'s body path; a
  deployment with the middleware sees the header win per upstream
  precedence (ctx > body > default).

What this product shell does NOT guarantee:

- Authentication. Real auth (OAuth / JWT / mTLS) belongs in a
  middleware that runs OUTSIDE `xUserIDIdentityMiddleware` and
  overwrites or rejects the header before the framework reads
  it. Today there is no such middleware; the binary trusts
  whatever browser hits it.
- Authorisation. Role-gated approvals (e.g., only admins can
  approve `delete_rule`) are not enforced. Add validation in
  product middleware before the dispatch handler runs.

## Consequences

- The semstreams beta.19 → beta.22 → beta.23 chain is consumed
  end-to-end. Tools listed in `agentic-tools.approval_required`
  pause the loop, surface in the UI, and resolve via human
  decision.
- The e2e-agentic.json journey `tool-approval-gate` is
  re-enabled. Prior skip cited a framework-level gap that closed
  in beta.19; the secondary skip (UI-side) closed in this branch.
- Cancel during `awaiting_approval` still uses
  `sendSignal("cancel")` — the loop-cancel path bypasses the
  approval gate, terminates the loop, and discards the pending
  approval. Approve/Reject/Modify resolve the gate; Cancel
  abandons the task.
- The userIdentity store has no UI for setting the value today.
  A later product decision (a settings panel, a `/whoami` slash
  command, an auth integration) will surface it. Until then any
  tab-aware tester can `localStorage.setItem("semteams.userIdentity", "...")`
  to test multi-approver scenarios.

## Future work

- Real auth middleware that authenticates the caller and either
  overwrites `X-User-Id` server-side or rejects requests without
  a verified identity. This belongs in this product shell, not
  upstream — the framework stays neutral on auth shape per ADR-030
  upstream.
- A settings UI for the identity value (and any future product
  preferences). Today the store is keyboard-accessible only via
  devtools.
- Surface the upstream `approval_timeout` config as a
  per-deployment setting once the auto-reject timer is wired in
  semstreams. Until then the timeout is operator-policy at the
  config layer.
