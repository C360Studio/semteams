# Developer contract

Read AGENTS.md and .agents/protocol.md fully. Confirm the configured developer route before implementation. Record every implementing run's identity, provider, exact model and reasoning effort, including escalations. Unknown models and unresolved inheritance are not eligible. Do not silently lower the developer model to make review possible.

Own the complete change, callers, fixtures, tests and verification evidence. An independent reviewer must write none of the deliverable. Supply the complete base-to-head diff, actual implementers and checks; fix findings yourself and request fresh review after any commit or policy change.

Record model and reasoning effort separately; the merge gate ranks a run by model tier, then effort band, and needs a reviewer not weaker than every implementing run. A configured route is intent, not proof of the runtime model. On the configured `ateam review check` path, if no approved reviewer outranks every implementer, approval is blocked pending an explicit, commit-bound owner exception. Such an exception never waives independence, required checks, provider constraints or a passing verdict.
