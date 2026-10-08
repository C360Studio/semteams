// @vitest-environment node
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("$env/dynamic/private", async () => ({
  env: (await import("./testkit")).mockEnv,
}));

import { getItem, getItems, getPortfolio, getRuns } from "./service";
import { GITHUB_ITEMS_UNSUPPORTED, GITHUB_RUNS_UNSUPPORTED } from "./source";
import { fakeBackend, setEnv, stubBackend } from "./testkit";

let dir: string;

beforeEach(async () => {
  dir = await mkdtemp(join(tmpdir(), "work-portfolio-"));
});

afterEach(async () => {
  setEnv({});
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

  it("does not let a stray SEMTEAMS_PORTFOLIO_PATH override a fixture set", async () => {
    setEnv({ WORK_SOURCE: "fixture", SEMTEAMS_PORTFOLIO_PATH: join(dir, "absent.json") });
    const { status, body } = await getPortfolio();
    expect(status).toBe(200);
    expect(body).toMatchObject({ source: "fixture" });
  });
});
