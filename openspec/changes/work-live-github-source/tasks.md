## 1. Claim and design

- [x] 1.1 Open the claim for issue #319 on `gemini/work-live-github-source` with `proposal.md`, `design.md`,
      `specs/observable-work/spec.md`, and `tasks.md` as the first content commit.
- [x] 1.2 `architect` design D1–D3 recorded in `design.md`: read-only GitHub REST client (`githubSource.ts`), PR claim
      and review-record comment derivation with short-SHA matching and `review_inspected` honesty, and
      `configs/portfolio-c360.json`.

## 2. Live GitHub WorkSource and baton derivation

- [ ] 2.1 Extend `PullRequestRef` (`review_inspected?: boolean`) and `deriveReviewOverlay()` in
      `ui/src/lib/server/work/baton.ts` to support short-SHA prefix matching and return `unknown` when PR review
      comments were not inspected.
- [ ] 2.2 Extend `ui/src/lib/server/work/config.ts` with `githubToken` (`GITHUB_TOKEN`) and `githubApiUrl`
      (`GITHUB_API_URL`, default `https://api.github.com`) and implement `createGithubSource()` in
      `ui/src/lib/server/work/githubSource.ts` (wired through `source.ts` and `service.ts`).
- [ ] 2.3 Add `configs/portfolio-c360.json` covering `C360Studio/semteams`, `C360Studio/semengine`, and
      `C360Studio/semstreams`.
- [ ] 2.4 Add unit and service tests in `ui/src/lib/server/work/githubSource.test.ts` and update
      `ui/src/lib/server/work/service.test.ts` and `ui/src/lib/server/work/baton.test.ts`.

## 3. Review and archive

- [ ] 3.1 Run `svelte-reviewer` on the changes and resolve any findings.
- [ ] 3.2 Archive `work-live-github-source` (`openspec archive work-live-github-source` and spec sync) as the landing
      PR's final content commit.
