import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import ApprovalPrompt from "./ApprovalPrompt.svelte";

// ---------------------------------------------------------------------------
// ApprovalAttachment fixture (inline to match HealthStatusCard.test.ts pattern)
// ---------------------------------------------------------------------------

interface ApprovalAttachment {
  kind: "approval";
  loopId: string;
  toolName: string;
  toolArgs: Record<string, unknown>;
  question: string;
  resolved?: boolean;
  resolution?: "approved" | "rejected";
}

function makeAttachment(
  overrides: Partial<ApprovalAttachment> = {},
): ApprovalAttachment {
  return {
    kind: "approval",
    loopId: "loop-123",
    toolName: "deploy_service",
    toolArgs: { service: "api-gateway", env: "production" },
    question: "Deploy api-gateway to production?",
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// Rendering — root element and testid
// ---------------------------------------------------------------------------

describe("ApprovalPrompt — root element", () => {
  it("renders with data-testid='approval-prompt'", () => {
    render(ApprovalPrompt, { props: { attachment: makeAttachment() } });

    expect(screen.getByTestId("approval-prompt")).toBeInTheDocument();
  });

  it("renders without throwing for minimal attachment", () => {
    expect(() =>
      render(ApprovalPrompt, { props: { attachment: makeAttachment() } }),
    ).not.toThrow();
  });
});

// ---------------------------------------------------------------------------
// Rendering — question text
// ---------------------------------------------------------------------------

describe("ApprovalPrompt — displays question", () => {
  it("displays the question text", () => {
    render(ApprovalPrompt, {
      props: {
        attachment: makeAttachment({
          question: "Allow schema migration on prod DB?",
        }),
      },
    });

    expect(screen.getByTestId("approval-prompt")).toHaveTextContent(
      /Allow schema migration on prod DB\?/,
    );
  });

  it("displays different question text correctly", () => {
    render(ApprovalPrompt, {
      props: {
        attachment: makeAttachment({
          question: "Execute batch delete of 500 records?",
        }),
      },
    });

    expect(screen.getByTestId("approval-prompt")).toHaveTextContent(
      /Execute batch delete of 500 records\?/,
    );
  });
});

// ---------------------------------------------------------------------------
// Rendering — tool name
// ---------------------------------------------------------------------------

describe("ApprovalPrompt — shows tool name when provided", () => {
  it("displays the tool name", () => {
    render(ApprovalPrompt, {
      props: {
        attachment: makeAttachment({ toolName: "run_migration" }),
      },
    });

    expect(screen.getByTestId("approval-prompt")).toHaveTextContent(
      /run_migration/,
    );
  });

  it("displays a different tool name", () => {
    render(ApprovalPrompt, {
      props: {
        attachment: makeAttachment({ toolName: "delete_records" }),
      },
    });

    expect(screen.getByTestId("approval-prompt")).toHaveTextContent(
      /delete_records/,
    );
  });
});

// ---------------------------------------------------------------------------
// Rendering — action buttons (unresolved state)
// ---------------------------------------------------------------------------

describe("ApprovalPrompt — shows Approve and Reject buttons when not resolved", () => {
  it("renders Approve button", () => {
    render(ApprovalPrompt, { props: { attachment: makeAttachment() } });

    expect(screen.getByTestId("approve-button")).toBeInTheDocument();
    expect(screen.getByTestId("approve-button")).toHaveTextContent(/Approve/i);
  });

  it("renders Reject button", () => {
    render(ApprovalPrompt, { props: { attachment: makeAttachment() } });

    expect(screen.getByTestId("reject-button")).toBeInTheDocument();
    expect(screen.getByTestId("reject-button")).toHaveTextContent(/Reject/i);
  });
});

// ---------------------------------------------------------------------------
// Interaction — Approve callback
// ---------------------------------------------------------------------------

describe("ApprovalPrompt — clicking Approve calls onApprove", () => {
  it("calls onApprove with loopId when clicked", async () => {
    const user = userEvent.setup();
    const onApprove = vi.fn();

    render(ApprovalPrompt, {
      props: {
        attachment: makeAttachment({ loopId: "loop-456" }),
        onApprove,
      },
    });

    await user.click(screen.getByTestId("approve-button"));

    expect(onApprove).toHaveBeenCalledOnce();
    expect(onApprove).toHaveBeenCalledWith("loop-456");
  });

  it("does not throw when onApprove is undefined", async () => {
    const user = userEvent.setup();

    render(ApprovalPrompt, {
      props: { attachment: makeAttachment() },
    });

    await expect(
      user.click(screen.getByTestId("approve-button")),
    ).resolves.not.toThrow();
  });
});

// ---------------------------------------------------------------------------
// Interaction — Reject flow (textarea then callback)
// ---------------------------------------------------------------------------

describe("ApprovalPrompt — Reject flow", () => {
  it("first click on Reject shows the reason textarea", async () => {
    const user = userEvent.setup();

    render(ApprovalPrompt, {
      props: { attachment: makeAttachment() },
    });

    // Textarea should not be visible initially
    expect(screen.queryByTestId("reject-reason")).not.toBeInTheDocument();

    await user.click(screen.getByTestId("reject-button"));

    // Now textarea should appear
    expect(screen.getByTestId("reject-reason")).toBeInTheDocument();
  });

  it("clicking Reject again without reason calls onReject with loopId only", async () => {
    const user = userEvent.setup();
    const onReject = vi.fn();

    render(ApprovalPrompt, {
      props: {
        attachment: makeAttachment({ loopId: "loop-789" }),
        onReject,
      },
    });

    // First click: show textarea
    await user.click(screen.getByTestId("reject-button"));
    // Second click: submit without reason
    await user.click(screen.getByTestId("reject-button"));

    expect(onReject).toHaveBeenCalledOnce();
    expect(onReject).toHaveBeenCalledWith("loop-789");
  });

  it("clicking Reject after typing reason calls onReject with loopId and reason", async () => {
    const user = userEvent.setup();
    const onReject = vi.fn();

    render(ApprovalPrompt, {
      props: {
        attachment: makeAttachment({ loopId: "loop-abc" }),
        onReject,
      },
    });

    // First click: show textarea
    await user.click(screen.getByTestId("reject-button"));

    // Type a reason
    await user.type(
      screen.getByTestId("reject-reason"),
      "Too risky for production",
    );

    // Second click: submit with reason
    await user.click(screen.getByTestId("reject-button"));

    expect(onReject).toHaveBeenCalledOnce();
    expect(onReject).toHaveBeenCalledWith(
      "loop-abc",
      "Too risky for production",
    );
  });

  it("does not throw when onReject is undefined", async () => {
    const user = userEvent.setup();

    render(ApprovalPrompt, {
      props: { attachment: makeAttachment() },
    });

    // First click: show textarea
    await user.click(screen.getByTestId("reject-button"));
    // Second click: submit without callback
    await expect(
      user.click(screen.getByTestId("reject-button")),
    ).resolves.not.toThrow();
  });
});

// ---------------------------------------------------------------------------
// Rendering — resolved state (approved)
// ---------------------------------------------------------------------------

describe("ApprovalPrompt — resolved as approved", () => {
  it("shows 'Approved' badge", () => {
    render(ApprovalPrompt, {
      props: {
        attachment: makeAttachment({ resolved: true, resolution: "approved" }),
      },
    });

    const card = screen.getByTestId("approval-prompt");
    expect(card).toHaveTextContent(/Approved/);
  });

  it("does not show Approve or Reject buttons", () => {
    render(ApprovalPrompt, {
      props: {
        attachment: makeAttachment({ resolved: true, resolution: "approved" }),
      },
    });

    expect(screen.queryByTestId("approve-button")).not.toBeInTheDocument();
    expect(screen.queryByTestId("reject-button")).not.toBeInTheDocument();
  });

  it("has data-resolution attribute set to approved", () => {
    render(ApprovalPrompt, {
      props: {
        attachment: makeAttachment({ resolved: true, resolution: "approved" }),
      },
    });

    const badge = screen
      .getByTestId("approval-prompt")
      .querySelector("[data-resolution]");
    expect(badge).not.toBeNull();
    expect(badge?.getAttribute("data-resolution")).toBe("approved");
  });
});

// ---------------------------------------------------------------------------
// Rendering — resolved state (rejected)
// ---------------------------------------------------------------------------

describe("ApprovalPrompt — resolved as rejected", () => {
  it("shows 'Rejected' badge", () => {
    render(ApprovalPrompt, {
      props: {
        attachment: makeAttachment({ resolved: true, resolution: "rejected" }),
      },
    });

    const card = screen.getByTestId("approval-prompt");
    expect(card).toHaveTextContent(/Rejected/);
  });

  it("does not show Approve or Reject buttons", () => {
    render(ApprovalPrompt, {
      props: {
        attachment: makeAttachment({ resolved: true, resolution: "rejected" }),
      },
    });

    expect(screen.queryByTestId("approve-button")).not.toBeInTheDocument();
    expect(screen.queryByTestId("reject-button")).not.toBeInTheDocument();
  });

  it("has data-resolution attribute set to rejected", () => {
    render(ApprovalPrompt, {
      props: {
        attachment: makeAttachment({ resolved: true, resolution: "rejected" }),
      },
    });

    const badge = screen
      .getByTestId("approval-prompt")
      .querySelector("[data-resolution]");
    expect(badge).not.toBeNull();
    expect(badge?.getAttribute("data-resolution")).toBe("rejected");
  });
});

// ---------------------------------------------------------------------------
// Table-driven: resolution badge rendering
// ---------------------------------------------------------------------------

describe("ApprovalPrompt — table-driven resolution badges", () => {
  it.each([
    {
      resolution: "approved" as const,
      label: "Approved",
    },
    {
      resolution: "rejected" as const,
      label: "Rejected",
    },
  ])(
    "resolved with '$resolution' shows '$label' badge",
    ({ resolution, label }) => {
      render(ApprovalPrompt, {
        props: {
          attachment: makeAttachment({ resolved: true, resolution }),
        },
      });

      expect(screen.getByTestId("approval-prompt")).toHaveTextContent(label);
      expect(screen.queryByTestId("approve-button")).not.toBeInTheDocument();
      expect(screen.queryByTestId("reject-button")).not.toBeInTheDocument();
    },
  );
});
