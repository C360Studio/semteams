import { describe, expect, it, vi } from "vitest";
import { getComposition } from "./compositionApi";

describe("running composition inventory", () => {
  it("reads the admitted graph and preserves validation findings", async () => {
    const graph = { nodes: [{ instance: "teams-loop", factory: "agentic-loop", type: "processor", inputs: [], outputs: [] }], edges: [] };
    const validation = { status: "warnings", errors: [], warnings: [{ component: "teams-loop", message: "External input", severity: "warning", type: "orphaned_port", suggestions: [] }], graph };
    const request = vi.fn().mockImplementation(async (url) => ({ ok: true, json: async () => url.endsWith("flowgraph") ? graph : validation }));
    await expect(getComposition(request)).resolves.toEqual({ graph, validation });
    expect(request.mock.calls.map(([url]) => url).sort()).toEqual(["/components/flowgraph", "/components/validate"]);
  });
  it("surfaces unavailable admission rather than claiming an empty inventory", async () => {
    const request = vi.fn().mockResolvedValue({ ok: false, status: 503, statusText: "Service Unavailable" });
    await expect(getComposition(request)).rejects.toThrow(/503/);
  });
});
