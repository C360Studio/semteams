import { describe, it, expect, vi, afterEach } from "vitest";
import { POST } from "./+server";
import { createMCPServer } from "$lib/server/mcp/server";
import { createClaudeClient } from "$lib/server/mcp/claude";
import { createAiProvider } from "$lib/server/ai/provider";

vi.mock("$env/dynamic/private", () => ({ env: {} }));
vi.mock("$lib/server/mcp/server", () => ({ createMCPServer: vi.fn() }));
vi.mock("$lib/server/mcp/claude", () => ({ createClaudeClient: vi.fn(), ClaudeApiError: class extends Error {} }));
vi.mock("$lib/server/ai/provider", () => ({ createAiProvider: vi.fn() }));
afterEach(() => { vi.restoreAllMocks(); vi.clearAllMocks(); });

describe("retired flow-authoring endpoint", () => {
  it.each([false, true])("returns 410 before parsing, model or backend access (malformed=%s)", async (malformed) => {
    const network = vi.spyOn(globalThis, "fetch").mockRejectedValue(new Error("Network must not be called"));
    const parse = vi.fn();
    if (malformed) parse.mockRejectedValue(new Error("invalid JSON"));
    else parse.mockResolvedValue({ prompt: "Create a flow", messages: [{ role: "user", content: "Create a flow" }], currentFlow: {} });
    const response = await POST({ request: { headers: new Headers(), json: parse } } as unknown as Parameters<typeof POST>[0]);
    expect(response.status).toBe(410);
    expect(await response.json()).toMatchObject({ code: "FLOW_AUTHORING_RETIRED" });
    expect(parse).not.toHaveBeenCalled();
    expect(network).not.toHaveBeenCalled();
    expect(createMCPServer).not.toHaveBeenCalled();
    expect(createClaudeClient).not.toHaveBeenCalled();
    expect(createAiProvider).not.toHaveBeenCalled();
  });
});
