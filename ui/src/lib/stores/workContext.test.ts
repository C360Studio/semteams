import { beforeEach, describe, expect, it } from "vitest";
import { makeItem } from "../../test-utils/work";
import type { Program } from "$lib/types/work";
import { formatWorkScopeHeader, resolveWorkScope, workContext } from "./workContext.svelte";

const PROGRAMS: Program[] = [
  {
    id: "c360",
    name: "c360",
    projects: [
      {
        id: "semteams",
        name: "SemTeams",
        repositories: [{ owner: "c360studio", name: "semteams" }],
      },
    ],
  },
];

describe("workContext store", () => {
  beforeEach(() => {
    workContext.clear();
  });

  it("resolves program and project lineage when repository matches portfolio", () => {
    const item = makeItem({
      ref: "c360studio/semteams#312",
      repository: "c360studio/semteams",
      number: 312,
      title: "Compare MQTT vs NATS for IoT edge deployments",
    });

    const scope = resolveWorkScope(item, PROGRAMS);
    expect(scope).toEqual({
      itemRef: "c360studio/semteams#312",
      title: "Compare MQTT vs NATS for IoT edge deployments",
      repository: "c360studio/semteams",
      programName: "c360",
      projectName: "SemTeams",
    });
    expect(formatWorkScopeHeader(scope)).toBe(
      "[Work item: c360studio/semteams#312 (c360 / SemTeams) — Compare MQTT vs NATS for IoT edge deployments]",
    );
  });

  it("falls back cleanly when repository is not found in programs", () => {
    const item = makeItem({
      ref: "acme/widgets#7",
      repository: "acme/widgets",
      number: 7,
      title: "Ship the widget",
    });

    const scope = resolveWorkScope(item, PROGRAMS);
    expect(scope).toEqual({
      itemRef: "acme/widgets#7",
      title: "Ship the widget",
      repository: "acme/widgets",
    });
    expect(formatWorkScopeHeader(scope)).toBe("[Work item: acme/widgets#7 — Ship the widget]");
  });

  it("sets and clears active scope", () => {
    expect(workContext.scope).toBeNull();
    workContext.setScope({
      itemRef: "c360studio/semteams#312",
      title: "Compare MQTT vs NATS",
      repository: "c360studio/semteams",
    });
    expect(workContext.scope?.itemRef).toBe("c360studio/semteams#312");
    workContext.clear();
    expect(workContext.scope).toBeNull();
  });
});
