// @vitest-environment node
import { describe, expect, it } from "vitest";
import type { Portfolio, Repository } from "$lib/types/work";
import { createFixtureSource, listFixtureSets, loadFixtureSet } from "./fixtureSource";
import { findRepository, validatePortfolio } from "./portfolio";
import { WorkConfigError } from "./config";
import type { LinkedRunBinding } from "./source";

const PROMPT =
  "Compare MQTT vs NATS for IoT edge deployments — which has lower latency on constrained ARM devices?";
const DEAD_RUN = "c360.semteams.chain.agent.execution.00000000-0000-4000-8000-00000000dead";

function boardMvp() {
  const set = loadFixtureSet("board-mvp");
  if (!set) throw new Error("board-mvp fixture set missing");
  const checked = validatePortfolio(set.portfolio);
  if (!checked.ok) throw new Error(JSON.stringify(checked.issues));
  const repo = (owner: string, name: string): Repository => {
    const found = findRepository(checked.portfolio, owner, name);
    if (!found) throw new Error(`${owner}/${name} missing`);
    return found;
  };
  return {
    portfolio: checked.portfolio,
    source: createFixtureSource(checked.portfolio, set.items),
    semteams: repo("c360studio", "semteams"),
    semsource: repo("c360studio", "semsource"),
  };
}

describe("fixture set discovery", () => {
  it("finds board-mvp and refuses anything that is not a bundled set name", () => {
    expect(listFixtureSets()).toContain("board-mvp");
    expect(loadFixtureSet("board-mvp")).toBeDefined();
    expect(loadFixtureSet("../board-mvp")).toBeUndefined();
    expect(loadFixtureSet("nope")).toBeUndefined();
  });
});

describe("board-mvp (design D5)", () => {
  it("has two repositories: one with a Project status board, one without", () => {
    const { semteams, semsource } = boardMvp();
    expect(semteams.project_board).toMatchObject({ status_field: "Status" });
    expect(semsource.project_board).toBeUndefined();
  });

  it("covers every Project status column on the board repository", async () => {
    const { source, semteams } = boardMvp();
    const result = await source.listItems(semteams);
    expect(result.lookup).toBe("complete");
    expect(result.columns).toEqual(["Todo", "In Progress", "In Review", "Done"]);
    for (const column of result.columns) {
      expect(result.items.some((i) => i.column === column), `no item in ${column}`).toBe(true);
    }
    expect(result.items.every((i) => result.columns.includes(i.column))).toBe(true);
  });

  it("covers closed, open+assignee and open+unassigned on the repository without a board", async () => {
    const { source, semsource } = boardMvp();
    const result = await source.listItems(semsource);
    expect(result.columns).toEqual(["Todo", "In Progress", "Done"]);
    const byColumn = (column: string) => result.items.filter((i) => i.column === column);
    expect(byColumn("Done").every((i) => i.state === "closed")).toBe(true);
    expect(byColumn("In Progress").every((i) => i.state === "open" && i.assignees.length > 0)).toBe(true);
    expect(byColumn("Todo").every((i) => i.state === "open" && i.assignees.length === 0)).toBe(true);
    for (const column of result.columns) expect(byColumn(column).length).toBeGreaterThan(0);
    expect(result.items.every((i) => i.pm_status === undefined && i.priority === undefined)).toBe(true);
  });

  it("has exactly one item with linked PRs as delivery context", async () => {
    const { source, semteams, semsource } = boardMvp();
    const items = [...(await source.listItems(semteams)).items, ...(await source.listItems(semsource)).items];
    const known = items.filter((i) => i.delivery.state === "known");
    expect(known).toHaveLength(1);
    const delivery = known[0].delivery;
    expect(delivery.state === "known" && delivery.value).toHaveLength(2);
    expect(items.filter((i) => i.delivery.state === "unknown").length).toBe(items.length - 1);
  });

  it("declares exactly the three linkage cases", async () => {
    const { source, semteams, semsource } = boardMvp();
    const declared: Record<string, LinkedRunBinding[]> = {};
    for (const repository of [semteams, semsource]) {
      for (const item of (await source.listItems(repository)).items) {
        const { bindings } = await source.linkedRunBindings({ repository, number: item.number });
        declared[item.ref] = bindings;
      }
    }
    const withBindings = Object.entries(declared).filter(([, b]) => b.length > 0);
    expect(withBindings.map(([, b]) => b)).toEqual(
      expect.arrayContaining([
        [{ by: "coordinator_prompt", equals: PROMPT }],
        [{ by: "run_entity_id", value: DEAD_RUN }],
      ]),
    );
    expect(withBindings).toHaveLength(2);

    // The explicit empty declaration (case B) is a key in the fixture; every
    // other undeclared item behaves the same in fixture mode.
    const raw = loadFixtureSet("board-mvp")!.items as Record<string, { items: { number: number; linked_runs?: unknown }[] }>;
    const declaredEmpty = Object.values(raw).flatMap((r) => r.items).filter((i) => Array.isArray(i.linked_runs) && i.linked_runs.length === 0);
    expect(declaredEmpty).toHaveLength(1);
  });
});

describe("fixture source behavior", () => {
  it("excerpts bodies, resolves refs against the configured spelling and keeps labels", async () => {
    const { source, semteams } = boardMvp();
    const { item } = await source.getItem({ repository: semteams, number: 314 });
    expect(item).toMatchObject({
      ref: "c360studio/semteams#314",
      repository: "c360studio/semteams",
      state: "open",
      column: "In Review",
      pm_status: "In Review",
      priority: "P1",
      milestone: "v0.3.0",
    });
    expect(item?.labels).toContain("type:feature");
    expect(item?.body_excerpt.length).toBeLessThanOrEqual(241);
  });

  it("answers complete with no item for an unknown number (the 404 path)", async () => {
    const { source, semteams } = boardMvp();
    expect(await source.getItem({ repository: semteams, number: 9999 })).toEqual({ lookup: "complete" });
    expect(await source.linkedRunBindings({ repository: semteams, number: 9999 })).toEqual({
      lookup: "complete",
      bindings: [],
    });
  });

  it("keeps dependencies informational and carries their source", async () => {
    const { source, semteams } = boardMvp();
    const { item } = await source.getItem({ repository: semteams, number: 310 });
    expect(item?.dependencies).toEqual([{ ref: "c360studio/semteams#314", source: "blocked-by" }]);
    expect(item?.column).toBe("Todo");
  });

  it("leaves PM status and priority unset where no source is configured, even if the fixture carries them", async () => {
    const portfolio: Portfolio = {
      programs: [{ id: "p", name: "p", projects: [{ id: "x", name: "x", repositories: [{ owner: "o", name: "plain" }] }] }],
    };
    const source = createFixtureSource(portfolio, {
      "o/plain": { items: [{ number: 1, title: "t", state: "open", pm_status: "Done", priority: "P0" }] },
    });
    const { items } = await source.listItems(portfolio.programs[0].projects[0].repositories[0]);
    expect(items[0]).not.toHaveProperty("pm_status");
    expect(items[0]).not.toHaveProperty("priority");
    expect(items[0].column).toBe("Todo");
  });

  it("puts an item with an empty Project status in No Status and appends that column", async () => {
    const portfolio: Portfolio = {
      programs: [
        {
          id: "p",
          name: "p",
          projects: [
            {
              id: "x",
              name: "x",
              repositories: [{ owner: "o", name: "b", project_board: { owner: "o", number: 1, status_field: "Status" } }],
            },
          ],
        },
      ],
    };
    const source = createFixtureSource(portfolio, {
      "o/b": { status_options: ["Todo", "Done"], items: [{ number: 1, title: "t", state: "open" }] },
    });
    const result = await source.listItems(portfolio.programs[0].projects[0].repositories[0]);
    expect(result.items[0].column).toBe("No Status");
    expect(result.columns).toEqual(["Todo", "Done", "No Status"]);
  });

  it("maps pull_requests: absent is unknown, empty is none, listed is known", async () => {
    const portfolio: Portfolio = {
      programs: [{ id: "p", name: "p", projects: [{ id: "x", name: "x", repositories: [{ owner: "o", name: "r" }] }] }],
    };
    const source = createFixtureSource(portfolio, {
      "o/r": {
        items: [
          { number: 1, title: "a", state: "open" },
          { number: 2, title: "b", state: "open", pull_requests: [] },
          { number: 3, title: "c", state: "open", pull_requests: [{ ref: "o/r#9" }] },
        ],
      },
    });
    const { items } = await source.listItems(portfolio.programs[0].projects[0].repositories[0]);
    expect(items.map((i) => i.delivery.state)).toEqual(["unknown", "none", "known"]);
    expect(items[0].delivery.reason).toBeTruthy();
  });

  it("rejects wrongly typed optional fields with named issues instead of throwing at read time", () => {
    const portfolio: Portfolio = {
      programs: [
        {
          id: "p",
          name: "p",
          projects: [
            {
              id: "x",
              name: "x",
              repositories: [{ owner: "o", name: "b", project_board: { owner: "o", number: 1, status_field: "Status" } }],
            },
          ],
        },
      ],
    };
    let error: unknown;
    try {
      createFixtureSource(portfolio, {
        "o/b": {
          items: [
            {
              number: 1,
              title: "t",
              state: "open",
              body: 7,
              milestone: false,
              pm_status: 5,
              priority: {},
              labels: "bug",
              assignees: [1],
              dependencies: [{ ref: "o/b#2", source: "mentions" }, "o/b#3"],
              pull_requests: [{ ref: "", title: 1, state: "draft" }],
            },
          ],
        },
      });
    } catch (err) {
      error = err;
    }
    expect(error).toBeInstanceOf(WorkConfigError);
    expect((error as WorkConfigError).code).toBe("FIXTURE_INVALID");
    const issues = (error as WorkConfigError).issues?.map((i) => `${i.path}: ${i.message}`) ?? [];
    expect(issues).toEqual(
      expect.arrayContaining([
        "$[o/b].items[0].body: must be a string",
        "$[o/b].items[0].milestone: must be a string",
        "$[o/b].items[0].pm_status: must be a string",
        "$[o/b].items[0].priority: must be a string",
        "$[o/b].items[0].labels: must be an array of strings",
        "$[o/b].items[0].assignees: must be an array of strings",
        '$[o/b].items[0].dependencies[0].source: must be "blocked-by" or "task-list"',
        "$[o/b].items[0].dependencies[1]: must be an object",
        "$[o/b].items[0].pull_requests[0].ref: required, must be a non-empty string",
        "$[o/b].items[0].pull_requests[0].title: must be a string",
        '$[o/b].items[0].pull_requests[0].state: must be "open", "merged" or "closed"',
      ]),
    );
  });

  it("accepts the optional fields when they are well typed", () => {
    const portfolio: Portfolio = {
      programs: [{ id: "p", name: "p", projects: [{ id: "x", name: "x", repositories: [{ owner: "o", name: "r" }] }] }],
    };
    expect(() =>
      createFixtureSource(portfolio, {
        "o/r": {
          items: [
            {
              number: 1,
              title: "t",
              state: "open",
              body: "b",
              milestone: "m",
              labels: ["a"],
              assignees: ["u"],
              dependencies: [{ ref: "o/r#2", source: "task-list" }],
              pull_requests: [{ ref: "o/r#3", title: "pr", state: "merged" }],
            },
          ],
        },
      }),
    ).not.toThrow();
  });

  it("rejects a malformed fixture with named issues", () => {
    const { portfolio } = boardMvp();
    let error: unknown;
    try {
      createFixtureSource(portfolio, {
        "c360studio/semteams": { items: [{ number: 1, title: "", state: "maybe" }, { number: 1, title: "x", state: "open" }] },
        "elsewhere/repo": { items: [] },
      });
    } catch (err) {
      error = err;
    }
    expect(error).toBeInstanceOf(WorkConfigError);
    const issues = (error as WorkConfigError).issues?.map((i) => `${i.path}: ${i.message}`) ?? [];
    expect((error as WorkConfigError).code).toBe("FIXTURE_INVALID");
    expect(issues).toContain("$[elsewhere/repo]: repository is not in the fixture portfolio");
    expect(issues).toContain("$[c360studio/semteams].items[0].title: required, must be a non-empty string");
    expect(issues).toContain('$[c360studio/semteams].items[0].state: must be "open" or "closed"');
    expect(issues).toContain("$[c360studio/semteams].items[1].number: duplicate item number 1");
  });
});
