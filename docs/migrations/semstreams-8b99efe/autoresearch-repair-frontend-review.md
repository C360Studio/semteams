# Independent autoresearch repair browser review

The independent frontend reviewer authored no UI, Playwright, fixture or runner changes in this repair.
**Assertion and fixture review approved, with no blocking findings.** Browser execution remains a separate gate;
this review does not replace the recorded outcomes or expand the existing product claims.

## Reviewed scope

- `ui/e2e/agentic/autoresearch_delivery.ts`
- `ui/e2e/agentic/autoresearch.spec.ts` and `autoresearch-guardrails.spec.ts`
- `ui/e2e/agentic/autoresearch-exit.spec.ts`
- Three mock exit fixtures: descended clarification, descended limitation status, and propose failure
- The corresponding migration matrix entries

The shared assertion correlates the actual submitted root, published terminal task, source loop and final typed
response. It requires native `ParentLoopID` and `RunID` equal to the root UUID, the full source entity reference,
expected allowlist, original HTTP channel/user, one terminal task and one response on that channel. Submission status
and unrelated logger traffic cannot satisfy the result check. Authoritative run triples confirm one run, its actual
origin, and the expected phase. This is bounded observation during the journey, not transactional exactly-once delivery.

The browser opens the original task, selects the actual terminal child, expands its Message log evidence, and compares
the rendered raw payload with the observed typed response. This proves inspection of the delivered envelope through
the existing UI. It does not prove rich artifact/model body rendering or close evidence-body limitation #261.

The main journey preserves successful artifact-emitter and graph-path proof before the stronger route checks.
Guardrails preserves its rejected metric-win assertions and now independently correlates the initial clarification
prompt instead of counting submission status or unrelated messages returned by an unsupported query filter.

The exit fixtures exercise actual advertised tools. The failure case repeats scratchpad until the real loop budget
is exhausted, asserts failed state/outcome/budget, and waits for the terminal ops observer. It excludes experiments,
artifacts and accidental off-policy tool errors. The clarification cases prove both supported coordinator decisions:
`ask_user` leaves the run awaiting approval, while `respond_direct` preserves the existing executing-phase limitation
policy. They do not relabel a limitation response as completed optimization. Neither continuation nor recovery after
that response is qualified by these fixtures.

## Gate and evidence boundary

The frontend author reports unchanged product UI source and passing lint, type check, unit tests and build, with
3,704 tests passed and 12 skipped. E2E files additionally passed direct TypeScript and ESLint checks. These logs are
under `/tmp/semteams-migration-8b99efe/repair/`; the final migration report must retain the completed browser results
and exact image/configuration/source identities separately. Earlier failed delivery evidence remains historical red
rather than being overwritten by the repair.

No new frontend behavior, test skip, longer deadline, retry-to-green policy, or fabricated graph fact is introduced
by these assertions. Static fixture provenance remains mock evidence. SemSource dogfooding, SemEngine acceptance,
program-pulse readiness and #261 remain separate boundaries.

## Observed focused execution

The [five focused repair cases](browser-evidence/repair-focused/results.json) all pass with source hashes unchanged
between build and execution. They exercise the actual graph owner and loop runtime in addition to the reviewed
assertions. Expanded raw Message log payloads match the delivered envelopes. Existing right-rail clipping means the
screenshots alone do not show every payload line; DOM payload assertions are the retained content proof. The broader
matrix is recorded separately, and neither result expands the evidence-body or live-model claims above.

The accepted [full stable matrix](browser-evidence/repair-final-stable/results.json) also passes all 24 active
scenarios, with five explicit skips, no failures and unchanged source hashes. The earlier
[invalidated attempt](browser-evidence/repair-final/INVALID-source-mutation.json) remains an orchestration failure;
its passing assertions were not used to waive the source guard. The rerun uses a fresh image and unchanged
assertions/deadlines. These recorded outcomes supplement the independent review; no additional UI implementation
was introduced during the repair.
