// @vitest-environment node
import { describe, expect, it } from "vitest";
import type { Repository } from "$lib/types/work";
import { columnFor, columnsFor, FALLBACK_COLUMNS, NO_STATUS_COLUMN } from "./columns";

const withBoard: Repository = {
  owner: "o",
  name: "r",
  project_board: { owner: "o", number: 1, status_field: "Status" },
};
const withoutBoard: Repository = { owner: "o", name: "r" };

describe("columnFor", () => {
  it("uses the Project status when a board is configured, regardless of state or assignees", () => {
    expect(columnFor({ state: "open", assignees: [], pm_status: "In Review" }, withBoard)).toBe("In Review");
    expect(columnFor({ state: "closed", assignees: ["a"], pm_status: "Todo" }, withBoard)).toBe("Todo");
  });

  it("puts an item with an empty Project status in No Status rather than guessing", () => {
    expect(columnFor({ state: "open", assignees: ["a"] }, withBoard)).toBe(NO_STATUS_COLUMN);
    expect(columnFor({ state: "closed", assignees: [], pm_status: "  " }, withBoard)).toBe(NO_STATUS_COLUMN);
  });

  it.each([
    ["closed", [], "Done"],
    ["closed", ["a"], "Done"],
    ["open", ["a"], "In Progress"],
    ["open", [], "Todo"],
  ] as const)("falls back to GitHub fields without a board: %s with %j assignees -> %s", (state, assignees, expected) => {
    expect(columnFor({ state, assignees: [...assignees] }, withoutBoard)).toBe(expected);
  });

  it("never reads execution state: a pm_status without a board is ignored", () => {
    expect(columnFor({ state: "open", assignees: [], pm_status: "Done" }, withoutBoard)).toBe("Todo");
  });
});

describe("columnsFor", () => {
  it("returns the fixed fallback columns without a board", () => {
    expect(columnsFor(withoutBoard, ["ignored"], ["Todo"])).toEqual([...FALLBACK_COLUMNS]);
  });

  it("keeps the board's status order and appends columns the options do not name", () => {
    expect(columnsFor(withBoard, ["Todo", "Done"], ["Done", NO_STATUS_COLUMN, "Todo"])).toEqual([
      "Todo",
      "Done",
      NO_STATUS_COLUMN,
    ]);
  });
});
