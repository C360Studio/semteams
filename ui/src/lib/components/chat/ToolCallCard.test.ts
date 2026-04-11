import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/svelte";
import ToolCallCard from "./ToolCallCard.svelte";

// ---------------------------------------------------------------------------
// ToolCallAttachment fixture (inline to match HealthStatusCard test pattern)
// ---------------------------------------------------------------------------

interface ToolCallAttachment {
  kind: "tool-call";
  toolName: string;
  args: Record<string, unknown>;
  result?: string;
  error?: string;
  status: "pending" | "running" | "complete" | "error";
  durationMs?: number;
}

function makeAttachment(
  overrides: Partial<ToolCallAttachment> = {},
): ToolCallAttachment {
  return {
    kind: "tool-call",
    toolName: "graph_search",
    args: { query: "find entities" },
    status: "complete",
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// Rendering — root element and testid
// ---------------------------------------------------------------------------

describe("ToolCallCard — root element", () => {
  it("renders with data-testid='tool-call-card'", () => {
    render(ToolCallCard, { props: { attachment: makeAttachment() } });

    expect(screen.getByTestId("tool-call-card")).toBeInTheDocument();
  });

  it("renders without throwing for minimal attachment", () => {
    expect(() =>
      render(ToolCallCard, {
        props: { attachment: makeAttachment({ args: {} }) },
      }),
    ).not.toThrow();
  });
});

// ---------------------------------------------------------------------------
// Rendering — tool name
// ---------------------------------------------------------------------------

describe("ToolCallCard — displays tool name", () => {
  it("displays the toolName", () => {
    render(ToolCallCard, {
      props: { attachment: makeAttachment({ toolName: "entity_lookup" }) },
    });

    expect(screen.getByTestId("tool-call-card")).toHaveTextContent(
      /entity_lookup/,
    );
  });

  it("displays different tool names correctly", () => {
    render(ToolCallCard, {
      props: { attachment: makeAttachment({ toolName: "component_health" }) },
    });

    expect(screen.getByTestId("tool-call-card")).toHaveTextContent(
      /component_health/,
    );
  });
});

// ---------------------------------------------------------------------------
// Rendering — status badge text and data-status attribute
// ---------------------------------------------------------------------------

describe("ToolCallCard — status badge", () => {
  it("displays status text", () => {
    render(ToolCallCard, {
      props: { attachment: makeAttachment({ status: "running" }) },
    });

    expect(screen.getByTestId("tool-call-card")).toHaveTextContent(/running/i);
  });

  it("has data-status attribute matching the status", () => {
    render(ToolCallCard, {
      props: { attachment: makeAttachment({ status: "complete" }) },
    });

    const card = screen.getByTestId("tool-call-card");
    const badge = card.querySelector("[data-status='complete']");
    expect(badge).not.toBeNull();
  });
});

// ---------------------------------------------------------------------------
// Rendering — arguments (collapsible details, conditionally shown)
// ---------------------------------------------------------------------------

describe("ToolCallCard — arguments", () => {
  it("shows arguments in details element when args is non-empty", () => {
    render(ToolCallCard, {
      props: {
        attachment: makeAttachment({
          args: { query: "test query", limit: 10 },
        }),
      },
    });

    const card = screen.getByTestId("tool-call-card");
    const details = card.querySelector("details.tool-args");
    expect(details).not.toBeNull();
    // Verify the arguments are rendered as JSON
    expect(card).toHaveTextContent(/test query/);
    expect(card).toHaveTextContent(/10/);
  });

  it("does NOT show arguments details when args is empty object", () => {
    render(ToolCallCard, {
      props: { attachment: makeAttachment({ args: {} }) },
    });

    const card = screen.getByTestId("tool-call-card");
    const details = card.querySelector("details.tool-args");
    expect(details).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// Rendering — result (collapsible details, conditionally shown)
// ---------------------------------------------------------------------------

describe("ToolCallCard — result", () => {
  it("shows result in details element when result is provided", () => {
    render(ToolCallCard, {
      props: {
        attachment: makeAttachment({
          result: "Found 3 entities matching criteria",
        }),
      },
    });

    const card = screen.getByTestId("tool-call-card");
    const details = card.querySelector("details.tool-result");
    expect(details).not.toBeNull();
    expect(card).toHaveTextContent(/Found 3 entities matching criteria/);
  });

  it("does NOT show result details when result is undefined", () => {
    render(ToolCallCard, {
      props: { attachment: makeAttachment({ result: undefined }) },
    });

    const card = screen.getByTestId("tool-call-card");
    const details = card.querySelector("details.tool-result");
    expect(details).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// Rendering — error (conditionally shown with data-testid)
// ---------------------------------------------------------------------------

describe("ToolCallCard — error", () => {
  it("shows error message when error is provided", () => {
    render(ToolCallCard, {
      props: {
        attachment: makeAttachment({
          status: "error",
          error: "Connection timeout",
        }),
      },
    });

    const errorEl = screen.getByTestId("tool-error");
    expect(errorEl).toBeInTheDocument();
    expect(errorEl).toHaveTextContent(/Connection timeout/);
  });

  it("does NOT show error element when error is undefined", () => {
    render(ToolCallCard, {
      props: { attachment: makeAttachment({ error: undefined }) },
    });

    expect(screen.queryByTestId("tool-error")).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// Rendering — duration (conditionally shown)
// ---------------------------------------------------------------------------

describe("ToolCallCard — duration", () => {
  it("shows duration when durationMs is provided", () => {
    render(ToolCallCard, {
      props: { attachment: makeAttachment({ durationMs: 245 }) },
    });

    expect(screen.getByTestId("tool-call-card")).toHaveTextContent(/245ms/);
  });

  it("does NOT show duration when durationMs is undefined", () => {
    render(ToolCallCard, {
      props: { attachment: makeAttachment({ durationMs: undefined }) },
    });

    const card = screen.getByTestId("tool-call-card");
    const durationEl = card.querySelector(".tool-duration");
    expect(durationEl).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// Table-driven: all 4 status values with CSS class validation
// ---------------------------------------------------------------------------

describe("ToolCallCard — table-driven status rendering", () => {
  it.each([
    { status: "pending" as const },
    { status: "running" as const },
    { status: "complete" as const },
    { status: "error" as const },
  ])(
    "status='$status' badge has CSS class and data-status attribute matching the status",
    ({ status }) => {
      render(ToolCallCard, {
        props: {
          attachment: makeAttachment({ status }),
        },
      });

      const card = screen.getByTestId("tool-call-card");
      // Check data-status attribute
      const badge = card.querySelector(`[data-status='${status}']`);
      expect(badge).not.toBeNull();
      // Check CSS class
      const byClass = card.querySelector(`.${status}, [class*='${status}']`);
      expect(byClass).not.toBeNull();
    },
  );
});

// ---------------------------------------------------------------------------
// Table-driven: tool name x status combinations
// ---------------------------------------------------------------------------

describe("ToolCallCard — tool name and status both visible", () => {
  it.each([
    { toolName: "graph_search", status: "pending" as const },
    { toolName: "entity_lookup", status: "running" as const },
    { toolName: "component_health", status: "complete" as const },
    { toolName: "flow_status", status: "error" as const },
  ])(
    "shows toolName='$toolName' and status='$status'",
    ({ toolName, status }) => {
      render(ToolCallCard, {
        props: { attachment: makeAttachment({ toolName, status }) },
      });

      const card = screen.getByTestId("tool-call-card");
      expect(card).toHaveTextContent(new RegExp(toolName));
      expect(card).toHaveTextContent(new RegExp(status, "i"));
    },
  );
});
