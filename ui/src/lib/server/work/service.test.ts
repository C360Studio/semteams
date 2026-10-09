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

import { getItem, getItems, getPortfolio, getRuns } from "./service";
import { GITHUB_ITEMS_UNSUPPORTED, GITHUB_RUNS_UNSUPPORTED } from "./source";
import { fakeBackend, setEnv, stubBackend } from "./testkit";

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
            { owner: "C360Studio", name: "semteams", project_board: { owner: "C360Studio", number: 3, status_field: "Status" } },
          ],
        },
      ],
    },
  ],
};

async function useDocument(document: unknown): Promise<string> {
  const path = join(dir, "portfolio.json");
  await writeFile(path, typeof document === "string" ? document : JSON.stringify(document));
  setEnv({ WORK_SOURCE: "github", SEMTEAMS_PORTFOLIO_PATH: path });
  return path;
}

describe("github source (not implemented until #273)", () => {
  it("serves the operator's portfolio document and names github as the source", async () => {
    await useDocument(validDocument);
    const { status, body } = await getPortfolio();
    expect(status).toBe(200);
    expect(body).toMatchObject({ source: "github", programs: [{ id: "c360" }] });
  });

  it("answers unsupported for items, never an empty complete list", async () => {
    await useDocument(validDocument);
    const { status, body } = await getItems("c360studio/semteams");
    expect(status).toBe(200);
    expect(body).toMatchObject({
      lookup: "unsupported",
      reason: GITHUB_ITEMS_UNSUPPORTED,
      repository: "C360Studio/semteams",
      items: [],
    });
  });

  it("answers unsupported for one item rather than 404", async () => {
    await useDocument(validDocument);
    const { status, body } = await getItem("C360Studio", "semteams", "296");
    expect(status).toBe(200);
    expect(body).toEqual({ lookup: "unsupported", reason: GITHUB_ITEMS_UNSUPPORTED });
  });

  it("answers unsupported for runs, never 'no linked run', and reads nothing from the backend", async () => {
    await useDocument(validDocument);
    const fetchMock = stubBackend(fakeBackend({}));
    const { status, body } = await getRuns("C360Studio", "semteams", "296");
    expect(status).toBe(200);
    expect(body).toEqual({ lookup: "unsupported", reason: GITHUB_RUNS_UNSUPPORTED, runs: [] });
    expect(fetchMock).not.toHaveBeenCalled();
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
