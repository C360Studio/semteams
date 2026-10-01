import { describe, expect, it } from "vitest";
import { parseEntityId } from "./graph";

describe("canonical framework entity identity", () => {
  it("reads system before domain and preserves an effective platform suffix", () => {
    expect(parseEntityId("c360.semteams-01jabc.agentic-loop.agent.execution.550e8400-e29b-41d4-a716-446655440000")).toEqual({
      org: "c360", platform: "semteams-01jabc", system: "agentic-loop", domain: "agent", type: "execution", instance: "550e8400-e29b-41d4-a716-446655440000",
    });
  });
});
