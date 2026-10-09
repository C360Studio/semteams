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

  it("closes from its button, which names the item it closes", async () => {
    const user = userEvent.setup();
    const { props } = renderOverview();

    await user.click(screen.getByRole("button", { name: /^Close\s*overview of acme\/widgets#1$/ }));

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

describe("WorkItemOverview focus", () => {
  it("moves focus to its heading when it opens, so keyboard and screen reader users land on the panel", () => {
    renderOverview({ item: makeItem({ ref: "acme/widgets#7", number: 7 }) });

    const heading = screen.getByRole("heading", { name: "acme/widgets#7" });
    expect(heading).toHaveFocus();
    // Focusable by script only: it must not become a tab stop.
    expect(heading).toHaveAttribute("tabindex", "-1");
  });

  it("moves focus to the heading again when a different item opens in the same panel", async () => {
    const { rerender } = renderOverview({ item: makeItem({ ref: "acme/widgets#7", number: 7 }) });
    screen.getByTestId("load-linked-runs").focus();

    await rerender({ item: makeItem({ ref: "acme/widgets#8", number: 8 }) });

    expect(screen.getByRole("heading", { name: "acme/widgets#8" })).toHaveFocus();
  });

  it("does not take focus back when the same item is re-read, e.g. after a refresh", async () => {
    const { rerender } = renderOverview({ item: makeItem({ ref: "acme/widgets#7", number: 7 }) });
    const button = screen.getByTestId("load-linked-runs");
    button.focus();

    await rerender({ item: makeItem({ ref: "acme/widgets#7", number: 7, title: "Retitled" }) });

    expect(button).toHaveFocus();
  });

  it("tabs from the heading to Close and then to the runs button", async () => {
    const user = userEvent.setup();
    renderOverview();

    await user.tab();
    expect(screen.getByTestId("work-overview-close")).toHaveFocus();
    await user.tab();
    expect(screen.getByTestId("load-linked-runs")).toHaveFocus();
  });
});

describe("WorkItemOverview linked-runs control and live region", () => {
  it("keeps the same button mounted, focused and unavailable while runs load, and ignores activation", async () => {
    const user = userEvent.setup();
    const { props, rerender } = renderOverview();
    const button = screen.getByTestId("load-linked-runs");
    button.focus();

    await rerender({ linkedRuns: { status: "loading" } });

    expect(screen.getByTestId("load-linked-runs")).toBe(button);
    expect(button).toHaveFocus();
    expect(button).toHaveAttribute("aria-busy", "true");
    expect(button).toHaveAttribute("aria-disabled", "true");
    await user.click(button);
    await user.keyboard("{Enter}");
    expect(props.onloadruns).not.toHaveBeenCalled();
  });

  it("keeps the same button through loading, loaded, reload and failure, and is available again afterwards", async () => {
    const user = userEvent.setup();
    const { props, rerender } = renderOverview();
    const button = screen.getByTestId("load-linked-runs");
    button.focus();

    await rerender({ linkedRuns: { status: "loading" } });
    await rerender({ linkedRuns: { status: "loaded", lookup: "complete", runs: [makeRun()] } });
    expect(screen.getByTestId("load-linked-runs")).toBe(button);
    expect(button).toHaveFocus();
    expect(button).not.toHaveAttribute("aria-busy", "true");
    expect(button).not.toHaveAttribute("aria-disabled", "true");
    expect(button).toHaveAccessibleName("Reload linked runs");

    await user.click(button);
    expect(props.onloadruns).toHaveBeenCalledOnce();

    await rerender({ linkedRuns: { status: "loading" } });
    await rerender({ linkedRuns: { status: "error", code: "HTTP_500", message: "boom" } });
    expect(screen.getByTestId("load-linked-runs")).toBe(button);
    expect(button).toHaveFocus();
    expect(button).toHaveAccessibleName("Try again");
  });

  it("announces the result through one live region that is mounted before anything is loaded", async () => {
    const { rerender } = renderOverview();
    const region = screen.getByRole("status");
    expect(region.textContent?.trim()).toBe("");

    await rerender({ linkedRuns: { status: "loading" } });
    expect(screen.getByRole("status")).toBe(region);
    expect(region).toHaveTextContent(/loading linked runs/i);

    await rerender({ linkedRuns: { status: "loaded", lookup: "complete", runs: [] } });
    expect(screen.getByRole("status")).toBe(region);
    expect(region).toHaveTextContent("no linked run");

    await rerender({
      linkedRuns: { status: "loaded", lookup: "partial", reason: "one run unreadable", runs: [makeRun()] },
    });
    expect(screen.getAllByRole("status")).toHaveLength(1);
    expect(region).toHaveTextContent("1 linked run read");
    expect(region).toHaveTextContent("partial result");
    expect(region).toHaveTextContent("one run unreadable");
  });
});

describe("WorkItemOverview visible reasons and wording", () => {
  it("shows the reason for every unknown run fact as visible text, not only in a title", () => {
    renderOverview({
      linkedRuns: {
        status: "loaded",
        lookup: "complete",
        runs: [
          makeRun({
            needs_you: { state: "unknown", reason: "coordinator loop unreadable" },
            verification: { state: "unknown", reason: "no verification fact for research runs" },
          }),
        ],
      },
    });

    const row = within(screen.getByTestId("linked-run-row"));
    expect(row.getByTestId("overlay-needs-you-reason")).toBeVisible();
    expect(row.getByTestId("overlay-needs-you-reason")).toHaveTextContent("coordinator loop unreadable");
    expect(row.getByTestId("overlay-verification-reason")).toHaveTextContent(
      "no verification fact for research runs",
    );
  });

  it('says "not recorded", not "no linked run", for a none fact inside a linked run', () => {
    renderOverview({
      linkedRuns: {
        status: "loaded",
        lookup: "complete",
        runs: [makeRun({ verification: { state: "none" } })],
      },
    });

    const badge = within(screen.getByTestId("linked-run-row")).getByTestId("overlay-verification");
    expect(badge).toHaveAttribute("data-state", "none");
    expect(badge).toHaveTextContent("not recorded");
    expect(badge).not.toHaveTextContent("no linked run");
  });

  // dom-accessibility-api trims the hidden span's text, so \s* stands in for the
  // single space a browser keeps between the visible and the hidden part.
  it("names the run in each drill-in link so several links are distinguishable", () => {
    renderOverview({
      linkedRuns: {
        status: "loaded",
        lookup: "complete",
        runs: [
          makeRun({ run_entity_id: "acme.platform.chain.agent.execution.run-1", coordinator_loop_id: "loop-1" }),
          makeRun({ run_entity_id: "acme.platform.chain.agent.execution.run-2", coordinator_loop_id: "loop-2" }),
        ],
      },
    });

    expect(screen.getByRole("link", { name: /^Open this run in the runs lens\s*\(run-1\)$/ })).toHaveAttribute(
      "href",
      "/?task=loop-1",
    );
    expect(screen.getByRole("link", { name: /^Open this run in the runs lens\s*\(run-2\)$/ })).toHaveAttribute(
      "href",
      "/?task=loop-2",
    );
  });
});
