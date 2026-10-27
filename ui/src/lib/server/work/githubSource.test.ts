// @vitest-environment node
import { readFile } from "node:fs/promises";
import { resolve } from "node:path";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Repository } from "$lib/types/work";
import {
  createGithubSource,
  extractClaimedIssueNumbers,
  extractIssueDependencies,
  GITHUB_PAGE_LIMIT,
  MAX_PR_COMMENT_PAGES,
  parseReviewComments,
} from "./githubSource";
import { validatePortfolio } from "./portfolio";
import { stubBackend } from "./testkit";

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

const REPO: Repository = { owner: "C360Studio", name: "semteams" };

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("githubSource parsing helpers", () => {
  it("extracts blocked-by and task-list dependencies and normalizes bare issue refs", () => {
    const body = [
      "Blocked by #314, C360Studio/semengine#44",
      "Depends on: #315",
      "- [x] #313 shared protocol",
      "- [ ] C360Studio/semstreams#1007 upstream counter",
    ].join("\n");

    expect(extractIssueDependencies(body, "C360Studio/semteams")).toEqual([
      { ref: "C360Studio/semteams#314", source: "blocked-by" },
      { ref: "C360Studio/semengine#44", source: "blocked-by" },
      { ref: "C360Studio/semteams#315", source: "blocked-by" },
      { ref: "C360Studio/semteams#313", source: "task-list" },
      { ref: "C360Studio/semstreams#1007", source: "task-list" },
    ]);
  });

  it("extracts claimed issue numbers from PR body closing keywords and PR title", () => {
    expect(
      extractClaimedIssueNumbers(
        "ui(work): inline drawer (#315)",
        "Closes #314\nFixes C360Studio/semteams#313\nCloses other/repo#99",
        REPO,
      ),
    ).toEqual([314, 313, 315]);
  });

  it("parses review request and review record comments in chronological order", () => {
    const comments = [
      {
        body: "<!-- semteams-review-request -->\ncommit: 1111111\nreviewer-lane: svelte-reviewer",
      },
      {
        body: "<!-- semteams-review-record -->\ncommit: 1111111\nverdict: blocking\nreviewer-lane: svelte-reviewer\nreviewed-by: codex",
      },
      {
        body: "<!-- semteams-review-request -->\ncommit: 2222222\nreviewer-lane: svelte-reviewer",
      },
      {
        body: "<!-- semteams-review-record -->\n- **commit:** `2222222`\n- **verdict:** `pass`\n- **reviewer-lane:** `svelte-reviewer`\n- **reviewed-by:** `svelte-reviewer (Vega)`",
      },
    ];

    expect(parseReviewComments(comments)).toEqual({
      review_request: {
        sha: "2222222",
        reviewer_lane: "svelte-reviewer",
      },
      review_record: {
        sha: "2222222",
        verdict: "pass",
        reviewer_lane: "svelte-reviewer",
        reviewed_by: "svelte-reviewer (Vega)",
      },
    });
  });

  it("parses ATeam APPROVE and REQUEST CHANGES review records", () => {
    expect(
      parseReviewComments([
        {
          body: [
            "reviewed-by: codex (gpt-6-astra, high)",
            "Reviewed at a1b2c3d4",
            "Verdict: APPROVE",
            "",
            "Checked base-to-head diff and ran local verification.",
          ].join("\n"),
        },
      ]),
    ).toEqual({
      review_record: {
        sha: "a1b2c3d4",
        verdict: "pass",
        reviewed_by: "codex (gpt-6-astra, high)",
      },
    });

    expect(
      parseReviewComments([
        {
          body: [
            "reviewed-by: gemini (gemini-4-argon, high)",
            "Reviewer lane: go-reviewer",
            "Reviewed at 99887766",
            "Verdict: REQUEST CHANGES",
            "",
            "1 BLOCKING finding in scripts/merge-check.sh.",
          ].join("\n"),
        },
      ]),
    ).toEqual({
      review_record: {
        sha: "99887766",
        verdict: "blocking",
        reviewer_lane: "go-reviewer",
        reviewed_by: "gemini (gemini-4-argon, high)",
      },
    });

    expect(
      parseReviewComments([
        {
          body: [
            "- **reviewed-by**: `claude (opus, high)`",
            "- **Re-reviewed at**: `feedface`",
            "- **Verdict**: CHANGES REQUESTED",
          ].join("\n"),
        },
      ]),
    ).toEqual({
      review_record: {
        sha: "feedface",
        verdict: "blocking",
        reviewed_by: "claude (opus, high)",
      },
    });
  });

  it("supports vendor-identifying ### Independent Cross-Agent Review and rejects vendor-less or multi-agent records", () => {
    expect(
      parseReviewComments([
        {
          body: [
            "### Independent Cross-Agent Review (claude)",
            "",
            "Reviewed at 0d948aac",
            "Verdict: APPROVE",
          ].join("\n"),
        },
      ]),
    ).toEqual({
      review_record: {
        sha: "0d948aac",
        verdict: "pass",
        reviewed_by: "claude",
      },
    });

    // Leading unambiguous agent (model, effort) allows a trailing clause mentioning another vendor.
    expect(
      parseReviewComments([
        {
          body: [
            "reviewed-by: claude (claude-opus-5-5, high) for gemini's PR",
            "Reviewed at 0d948aac",
            "Verdict: APPROVE",
          ].join("\n"),
        },
      ]),
    ).toEqual({
      review_record: {
        sha: "0d948aac",
        verdict: "pass",
        reviewed_by: "claude (claude-opus-5-5, high) for gemini's PR",
      },
    });

    // Vendor-less ### Independent Cross-Agent Review must fail closed with review_unparsed.
    expect(
      parseReviewComments([
        {
          body: [
            "### Independent Cross-Agent Review",
            "",
            "Reviewed at 0d948aac",
            "Verdict: APPROVE",
          ].join("\n"),
        },
      ]),
    ).toEqual({
      review_unparsed: true,
    });

    // Multiple distinct agents in reviewed-by or heading must fail closed with review_unparsed.
    for (const body of [
      [
        "reviewed-by: claude | codex | gemini",
        "Reviewed at 0d948aac",
        "Verdict: APPROVE",
      ].join("\n"),
      [
        "reviewed-by: claude / codex",
        "Reviewed at 0d948aac",
        "Verdict: APPROVE",
      ].join("\n"),
      [
        "reviewed-by: claude (opus, high)",
        "reviewed-by: codex (gpt-6-astra, high)",
        "Reviewed at 0d948aac",
        "Verdict: APPROVE",
      ].join("\n"),
      [
        "### Independent Cross-Agent Review (claude, codex)",
        "Reviewed at 0d948aac",
        "Verdict: APPROVE",
      ].join("\n"),
    ]) {
      expect(parseReviewComments([{ body }])).toEqual({
        review_unparsed: true,
      });
    }
  });

  it("ignores 0..3-space fenced code blocks and blockquotes while treating 4-space indented fences as content", () => {
    // Template example inside 0..3-space backtick or tilde fences alone never creates a record or unparsed flag.
    expect(
      parseReviewComments([
        {
          body: [
            "Copy the canonical record format:",
            "```text",
            "reviewed-by: codex (gpt-6-astra, high)",
            "Reviewed at 1234567",
            "Verdict: APPROVE | REQUEST CHANGES",
            "```",
            "   ~~~markdown",
            "### Independent Cross-Agent Review (claude)",
            "Reviewed at 7654321",
            "Verdict: HOLD",
            "   ~~~",
          ].join("\n"),
        },
      ]),
    ).toEqual({});

    // Live record quoting a fenced example with a different SHA and verdict is not rejected.
    expect(
      parseReviewComments([
        {
          body: [
            "reviewed-by: codex (gpt-6-astra, high)",
            "Reviewed at a1b2c3d4",
            "Verdict: APPROVE",
            "",
            "Verified parser ignores fenced examples such as:",
            "```text",
            "reviewed-by: claude (opus, high)",
            "Reviewed at 1234567",
            "Verdict: REQUEST CHANGES",
            "```",
          ].join("\n"),
        },
      ]),
    ).toEqual({
      review_record: {
        sha: "a1b2c3d4",
        verdict: "pass",
        reviewed_by: "codex (gpt-6-astra, high)",
      },
    });

    // CommonMark 4-space indented fence line is NOT an opening fence delimiter:
    // 1) It does not swallow a real review record below it.
    expect(
      parseReviewComments([
        {
          body: [
            "    ```text",
            "reviewed-by: codex (gpt-6-astra, high)",
            "Reviewed at a1b2c3d4",
            "Verdict: APPROVE",
          ].join("\n"),
        },
      ]),
    ).toEqual({
      review_record: {
        sha: "a1b2c3d4",
        verdict: "pass",
        reviewed_by: "codex (gpt-6-astra, high)",
      },
    });

    // 2) It does not hide an invalid Verdict line below it, nor prematurely close an open 0..3-space fence.
    expect(
      parseReviewComments([
        {
          body: ["    ```text", "Verdict: HOLD", "    ```"].join("\n"),
        },
      ]),
    ).toEqual({
      review_unparsed: true,
    });
    expect(
      parseReviewComments([
        {
          body: ["  ```text", "    ```", "Verdict: HOLD", "  ```"].join("\n"),
        },
      ]),
    ).toEqual({});

    // Human reply quoting a review record or verdict with `>` neither forges a record nor sets review_unparsed,
    // even when posted after a valid APPROVE record.
    expect(
      parseReviewComments([
        {
          body: [
            "> reviewed-by: codex (gpt-6-astra, high)",
            "> Reviewed at a1b2c3d4",
            "> Verdict: APPROVE",
            "  > Verdict: HOLD",
            "",
            "Thanks — will merge once CI finishes.",
          ].join("\n"),
        },
      ]),
    ).toEqual({});
    expect(
      parseReviewComments([
        {
          body: [
            "reviewed-by: codex (gpt-6-astra, high)",
            "Reviewed at a1b2c3d4",
            "Verdict: APPROVE",
          ].join("\n"),
        },
        {
          body: [
            "> reviewed-by: codex (gpt-6-astra, high)",
            "> Reviewed at a1b2c3d4",
            "> Verdict: APPROVE",
            "",
            "Acknowledged, proceeding to archive.",
          ].join("\n"),
        },
      ]),
    ).toEqual({
      review_record: {
        sha: "a1b2c3d4",
        verdict: "pass",
        reviewed_by: "codex (gpt-6-astra, high)",
      },
    });
  });

  it("requires canonical Reviewed at <sha> and rejects non-Reviewed-at or multi-SHA comments", () => {
    // Multiple distinct Reviewed at SHAs fail closed with review_unparsed.
    expect(
      parseReviewComments([
        {
          body: [
            "reviewed-by: codex (gpt-6-astra, high)",
            "Reviewed at a1b2c3d",
            "Reviewed at 9988776",
            "Verdict: APPROVE",
          ].join("\n"),
        },
      ]),
    ).toEqual({
      review_unparsed: true,
    });

    // Prefix-matching Reviewed at SHAs resolve to the longest SHA.
    expect(
      parseReviewComments([
        {
          body: [
            "reviewed-by: codex (gpt-6-astra, high)",
            "Reviewed at a1b2c3d",
            "Re-reviewed at a1b2c3d4e5f6",
            "Verdict: APPROVE",
          ].join("\n"),
        },
      ]),
    ).toEqual({
      review_record: {
        sha: "a1b2c3d4e5f6",
        verdict: "pass",
        reviewed_by: "codex (gpt-6-astra, high)",
      },
    });

    // Non-canonical SHA forms without `Reviewed at <sha>` fail closed with review_unparsed.
    for (const shaLine of [
      "Review of a1b2c3d4",
      "Reviewed commit: a1b2c3d4",
      "Reviewed head: a1b2c3d4",
      "Reviewed base 11111111 -> head a1b2c3d4",
      "Head: a1b2c3d4",
    ]) {
      expect(
        parseReviewComments([
          {
            body: [
              "reviewed-by: claude (opus, high)",
              shaLine,
              "Verdict: APPROVED",
            ].join("\n"),
          },
        ]),
      ).toEqual({
        review_unparsed: true,
      });
    }
  });

  it("parses routine Verdict clauses while failing closed with review_unparsed on missing, unrecognized, or ambiguous verdicts", () => {
    // Routine reviewer verdict lines with comma, colon, or trailing clause words parse cleanly.
    expect(
      parseReviewComments([
        {
          body: [
            "reviewed-by: codex (gpt-6-astra, high)",
            "Reviewed at a1b2c3d4",
            "Verdict: REQUEST CHANGES, see H1",
          ].join("\n"),
        },
      ]),
    ).toEqual({
      review_record: {
        sha: "a1b2c3d4",
        verdict: "blocking",
        reviewed_by: "codex (gpt-6-astra, high)",
      },
    });
    expect(
      parseReviewComments([
        {
          body: [
            "reviewed-by: codex (gpt-6-astra, high)",
            "Reviewed at a1b2c3d4",
            "Verdict: REQUEST CHANGES: two HIGH",
          ].join("\n"),
        },
      ]),
    ).toEqual({
      review_record: {
        sha: "a1b2c3d4",
        verdict: "blocking",
        reviewed_by: "codex (gpt-6-astra, high)",
      },
    });
    for (const [verdictLine, expectedVerdict] of [
      ["Verdict: REQUEST CHANGES see H1", "blocking"],
      ["Verdict: REQUEST CHANGES at a1b2c3d4", "blocking"],
      ["Verdict: REQUEST CHANGES for H1 and M1", "blocking"],
      ["Verdict: REQUEST CHANGES pending H1 fix", "blocking"],
      ["Verdict: REQUEST CHANGES until H1 lands", "blocking"],
      ["Verdict: REQUEST CHANGES because H1 fails open", "blocking"],
    ] as const) {
      expect(
        parseReviewComments([
          {
            body: [
              "reviewed-by: codex (gpt-6-astra, high)",
              "Reviewed at a1b2c3d4",
              verdictLine,
            ].join("\n"),
          },
        ]),
      ).toEqual({
        review_record: {
          sha: "a1b2c3d4",
          verdict: expectedVerdict,
          reviewed_by: "codex (gpt-6-astra, high)",
        },
      });
    }
    expect(
      parseReviewComments([
        {
          body: [
            "reviewed-by: codex (gpt-6-astra, high)",
            "Reviewed at a1b2c3d4",
            "Verdict: REQUEST CHANGES — cannot approve until race is fixed",
          ].join("\n"),
        },
      ]),
    ).toEqual({
      review_record: {
        sha: "a1b2c3d4",
        verdict: "blocking",
        reviewed_by: "codex (gpt-6-astra, high)",
      },
    });

    const invalidComments = [
      {
        body: [
          "reviewed-by: codex (gpt-6-astra, high)",
          "Reviewed at a1b2c3d4",
          "",
          "All checks look good, APPROVE.",
        ].join("\n"),
      },
      {
        body: [
          "reviewed-by: codex (gpt-6-astra, high)",
          "Reviewed at a1b2c3d4",
          "Verdict: APPROVE | REQUEST CHANGES",
        ].join("\n"),
      },
      {
        body: [
          "reviewed-by: codex (gpt-6-astra, high)",
          "Reviewed at a1b2c3d4",
          "Verdict: APPROVE / REQUEST CHANGES",
        ].join("\n"),
      },
      {
        body: [
          "reviewed-by: codex (gpt-6-astra, high)",
          "Reviewed at a1b2c3d4",
          "Verdict: APPROVE; REQUEST CHANGES",
        ].join("\n"),
      },
      {
        body: [
          "reviewed-by: codex (gpt-6-astra, high)",
          "Reviewed at a1b2c3d4",
          "Verdict: APPROVE or REQUEST CHANGES",
        ].join("\n"),
      },
      {
        body: [
          "reviewed-by: codex (gpt-6-astra, high)",
          "Reviewed at a1b2c3d4",
          "Verdict: HOLD",
        ].join("\n"),
      },
      {
        body: [
          "reviewed-by: codex (gpt-6-astra, high)",
          "Reviewed at a1b2c3d4",
          "Verdict: LGTM",
        ].join("\n"),
      },
      {
        body: [
          "reviewed-by: codex (gpt-6-astra, high)",
          "Reviewed at a1b2c3d4",
          "Verdict: APPROVE",
          "Verdict: REQUEST CHANGES",
        ].join("\n"),
      },
      {
        body: "<!-- semteams-review-record -->\ncommit: a1b2c3d4\nverdict: hold",
      },
    ];

    for (const invalid of invalidComments) {
      expect(parseReviewComments([invalid])).toEqual({
        review_unparsed: true,
      });
    }

    // H1 mirror probe: an earlier APPROVE at HEAD followed by an unparseable record keeps review_unparsed: true so
    // callers fail closed instead of reporting current/Ready to land.
    const approvedAtHead = {
      body: [
        "reviewed-by: codex (gpt-6-astra, high)",
        "Reviewer lane: go-reviewer",
        "Reviewed at a1b2c3d4",
        "Verdict: APPROVE",
      ].join("\n"),
    };
    for (const unparseable of [
      {
        body: [
          "reviewed-by: claude (opus, high)",
          "Reviewer lane: svelte-reviewer",
          "Reviewed at a1b2c3d4",
          "Verdict: HOLD",
        ].join("\n"),
      },
      {
        body: [
          "reviewed-by: claude (opus, high)",
          "Reviewed at a1b2c3d4",
          "Reviewed at 99887766",
          "Verdict: REQUEST CHANGES",
        ].join("\n"),
      },
      {
        body: [
          "reviewed-by: claude (opus, high)",
          "Reviewed at a1b2c3d4",
          "Verdict: APPROVE | REQUEST CHANGES",
        ].join("\n"),
      },
    ]) {
      expect(parseReviewComments([approvedAtHead, unparseable])).toEqual({
        review_record: {
          sha: "a1b2c3d4",
          verdict: "pass",
          reviewer_lane: "go-reviewer",
          reviewed_by: "codex (gpt-6-astra, high)",
        },
        review_unparsed: true,
      });
    }
  });

  it("matches ATeam gateVerdict: only an exact `Verdict: APPROVE` line passes; conditional or decorated APPROVE fails closed", () => {
    const record = (verdictLines: string[]) => ({
      body: ["reviewed-by: codex (gpt-6-astra, high)", "Reviewed at a1b2c3d4", ...verdictLines].join("\n"),
    });
    const pass = {
      review_record: { sha: "a1b2c3d4", verdict: "pass", reviewed_by: "codex (gpt-6-astra, high)" },
    };

    // Oracle: ateam internal/review/gate.go gateVerdict (36b999b:823-840) trims the line, requires a `Verdict:`
    // prefix (case-insensitive), and approves only when the whole remaining value equals APPROVE.
    for (const line of ["Verdict: APPROVE", "verdict: approve", "  Verdict:   Approve  "]) {
      expect(parseReviewComments([record([line])])).toEqual(pass);
    }

    for (const line of [
      "Verdict: APPROVE pending H1 fix",
      "Verdict: APPROVE once H1 lands",
      "Verdict: APPROVE until the flake recurs",
      "Verdict: APPROVE after H1 is fixed",
      "Verdict: APPROVE if H1 is fixed",
      "Verdict: APPROVE, but REQUEST CHANGES on H1",
      "Verdict: APPROVE with nits",
      "Verdict: APPROVE (0 BLOCKING, 1 NIT)",
      "Verdict: APPROVE on a1b2c3d4",
      "Verdict: APPROVED",
      "Verdict: PASS",
      "Verdict: `APPROVE`",
      "**Verdict**: APPROVE",
      "- Verdict: APPROVE",
    ]) {
      expect(parseReviewComments([record([line])]), line).toEqual({ review_unparsed: true });
    }

    // Exactly one Verdict line: a duplicated APPROVE line does not approve (gateVerdict "has 2 Verdict: lines").
    expect(parseReviewComments([record(["Verdict: APPROVE", "Verdict: APPROVE"])])).toEqual({
      review_unparsed: true,
    });

    // Verdict text inside a code fence is ignored, so the single unfenced line decides.
    expect(
      parseReviewComments([
        record(["```text", "Verdict: APPROVE pending H1", "```", "Verdict: APPROVE"]),
      ]),
    ).toEqual(pass);
    expect(
      parseReviewComments([
        record(["```text", "Verdict: APPROVE", "```", "Verdict: APPROVE pending H1"]),
      ]),
    ).toEqual({ review_unparsed: true });
  });

  it("lets a later valid review record clear an earlier unparseable comment, but not one posted after it", () => {
    const implementerReply = {
      // Implementer reply bullets quoting a verdict and SHA outside `>` blockquotes carry record signals but no
      // reviewer identity, so they cannot parse as a record.
      body: ["Addressed the review:", "- Reviewed at 99887766", "- Verdict: REQUEST CHANGES was for H1"].join("\n"),
    };
    const approveAll = {
      body: ["reviewed-by: codex (gpt-6-astra, high)", "Reviewed at a1b2c3d4", "Verdict: APPROVE"].join("\n"),
    };
    const approveRecord = {
      review_record: { sha: "a1b2c3d4", verdict: "pass", reviewed_by: "codex (gpt-6-astra, high)" },
    };

    expect(parseReviewComments([implementerReply])).toEqual({ review_unparsed: true });
    // ATeam record() skips non-records and keeps the latest valid record (gate.go 36b999b:500-527): no stickiness.
    expect(parseReviewComments([implementerReply, approveAll], "a1b2c3d4e5f6")).toEqual(approveRecord);
    // A malformed record-shaped comment AFTER the latest valid record still fails closed.
    expect(parseReviewComments([approveAll, implementerReply], "a1b2c3d4e5f6")).toEqual({
      ...approveRecord,
      review_unparsed: true,
    });
    // Historical legacy-marker records that fail to parse are cleared the same way.
    const legacyMalformed = { body: "<!-- semteams-review-record -->\ncommit: 99887766\nverdict: hold" };
    expect(parseReviewComments([legacyMalformed, approveAll])).toEqual(approveRecord);
    expect(parseReviewComments([approveAll, legacyMalformed])).toEqual({ ...approveRecord, review_unparsed: true });

    // A lane-scoped unparseable comment is cleared only by a later record covering its lane (or an all-lanes record):
    // a go-reviewer APPROVE must not clear an unreadable svelte-reviewer record.
    const svelteUnreadable = {
      body: [
        "reviewed-by: claude (opus, high)",
        "Reviewer lane: svelte-reviewer",
        "Reviewed at a1b2c3d4",
        "Verdict: HOLD",
      ].join("\n"),
    };
    const goApprove = {
      body: [
        "reviewed-by: codex (gpt-6-astra, high)",
        "Reviewer lane: go-reviewer",
        "Reviewed at a1b2c3d4",
        "Verdict: APPROVE",
      ].join("\n"),
    };
    const svelteApprove = {
      body: [
        "reviewed-by: codex (gpt-6-astra, high)",
        "Reviewer lane: svelte-reviewer",
        "Reviewed at a1b2c3d4",
        "Verdict: APPROVE",
      ].join("\n"),
    };
    expect(parseReviewComments([svelteUnreadable, goApprove]).review_unparsed).toBe(true);
    expect(parseReviewComments([svelteUnreadable, goApprove, svelteApprove]).review_unparsed).toBeUndefined();
    expect(parseReviewComments([svelteUnreadable, approveAll]).review_unparsed).toBeUndefined();
  });

  it("aggregates review records at HEAD across reviewer lanes so a blocking lane is not hidden by another lane's APPROVE", () => {
    const head = "a1b2c3d4e5f6789012345678901234567890abcd";
    const laneRecord = (lane: string | undefined, verdict: "APPROVE" | "REQUEST CHANGES", sha = "a1b2c3d4") => ({
      body: [
        "reviewed-by: codex (gpt-6-astra, high)",
        ...(lane ? [`Reviewer lane: ${lane}`] : []),
        `Reviewed at ${sha}`,
        `Verdict: ${verdict}`,
      ].join("\n"),
    });
    const verdictOf = (comments: unknown[]) => parseReviewComments(comments, head).review_record?.verdict;

    const svelteBlocks = laneRecord("svelte-reviewer", "REQUEST CHANGES");
    const goApproves = laneRecord("go-reviewer", "APPROVE");
    const svelteApproves = laneRecord("svelte-reviewer", "APPROVE");

    // #366: svelte REQUEST CHANGES then go APPROVE at HEAD stays blocking, and the blocking record is reported.
    expect(parseReviewComments([svelteBlocks, goApproves], head)).toEqual({
      review_record: {
        sha: "a1b2c3d4",
        verdict: "blocking",
        reviewer_lane: "svelte-reviewer",
        reviewed_by: "codex (gpt-6-astra, high)",
      },
    });
    // A newer record for the same lane supersedes its blocking verdict.
    expect(verdictOf([svelteBlocks, goApproves, svelteApproves])).toBe("pass");
    // An all-lanes record (no lane, or a record naming both lanes) supersedes every lane.
    expect(verdictOf([svelteBlocks, goApproves, laneRecord(undefined, "APPROVE")])).toBe("pass");
    expect(verdictOf([svelteBlocks, laneRecord("go-reviewer, svelte-reviewer", "APPROVE")])).toBe("pass");
    // An all-lanes blocking record is not cleared by a single lane's APPROVE.
    expect(verdictOf([laneRecord(undefined, "REQUEST CHANGES"), goApproves])).toBe("blocking");
    // Lane text is matched by `<name>-reviewer` token, so trailing prose does not create a distinct lane.
    expect(
      verdictOf([svelteBlocks, laneRecord("svelte-reviewer (cross-agent: codex or gemini)", "APPROVE")]),
    ).toBe("pass");
    // An unrecognized lane name is its own lane and never clears a named lane.
    expect(verdictOf([svelteBlocks, laneRecord("ui", "APPROVE")])).toBe("blocking");
    // Records at an older SHA do not take part in the HEAD aggregate.
    expect(verdictOf([laneRecord("svelte-reviewer", "REQUEST CHANGES", "99887766"), goApproves])).toBe("pass");
    // With no record at HEAD the latest record is reported (the baton derives stale from it).
    expect(parseReviewComments([goApproves], "ffffffffffffffff").review_record?.sha).toBe("a1b2c3d4");
    // Without a HEAD SHA, records at the latest record's SHA are aggregated.
    expect(parseReviewComments([svelteBlocks, goApproves]).review_record?.verdict).toBe("blocking");
  });

  it("parses comments with 65,536 consecutive blank lines in under 100ms without quadratic backtracking", () => {
    const body = `x\n${"\n".repeat(65_536)}y`;
    const start = performance.now();
    const parsed = parseReviewComments([{ body }]);
    const elapsedMs = performance.now() - start;

    expect(parsed).toEqual({});
    expect(elapsedMs).toBeLessThan(100);
  });

  it("ignores inline-code meta discussion of `Verdict`, `reviewed-by`, and `Reviewed at <sha>` in requests and records", () => {
    expect(
      parseReviewComments([
        {
          body: [
            "## Review request",
            "",
            "- **Reviewer lanes:** `go-reviewer`, `svelte-reviewer`",
            "- **Commit SHA:** `12f8042a71239c8eb1e09d88f05a556902575fd4`",
            "- `Verdict`: splits clauses on `at` and `citing`",
            "- `reviewed-by`: checks vendor token",
            "- Narrowed `Reviewed at 43d0967e` in `githubSource.ts`",
          ].join("\n"),
        },
        {
          body: [
            "reviewed-by: claude (`claude-opus-5-5, high`)",
            "Reviewed at `12f8042a71239c8eb1e09d88f05a556902575fd4`",
            "Verdict: APPROVE",
            "",
            "Verified parser fixes against earlier `Reviewed at 43d0967e`:",
            "- `Verdict`: `at` and `citing` clauses parse cleanly",
            "- `reviewed-by`: full Claude model ID accepted",
          ].join("\n"),
        },
      ]),
    ).toEqual({
      review_request: {
        sha: "12f8042a71239c8eb1e09d88f05a556902575fd4",
        reviewer_lane: "go-reviewer, svelte-reviewer",
      },
      review_record: {
        sha: "12f8042a71239c8eb1e09d88f05a556902575fd4",
        verdict: "pass",
        reviewed_by: "claude (claude-opus-5-5, high)",
      },
    });
  });

  it("parses plain ## Review request and #-less Review request comments and never treats records as requests", () => {
    expect(
      parseReviewComments([
        {
          body: [
            "## Review request",
            "",
            "Commit: a1b2c3d",
            "Lane: go-reviewer, svelte-reviewer",
            "Diff range: 0000000..a1b2c3d",
          ].join("\n"),
        },
      ]),
    ).toEqual({
      review_request: {
        sha: "a1b2c3d",
        reviewer_lane: "go-reviewer, svelte-reviewer",
      },
    });

    expect(
      parseReviewComments([
        {
          body: [
            "**Review request**",
            "",
            "Commit SHA: b2c3d4e5",
            "Review lanes: go-reviewer, svelte-reviewer",
          ].join("\n"),
        },
      ]),
    ).toEqual({
      review_request: {
        sha: "b2c3d4e5",
        reviewer_lane: "go-reviewer, svelte-reviewer",
      },
    });

    // A review record is never simultaneously treated as a review_request.
    expect(
      parseReviewComments([
        {
          body: [
            "reviewed-by: codex (gpt-6-astra, high)",
            "Reviewed at a1b2c3d",
            "Verdict: APPROVE",
          ].join("\n"),
        },
      ]),
    ).toEqual({
      review_record: {
        sha: "a1b2c3d",
        verdict: "pass",
        reviewed_by: "codex (gpt-6-astra, high)",
      },
    });
  });

  it("chronologically supersedes across legacy HTML markers, plain requests, and ATeam re-reviews", () => {
    const comments = [
      {
        body: "<!-- semteams-review-request -->\ncommit: 1111111\nreviewer-lane: go-reviewer",
      },
      {
        body: "<!-- semteams-review-record -->\ncommit: 1111111\nverdict: blocking\nreviewer-lane: go-reviewer\nreviewed-by: claude",
      },
      {
        body: [
          "### Review request",
          "- **Commit to read:** `2222222` (clean tree)",
          "- **Reviewer lanes:** `go-reviewer`, `svelte-reviewer`",
        ].join("\n"),
      },
      {
        body: [
          "reviewed-by: codex (gpt-6-astra, high)",
          "Reviewed at 2222222",
          "Verdict: REQUEST CHANGES",
        ].join("\n"),
      },
      {
        body: [
          "## Review request",
          "Commit: 3333333",
          "Lane: go-reviewer, svelte-reviewer",
        ].join("\n"),
      },
      {
        body: [
          "reviewed-by: codex (gpt-6-astra, high)",
          "Reviewed at 3333333",
          "Verdict: APPROVE",
          "",
          "Verified `<!-- semteams-review-record -->` backward compatibility.",
        ].join("\n"),
      },
    ];

    expect(parseReviewComments(comments.slice(0, 2))).toEqual({
      review_request: {
        sha: "1111111",
        reviewer_lane: "go-reviewer",
      },
      review_record: {
        sha: "1111111",
        verdict: "blocking",
        reviewer_lane: "go-reviewer",
        reviewed_by: "claude",
      },
    });

    expect(parseReviewComments(comments.slice(0, 4))).toEqual({
      review_request: {
        sha: "2222222",
        reviewer_lane: "go-reviewer, svelte-reviewer",
      },
      review_record: {
        sha: "2222222",
        verdict: "blocking",
        reviewed_by: "codex (gpt-6-astra, high)",
      },
    });

    expect(parseReviewComments(comments)).toEqual({
      review_request: {
        sha: "3333333",
        reviewer_lane: "go-reviewer, svelte-reviewer",
      },
      review_record: {
        sha: "3333333",
        verdict: "pass",
        reviewed_by: "codex (gpt-6-astra, high)",
      },
    });
  });
});

describe("createGithubSource", () => {
  it("enumerates issues, links PR claims, inspects review comments, and propagates flake holds", async () => {
    const authHeaders: (string | null)[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Parameters<typeof fetch>[0], init?: RequestInit) => {
        const url = new URL(String(input));
        const headers = new Headers(init?.headers);
        authHeaders.push(headers.get("Authorization"));

        if (url.pathname === "/repos/C360Studio/semteams/issues") {
          return jsonResponse([
            {
              number: 309,
              title: "Flaky e2e test",
              state: "open",
              labels: [{ name: "class:flake" }],
              assignees: [],
            },
            {
              number: 315,
              title: "Inline agentic drawer",
              body: "Implement the inline workbench.",
              state: "open",
              labels: [{ name: "type:feature" }, { name: "area:ui" }],
              assignees: [{ login: "coby" }],
              milestone: { title: "v0.2.0" },
            },
          ]);
        }
        if (url.pathname === "/repos/C360Studio/semteams/pulls") {
          return jsonResponse([
            {
              number: 318,
              title: "ui(work): inline drawer (#315)",
              body: "Closes #315\n\nimplemented-by: Sol\nstop-point: Waiting on CI",
              state: "open",
              draft: true,
              head: {
                ref: "codex/work-inline-drawer",
                sha: "fd261aabf8b3d83869af662585d10f0937cd97ec",
              },
            },
          ]);
        }
        if (url.pathname === "/repos/C360Studio/semteams/issues/318/comments") {
          return jsonResponse([
            {
              body: "<!-- semteams-review-record -->\ncommit: fd261aab\nverdict: pass\nreviewer-lane: svelte-reviewer\nreviewed-by: svelte-reviewer",
            },
          ]);
        }
        return jsonResponse({ message: "not found" }, 404);
      }),
    );

    const source = createGithubSource({
      apiUrl: "http://github.test",
      token: "ghp_unit_test",
    });

    const listed = await source.listItems(REPO);
    expect(authHeaders.every((h) => h === "Bearer ghp_unit_test")).toBe(true);
    expect(listed.lookup).toBe("complete");
    expect(listed.columns).toEqual(["Todo", "In Progress", "Done"]);
    expect(listed.items).toHaveLength(2);

    const item315 = listed.items.find((i) => i.number === 315)!;
    expect(item315).toMatchObject({
      ref: "C360Studio/semteams#315",
      column: "In Progress",
      milestone: "v0.2.0",
      claim: {
        state: "known",
        value: {
          kind: "agent-pr",
          agent_vendor: "codex",
          holder: "Sol",
          pull_request: "C360Studio/semteams#318",
          draft: true,
          stop_point: "Waiting on CI",
        },
      },
      review: {
        state: "known",
        value: {
          pull_request: "C360Studio/semteams#318",
          status: "current",
          reviewer_lane: "svelte-reviewer",
        },
      },
      gate: {
        state: "known",
        value: [
          {
            kind: "flake-hold",
            ref: "C360Studio/semteams#309",
          },
        ],
      },
    });

    const runs = await source.linkedRunBindings({ repository: REPO, number: 315 });
    expect(runs).toEqual({
      lookup: "complete",
      bindings: [{ by: "work_item_ref", ref: "C360Studio/semteams#315" }],
    });
  });

  it("degrades to partial with unknown delivery/review when /pulls or PR comment reads fail", async () => {
    stubBackend((url) => {
      if (url.pathname === "/repos/C360Studio/semteams/issues") {
        return jsonResponse([
          {
            number: 315,
            title: "Inline agentic drawer",
            state: "open",
            labels: [],
            assignees: [{ login: "coby" }],
          },
        ]);
      }
      if (url.pathname === "/repos/C360Studio/semteams/pulls") {
        return jsonResponse({ message: "server error" }, 500);
      }
      return jsonResponse({ message: "not found" }, 404);
    });

    const source = createGithubSource({ apiUrl: "http://github.test" });
    const listed = await source.listItems(REPO);
    expect(listed.lookup).toBe("partial");
    expect(listed.reason).toContain("GET /repos/C360Studio/semteams/pulls answered 500");
    expect(listed.items[0].delivery).toEqual({
      state: "unknown",
      reason: "GET /repos/C360Studio/semteams/pulls answered 500",
    });
    expect(listed.items[0].claim).toEqual({
      state: "known",
      value: { kind: "assignee", holder: "coby" },
      reason: "assigned on issue; linked pull requests unknown",
    });
    expect(listed.items[0].review).toEqual({
      state: "unknown",
      reason: "GET /repos/C360Studio/semteams/pulls answered 500",
    });
  });

  it("marks review as unknown when an open PR's comment fetch fails", async () => {
    stubBackend((url) => {
      if (url.pathname === "/repos/C360Studio/semteams/issues/315") {
        return jsonResponse({
          number: 315,
          title: "Inline agentic drawer",
          state: "open",
          labels: [],
          assignees: [],
        });
      }
      if (url.pathname === "/repos/C360Studio/semteams/issues") {
        return jsonResponse([]);
      }
      if (url.pathname === "/repos/C360Studio/semteams/pulls") {
        return jsonResponse([
          {
            number: 318,
            title: "ui(work): inline drawer (#315)",
            body: "Closes #315",
            state: "open",
            head: { ref: "gemini/work-inline-drawer", sha: "abcdef1234567" },
          },
        ]);
      }
      if (url.pathname === "/repos/C360Studio/semteams/issues/318/comments") {
        return jsonResponse({ message: "unavailable" }, 503);
      }
      return jsonResponse({ message: "not found" }, 404);
    });

    const source = createGithubSource({ apiUrl: "http://github.test" });
    const single = await source.getItem({ repository: REPO, number: 315 });
    expect(single.lookup).toBe("partial");
    expect(single.reason).toContain("GET /repos/C360Studio/semteams/issues/318/comments answered 503");
    expect(single.item?.claim).toMatchObject({
      state: "known",
      value: { kind: "agent-pr", agent_vendor: "gemini" },
    });
    expect(single.item?.review).toEqual({
      state: "unknown",
      reason: "PR review comments not inspected",
    });
  });

  it("reports partial when pull request listing hits GITHUB_PAGE_LIMIT and leaves unmatched delivery unknown", async () => {
    const manyPulls = Array.from({ length: GITHUB_PAGE_LIMIT }, (_, idx) => ({
      number: idx + 1,
      title: `PR ${idx + 1}`,
      body: "Closes #999",
      state: "closed",
      head: { ref: `feat/pr-${idx + 1}`, sha: "1234567" },
    }));

    stubBackend((url) => {
      if (url.pathname === "/repos/C360Studio/semteams/issues") {
        return jsonResponse([{ number: 315, title: "Unmatched issue", state: "open", labels: [], assignees: [] }]);
      }
      if (url.pathname === "/repos/C360Studio/semteams/pulls") {
        return jsonResponse(manyPulls);
      }
      return jsonResponse([], 200);
    });

    const source = createGithubSource({ apiUrl: "http://github.test" });
    const listed = await source.listItems(REPO);
    expect(listed.lookup).toBe("partial");
    expect(listed.items[0].delivery).toEqual({
      state: "unknown",
      reason: `GET /repos/C360Studio/semteams/pulls truncated at ${GITHUB_PAGE_LIMIT} items`,
    });
  });

  it("hydrates GitHub Projects v2 status options, pm_status, and priority when project_board is configured", async () => {
    const boardRepo: Repository = {
      owner: "C360Studio",
      name: "semteams",
      project_board: {
        owner: "C360Studio",
        number: 4,
        status_field: "Status",
        priority_field: "Priority",
      },
    };

    const graphqlPayloads: { query?: string; variables?: Record<string, unknown> }[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Parameters<typeof fetch>[0], init?: RequestInit) => {
        const url = new URL(String(input));
        if (url.pathname === "/graphql") {
          graphqlPayloads.push(JSON.parse(String(init?.body ?? "{}")));
          return jsonResponse({
            data: {
              organization: {
                projectV2: {
                  fields: {
                    nodes: [
                      {
                        __typename: "ProjectV2SingleSelectField",
                        name: "Status",
                        options: [
                          { name: "Todo" },
                          { name: "In Progress" },
                          { name: "In Review" },
                          { name: "Done" },
                        ],
                      },
                      {
                        __typename: "ProjectV2SingleSelectField",
                        name: "Priority",
                        options: [{ name: "P0" }, { name: "P1" }, { name: "P2" }],
                      },
                    ],
                  },
                  items: {
                    pageInfo: { hasNextPage: false },
                    nodes: [
                      {
                        content: {
                          __typename: "Issue",
                          number: 315,
                          repository: { nameWithOwner: "C360Studio/semteams" },
                        },
                        fieldValues: {
                          nodes: [
                            {
                              __typename: "ProjectV2ItemFieldSingleSelectValue",
                              name: "In Review",
                              field: { name: "Status" },
                            },
                            {
                              __typename: "ProjectV2ItemFieldSingleSelectValue",
                              name: "P1",
                              field: { name: "Priority" },
                            },
                          ],
                        },
                      },
                      {
                        content: {
                          __typename: "Issue",
                          number: 315,
                          repository: { nameWithOwner: "C360Studio/semengine" },
                        },
                        fieldValues: {
                          nodes: [
                            {
                              __typename: "ProjectV2ItemFieldSingleSelectValue",
                              name: "Done",
                              field: { name: "Status" },
                            },
                          ],
                        },
                      },
                    ],
                  },
                },
              },
              user: null,
            },
            errors: [
              {
                type: "NOT_FOUND",
                path: ["user"],
                message: "Could not resolve to a User with the login of 'C360Studio'.",
              },
            ],
          });
        }
        if (url.pathname === "/repos/C360Studio/semteams/issues") {
          return jsonResponse([
            {
              number: 315,
              title: "Inline agentic drawer",
              state: "open",
              labels: [],
              assignees: [{ login: "coby" }],
            },
            {
              number: 316,
              title: "Unboarded issue",
              state: "open",
              labels: [],
              assignees: [],
            },
          ]);
        }
        if (url.pathname === "/repos/C360Studio/semteams/issues/315") {
          return jsonResponse({
            number: 315,
            title: "Inline agentic drawer",
            state: "open",
            labels: [],
            assignees: [{ login: "coby" }],
          });
        }
        if (url.pathname === "/repos/C360Studio/semteams/pulls") {
          return jsonResponse([]);
        }
        return jsonResponse({ message: "not found" }, 404);
      }),
    );

    const source = createGithubSource({ apiUrl: "http://github.test", token: "ghp_board" });
    const listed = await source.listItems(boardRepo);
    expect(graphqlPayloads).toHaveLength(1);
    expect(graphqlPayloads[0].variables).toEqual({
      owner: "C360Studio",
      number: 4,
      itemLimit: GITHUB_PAGE_LIMIT,
    });
    expect(listed.lookup).toBe("complete");
    expect(listed.columns).toEqual(["Todo", "In Progress", "In Review", "Done", "No Status"]);

    const item315 = listed.items.find((i) => i.number === 315)!;
    expect(item315).toMatchObject({
      number: 315,
      column: "In Review",
      pm_status: "In Review",
      priority: "P1",
    });

    const item316 = listed.items.find((i) => i.number === 316)!;
    expect(item316.column).toBe("No Status");
    expect(item316).not.toHaveProperty("pm_status");
    expect(item316).not.toHaveProperty("priority");

    // Single item lookup also hydrates project_board fields.
    const single = await source.getItem({ repository: boardRepo, number: 315 });
    expect(single.lookup).toBe("complete");
    expect(single.item).toMatchObject({
      number: 315,
      column: "In Review",
      pm_status: "In Review",
      priority: "P1",
    });
  });

  it("degrades to partial and falls back to issue-state columns when the Projects v2 GraphQL read fails", async () => {
    const boardRepo: Repository = {
      owner: "C360Studio",
      name: "semteams",
      project_board: {
        owner: "C360Studio",
        number: 4,
        status_field: "Status",
        priority_field: "Priority",
      },
    };

    stubBackend((url) => {
      if (url.pathname === "/graphql") {
        return jsonResponse(
          {
            data: { organization: null, user: null },
            errors: [
              {
                type: "INSUFFICIENT_SCOPES",
                message: "Your token has not been granted the required scopes to execute this query.",
              },
            ],
          },
          200,
        );
      }
      if (url.pathname === "/repos/C360Studio/semteams/issues") {
        return jsonResponse([
          {
            number: 315,
            title: "Inline agentic drawer",
            state: "open",
            labels: [],
            assignees: [{ login: "coby" }],
          },
        ]);
      }
      if (url.pathname === "/repos/C360Studio/semteams/pulls") {
        return jsonResponse([]);
      }
      return jsonResponse({ message: "not found" }, 404);
    });

    const source = createGithubSource({ apiUrl: "http://github.test" });
    const listed = await source.listItems(boardRepo);
    expect(listed.lookup).toBe("partial");
    expect(listed.reason).toContain("POST /graphql (ProjectV2 C360Studio#4)");
    expect(listed.reason).toContain("required scopes");
    expect(listed.columns).toEqual(["Todo", "In Progress", "Done"]);
    expect(listed.items[0].column).toBe("In Progress");
    expect(listed.items[0]).not.toHaveProperty("pm_status");
    expect(listed.items[0]).not.toHaveProperty("priority");
  });

  it("supports user-owned ProjectV2 boards and reports partial when project items are truncated", async () => {
    const userBoardRepo: Repository = {
      owner: "coby",
      name: "semteams",
      project_board: {
        owner: "coby",
        number: 2,
        status_field: "Status",
      },
    };

    stubBackend((url) => {
      if (url.pathname === "/graphql") {
        return jsonResponse({
          data: {
            organization: null,
            user: {
              projectV2: {
                fields: {
                  nodes: [
                    {
                      __typename: "ProjectV2SingleSelectField",
                      name: "Status",
                      options: [{ name: "Todo" }, { name: "Done" }],
                    },
                  ],
                },
                items: {
                  pageInfo: { hasNextPage: true },
                  nodes: [
                    {
                      content: {
                        __typename: "Issue",
                        number: 10,
                        repository: { nameWithOwner: "coby/semteams" },
                      },
                      fieldValues: {
                        nodes: [
                          {
                            __typename: "ProjectV2ItemFieldSingleSelectValue",
                            name: "Done",
                            field: { name: "Status" },
                          },
                          {
                            __typename: "ProjectV2ItemFieldSingleSelectValue",
                            name: "P0",
                            field: { name: "Priority" },
                          },
                        ],
                      },
                    },
                  ],
                },
              },
            },
          },
        });
      }
      if (url.pathname === "/repos/coby/semteams/issues") {
        return jsonResponse([
          { number: 10, title: "User board item", state: "open", labels: [], assignees: [] },
        ]);
      }
      if (url.pathname === "/repos/coby/semteams/pulls") {
        return jsonResponse([]);
      }
      return jsonResponse({ message: "not found" }, 404);
    });

    const source = createGithubSource({ apiUrl: "http://github.test" });
    const listed = await source.listItems(userBoardRepo);
    expect(listed.lookup).toBe("partial");
    expect(listed.reason).toContain(
      `POST /graphql (ProjectV2 coby#2) items truncated at ${GITHUB_PAGE_LIMIT} items`,
    );
    expect(listed.columns).toEqual(["Todo", "Done"]);
    expect(listed.items[0]).toMatchObject({
      number: 10,
      column: "Done",
      pm_status: "Done",
    });
    // priority_field is not configured on userBoardRepo.project_board, so priority stays unset.
    expect(listed.items[0]).not.toHaveProperty("priority");
  });

  it("derives claim and current review overlays from ATeam PR body lines and PR comments", async () => {
    stubBackend((url) => {
      if (url.pathname === "/repos/C360Studio/semteams/issues") {
        return jsonResponse([
          {
            number: 362,
            title: "Adopt ateam v0.3.2",
            body: "Clean upstream adoption.",
            state: "open",
            labels: [{ name: "area:governance" }],
            assignees: [],
          },
        ]);
      }
      if (url.pathname === "/repos/C360Studio/semteams/pulls") {
        return jsonResponse([
          {
            number: 363,
            title: "chore(harness): adopt ateam v0.3.2 (#362)",
            body: [
              "Closes #362",
              "",
              "implemented-by: gemini (gemini-4-argon, high)",
              "reviewed-by: codex (gpt-6-astra, high)",
            ].join("\n"),
            state: "open",
            draft: false,
            head: {
              ref: "gemini/gh362-adopt-ateam-v032",
              sha: "a1b2c3d4e5f6789012345678901234567890abcd",
            },
          },
        ]);
      }
      if (url.pathname === "/repos/C360Studio/semteams/issues/363/comments") {
        return jsonResponse([
          {
            body: "## Review request\nCommit: a1b2c3d4\nLane: go-reviewer, svelte-reviewer",
          },
          {
            body: [
              "reviewed-by: codex (gpt-6-astra, high)",
              "Reviewed at a1b2c3d4",
              "Verdict: APPROVE",
            ].join("\n"),
          },
        ]);
      }
      return jsonResponse({ message: "not found" }, 404);
    });

    const source = createGithubSource({ apiUrl: "http://github.test" });
    const listed = await source.listItems(REPO);
    expect(listed.lookup).toBe("complete");

    const item362 = listed.items.find((i) => i.number === 362)!;
    expect(item362).toMatchObject({
      ref: "C360Studio/semteams#362",
      claim: {
        state: "known",
        value: {
          kind: "agent-pr",
          agent_vendor: "gemini",
          holder: "gemini (gemini-4-argon, high)",
          implemented_by: "gemini (gemini-4-argon, high)",
          pull_request: "C360Studio/semteams#363",
          draft: false,
        },
      },
      review: {
        state: "known",
        value: {
          pull_request: "C360Studio/semteams#363",
          status: "current",
          head_sha: "a1b2c3d4e5f6789012345678901234567890abcd",
          request_sha: "a1b2c3d4",
          record_sha: "a1b2c3d4",
          reviewer_lane: "go-reviewer, svelte-reviewer",
          implemented_by: "gemini (gemini-4-argon, high)",
          reviewed_by: "codex (gpt-6-astra, high)",
        },
      },
    });
  });

  it("degrades review overlay to unknown and lookup to partial when an unparseable review record follows an earlier APPROVE at HEAD", async () => {
    stubBackend((url) => {
      if (url.pathname === "/repos/C360Studio/semteams/issues") {
        return jsonResponse([
          {
            number: 362,
            title: "Adopt ateam v0.3.2",
            body: "Clean upstream adoption.",
            state: "open",
            labels: [{ name: "area:governance" }],
            assignees: [],
          },
        ]);
      }
      if (url.pathname === "/repos/C360Studio/semteams/issues/362") {
        return jsonResponse({
          number: 362,
          title: "Adopt ateam v0.3.2",
          body: "Clean upstream adoption.",
          state: "open",
          labels: [{ name: "area:governance" }],
          assignees: [],
        });
      }
      if (url.pathname === "/repos/C360Studio/semteams/pulls") {
        return jsonResponse([
          {
            number: 363,
            title: "chore(harness): adopt ateam v0.3.2 (#362)",
            body: [
              "Closes #362",
              "",
              "implemented-by: gemini (gemini-4-argon, high)",
            ].join("\n"),
            state: "open",
            draft: false,
            head: {
              ref: "gemini/gh362-adopt-ateam-v032",
              sha: "a1b2c3d4e5f6789012345678901234567890abcd",
            },
          },
        ]);
      }
      if (url.pathname === "/repos/C360Studio/semteams/issues/363/comments") {
        return jsonResponse([
          {
            body: [
              "reviewed-by: codex (gpt-6-astra, high)",
              "Reviewer lane: go-reviewer",
              "Reviewed at a1b2c3d4",
              "Verdict: APPROVE",
            ].join("\n"),
          },
          {
            body: [
              "reviewed-by: claude (opus, high)",
              "Reviewer lane: svelte-reviewer",
              "Reviewed at a1b2c3d4",
              "Verdict: HOLD",
            ].join("\n"),
          },
        ]);
      }
      return jsonResponse({ message: "not found" }, 404);
    });

    const source = createGithubSource({ apiUrl: "http://github.test" });
    const listed = await source.listItems(REPO);
    expect(listed.lookup).toBe("partial");
    expect(listed.reason).toContain(
      "GET /repos/C360Studio/semteams/issues/363/comments contains an unparseable review record",
    );

    const item362 = listed.items.find((i) => i.number === 362)!;
    expect(item362.delivery).toMatchObject({
      state: "known",
      value: [
        {
          ref: "C360Studio/semteams#363",
          review_inspected: false,
        },
      ],
    });
    expect(item362.review).toEqual({
      state: "unknown",
      reason: "PR review comments not inspected",
    });

    const single = await source.getItem({ repository: REPO, number: 362 });
    expect(single.lookup).toBe("partial");
    expect(single.reason).toContain(
      "GET /repos/C360Studio/semteams/issues/363/comments contains an unparseable review record",
    );
    expect(single.item?.review).toEqual({
      state: "unknown",
      reason: "PR review comments not inspected",
    });
  });

  describe("paginated PR comment reads", () => {
    const head = "a1b2c3d4e5f6789012345678901234567890abcd";
    const record = (lane: string, verdict: string) => ({
      body: [
        "reviewed-by: codex (gpt-6-astra, high)",
        `Reviewer lane: ${lane}`,
        "Reviewed at a1b2c3d4",
        `Verdict: ${verdict}`,
      ].join("\n"),
    });
    const filler = (n: number) =>
      Array.from({ length: n }, (_, idx) => ({ body: `Discussion comment ${idx}` }));

    function stubPr(pageFor: (page: number) => unknown[]) {
      return stubBackend((url) => {
        if (url.pathname === "/repos/C360Studio/semteams/issues") {
          return jsonResponse([
            { number: 362, title: "Adopt", body: "", state: "open", labels: [], assignees: [] },
          ]);
        }
        if (url.pathname === "/repos/C360Studio/semteams/pulls") {
          return jsonResponse([
            {
              number: 363,
              title: "chore: adopt (#362)",
              body: "Closes #362",
              state: "open",
              draft: false,
              head: { ref: "gemini/gh362", sha: head },
            },
          ]);
        }
        if (url.pathname === "/repos/C360Studio/semteams/issues/363/comments") {
          return jsonResponse(pageFor(Number(url.searchParams.get("page") ?? "1")));
        }
        return jsonResponse({ message: "not found" }, 404);
      });
    }

    it("reads past a full first page so a later REQUEST CHANGES at HEAD is not hidden by an earlier APPROVE", async () => {
      const fetchMock = stubPr((page) =>
        page === 1
          ? [record("svelte-reviewer", "APPROVE"), ...filler(GITHUB_PAGE_LIMIT - 1)]
          : page === 2
            ? [record("svelte-reviewer", "REQUEST CHANGES")]
            : [],
      );
      const listed = await createGithubSource({ apiUrl: "http://github.test" }).listItems(REPO);
      expect(listed.lookup).toBe("complete");
      const item = listed.items.find((i) => i.number === 362)!;
      expect(item.review).toMatchObject({
        state: "known",
        value: { status: "changes_requested", reviewer_lane: "svelte-reviewer" },
      });
      const pages = fetchMock.mock.calls
        .map(([input]) => new URL(String(input)))
        .filter((u) => u.pathname.endsWith("/issues/363/comments"))
        .map((u) => u.searchParams.get("page"));
      expect(pages).toEqual(["1", "2"]);
    });

    it("fails closed to unknown when every comment page up to MAX_PR_COMMENT_PAGES is full", async () => {
      stubPr((page) =>
        page === 1
          ? [record("svelte-reviewer", "APPROVE"), ...filler(GITHUB_PAGE_LIMIT - 1)]
          : filler(GITHUB_PAGE_LIMIT),
      );
      const listed = await createGithubSource({ apiUrl: "http://github.test" }).listItems(REPO);
      expect(listed.lookup).toBe("partial");
      expect(listed.reason).toContain(
        `GET /repos/C360Studio/semteams/issues/363/comments truncated at ${MAX_PR_COMMENT_PAGES * GITHUB_PAGE_LIMIT} comments`,
      );
      const item = listed.items.find((i) => i.number === 362)!;
      expect(item.review).toEqual({ state: "unknown", reason: "PR review comments not inspected" });
    });

    it("keeps the per-lane blocking verdict through the HEAD aggregate on the live read path", async () => {
      stubPr(() => [record("svelte-reviewer", "REQUEST CHANGES"), record("go-reviewer", "APPROVE")]);
      const listed = await createGithubSource({ apiUrl: "http://github.test" }).listItems(REPO);
      const item = listed.items.find((i) => i.number === 362)!;
      expect(item.review).toMatchObject({
        state: "known",
        value: { status: "changes_requested", reviewer_lane: "svelte-reviewer" },
      });
    });
  });

  it("validates configs/portfolio-c360.json against validatePortfolio", async () => {
    const raw = JSON.parse(
      await readFile(resolve(process.cwd(), "../configs/portfolio-c360.json"), "utf8"),
    );
    const checked = validatePortfolio(raw);
    expect(checked.ok).toBe(true);
    if (checked.ok) {
      expect(checked.portfolio.programs[0].projects.map((p) => p.id)).toEqual([
        "semteams",
        "semengine",
        "semstreams",
      ]);
    }
  });
});

