import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/svelte";
import AgentLoopCard from "./AgentLoopCard.svelte";
import type { AgentLoopState } from "$lib/types/agent";

// ---------------------------------------------------------------------------
// AgentLoopAttachment fixture (inline to match HealthStatusCard test pattern)
// ---------------------------------------------------------------------------

interface AgentLoopAttachment {
  kind: "agent-loop";
  loopId: string;
  state: AgentLoopState;
  role: string;
  iterations: number;
  maxIterations: number;
  parentLoopId?: string;
}

function makeAttachment(
  overrides: Partial<AgentLoopAttachment> = {},
): AgentLoopAttachment {
  return {
    kind: "agent-loop",
    loopId: "loop-abc123def456",
    state: "exploring",
    role: "architect",
    iterations: 2,
    maxIterations: 10,
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// Rendering — root element and testid
// ---------------------------------------------------------------------------

describe("AgentLoopCard — root element", () => {
  it("renders with data-testid='agent-loop-card'", () => {
    render(AgentLoopCard, { props: { attachment: makeAttachment() } });

    expect(screen.getByTestId("agent-loop-card")).toBeInTheDocument();
  });

  it("renders without throwing for minimal attachment", () => {
    expect(() =>
      render(AgentLoopCard, { props: { attachment: makeAttachment() } }),
    ).not.toThrow();
  });
});

// ---------------------------------------------------------------------------
// Rendering — role text
// ---------------------------------------------------------------------------

describe("AgentLoopCard — displays role", () => {
  it("displays the role text", () => {
    render(AgentLoopCard, {
      props: { attachment: makeAttachment({ role: "architect" }) },
    });

    expect(screen.getByTestId("agent-loop-card")).toHaveTextContent(
      /architect/i,
    );
  });

  it("displays different role names correctly", () => {
    render(AgentLoopCard, {
      props: { attachment: makeAttachment({ role: "builder" }) },
    });

    expect(screen.getByTestId("agent-loop-card")).toHaveTextContent(/builder/i);
  });
});

// ---------------------------------------------------------------------------
// Rendering — state badge text (underscores replaced with spaces)
// ---------------------------------------------------------------------------

describe("AgentLoopCard — state badge text", () => {
  it("displays state text with underscores replaced by spaces", () => {
    render(AgentLoopCard, {
      props: { attachment: makeAttachment({ state: "awaiting_approval" }) },
    });

    expect(screen.getByTestId("agent-loop-card")).toHaveTextContent(
      /awaiting approval/i,
    );
  });

  it("displays single-word state correctly", () => {
    render(AgentLoopCard, {
      props: { attachment: makeAttachment({ state: "exploring" }) },
    });

    expect(screen.getByTestId("agent-loop-card")).toHaveTextContent(
      /exploring/i,
    );
  });
});

// ---------------------------------------------------------------------------
// Rendering — state badge data-state attribute
// ---------------------------------------------------------------------------

describe("AgentLoopCard — state badge has data-state attribute", () => {
  it("has data-state attribute matching the state", () => {
    render(AgentLoopCard, {
      props: { attachment: makeAttachment({ state: "planning" }) },
    });

    const card = screen.getByTestId("agent-loop-card");
    const badge = card.querySelector("[data-state='planning']");
    expect(badge).not.toBeNull();
  });

  it("has data-state attribute for awaiting_approval", () => {
    render(AgentLoopCard, {
      props: { attachment: makeAttachment({ state: "awaiting_approval" }) },
    });

    const card = screen.getByTestId("agent-loop-card");
    const badge = card.querySelector("[data-state='awaiting_approval']");
    expect(badge).not.toBeNull();
  });
});

// ---------------------------------------------------------------------------
// Rendering — progress bar when maxIterations > 0
// ---------------------------------------------------------------------------

describe("AgentLoopCard — progress bar", () => {
  it("shows progress bar when maxIterations > 0", () => {
    render(AgentLoopCard, {
      props: {
        attachment: makeAttachment({ iterations: 3, maxIterations: 10 }),
      },
    });

    const card = screen.getByTestId("agent-loop-card");
    const progressBar = card.querySelector(".progress-bar");
    expect(progressBar).not.toBeNull();
  });

  it("shows correct progress text", () => {
    render(AgentLoopCard, {
      props: {
        attachment: makeAttachment({ iterations: 3, maxIterations: 10 }),
      },
    });

    expect(screen.getByTestId("agent-loop-card")).toHaveTextContent("3/10");
  });

  it("shows correct progress percentage via width style", () => {
    render(AgentLoopCard, {
      props: {
        attachment: makeAttachment({ iterations: 5, maxIterations: 10 }),
      },
    });

    const card = screen.getByTestId("agent-loop-card");
    const fill = card.querySelector(".progress-fill") as HTMLElement;
    expect(fill).not.toBeNull();
    expect(fill.style.width).toBe("50%");
  });

  it("caps progress at 100%", () => {
    render(AgentLoopCard, {
      props: {
        attachment: makeAttachment({ iterations: 15, maxIterations: 10 }),
      },
    });

    const card = screen.getByTestId("agent-loop-card");
    const fill = card.querySelector(".progress-fill") as HTMLElement;
    expect(fill).not.toBeNull();
    expect(fill.style.width).toBe("100%");
  });

  it("does NOT show progress bar when maxIterations is 0", () => {
    render(AgentLoopCard, {
      props: {
        attachment: makeAttachment({ iterations: 0, maxIterations: 0 }),
      },
    });

    const card = screen.getByTestId("agent-loop-card");
    const progressBar = card.querySelector(".progress-bar");
    expect(progressBar).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// Rendering — truncated loop ID
// ---------------------------------------------------------------------------

describe("AgentLoopCard — truncated loop ID", () => {
  it("shows first 12 characters of loop ID", () => {
    render(AgentLoopCard, {
      props: {
        attachment: makeAttachment({ loopId: "loop-abc123def456" }),
      },
    });

    expect(screen.getByTestId("agent-loop-card")).toHaveTextContent(
      "loop-abc123d",
    );
  });

  it("shows short loop ID as-is when shorter than 12 chars", () => {
    render(AgentLoopCard, {
      props: {
        attachment: makeAttachment({ loopId: "short" }),
      },
    });

    expect(screen.getByTestId("agent-loop-card")).toHaveTextContent("short");
  });
});

// ---------------------------------------------------------------------------
// Table-driven: all state values render with correct CSS class on badge
// ---------------------------------------------------------------------------

describe("AgentLoopCard — table-driven state rendering", () => {
  it.each([
    { state: "exploring" as const },
    { state: "planning" as const },
    { state: "architecting" as const },
    { state: "executing" as const },
    { state: "reviewing" as const },
    { state: "paused" as const },
    { state: "awaiting_approval" as const },
    { state: "complete" as const },
    { state: "failed" as const },
    { state: "cancelled" as const },
  ])("state='$state' badge has CSS class matching the state", ({ state }) => {
    render(AgentLoopCard, {
      props: {
        attachment: makeAttachment({ state }),
      },
    });

    const card = screen.getByTestId("agent-loop-card");
    const badge = card.querySelector(
      `[data-state='${state}'], .${state}, [class*='${state}']`,
    );
    expect(badge).not.toBeNull();
  });
});

// ---------------------------------------------------------------------------
// Table-driven: role x state combinations
// ---------------------------------------------------------------------------

describe("AgentLoopCard — role and state both visible", () => {
  it.each([
    { role: "architect", state: "exploring" as const },
    { role: "builder", state: "executing" as const },
    { role: "reviewer", state: "reviewing" as const },
    { role: "debugger", state: "failed" as const },
  ])("shows role='$role' and state='$state'", ({ role, state }) => {
    render(AgentLoopCard, {
      props: { attachment: makeAttachment({ role, state }) },
    });

    const card = screen.getByTestId("agent-loop-card");
    expect(card).toHaveTextContent(new RegExp(role, "i"));
    // State text has underscores replaced with spaces
    expect(card).toHaveTextContent(new RegExp(state.replace("_", " "), "i"));
  });
});
