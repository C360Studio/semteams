// Package sandboxruntime hosts chain-scoped runtime adapters that
// route bash tool calls into the right execution environment for the
// calling chain.
//
// # The shell vs the workspace
//
// Two sandbox surfaces with distinct purposes — the heuristic any
// future maintainer needs on first read:
//
//	┌──────────────────────────┬────────────────────────────────────┐
//	│ Surface                  │ What it's for                      │
//	├──────────────────────────┼────────────────────────────────────┤
//	│ The shell                │ Coordinator's quick peeks.         │
//	│ (always-warm, shared)    │ Persona scratchpad dumps.          │
//	│ One per backend; generic │ Anywhere state doesn't matter and  │
//	│ toolchain; chain-scoped  │ reproducibility doesn't either.    │
//	│ workspace bucket via     │                                    │
//	│ task_id.                 │                                    │
//	├──────────────────────────┼────────────────────────────────────┤
//	│ The workspace            │ The chain's actual workplace.      │
//	│ (per-tenant devcontainer)│ Anywhere the chain mutates state,  │
//	│ Provisioned by           │ runs measured commands, depends on │
//	│ request_sandbox.         │ baseline conditions across         │
//	│ Profile-matched,         │ iterations (autoresearch is the    │
//	│ capability-verified,     │ paradigm case), needs a            │
//	│ admission-gated,         │ profile-verified toolchain, needs  │
//	│ isolated host filesystem.│ capability isolation.              │
//	└──────────────────────────┴────────────────────────────────────┘
//
// Today upstream's BashExecutor holds one fixed runner pointing at
// the always-warm shared sandbox. Every chain-scoped bash call goes
// there, regardless of whether the coordinator already provisioned
// an attested per-tenant container. This package closes that gap:
// AttestationRunner reads the chain entity for
// `sandbox.attestation.host_workspace_folder` (stamped by
// sandboxmanager.Manager.Request after a successful `devcontainer
// up`) and, when present, wraps each bash command in
// `devcontainer exec --workspace-folder <wsf> bash -lc <cmd>` before
// delegating to the default runner. The wrapped invocation runs INSIDE
// the per-tenant container; the outer transport remains the always-
// warm sandbox's /exec endpoint (which is where devcontainer-cli
// lives in the DooD topology).
//
// Wire shape, end-to-end:
//
//	chain bash tool
//	   ↓
//	chainbash (rewrites task_id → chain_id)
//	   ↓
//	upstream BashExecutor with WithRunner(AttestationRunner)
//	   ↓
//	AttestationRunner.Exec(ctx, chainID, command, timeoutMs)
//	   ├─ reads {org}.{platform}.agent.chain.execution.{chainID}
//	   │       for sandbox.attestation.{ready,host_workspace_folder}
//	   ├─ if found+ready+wsf:
//	   │       wraps `cmd` → `devcontainer exec --workspace-folder <wsf>
//	   │                       bash -lc 'cmd'`
//	   └─ delegates to defaultRunner (upstream runner.Client →
//	      always-warm sandbox /exec → host devcontainer-cli →
//	      per-tenant devcontainer)
//
// Fall-back contract: ANY failure reading the chain entity (entity
// missing, no attestation triples, ready=false, host_workspace_folder
// empty, network error) → passthrough with the original command.
// Two design properties this preserves:
//
//   - **Single-loop chains** (the dispatch coordinator alone, no
//     request_sandbox) — no attestation triple ever lands; passthrough
//     hits the always-warm sandbox as today.
//
//   - **Admission-denied chains** (request_sandbox returned terminal
//     without bringing up a container) — Phase A's omit-when-empty
//     discipline on host_workspace_folder means the triple is absent
//     entirely. Passthrough is the correct behavior; there is no
//     per-tenant container to route to.
//
// AttestationRunner does NOT speculate: when in doubt, the always-
// warm sandbox is the safe default. The cost of a wrongful passthrough
// is "command ran in the wrong sandbox"; the cost of a wrongful wrap
// is "command failed opaquely inside an unprepared devcontainer." The
// asymmetry justifies the conservative fallback.
//
// Phase D bundles the runner with `executors.WithRunner(...)` once
// upstream's Runner interface tags in the next semstreams beta. Until
// then this package is built but unwired — Phase B leaves the
// integration seam for Phase D to close in one commit.
package sandboxruntime
