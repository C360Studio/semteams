import { describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import WorkItemOverview from "./WorkItemOverview.svelte";
import { makeItem, makeRun } from "../../../test-utils/work";
import type { LinkedRunsState } from "$lib/utils/workView";
import type { WorkItem } from "$lib/types/work";

function renderOverview(
  overrides: { item?: WorkItem; linkedRuns?: LinkedRunsState; onloadruns?: () => void; onclose?: () => void } = {},
) {
  const props = {
    item: overrides.item ?? makeItem(),
    linkedRuns: overrides.linkedRuns ?? ({ status: "idle" } as LinkedRunsState),
    onloadruns: overrides.onloadruns ?? vi.fn(),
    onclose: overrides.onclose ?? vi.fn(),
  };
  return { ...render(WorkItemOverview, { props }), props };
}

describe("WorkItemOverview facts", () => {
  it("shows purpose, owner, priority, milestone and labels", () => {
    renderOverview({
      item: makeItem({
        ref: "acme/widgets#7",
        number: 7,
        title: "Ship the widget",
        body_excerpt: "Customers keep asking for widgets.",
        assignees: ["coby"],
        priority: "P1",
        milestone: "v0.3.0",
        labels: ["type:feature", "area:ui"],
      }),
    });

    expect(screen.getByRole("heading", { name: "acme/widgets#7" })).toBeInTheDocument();
    expect(screen.getByText("Ship the widget")).toBeInTheDocument();
    expect(screen.getByTestId("overview-purpose")).toHaveTextContent("Customers keep asking for widgets.");
    expect(screen.getByTestId("overview-owner")).toHaveTextContent("coby");
    expect(screen.getByTestId("overview-priority")).toHaveTextContent("P1");
    expect(screen.getByTestId("overview-milestone")).toHaveTextContent("v0.3.0");
    expect(screen.getByTestId("overview-labels")).toHaveTextContent("type:feature, area:ui");
  });

  it("says so when owner, priority and milestone are not set", () => {
    renderOverview();

    expect(screen.getByTestId("overview-owner")).toHaveTextContent("unassigned");
    expect(screen.getByTestId("overview-priority")).toHaveTextContent("not set");
    expect(screen.getByTestId("overview-milestone")).toHaveTextContent("none");
  });

  it("lists dependencies with where each came from", () => {
    renderOverview({
      item: makeItem({
        dependencies: [
          { ref: "acme/widgets#3", source: "blocked-by" },
          { ref: "acme/widgets#4", source: "task-list" },
        ],
      }),
    });

    const dependencies = screen.getByTestId("overview-dependencies");
    expect(dependencies).toHaveTextContent("acme/widgets#3 (blocked by)");
    expect(dependencies).toHaveTextContent("acme/widgets#4 (task list)");
  });

  it("renders delivery context as pull requests, none, or unknown with its reason", () => {
    const { unmount } = renderOverview({
      item: makeItem({
        delivery: {
          state: "known",
          value: [{ ref: "acme/widgets#20", title: "feat: widget", state: "open" }],
        },
      }),
    });
    expect(screen.getByTestId("overview-delivery")).toHaveTextContent("acme/widgets#20");
    expect(screen.getByTestId("overview-delivery")).toHaveTextContent("feat: widget");
    unmount();

    const none = renderOverview({ item: makeItem({ delivery: { state: "none" } }) });
    expect(screen.getByTestId("overview-delivery")).toHaveTextContent("no linked pull requests");
    none.unmount();

    renderOverview({
      item: makeItem({ delivery: { state: "unknown", reason: "source does not supply linked pull requests" } }),
    });
    const delivery = screen.getByTestId("overview-delivery");
    expect(delivery).toHaveAttribute("data-state", "unknown");
    expect(delivery).toHaveTextContent("unknown");
    expect(delivery).toHaveTextContent("source does not supply linked pull requests");
  });

  it("closes from its button", async () => {
    const user = userEvent.setup();
    const { props } = renderOverview();

    await user.click(screen.getByTestId("work-overview-close"));

    expect(props.onclose).toHaveBeenCalledOnce();
  });
});

describe("WorkItemOverview linked runs", () => {
  it("reads nothing until asked, then asks once", async () => {
    const user = userEvent.setup();
    const { props } = renderOverview();

    const section = screen.getByTestId("work-linked-runs");
    expect(section).toHaveAttribute("data-lookup", "not-loaded");
    expect(screen.queryByTestId("linked-run-row")).not.toBeInTheDocument();
    expect(props.onloadruns).not.toHaveBeenCalled();

    await user.click(screen.getByRole("button", { name: "Load linked runs" }));
    expect(props.onloadruns).toHaveBeenCalledOnce();
  });

  it("can be requested from the keyboard", async () => {
    const user = userEvent.setup();
    const { props } = renderOverview();

    const button = screen.getByTestId("load-linked-runs");
    button.focus();
    await user.keyboard("{Enter}");

    expect(props.onloadruns).toHaveBeenCalledOnce();
  });

  it("shows a loading status while the runs are read", () => {
    renderOverview({ linkedRuns: { status: "loading" } });

    expect(screen.getByRole("status")).toHaveTextContent(/loading linked runs/i);
    expect(screen.getByTestId("work-linked-runs")).toHaveAttribute("data-lookup", "loading");
  });

  it("shows a failed read with its code and a way to retry", async () => {
    const user = userEvent.setup();
    const { props } = renderOverview({
      linkedRuns: { status: "error", code: "ITEM_NOT_FOUND", message: "acme/widgets#1 does not exist" },
    });

    expect(screen.getByRole("alert")).toHaveTextContent("ITEM_NOT_FOUND");
    expect(screen.getByRole("alert")).toHaveTextContent("acme/widgets#1 does not exist");
    await user.click(screen.getByRole("button", { name: "Try again" }));
    expect(props.onloadruns).toHaveBeenCalledOnce();
  });

  it('says "no linked run" for a complete lookup that found none', () => {
    renderOverview({ linkedRuns: { status: "loaded", lookup: "complete", runs: [] } });

    expect(screen.getByTestId("work-linked-runs")).toHaveAttribute("data-lookup", "complete");
    expect(screen.getByTestId("no-linked-run")).toHaveTextContent("no linked run");
    expect(screen.queryByTestId("work-linked-runs-lookup")).not.toBeInTheDocument();
  });

  it.each(["partial", "failed", "unsupported"] as const)(
    'never says "no linked run" for a %s lookup; it says unknown with the reason',
    (lookup) => {
      renderOverview({
        linkedRuns: { status: "loaded", lookup, reason: "no run.issue.ref writer is wired", runs: [] },
      });

      const section = screen.getByTestId("work-linked-runs");
      expect(section).toHaveAttribute("data-lookup", lookup);
      expect(screen.queryByTestId("no-linked-run")).not.toBeInTheDocument();
      expect(section).not.toHaveTextContent(/^no linked run$/m);
      expect(screen.getByTestId("linked-runs-unknown")).toHaveTextContent("unknown");
      expect(screen.getByTestId("work-linked-runs-lookup")).toHaveTextContent("no run.issue.ref writer is wired");
    },
  );

  it("renders one row per run with separate stage, needs-you and verification badges", () => {
    renderOverview({
      linkedRuns: {
        status: "loaded",
        lookup: "complete",
        runs: [
          makeRun({ execution_stage: { state: "known", value: "awaiting_approval" }, needs_you: { state: "known", value: true } }),
          makeRun({
            run_entity_id: "acme.platform.chain.agent.execution.run-2",
            coordinator_loop_id: "loop-2",
            execution_stage: { state: "known", value: "completed" },
          }),
        ],
      },
    });

    const rows = screen.getAllByTestId("linked-run-row");
    expect(rows).toHaveLength(2);
    const first = within(rows[0]);
    expect(first.getByTestId("overlay-execution-stage")).toHaveTextContent("awaiting_approval");
    expect(first.getByTestId("overlay-needs-you")).toHaveTextContent("yes");
    expect(first.getByTestId("overlay-verification")).toHaveAttribute("data-state", "unknown");
    expect(within(rows[1]).getByTestId("overlay-execution-stage")).toHaveTextContent("completed");
  });

  it("shows a partial run as unknown with its reason, not as none", () => {
    renderOverview({
      linkedRuns: {
        status: "loaded",
        lookup: "partial",
        reason: "run entity c360.x.chain.agent.execution.dead has no facts",
        runs: [
          makeRun({
            run_entity_id: "c360.x.chain.agent.execution.dead",
            coordinator_loop_id: null,
            lookup: "partial",
            reason: "run entity c360.x.chain.agent.execution.dead has no facts",
            execution_stage: { state: "unknown", reason: "run entity c360.x.chain.agent.execution.dead has no facts" },
            needs_you: { state: "unknown", reason: "coordinator loop id unknown" },
          }),
        ],
      },
    });

    const row = within(screen.getByTestId("linked-run-row"));
    expect(row.getByTestId("overlay-execution-stage")).toHaveAttribute("data-state", "unknown");
    expect(row.getByTestId("overlay-needs-you")).toHaveAttribute("data-state", "unknown");
    expect(row.getByTestId("overlay-execution-stage")).toHaveAttribute(
      "title",
      "run entity c360.x.chain.agent.execution.dead has no facts",
    );
    expect(screen.getAllByText(/has no facts/).length).toBeGreaterThan(0);
  });

  it("does not trust a none overlay from an incomplete run lookup", () => {
    renderOverview({
      linkedRuns: {
        status: "loaded",
        lookup: "partial",
        reason: "loop unreadable",
        runs: [makeRun({ lookup: "partial", reason: "loop unreadable", execution_stage: { state: "none" } })],
      },
    });

    expect(screen.getByTestId("overlay-execution-stage")).toHaveAttribute("data-state", "unknown");
    expect(screen.getByTestId("overlay-execution-stage")).not.toHaveTextContent("no linked run");
  });

  it("links to the runs lens only for a run with a coordinator loop id", () => {
    renderOverview({
      linkedRuns: {
        status: "loaded",
        lookup: "complete",
        runs: [
          makeRun({ coordinator_loop_id: "loop-abc" }),
          makeRun({ run_entity_id: "acme.platform.chain.agent.execution.run-2", coordinator_loop_id: null }),
        ],
      },
    });

    const [withLoop, withoutLoop] = screen.getAllByTestId("linked-run-row");
    const link = within(withLoop).getByTestId("work-drill-in");
    expect(link).toHaveAttribute("href", "/?task=loop-abc");
    expect(within(withoutLoop).queryByTestId("work-drill-in")).not.toBeInTheDocument();
    expect(within(withoutLoop).getByTestId("work-drill-in-unavailable")).toHaveTextContent(
      /coordinator loop .* could not be resolved/,
    );
  });

  it("reloads from the loaded state", async () => {
    const user = userEvent.setup();
    const { props } = renderOverview({
      linkedRuns: { status: "loaded", lookup: "complete", runs: [makeRun()] },
    });

    await user.click(screen.getByRole("button", { name: "Reload linked runs" }));

    expect(props.onloadruns).toHaveBeenCalledOnce();
  });
});
