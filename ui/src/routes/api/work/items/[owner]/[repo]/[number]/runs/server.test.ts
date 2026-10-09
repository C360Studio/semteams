// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("$env/dynamic/private", async () => ({
  env: (await import("$lib/server/work/testkit")).mockEnv,
}));

import * as route from "./+server";
import { fakeBackend, requestedUrls, setEnv, stubBackend, triple } from "$lib/server/work/testkit";

afterEach(() => {
  setEnv({});
  vi.unstubAllGlobals();
});

const call = (owner: string, repo: string, number: string) =>
  route.GET({ params: { owner, repo, number } } as unknown as Parameters<typeof route.GET>[0]);

const PROMPT =
  "Compare MQTT vs NATS for IoT edge deployments — which has lower latency on constrained ARM devices?";
const LOOP_ID = "loop_7b3b867e";
const LOOP_ENTITY = `c360.semteams-e2e.agentic-loop.agent.execution.${LOOP_ID}`;
const RUN = `c360.semteams-e2e.chain.agent.execution.${LOOP_ID}`;

const liveWorld = {
  triples: [
    triple(LOOP_ENTITY, "agent.loop.description", PROMPT),
    triple(LOOP_ENTITY, "agent.loop.role", "coordinator"),
    triple(LOOP_ENTITY, "agent.loop.run", LOOP_ID),
    triple(LOOP_ENTITY, "agent.run.entity-id", RUN),
    triple(RUN, "agent.run.phase", "completed"),
    triple(RUN, "agent.run.outcome", "success"),
  ],
  loops: { [LOOP_ID]: { loop_id: LOOP_ID, state: "complete" } },
};

describe("GET /api/work/items/{owner}/{repo}/{number}/runs", () => {
  it("binds item A by coordinator prompt to the run it created, with separate overlays", async () => {
    setEnv({ WORK_SOURCE: "fixture", BACKEND_HOST: "backend:8080" });
    const fetchMock = stubBackend(fakeBackend(liveWorld));
    const response = await call("c360studio", "semteams", "312");
    expect(response.status).toBe(200);
    expect(await response.json()).toEqual({
      lookup: "complete",
      runs: [
        {
          run_entity_id: RUN,
          coordinator_loop_id: LOOP_ID,
          lookup: "complete",
          execution_stage: { state: "known", value: "completed" },
          needs_you: { state: "known", value: false },
          verification: { state: "unknown", reason: "no verification fact for research runs" },
        },
      ],
    });
    // Server-side reads go to the configured backend host.
    expect(new Set(requestedUrls(fetchMock).map((u) => u.host))).toEqual(new Set(["backend:8080"]));
  });

  it("answers complete with no runs for item B, which declares none, without touching the backend", async () => {
    setEnv({ WORK_SOURCE: "fixture" });
    const fetchMock = stubBackend(fakeBackend({}));
    const response = await call("c360studio", "semteams", "311");
    expect(response.status).toBe(200);
    expect(await response.json()).toEqual({ lookup: "complete", runs: [] });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("answers partial for item C, whose run entity does not exist, naming the id", async () => {
    setEnv({ WORK_SOURCE: "fixture" });
    stubBackend(fakeBackend(liveWorld));
    const body = await (await call("c360studio", "semteams", "313")).json();
    expect(body.lookup).toBe("partial");
    expect(body.reason).toContain("00000000-0000-4000-8000-00000000dead");
    expect(body.runs).toHaveLength(1);
    expect(body.runs[0]).toMatchObject({
      run_entity_id: "c360.semteams.chain.agent.execution.00000000-0000-4000-8000-00000000dead",
      execution_stage: { state: "unknown" },
    });
  });

  it("answers partial, not 'no linked run', for item A when no coordinator with its prompt is in the graph", async () => {
    setEnv({ WORK_SOURCE: "fixture" });
    stubBackend(fakeBackend({}));
    expect(await (await call("c360studio", "semteams", "312")).json()).toEqual({
      lookup: "partial",
      reason: "prompt binding matched no coordinator loop",
      runs: [],
    });
  });

  it("treats an item with no linked_runs key like item B in fixture mode", async () => {
    setEnv({ WORK_SOURCE: "fixture" });
    stubBackend(fakeBackend({}));
    expect(await (await call("c360studio", "semteams", "310")).json()).toEqual({ lookup: "complete", runs: [] });
  });

  it("reports failed, not none, when the backend is unreachable", async () => {
    setEnv({ WORK_SOURCE: "fixture" });
    stubBackend(() => {
      throw new Error("connect ECONNREFUSED");
    });
    const body = await (await call("c360studio", "semteams", "312")).json();
    expect(body.lookup).toBe("failed");
    expect(body.runs).toEqual([]);
    expect(body.reason).toContain("ECONNREFUSED");
  });

  it("answers 404 for an unknown item or repository and 400 for a bad number", async () => {
    setEnv({ WORK_SOURCE: "fixture" });
    const fetchMock = stubBackend(fakeBackend({}));
    expect((await call("c360studio", "semteams", "9999")).status).toBe(404);
    expect((await call("someone", "else", "1")).status).toBe(404);
    expect((await call("c360studio", "semteams", "x")).status).toBe(400);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("answers 503 WORK_SOURCE_UNCONFIGURED, without touching the backend, when the work source is unconfigured", async () => {
    setEnv({});
    const fetchMock = stubBackend(fakeBackend({}));
    const response = await call("c360studio", "semteams", "312");
    expect(response.status).toBe(503);
    expect(await response.json()).toMatchObject({ code: "WORK_SOURCE_UNCONFIGURED" });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("is read-only: only GET is exported, so POST answers 405", () => {
    expect(Object.keys(route)).toEqual(["GET"]);
  });
});
