import { beforeEach, describe, expect, it, vi } from "vitest";
import { agentApi } from "./agentApi";
import { userIdentity } from "$lib/stores/userIdentity.svelte";

const fetchMock = vi.fn();
beforeEach(() => {
  vi.stubGlobal("fetch", fetchMock);
  fetchMock.mockReset();
});

describe("frozen SemStreams dispatch boundary", () => {
  it("cancels through the command ingress with the requesting identity", async () => {
    fetchMock.mockResolvedValue({
      ok: true,
      json: async () => ({ status: "accepted" }),
    });
    await agentApi.cancelLoop("loop-current");
    const [url, options] = fetchMock.mock.calls[0];
    expect(url).toBe("/teams-dispatch/message");
    expect(JSON.parse(options.body)).toMatchObject({
      content: "/cancel loop-current",
      user_id: userIdentity.value,
    });
  });

  it("surfaces typed command refusal even when HTTP transport succeeded", async () => {
    fetchMock.mockResolvedValue({ ok: true, json: async () => ({ type: "error", content: "Not authorized to cancel this loop" }) });
    await expect(agentApi.cancelLoop("foreign-loop")).rejects.toThrow(/Not authorized/);
  });

  it("fails closed when a pending approval has no execution identity", async () => {
    await expect(
      agentApi.submitApproval("loop-current", {
        decision: "approve",
        execution_id: "",
      }),
    ).rejects.toThrow(/execution/i);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("forwards the reviewed execution unchanged and exposes stale-request conflicts", async () => {
    fetchMock.mockResolvedValue({
      ok: false,
      status: 409,
      statusText: "Conflict",
      json: async () => ({ error: "stale execution" }),
    });
    await expect(
      agentApi.submitApproval("loop-current", {
        decision: "approve",
        execution_id: "reviewed-execution",
      }),
    ).rejects.toMatchObject({ statusCode: 409 });
    expect(JSON.parse(fetchMock.mock.calls[0][1].body).execution_id).toBe(
      "reviewed-execution",
    );
  });
});
