// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("$env/dynamic/private", async () => ({
  env: (await import("$lib/server/work/testkit")).mockEnv,
}));

import * as route from "./+server";
import { setEnv } from "$lib/server/work/testkit";

afterEach(() => setEnv({}));

const call = (owner: string, repo: string, number: string) =>
  route.GET({ params: { owner, repo, number } } as unknown as Parameters<typeof route.GET>[0]);

describe("GET /api/work/items/{owner}/{repo}/{number}", () => {
  it("returns the overview fields, with two linked PRs as delivery context", async () => {
    setEnv({ WORK_SOURCE: "fixture" });
    const response = await call("c360studio", "semteams", "314");
    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.lookup).toBe("complete");
    expect(body.item).toMatchObject({
      ref: "c360studio/semteams#314",
      title: "Stamp run.issue.ref when a run starts from a work item",
      assignees: ["codex"],
      milestone: "v0.3.0",
      priority: "P1",
      column: "In Review",
    });
    expect(body.item.delivery.state).toBe("known");
    expect(body.item.delivery.value.map((pr: { ref: string }) => pr.ref)).toEqual([
      "c360studio/semteams#320",
      "c360studio/semteams#321",
    ]);
  });

  it("reports delivery context and dependencies honestly: unknown PRs, informational dependencies with a source", async () => {
    setEnv({ WORK_SOURCE: "fixture" });
    const body = await (await call("c360studio", "semteams", "310")).json();
    expect(body.item.delivery).toMatchObject({ state: "unknown" });
    expect(body.item.dependencies).toEqual([{ ref: "c360studio/semteams#314", source: "blocked-by" }]);
  });

  it("answers 404 for an item that does not exist and for a repository outside the portfolio", async () => {
    setEnv({ WORK_SOURCE: "fixture" });
    expect((await call("c360studio", "semteams", "9999")).status).toBe(404);
    expect((await call("someone", "else", "1")).status).toBe(404);
  });

  it("answers 503 WORK_SOURCE_UNCONFIGURED when the work source is unconfigured", async () => {
    setEnv({});
    const response = await call("c360studio", "semteams", "314");
    expect(response.status).toBe(503);
    expect(await response.json()).toMatchObject({ code: "WORK_SOURCE_UNCONFIGURED" });
  });

  it.each(["abc", "0", "-3", "1.5", "07x"])("answers 400 for item number %j", async (number) => {
    setEnv({ WORK_SOURCE: "fixture" });
    expect((await call("c360studio", "semteams", number)).status).toBe(400);
  });

  it("is read-only: only GET is exported, so POST answers 405", () => {
    expect(Object.keys(route)).toEqual(["GET"]);
  });
});
