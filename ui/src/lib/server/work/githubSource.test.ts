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
