## 1. Approval

- [ ] 1.1 Owner approves `proposal.md` and `design.md` by filling `Approved-by: <name> (<YYYY-MM-DD>)` and removing
  this hold in the same edit, ruling on design Open Question 1 (#393).
  Hold: OPEN-Q proposal awaits owner approval

## 2. Re-cite

- [ ] 2.1 Point the `// spec:` citations on `TestValidateScriptCommandCompositionAndWorktreePreservation` and
  `TestValidateTaskAndDocsContract` at the new Requirement. Acceptance: `task test:spec` passes, and both tests still
  pass.

## 3. Spec

- [ ] 3.1 Apply the `repository-governance` delta. Acceptance: `task spec:check` and `task validate` pass on a clean
  tree.
