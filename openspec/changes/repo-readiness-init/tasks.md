# Tasks

## 1. Command And Routing
- [ ] 1.1 Decide final command naming for repo readiness, with `/init-repo <path>` as the scoped default and `/init` only
  as a contextual alias if disambiguation is available.
- [ ] 1.2 Add coordinator routing for a `repo_readiness` category without creating a new control plane.
- [ ] 1.3 Add UI affordances for Initialize repo, resolved path confirmation, and non-dev initializer disambiguation.

## 2. Inventory And Classification
- [ ] 2.1 Implement read-only repo inventory for languages, package managers, test commands, container support, and
  framework/product markers.
- [ ] 2.2 Add repo-shape classification for Java/Gradle, OpenSensorHub addon, MAVSDK/MAVLink, and Connected Systems API
  markers.
- [ ] 2.3 Emit `repo_readiness.*` facts plus candidate `proof.dependency.*` and `proof.harness_profile.*` facts.

## 3. Harness Plan Production
- [ ] 3.1 Produce a reviewed OpenSpec harness plan when required harnesses are missing or unproved.
- [ ] 3.2 Require human approval before any harness-bootstrap action mutates repo files or installs tooling.
- [ ] 3.3 Ensure draft harness profiles feed `proof_readiness` as blockers until readiness records or waivers exist.

## 4. OSH Dogfood
- [ ] 4.1 Run the initializer against an OSH-shaped repo/folder and produce a harness plan for Gradle build, OSH lifecycle,
  CS API smoke, MAVLink/PX4 SITL/MAVSDK, and plugin coverage.
- [ ] 4.2 Export the harness OpenSpec plan as a handoff to Codex, Claude Code, or a human implementer.
