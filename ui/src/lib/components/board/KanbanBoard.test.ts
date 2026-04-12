import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import KanbanBoard from "./KanbanBoard.svelte";

// Mock taskStore
vi.mock("$lib/stores/taskStore.svelte", () => {
  const counts = { thinking: 1, executing: 2, needs_you: 0, done: 3, failed: 0 };
  return {
    taskStore: {
      get columnCounts() {
        return counts;
      },
      getColumn(_col: string) {
        return [];
      },
      get selectedId() {
        return null;
      },
      toggleTask: vi.fn(),
    },
  };
});

beforeEach(() => {
  vi.clearAllMocks();
  // Clear localStorage between tests
  localStorage.clear();
});

describe("KanbanBoard", () => {
  it("renders the board container", () => {
    render(KanbanBoard);

    expect(screen.getByTestId("kanban-board")).toBeInTheDocument();
  });

  it("renders column toggle chips", () => {
    render(KanbanBoard);

    expect(screen.getByTestId("column-toggles")).toBeInTheDocument();
    expect(screen.getByTestId("toggle-thinking")).toBeInTheDocument();
    expect(screen.getByTestId("toggle-executing")).toBeInTheDocument();
    expect(screen.getByTestId("toggle-needs_you")).toBeInTheDocument();
    expect(screen.getByTestId("toggle-done")).toBeInTheDocument();
    expect(screen.getByTestId("toggle-failed")).toBeInTheDocument();
  });

  it("renders 4 visible columns by default (Failed hidden)", () => {
    render(KanbanBoard);

    const columns = screen.getAllByTestId("kanban-column");
    expect(columns).toHaveLength(4);

    const columnIds = columns.map((c) => c.getAttribute("data-column-id"));
    expect(columnIds).toContain("thinking");
    expect(columnIds).toContain("executing");
    expect(columnIds).toContain("needs_you");
    expect(columnIds).toContain("done");
    expect(columnIds).not.toContain("failed");
  });

  it("toggle chips show column counts", () => {
    render(KanbanBoard);

    // Counts from mock: thinking=1, executing=2, needs_you=0, done=3, failed=0
    const thinkingChip = screen.getByTestId("toggle-thinking");
    expect(thinkingChip.textContent).toContain("1");

    const executingChip = screen.getByTestId("toggle-executing");
    expect(executingChip.textContent).toContain("2");
  });

  it("toggle chip has aria-pressed matching visibility", () => {
    render(KanbanBoard);

    // Visible by default
    expect(screen.getByTestId("toggle-thinking")).toHaveAttribute(
      "aria-pressed",
      "true",
    );

    // Hidden by default
    expect(screen.getByTestId("toggle-failed")).toHaveAttribute(
      "aria-pressed",
      "false",
    );
  });

  it("clicking a toggle chip toggles column visibility", async () => {
    const user = userEvent.setup();
    render(KanbanBoard);

    // Failed column hidden by default
    let columns = screen.getAllByTestId("kanban-column");
    expect(columns.map((c) => c.getAttribute("data-column-id"))).not.toContain(
      "failed",
    );

    // Click to show Failed
    await user.click(screen.getByTestId("toggle-failed"));

    columns = screen.getAllByTestId("kanban-column");
    expect(columns.map((c) => c.getAttribute("data-column-id"))).toContain(
      "failed",
    );

    // Click again to hide
    await user.click(screen.getByTestId("toggle-failed"));

    columns = screen.getAllByTestId("kanban-column");
    expect(columns.map((c) => c.getAttribute("data-column-id"))).not.toContain(
      "failed",
    );
  });

  it("persists column visibility to localStorage", async () => {
    const user = userEvent.setup();
    render(KanbanBoard);

    await user.click(screen.getByTestId("toggle-failed"));

    const stored = JSON.parse(
      localStorage.getItem("semteams:board:columns") ?? "[]",
    );
    expect(stored).toContain("failed");
  });
});
