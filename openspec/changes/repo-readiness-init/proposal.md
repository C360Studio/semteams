# Proposal: Repo Readiness Initialization

## Intent
SemTeams should support a post-MVP initialization step that inventories a repository or folder, classifies its shape,
and proposes the proof harnesses needed before expensive spec-driven development begins. This captures the "prepare the
ground" loop without treating missing infrastructure as something the coordinator magically infers during implementation.

## Scope
In scope:
- A repo-scoped readiness command for dev workflows, tentatively `/init-repo <path>`.
- Repository and folder inventory through governed tool surfaces.
- Repo-shape classification, such as Go service, Svelte app, Java/Gradle, OpenSensorHub addon, or MAVLink/OSH hybrid.
- Draft harness profiles, readiness dependencies, smoke commands, and evidence expectations.
- OpenSpec changes that describe harness/tooling work required before future implementation specs can run safely.
- Human review and approval before harness-building or repository mutation.
- OSH-shaped repo dogfood coverage as the first hard-domain target.

Out of scope:
- Making generic `/init` the default command for every SemTeams category.
- Mutating repository files during the initial inventory step.
- Claiming a repo is implementation-ready when harness profiles are missing, rejected, stale, or unapproved.
- Replacing `create_change`, `proof_readiness`, `test_harness`, or `dev_from_task` with a second control plane.
- Building arbitrary cloud or cluster infrastructure without explicit operator approval.

## Approach
Model initialization as `repo_readiness`, a post-MVP category pack that produces governed graph facts and an OpenSpec
harness plan. The coordinator routes `/init-repo <path>` or an equivalent UI action to inventory the repository, classify
its shape, and emit candidate `proof.dependency.*`, `proof.harness_profile.*`, and readiness expectations. If required
harnesses are missing, SemTeams creates a reviewed OpenSpec change for building them. Only after approval should a
separate harness-bootstrap action create files, run tooling, or produce readiness records.

Command naming should stay scoped. `/init-repo` is the explicit dev-workspace command. `/init` can be reserved as a
future contextual namespace or alias only when the UI can disambiguate repo initialization from other SemTeams product
uses, such as research corpus setup, operations monitoring setup, or domain-specific onboarding.
