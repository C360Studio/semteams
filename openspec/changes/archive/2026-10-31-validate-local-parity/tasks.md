## 1. Approval

- [x] 1.1 Owner approves `proposal.md` and `design.md` by filling `Approved-by: <name> (<YYYY-MM-DD>)` and removing
  this hold in the same edit, ruling on design Open Question 1 (#393). Evidence: the owner approved in Enjoy on
  2026-10-31 ("approve D1 A + D2 A and finish #394"); Claude wrote the header at their request.

## 2. Re-cite

- [x] 2.1 Point the `// spec:` citations on `TestValidateScriptCommandCompositionAndWorktreePreservation` and
  `TestValidateTaskAndDocsContract` at the new Requirement. Acceptance: `task test:spec` passes, and both tests still
  pass. Evidence: both citations name the new Requirement; `task test:spec` and `go test ./test/contract/...` pass.

## 3. Spec

- [x] 3.1 Apply the `repository-governance` delta. Acceptance: `task spec:check` and `task validate` pass on a clean
  tree. Evidence: `openspec archive validate-local-parity` synced the delta; `task validate` passed on that clean tree.
