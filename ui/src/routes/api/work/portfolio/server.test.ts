// @vitest-environment node
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("$env/dynamic/private", async () => ({
  env: (await import("$lib/server/work/testkit")).mockEnv,
}));

import * as route from "./+server";
import { setEnv } from "$lib/server/work/testkit";

// Config problems are logged server-side; keep the run quiet.
beforeEach(() => {
  vi.spyOn(console, "error").mockImplementation(() => {});
});

afterEach(() => {
  setEnv({});
  vi.restoreAllMocks();
});

const call = () => route.GET({} as unknown as Parameters<typeof route.GET>[0]);

describe("GET /api/work/portfolio", () => {
  it("serves the fixture set's programs, projects and repositories", async () => {
    setEnv({ WORK_SOURCE: "fixture", WORK_FIXTURE_SET: "board-mvp" });
    const response = await call();
    expect(response.status).toBe(200);
    expect(response.headers.get("Cache-Control")).toBe("no-store");
    const body = await response.json();
    expect(body.source).toBe("fixture");
    const repositories = body.programs.flatMap((p: { projects: { repositories: unknown[] }[] }) =>
      p.projects.flatMap((project) => project.repositories),
    );
    expect(repositories).toHaveLength(2);
    expect(repositories[0]).toMatchObject({ owner: "c360studio", name: "semteams", project_board: { status_field: "Status" } });
  });

  it("defaults to the board-mvp set when WORK_FIXTURE_SET is unset", async () => {
    setEnv({ WORK_SOURCE: "fixture" });
    expect((await (await call()).json()).programs).toHaveLength(1);
  });

  it("is unconfigured, with no programs, when WORK_SOURCE is unset", async () => {
    setEnv({});
    const response = await call();
    expect(response.status).toBe(200);
    expect(await response.json()).toEqual({ source: "unconfigured", programs: [] });
  });

  it("answers 503 with a code, not an empty board, for a WORK_SOURCE it does not know", async () => {
    setEnv({ WORK_SOURCE: "gitlab" });
    const response = await call();
    expect(response.status).toBe(503);
    expect(await response.json()).toMatchObject({ code: "WORK_CONFIG_INVALID" });
  });

  it("answers 503 naming the available sets for an unknown fixture set", async () => {
    setEnv({ WORK_SOURCE: "fixture", WORK_FIXTURE_SET: "nope" });
    const response = await call();
    expect(response.status).toBe(503);
    const body = await response.json();
    expect(body.code).toBe("FIXTURE_SET_UNKNOWN");
    expect(body.error).toContain("board-mvp");
  });

  it("is read-only: only GET is exported, so SvelteKit answers 405 for every other method", () => {
    const exported = Object.keys(route);
    expect(exported).toEqual(["GET"]);
  });
});
