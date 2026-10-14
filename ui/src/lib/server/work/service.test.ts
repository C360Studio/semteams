// @vitest-environment node
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { MockInstance } from "vitest";

vi.mock("$env/dynamic/private", async () => ({
  env: (await import("./testkit")).mockEnv,
}));

// The bundled fixture sets are valid by construction, so reaching the
// FIXTURE_INVALID path through the service needs a stand-in for their items.
const fixtureOverride = vi.hoisted(() => ({ items: undefined as unknown }));
vi.mock("./fixtureSource", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./fixtureSource")>();
  return {
    ...actual,
    loadFixtureSet: (name: string) => {
      const set = actual.loadFixtureSet(name);
      return set && fixtureOverride.items !== undefined ? { ...set, items: fixtureOverride.items } : set;
    },
  };
});

import { WORK_ITEM_REF_UNMATCHED_REASON } from "./overlays";
import { getItem, getItems, getPortfolio, getRuns } from "./service";
import { fakeBackend, setEnv, stubBackend, triple } from "./testkit";

let dir: string;
// Config problems are logged server-side; capture them so they neither clutter
// the run nor go unasserted.
let logged: MockInstance<typeof console.error>;

beforeEach(async () => {
  logged = vi.spyOn(console, "error").mockImplementation(() => {});
  dir = await mkdtemp(join(tmpdir(), "work-portfolio-"));
});

afterEach(async () => {
  setEnv({});
  fixtureOverride.items = undefined;
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  await rm(dir, { recursive: true, force: true });
});

const validDocument = {
  programs: [
    {
      id: "c360",
      name: "c360",
      projects: [
        {
          id: "semteams",
          name: "SemTeams",
          repositories: [
            { owner: "C360Studio", name: "semteams" },
          ],
        },
      ],
    },
  ],
};

async function useDocument(document: unknown): Promise<string> {
  const path = join(dir, "portfolio.json");
  await writeFile(path, typeof document === "string" ? document : JSON.stringify(document));
  setEnv({
    WORK_SOURCE: "github",
    SEMTEAMS_PORTFOLIO_PATH: path,
    GITHUB_API_URL: "http://github.test",
    GITHUB_TOKEN: "ghp_secret",
  });
  return path;
}

describe("github source", () => {
  it("serves the operator's portfolio document and names github as the source", async () => {
    await useDocument(validDocument);
    const { status, body } = await getPortfolio();
    expect(status).toBe(200);
    expect(body).toMatchObject({ source: "github", programs: [{ id: "c360" }] });
  });

  it("enumerates GitHub issues, PR claims, and review records through /api/work/items", async () => {
    await useDocument(validDocument);
    stubBackend((url) => {
      if (url.pathname === "/repos/C360Studio/semteams/issues") {
        return new Response(
          JSON.stringify([
            {
              number: 296,
              title: "Read-only work board",
              body: "Blocked by #290\n- [ ] #291 child",
              state: "open",
              labels: [{ name: "area:ui" }, { name: "status:needs-decision" }],
              assignees: [{ login: "coby" }],
              milestone: { title: "v0.2.0" },
            },
            {
              number: 318,
              title: "PR returned in issues endpoint should be ignored",
              state: "open",
              pull_request: { url: "https://api.github.com/repos/C360Studio/semteams/pulls/318" },
            },
          ]),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }
      if (url.pathname === "/repos/C360Studio/semteams/pulls") {
        return new Response(
          JSON.stringify([
            {
              number: 318,
              title: "ui(work): inline drawer (#296)",
              body: "Closes #296\n\nimplemented-by: Vega\nstop-point: Ready for review",
              state: "open",
              draft: false,
              head: { ref: "gemini/work-inline-drawer", sha: "0d948aaceb506471642117aa7236d3bda35cb4ed" },
            },
          ]),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }
      if (url.pathname === "/repos/C360Studio/semteams/issues/318/comments") {
        return new Response(
          JSON.stringify([
            {
              body: "<!-- semteams-review-record -->\ncommit: 0d948aac\nverdict: pass\nreviewer-lane: svelte-reviewer\nreviewed-by: svelte-reviewer",
            },
          ]),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }
      return new Response(JSON.stringify({ message: "not found" }), { status: 404 });
    });

    const { status, body } = await getItems("c360studio/semteams");
    expect(status).toBe(200);
    expect(body).toMatchObject({
      lookup: "complete",
      repository: "C360Studio/semteams",
      columns: ["Todo", "In Progress", "Done"],
      items: [
        {
          ref: "C360Studio/semteams#296",
          number: 296,
          column: "In Progress",
          claim: {
            state: "known",
            value: {
              kind: "agent-pr",
              agent_vendor: "gemini",
              holder: "Vega",
              pull_request: "C360Studio/semteams#318",
              stop_point: "Ready for review",
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
              { kind: "needs-decision" },
              { kind: "blocked", ref: "C360Studio/semteams#290" },
            ],
          },
        },
      ],
    });
  });

  it("answers failed for items when GitHub returns an error, never an empty complete list", async () => {
    await useDocument(validDocument);
    stubBackend(() => new Response(JSON.stringify({ message: "rate limit" }), { status: 403 }));
    const { status, body } = await getItems("c360studio/semteams");
    expect(status).toBe(200);
    expect(body).toMatchObject({
      lookup: "failed",
      reason: "GET /repos/C360Studio/semteams/issues answered 403",
      repository: "C360Studio/semteams",
      items: [],
    });
  });

  it("answers failed for one item when GitHub errors, and 404 when GitHub answers 404", async () => {
    await useDocument(validDocument);
    stubBackend(() => new Response(JSON.stringify({ message: "unavailable" }), { status: 502 }));
    const failed = await getItem("C360Studio", "semteams", "296");
    expect(failed.status).toBe(200);
    expect(failed.body).toEqual({
      lookup: "failed",
      reason: "GET /repos/C360Studio/semteams/issues/296 answered 502",
    });

    stubBackend(() => new Response(JSON.stringify({ message: "not found" }), { status: 404 }));
    const missing = await getItem("C360Studio", "semteams", "296");
    expect(missing.status).toBe(404);
    expect(missing.body).toMatchObject({ code: "ITEM_NOT_FOUND" });
  });

  it("resolves linked runs via work-scoped prompt or run.issue.ref, and reports partial when unmatched", async () => {
    await useDocument(validDocument);
    const loopId = "loop_296";
    const loopEntity = `c360.semteams.agentic-loop.agent.execution.${loopId}`;
    const runEntity = `c360.semteams.chain.agent.execution.${loopId}`;

    stubBackend((url) => {
      if (url.hostname === "github.test") {
        if (url.pathname === "/repos/C360Studio/semteams/issues/296") {
          return new Response(
            JSON.stringify({ number: 296, title: "Item 296", state: "open", labels: [], assignees: [] }),
            { status: 200, headers: { "Content-Type": "application/json" } },
          );
        }
        return new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } });
      }
      return fakeBackend({
        triples: [
          triple(
            loopEntity,
            "agent.loop.description",
            "[Work item: C360Studio/semteams#296 — Item 296]\nInvestigate board",
          ),
          triple(loopEntity, "agent.loop.role", "coordinator"),
          triple(loopEntity, "agent.loop.run", loopId),
          triple(loopEntity, "agent.run.entity-id", runEntity),
          triple(runEntity, "agent.run.phase", "executing"),
          triple(runEntity, "agent.run.handoff", loopId),
        ],
        loops: { [loopId]: { loop_id: loopId, state: "executing" } },
      })(url);
    });
    const matched = await getRuns("C360Studio", "semteams", "296");
    expect(matched.status).toBe(200);
    expect(matched.body).toMatchObject({
      lookup: "complete",
      runs: [
        {
          run_entity_id: runEntity,
          coordinator_loop_id: loopId,
          lookup: "complete",
          execution_stage: { state: "known", value: "executing" },
          needs_you: { state: "known", value: false },
        },
      ],
    });

    stubBackend((url) => {
      if (url.hostname === "github.test") {
        if (url.pathname === "/repos/C360Studio/semteams/issues/296") {
          return new Response(
            JSON.stringify({ number: 296, title: "Item 296", state: "open", labels: [], assignees: [] }),
            { status: 200, headers: { "Content-Type": "application/json" } },
          );
        }
        return new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } });
      }
      return fakeBackend({ triples: [] })(url);
    });
    const unmatched = await getRuns("C360Studio", "semteams", "296");
    expect(unmatched.status).toBe(200);
    expect(unmatched.body).toEqual({
      lookup: "partial",
      reason: WORK_ITEM_REF_UNMATCHED_REASON,
      runs: [],
    });
  });

  it("hydrates GitHub Projects v2 columns and pm_status through /api/work/items when project_board is configured", async () => {
    await useDocument({
      programs: [
        {
          id: "c360",
          name: "c360",
          projects: [
            {
              id: "semteams",
              name: "SemTeams",
              repositories: [
                {
                  owner: "C360Studio",
                  name: "semteams",
                  project_board: {
                    owner: "C360Studio",
                    number: 4,
                    status_field: "Status",
                    priority_field: "Priority",
                  },
                },
              ],
            },
          ],
        },
      ],
    });

    stubBackend((url) => {
      if (url.pathname === "/graphql") {
        return new Response(
          JSON.stringify({
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
                    ],
                  },
                  items: {
                    pageInfo: { hasNextPage: false },
                    nodes: [
                      {
                        content: {
                          __typename: "Issue",
                          number: 296,
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
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }
      if (url.pathname === "/repos/C360Studio/semteams/issues") {
        return new Response(
          JSON.stringify([
            {
              number: 296,
              title: "Read-only work board",
              state: "open",
              labels: [],
              assignees: [{ login: "coby" }],
            },
          ]),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }
      if (url.pathname === "/repos/C360Studio/semteams/pulls") {
        return new Response(JSON.stringify([]), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      return new Response(JSON.stringify({ message: "not found" }), { status: 404 });
    });

    const { status, body } = await getItems("c360studio/semteams");
    expect(status).toBe(200);
    expect(body).toMatchObject({
      lookup: "complete",
      columns: ["Todo", "In Progress", "In Review", "Done"],
      items: [
        {
          ref: "C360Studio/semteams#296",
          column: "In Review",
          pm_status: "In Review",
          priority: "P0",
        },
      ],
    });
  });

  it("still scopes to configured repositories", async () => {
    await useDocument(validDocument);
    expect((await getItems("someone/else")).status).toBe(404);
    expect((await getRuns("someone", "else", "1")).status).toBe(404);
  });
});

describe("invalid or missing portfolio document refuses the work API", () => {
  it("answers 503 with every validation issue on all four endpoints", async () => {
    await useDocument({
      programs: [
        { id: "c360", name: "c360", projects: [{ id: "x", name: "x", repositories: [{ owner: "o" }] }] },
        { id: "c360", name: "dup", projects: [] },
      ],
    });
    const results = [
      await getPortfolio(),
      await getItems("o/r"),
      await getItem("o", "r", "1"),
      await getRuns("o", "r", "1"),
    ];
    for (const { status, body } of results) {
      expect(status).toBe(503);
      expect(body).toMatchObject({ code: "PORTFOLIO_INVALID" });
      const issues = (body as { issues: { path: string; message: string }[] }).issues.map((i) => `${i.path}: ${i.message}`);
      expect(issues).toContain("$.programs[0].projects[0].repositories[0].name: required, must be a non-empty string");
      expect(issues).toContain('$.programs[1].id: duplicate program id "c360"');
    }
  });

  it("answers 503 when SEMTEAMS_PORTFOLIO_PATH is missing in github mode", async () => {
    setEnv({ WORK_SOURCE: "github" });
    const { status, body } = await getPortfolio();
    expect(status).toBe(503);
    expect(body).toMatchObject({ code: "PORTFOLIO_PATH_REQUIRED" });
  });

  it("answers 503 for an unreadable file and for malformed JSON", async () => {
    setEnv({ WORK_SOURCE: "github", SEMTEAMS_PORTFOLIO_PATH: join(dir, "absent.json") });
    expect(await getPortfolio()).toMatchObject({ status: 503, body: { code: "PORTFOLIO_UNREADABLE" } });
    await useDocument("{ not json");
    expect(await getPortfolio()).toMatchObject({ status: 503, body: { code: "PORTFOLIO_UNREADABLE" } });
  });

  it("logs the path and the raw read/parse error server-side and never sends them", async () => {

    setEnv({ WORK_SOURCE: "github", SEMTEAMS_PORTFOLIO_PATH: join(dir, "absent.json") });
    const missing = await getPortfolio();
    const path = await useDocument("{ not json");
    const malformed = await getPortfolio();

    for (const result of [missing, malformed]) {
      expect(result.status).toBe(503);
      const wire = JSON.stringify(result.body);
      expect(wire).not.toContain(dir);
      expect(wire).not.toContain("ENOENT");
      expect(wire).not.toMatch(/JSON|Unexpected|position/);
      expect(Object.keys(result.body).sort()).toEqual(["code", "error"]);
    }

    const lines = logged.mock.calls.map((call) => call.join(" "));
    expect(lines.some((l) => l.includes("PORTFOLIO_UNREADABLE") && l.includes(join(dir, "absent.json")) && l.includes("ENOENT"))).toBe(true);
    expect(lines.some((l) => l.includes(path) && /JSON|Unexpected|position/.test(l))).toBe(true);
  });

  it("keeps the document path out of the invalid-document 503 while still naming every issue", async () => {
    const path = await useDocument({ programs: [{ id: "p" }] });
    const { status, body } = await getPortfolio();
    expect(status).toBe(503);
    expect(JSON.stringify(body)).not.toContain(path);
    expect(body).toMatchObject({ code: "PORTFOLIO_INVALID" });
    expect((body as { issues: unknown[] }).issues.length).toBeGreaterThan(0);
    expect(logged.mock.calls.some((call) => call.join(" ").includes(path))).toBe(true);
  });

  it("does not echo an unrecognised WORK_SOURCE value back to the browser", async () => {
    setEnv({ WORK_SOURCE: "gitlab-secret-token" });
    const { status, body } = await getPortfolio();
    expect(status).toBe(503);
    expect(body).toMatchObject({ code: "WORK_CONFIG_INVALID" });
    expect(JSON.stringify(body)).not.toContain("gitlab-secret-token");
    expect(logged.mock.calls.some((call) => call.join(" ").includes("gitlab-secret-token"))).toBe(true);
  });

  it("answers 503 FIXTURE_INVALID with the issues when a fixture's fields are mistyped", async () => {
    setEnv({ WORK_SOURCE: "fixture" });
    // A non-string pm_status used to throw at read time and surface as a 500.
    fixtureOverride.items = {
      "c360studio/semteams": { items: [{ number: 1, title: "t", state: "open", pm_status: 5 }] },
    };
    for (const result of [
      await getPortfolio(),
      await getItems("c360studio/semteams"),
      await getItem("c360studio", "semteams", "1"),
      await getRuns("c360studio", "semteams", "1"),
    ]) {
      expect(result.status).toBe(503);
      expect(result.body).toMatchObject({
        code: "FIXTURE_INVALID",
        issues: [{ path: "$[c360studio/semteams].items[0].pm_status", message: "must be a string" }],
      });
    }
  });

  it("does not let a stray SEMTEAMS_PORTFOLIO_PATH override a fixture set", async () => {
    setEnv({ WORK_SOURCE: "fixture", SEMTEAMS_PORTFOLIO_PATH: join(dir, "absent.json") });
    const { status, body } = await getPortfolio();
    expect(status).toBe(200);
    expect(body).toMatchObject({ source: "fixture" });
  });
});

describe("unconfigured work source", () => {
  it("serves an empty portfolio marked unconfigured", async () => {
    setEnv({});
    expect(await getPortfolio()).toEqual({ status: 200, body: { source: "unconfigured", programs: [] } });
  });

  it("answers 503 WORK_SOURCE_UNCONFIGURED on items, item and runs, not a 404 about a repository", async () => {
    setEnv({});
    const fetchMock = stubBackend(fakeBackend({}));
    for (const result of [
      await getItems("c360studio/semteams"),
      await getItem("c360studio", "semteams", "1"),
      await getRuns("c360studio", "semteams", "1"),
    ]) {
      expect(result.status).toBe(503);
      expect(result.body).toMatchObject({ code: "WORK_SOURCE_UNCONFIGURED" });
    }
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("still answers a parameter error as 400 before looking at configuration", async () => {
    setEnv({});
    expect((await getItems(null)).status).toBe(400);
    expect((await getItem("o", "r", "x")).status).toBe(400);
    expect((await getRuns("o", "r", "x")).status).toBe(400);
  });
});
