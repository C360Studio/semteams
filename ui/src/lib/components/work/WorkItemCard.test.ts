import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import WorkItemCard from "./WorkItemCard.svelte";
import { makeItem } from "../../../test-utils/work";
import { RUNS_NOT_LOADED, summarizeItemOverlays } from "$lib/utils/workView";

const unloaded = summarizeItemOverlays(undefined);

describe("WorkItemCard", () => {
  it("shows the reference, title, labels, assignees and milestone", () => {
    render(WorkItemCard, {
      props: {
        item: makeItem({
          ref: "acme/widgets#7",
          number: 7,
          title: "Ship the widget",
          labels: ["type:feature", "area:ui"],
          assignees: ["coby", "claude"],
          milestone: "v0.3.0",
        }),
        overlays: unloaded,
      },
    });

    expect(screen.getByText("acme/widgets#7")).toBeInTheDocument();
    expect(screen.getByText("Ship the widget")).toBeInTheDocument();
    expect(screen.getByText("type:feature")).toBeInTheDocument();
    expect(screen.getByText("area:ui")).toBeInTheDocument();
    expect(screen.getByTestId("card-assignees")).toHaveTextContent("Assigned: coby, claude");
    expect(screen.getByTestId("card-milestone")).toHaveTextContent("Milestone: v0.3.0");
  });

  it("says unassigned and omits the milestone when there are none", () => {
    render(WorkItemCard, { props: { item: makeItem(), overlays: unloaded } });

    expect(screen.getByTestId("card-assignees")).toHaveTextContent("Unassigned");
    expect(screen.queryByTestId("card-milestone")).not.toBeInTheDocument();
  });

  it("renders PM status, stage, needs-you and verification as four separate badges", () => {
    render(WorkItemCard, {
      props: { item: makeItem({ pm_status: "In Review", column: "In Review" }), overlays: unloaded },
    });

    const pm = screen.getByTestId("overlay-pm-status");
    const stage = screen.getByTestId("overlay-execution-stage");
    const needsYou = screen.getByTestId("overlay-needs-you");
    const verification = screen.getByTestId("overlay-verification");

    expect(new Set([pm, stage, needsYou, verification]).size).toBe(4);
    expect(pm).toHaveAttribute("data-state", "known");
    expect(pm).toHaveTextContent("In Review");
    // Run facts are not read for the board: unknown with the reason, never none.
    for (const badge of [stage, needsYou, verification]) {
      expect(badge).toHaveAttribute("data-state", "unknown");
      expect(badge).toHaveAttribute("title", RUNS_NOT_LOADED);
    }
  });

  it("uses the column as PM status, labelled as derived, when no Project status is set", () => {
    render(WorkItemCard, {
      props: { item: makeItem({ column: "In Progress" }), overlays: unloaded },
    });

    const pm = screen.getByTestId("overlay-pm-status");
    expect(pm).toHaveTextContent("In Progress");
    expect(pm.getAttribute("title")).toMatch(/derived from GitHub fields/);
  });

  it("shows loaded run facts without moving the card's column", () => {
    const overlays = summarizeItemOverlays({
      status: "loaded",
      lookup: "complete",
      runs: [
        {
          run_entity_id: "r1",
          coordinator_loop_id: "l1",
          lookup: "complete",
          execution_stage: { state: "known", value: "failed" },
          needs_you: { state: "known", value: true, reason: "coordinator loop is awaiting approval" },
          verification: { state: "unknown", reason: "no verification fact for research runs" },
        },
      ],
    });
    render(WorkItemCard, { props: { item: makeItem({ column: "Todo" }), overlays } });

    expect(screen.getByTestId("overlay-execution-stage")).toHaveTextContent("failed");
    expect(screen.getByTestId("overlay-needs-you")).toHaveTextContent("yes");
    expect(screen.getByTestId("work-item-card")).toHaveAttribute("data-column", "Todo");
  });

  it("is a button with no drag affordance anywhere", () => {
    const { container } = render(WorkItemCard, { props: { item: makeItem(), overlays: unloaded } });

    const card = screen.getByRole("button");
    expect(card).toBe(screen.getByTestId("work-item-card"));
    expect(card).toHaveAttribute("type", "button");
    expect(container.querySelector("[draggable]")).toBeNull();
  });

  it("selects on click and reflects selection with aria-pressed", async () => {
    const user = userEvent.setup();
    const onselect = vi.fn();
    const { rerender } = render(WorkItemCard, {
      props: { item: makeItem({ ref: "acme/widgets#4", number: 4 }), overlays: unloaded, onselect },
    });
    const card = screen.getByRole("button");
    expect(card).toHaveAttribute("aria-pressed", "false");

    await user.click(card);
    expect(onselect).toHaveBeenCalledWith("acme/widgets#4");

    await rerender({ selected: true });
    expect(card).toHaveAttribute("aria-pressed", "true");
  });

  it("is operable from the keyboard", async () => {
    const user = userEvent.setup();
    const onselect = vi.fn();
    render(WorkItemCard, { props: { item: makeItem(), overlays: unloaded, onselect } });

    await user.tab();
    expect(screen.getByRole("button")).toHaveFocus();
    await user.keyboard("{Enter}");
    await user.keyboard(" ");

    expect(onselect).toHaveBeenCalledTimes(2);
  });

  it("is named by its reference and title, and described by everything else on it", () => {
    render(WorkItemCard, {
      props: {
        item: makeItem({
          ref: "acme/widgets#7",
          number: 7,
          title: "Ship the widget",
          labels: ["type:feature"],
          assignees: ["coby"],
          milestone: "v0.3.0",
          pm_status: "In Review",
          column: "In Review",
        }),
        overlays: unloaded,
      },
    });

    const card = screen.getByRole("button", { name: "acme/widgets#7 Ship the widget" });
    // A long badge/meta string is a description, not part of the name a screen reader lists.
    expect(card).toHaveAccessibleDescription(/type:feature/);
    expect(card).toHaveAccessibleDescription(/Assigned: coby/);
    expect(card).toHaveAccessibleDescription(/Milestone: v0\.3\.0/);
    expect(card).toHaveAccessibleDescription(/PM status\s*In Review/);
    expect(card).toHaveAccessibleDescription(/Needs you unknown/);
    expect(card).toHaveAccessibleDescription(new RegExp(RUNS_NOT_LOADED));
  });

  it("gives each card its own name and description ids", () => {
    render(WorkItemCard, { props: { item: makeItem({ ref: "acme/widgets#1" }), overlays: unloaded } });
    render(WorkItemCard, { props: { item: makeItem({ ref: "acme/widgets#2", number: 2 }), overlays: unloaded } });

    const [first, second] = screen.getAllByTestId("work-item-card");
    expect(first.getAttribute("aria-labelledby")).not.toBe(second.getAttribute("aria-labelledby"));
    expect(first.getAttribute("aria-describedby")).not.toBe(second.getAttribute("aria-describedby"));
  });
});
