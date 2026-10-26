## 1. Record the decision

- [x] 1.1 Add ADR-064 for rule-pack coordination, source observation, runtime/substrate ownership, and explicit current-versus-target guarantees.
- [x] 1.2 Align the product roadmap and link ADR-064 outside the product document's References section.

## 2. Admit a bounded follow-up

- [x] 2.1 Specify the first GitHub issue polling proof and its limits, using the existing `github_read` cursor behavior as evidence rather than promising generic execution durability.
- [x] 2.2 Record design admission for explicit scope, bounded inter-manager work, untrusted-input fencing, and redaction before future implementation claims.

## 3. Verify and review this documentation

- [x] 3.1 Run OpenSpec validation and the applicable docs-only local verification; both passed on design commit `9d22a25e1c89436808700a9b009653d3121902b6` (`task validate`: Go 225s, UI 46s, Governance 10s; local `task openspec:validate` and `task openspec:queue-test` passed).
- [x] 3.2 Obtain independent design/document review and resolve findings ([review record](https://github.com/C360Studio/semteams/pull/358#issuecomment-6415156733): APPROVE on `9d22a25e1c89436808700a9b009653d3121902b6`, independent docs lane PASS, no findings).
- [x] 3.3 Archive this docs-only change and synchronize its accepted design requirement (`openspec archive event-driven-coordination-design --yes`: one requirement added to `openspec/specs/coordination-design/spec.md`).
