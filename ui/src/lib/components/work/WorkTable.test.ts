import { describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import WorkTable from "./WorkTable.svelte";
import { makeItem, makePortfolio, makeRun } from "../../../test-utils/work";
import { summarizeItemOverlays } from "$lib/utils/workView";
import type { RepositoryBoard } from "$lib/stores/workStore.svelte";

const overlaysFor = () => summarizeItemOverlays(undefined);

const items = [
  makeItem({ ref: "acme/widgets#1", number: 1, title: "Charlie", pm_status: "Todo", column: "Todo", milestone: "v2" }),
  makeItem({ ref: "acme/widgets#2", number: 2, title: "alpha", pm_status: "Done", column: "Done" }),
  makeItem({ ref: "acme/widgets#3", number: 3, title: "Bravo", pm_status: "In Review", column: "In Review", milestone: "v1" }),
];

function board(overrides: Partial<RepositoryBoard> = {}): RepositoryBoard {
  return {
    repository: "acme/widgets",
    loading: false,
    lookup: "complete",
    columns: ["Todo", "In Review", "Done"],
    items,
    ...overrides,
  };
}

function renderTable(extra: Record<string, unknown> = {}) {
  return render(WorkTable, {
    props: {
      programs: makePortfolio(["acme/widgets"]).programs,
      boards: { "acme/widgets": board() },
      overlaysFor,
      ...extra,
    },
  });
}

const order = () => screen.getAllByTestId("work-item-row").map((row) => row.getAttribute("data-item"));

describe("WorkTable", () => {
  it("renders one row per item under the documented columns", () => {
    renderTable();

    const headers = screen.getAllByRole("columnheader").map((h) => h.textContent?.replace(/[▲▼⇅]/g, "").trim());
    expect(headers).toEqual([
      "Item",
      "Title",
      "PM status",
      "Priority",
      "Milestone",
      "Assignees",
      "Execution stage",
      "Needs you",
      "Verification",
      "Linked runs",
    ]);
    expect(order()).toEqual(["acme/widgets#1", "acme/widgets#2", "acme/widgets#3"]);
    expect(screen.getByTestId("work-table")).toBeInTheDocument();
  });

  it("shows run facts as unknown with the reason until linked runs are loaded", () => {
    renderTable();

    const row = within(screen.getAllByTestId("work-item-row")[0]);
    for (const name of ["execution-stage", "needs-you", "verification", "linked-runs"]) {
      expect(row.getByTestId(`overlay-${name}`)).toHaveAttribute("data-state", "unknown");
    }
  });

  it("shows loaded run facts and a run count", () => {
    renderTable({
      overlaysFor: (ref: string) =>
        summarizeItemOverlays(
          ref === "acme/widgets#1"
            ? { status: "loaded", lookup: "complete", runs: [makeRun(), makeRun({ run_entity_id: "r2" })] }
            : undefined,
        ),
    });

    const row = within(screen.getAllByTestId("work-item-row")[0]);
    expect(row.getByTestId("overlay-linked-runs")).toHaveTextContent("2 runs");
    expect(row.getByTestId("overlay-execution-stage")).toHaveTextContent("completed");
  });

  it("sorts by title, flips direction, and exposes the sort in aria-sort", async () => {
    const user = userEvent.setup();
    renderTable();
    const titleHeader = screen.getByRole("columnheader", { name: /^Title/ });
    expect(titleHeader).toHaveAttribute("aria-sort", "none");

    await user.click(screen.getByTestId("sort-title"));
    expect(order()).toEqual(["acme/widgets#2", "acme/widgets#3", "acme/widgets#1"]);
    expect(titleHeader).toHaveAttribute("aria-sort", "ascending");

    await user.click(screen.getByTestId("sort-title"));
    expect(order()).toEqual(["acme/widgets#1", "acme/widgets#3", "acme/widgets#2"]);
    expect(titleHeader).toHaveAttribute("aria-sort", "descending");
  });

  it("sorts items without a milestone last in either direction", async () => {
    const user = userEvent.setup();
    renderTable();

    await user.click(screen.getByTestId("sort-milestone"));
    expect(order()).toEqual(["acme/widgets#3", "acme/widgets#1", "acme/widgets#2"]);

    await user.click(screen.getByTestId("sort-milestone"));
    expect(order()).toEqual(["acme/widgets#1", "acme/widgets#3", "acme/widgets#2"]);
  });

  it("sorts by PM status, using the column when no Project status is set", async () => {
    const user = userEvent.setup();
    renderTable();

    await user.click(screen.getByTestId("sort-pm_status"));

    expect(order()).toEqual(["acme/widgets#2", "acme/widgets#3", "acme/widgets#1"]);
  });

  it("sorts from the keyboard", async () => {
    const user = userEvent.setup();
    renderTable();

    screen.getByTestId("sort-title").focus();
    await user.keyboard("{Enter}");

    expect(order()[0]).toBe("acme/widgets#2");
  });

  it("selects an item from its row button and marks it pressed", async () => {
    const user = userEvent.setup();
    const onselect = vi.fn();
    renderTable({ onselect, selectedRef: "acme/widgets#3" });

    const buttons = screen.getAllByRole("button", { name: /^acme\/widgets#/ });
    expect(buttons[2]).toHaveAttribute("aria-pressed", "true");
    expect(buttons[0]).toHaveAttribute("aria-pressed", "false");

    await user.click(buttons[0]);
    expect(onselect).toHaveBeenCalledWith("acme/widgets#1");
  });

  it("states the repository lookup above the table", () => {
    renderTable({
      boards: { "acme/widgets": board({ lookup: "partial", reason: "project board read timed out" }) },
    });

    expect(screen.getByTestId("work-lookup")).toHaveTextContent("project board read timed out");
  });
});
