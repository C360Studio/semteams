import { describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import WorkBoard from "./WorkBoard.svelte";
import { makeItem, makePortfolio } from "../../../test-utils/work";
import { RUN_FACTS_LENS_NOTE, summarizeItemOverlays } from "$lib/utils/workView";
import type { RepositoryBoard } from "$lib/stores/workStore.svelte";

const overlaysFor = () => summarizeItemOverlays(undefined);

function board(repository: string, overrides: Partial<RepositoryBoard> = {}): RepositoryBoard {
  return {
    repository,
    loading: false,
    lookup: "complete",
    columns: ["Todo", "In Progress", "Done"],
    items: [],
    ...overrides,
  };
}

describe("WorkBoard", () => {
  it("renders the source's columns in order, including empty ones, with cards inside", () => {
    const widgets = board("acme/widgets", {
      columns: ["Todo", "In Review", "Done"],
      items: [
        makeItem({ ref: "acme/widgets#1", number: 1, title: "First", column: "Done" }),
        makeItem({ ref: "acme/widgets#2", number: 2, title: "Second", column: "Todo" }),
      ],
    });
    render(WorkBoard, {
      props: { programs: makePortfolio(["acme/widgets"]).programs, boards: { "acme/widgets": widgets }, overlaysFor },
    });

    const repository = within(screen.getByTestId("work-repository-acme-widgets"));
    const columns = repository.getAllByTestId(/^work-column-/);
    expect(columns.map((c) => c.getAttribute("data-testid"))).toEqual([
      "work-column-todo",
      "work-column-in-review",
      "work-column-done",
    ]);
    expect(within(columns[1]).getByText("No items")).toBeInTheDocument();
    expect(within(columns[0]).getByTestId("work-item-card")).toHaveAttribute("data-item", "acme/widgets#2");
    expect(within(columns[2]).getByTestId("work-item-card")).toHaveAttribute("data-item", "acme/widgets#1");
  });

  it("keeps an item whose column the source did not name", () => {
    const widgets = board("acme/widgets", {
      columns: ["Todo"],
      items: [makeItem({ column: "No Status" })],
    });
    render(WorkBoard, {
      props: { programs: makePortfolio(["acme/widgets"]).programs, boards: { "acme/widgets": widgets }, overlaysFor },
    });

    expect(within(screen.getByTestId("work-column-no-status")).getByTestId("work-item-card")).toBeInTheDocument();
  });

  it("renders program, project and repository sections with headings", () => {
    render(WorkBoard, {
      props: {
        programs: makePortfolio(["acme/widgets", "acme/gadgets"]).programs,
        boards: { "acme/widgets": board("acme/widgets"), "acme/gadgets": board("acme/gadgets") },
        overlaysFor,
      },
    });

    expect(screen.getByRole("heading", { level: 2, name: "Acme" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 3, name: "Platform" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 4, name: "acme/widgets" })).toBeInTheDocument();
    expect(screen.getByTestId("work-repository-acme-gadgets")).toBeInTheDocument();
    expect(screen.getByTestId("work-board")).toBeInTheDocument();
  });

  it("states a repository's lookup status and reason above its columns when not complete", () => {
    render(WorkBoard, {
      props: {
        programs: makePortfolio(["acme/widgets", "acme/gadgets"]).programs,
        boards: {
          "acme/widgets": board("acme/widgets", { lookup: "partial", reason: "project board read timed out" }),
          "acme/gadgets": board("acme/gadgets"),
        },
        overlaysFor,
      },
    });

    const widgets = within(screen.getByTestId("work-repository-acme-widgets"));
    expect(widgets.getByTestId("work-lookup")).toHaveAttribute("data-lookup", "partial");
    expect(widgets.getByTestId("work-lookup")).toHaveTextContent("project board read timed out");
    expect(within(screen.getByTestId("work-repository-acme-gadgets")).queryByTestId("work-lookup")).toBeNull();
  });

  it("does not present a failed repository as an empty board", () => {
    render(WorkBoard, {
      props: {
        programs: makePortfolio(["acme/widgets"]).programs,
        boards: {
          "acme/widgets": board("acme/widgets", { lookup: "failed", reason: "items request failed", columns: [] }),
        },
        overlaysFor,
      },
    });

    expect(screen.getByTestId("work-lookup")).toHaveTextContent("lookup failed");
    expect(screen.getByText("No items could be read for this repository.")).toBeInTheDocument();
    expect(screen.queryByTestId(/^work-column-/)).not.toBeInTheDocument();
  });

  it("shows a loading status for a repository still being read", () => {
    render(WorkBoard, {
      props: {
        programs: makePortfolio(["acme/widgets"]).programs,
        boards: { "acme/widgets": board("acme/widgets", { loading: true }) },
        overlaysFor,
      },
    });

    expect(within(screen.getByTestId("work-repository-acme-widgets")).getByRole("status")).toHaveTextContent(
      /loading items/i,
    );
  });

  it("selects through the cards and marks the selected one pressed", async () => {
    const user = userEvent.setup();
    const onselect = vi.fn();
    const widgets = board("acme/widgets", {
      items: [
        makeItem({ ref: "acme/widgets#1", number: 1, title: "First" }),
        makeItem({ ref: "acme/widgets#2", number: 2, title: "Second" }),
      ],
    });
    render(WorkBoard, {
      props: {
        programs: makePortfolio(["acme/widgets"]).programs,
        boards: { "acme/widgets": widgets },
        overlaysFor,
        selectedRef: "acme/widgets#2",
        onselect,
      },
    });

    const cards = screen.getAllByTestId("work-item-card");
    expect(cards[0]).toHaveAttribute("aria-pressed", "false");
    expect(cards[1]).toHaveAttribute("aria-pressed", "true");

    await user.click(cards[0]);
    expect(onselect).toHaveBeenCalledWith("acme/widgets#1");
  });

  it("offers no drag affordance on the board", () => {
    const widgets = board("acme/widgets", { items: [makeItem()] });
    const { container } = render(WorkBoard, {
      props: { programs: makePortfolio(["acme/widgets"]).programs, boards: { "acme/widgets": widgets }, overlaysFor },
    });

    expect(container.querySelector("[draggable]")).toBeNull();
  });

  it("says once, in visible text, why run-derived badges are unknown, instead of on every card", () => {
    const widgets = board("acme/widgets", {
      items: [
        makeItem({ ref: "acme/widgets#1", number: 1 }),
        makeItem({ ref: "acme/widgets#2", number: 2 }),
      ],
    });
    render(WorkBoard, {
      props: { programs: makePortfolio(["acme/widgets"]).programs, boards: { "acme/widgets": widgets }, overlaysFor },
    });

    const note = screen.getByTestId("run-facts-note");
    expect(note).toBeVisible();
    expect(note).toHaveTextContent(RUN_FACTS_LENS_NOTE);
    expect(screen.getAllByTestId("run-facts-note")).toHaveLength(1);
  });
});
