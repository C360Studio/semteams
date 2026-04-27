import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import TaskCard from "./TaskCard.svelte";
import type { TaskInfo } from "$lib/types/task";
import type { AgentLoop } from "$lib/types/agent";

function makeTask(overrides: Partial<TaskInfo> = {}): TaskInfo {
  const loop: AgentLoop = {
    loop_id: "loop_001",
    task_id: "task_001",
    state: "executing",
    role: "general",
    iterations: 3,
    max_iterations: 10,
    user_id: "user-1",
    channel_type: "web",
    parent_loop_id: "",
    outcome: "",
    error: "",
  };
  return {
    id: "loop_001",
    shortRef: null,
    aliases: [],
    titleEdited: false,
    title: "Test task title",
    column: "executing",
    state: "executing",
    role: "general",
    iterations: 3,
    maxIterations: 10,
    primaryLoop: loop,
    childLoops: [],
    childNeedsAttention: false,
    childAttentionCount: 0,
    ...overrides,
  };
}

describe("TaskCard", () => {
  it("renders title and role", () => {
    render(TaskCard, { props: { task: makeTask({ title: "My task", role: "editor" }) } });

    expect(screen.getByText("My task")).toBeInTheDocument();
    expect(screen.getByText("editor")).toBeInTheDocument();
  });

  it("renders progress bar when maxIterations > 0", () => {
    render(TaskCard, { props: { task: makeTask({ iterations: 5, maxIterations: 10 }) } });

    expect(screen.getByText("5/10")).toBeInTheDocument();
  });

  it("does not render progress bar when maxIterations is 0", () => {
    render(TaskCard, { props: { task: makeTask({ maxIterations: 0 }) } });

    expect(screen.queryByText(/\/10/)).not.toBeInTheDocument();
  });

  it("shows child attention indicator when children need attention", () => {
    render(TaskCard, {
      props: {
        task: makeTask({ childNeedsAttention: true, childAttentionCount: 2 }),
      },
    });

    expect(screen.getByTestId("child-attention")).toBeInTheDocument();
    expect(screen.getByText(/2 children awaiting approval/)).toBeInTheDocument();
  });

  it("hides child attention when no children need attention", () => {
    render(TaskCard, { props: { task: makeTask() } });

    expect(screen.queryByTestId("child-attention")).not.toBeInTheDocument();
  });

  it("fires onclick with task id when clicked", async () => {
    const handler = vi.fn();
    const user = userEvent.setup();
    render(TaskCard, { props: { task: makeTask({ id: "loop_xyz" }), onclick: handler } });

    await user.click(screen.getByTestId("task-card"));

    expect(handler).toHaveBeenCalledWith("loop_xyz");
  });

  it("applies selected class when selected=true", () => {
    render(TaskCard, { props: { task: makeTask(), selected: true } });

    expect(screen.getByTestId("task-card")).toHaveClass("selected");
  });

  it("sets data-column attribute", () => {
    render(TaskCard, { props: { task: makeTask({ column: "needs_you" }) } });

    expect(screen.getByTestId("task-card")).toHaveAttribute("data-column", "needs_you");
  });
});
