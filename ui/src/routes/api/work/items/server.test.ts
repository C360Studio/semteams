// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("$env/dynamic/private", async () => ({
  env: (await import("$lib/server/work/testkit")).mockEnv,
}));

import * as route from "./+server";
import { setEnv } from "$lib/server/work/testkit";

afterEach(() => setEnv({}));

const call = (query: string) =>
  route.GET({ url: new URL(`http://ui.test/api/work/items${query}`) } as unknown as Parameters<typeof route.GET>[0]);

describe("GET /api/work/items", () => {
  it("lists a board repository's items with Project columns and PM fields", async () => {
    setEnv({ WORK_SOURCE: "fixture" });
    const response = await call("?repository=c360studio/semteams");
    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body).toMatchObject({
      lookup: "complete",
      repository: "c360studio/semteams",
      columns: ["Todo", "In Progress", "In Review", "Done"],
    });
    expect(body.reason).toBeUndefined();
    expect(body.next_cursor).toBeUndefined();
    const item = body.items.find((i: { number: number }) => i.number === 314);
    expect(item).toMatchObject({
      ref: "c360studio/semteams#314",
      column: "In Review",
      pm_status: "In Review",
      priority: "P1",
      state: "open",
    });
    // No attention field exists until Program Pulse (#267); it is never simulated.
    for (const entry of body.items) expect(entry).not.toHaveProperty("attention");
  });

  it("applies the GitHub-field fallback where there is no board, and leaves PM fields unset", async () => {
    setEnv({ WORK_SOURCE: "fixture" });
    const body = await (await call("?repository=c360studio/semsource")).json();
    expect(body.columns).toEqual(["Todo", "In Progress", "Done"]);
    const columns = Object.fromEntries(body.items.map((i: { number: number; column: string }) => [i.number, i.column]));
    expect(columns).toEqual({ 51: "In Progress", 52: "Todo", 53: "Done", 54: "Todo" });
    for (const entry of body.items) {
      expect(entry).not.toHaveProperty("pm_status");
      expect(entry).not.toHaveProperty("priority");
    }
  });

  it("matches the repository case-insensitively and reports the configured spelling", async () => {
    setEnv({ WORK_SOURCE: "fixture" });
    const body = await (await call("?repository=C360Studio/SemSource")).json();
    expect(body.repository).toBe("c360studio/semsource");
  });

  it("answers 400 when repository is missing or malformed", async () => {
    setEnv({ WORK_SOURCE: "fixture" });
    for (const query of ["", "?repository=", "?repository=semteams", "?repository=a/b/c", "?repository=/b"]) {
      const response = await call(query);
      expect(response.status, query).toBe(400);
      expect(await response.json()).toHaveProperty("code");
    }
  });

  it("answers 404 for a repository that is not in the portfolio", async () => {
    setEnv({ WORK_SOURCE: "fixture" });
    const response = await call("?repository=someone/else");
    expect(response.status).toBe(404);
    expect(await response.json()).toMatchObject({ code: "REPOSITORY_NOT_CONFIGURED" });
  });

  it("answers 503 WORK_SOURCE_UNCONFIGURED, not a repository 404, when the work source is unconfigured", async () => {
    setEnv({});
    const response = await call("?repository=c360studio/semteams");
    expect(response.status).toBe(503);
    expect(await response.json()).toMatchObject({ code: "WORK_SOURCE_UNCONFIGURED" });
    // Parameter errors still win over configuration state.
    expect((await call("")).status).toBe(400);
  });

  it("is read-only: only GET is exported, so POST answers 405", () => {
    expect(Object.keys(route)).toEqual(["GET"]);
  });
});
