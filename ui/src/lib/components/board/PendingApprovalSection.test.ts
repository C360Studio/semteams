import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import PendingApprovalSection from "./PendingApprovalSection.svelte";
import type { PendingApproval } from "$lib/types/agent";

vi.mock("$lib/services/agentApi", () => ({
  agentApi: {
    submitApproval: vi.fn().mockResolvedValue({
      loop_id: "loop_001",
      decision: "approve",
      accepted: true,
      timestamp: "2026-04-29T11:00:00Z",
    }),
  },
  AgentApiError: class AgentApiError extends Error {
    statusCode: number;
    details?: unknown;
    constructor(message: string, statusCode: number, details?: unknown) {
      super(message);
      this.name = "AgentApiError";
      this.statusCode = statusCode;
      this.details = details;
    }
  },
}));

import { agentApi, AgentApiError } from "$lib/services/agentApi";

function makePending(overrides: Partial<PendingApproval> = {}): PendingApproval {
  return {
    call_id: "call-default",
    tool_name: "create_rule",
    arguments: { name: "high-temperature-alert", threshold: 100 },
    reason: "approval_required: rule write requires human approval",
    requested_at: "2026-04-29T11:00:00Z",
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(agentApi.submitApproval).mockResolvedValue({
    loop_id: "loop_001",
    decision: "approve",
    accepted: true,
    timestamp: "2026-04-29T11:00:00Z",
  });
});

describe("PendingApprovalSection", () => {
  describe("rendering", () => {
    it("renders the tool name, reason, and formatted args", () => {
      render(PendingApprovalSection, {
        props: { loopId: "loop_001", pendingApproval: makePending() },
      });

      expect(screen.getByTestId("approval-tool-name")).toHaveTextContent(
        "create_rule",
      );
      expect(screen.getByTestId("approval-reason")).toHaveTextContent(
        "rule write requires human approval",
      );
      const args = screen.getByTestId("approval-args-display");
      expect(args).toHaveTextContent("high-temperature-alert");
      expect(args).toHaveTextContent("100");
    });

    it("renders with empty arguments object when none supplied", () => {
      render(PendingApprovalSection, {
        props: {
          loopId: "loop_001",
          pendingApproval: makePending({ arguments: undefined }),
        },
      });

      expect(screen.getByTestId("approval-args-display")).toHaveTextContent("{}");
    });

    it("approve and reject buttons advertise the tool name in aria-label", () => {
      render(PendingApprovalSection, {
        props: {
          loopId: "loop_001",
          pendingApproval: makePending({ tool_name: "delete_rule" }),
        },
      });

      expect(screen.getByTestId("approval-approve")).toHaveAttribute(
        "aria-label",
        "Approve delete_rule",
      );
      expect(screen.getByTestId("approval-reject")).toHaveAttribute(
        "aria-label",
        "Reject delete_rule",
      );
    });

    it("aria-label tracks in-flight verb (WCAG 2.5.3 Label-in-Name)", async () => {
      // Hold the request open so we can observe the in-flight state.
      type Accept = Awaited<ReturnType<typeof agentApi.submitApproval>>;
      let resolve: ((value: Accept) => void) | undefined;
      vi.mocked(agentApi.submitApproval).mockReturnValueOnce(
        new Promise<Accept>((r) => {
          resolve = r;
        }),
      );
      const user = userEvent.setup();
      render(PendingApprovalSection, {
        props: {
          loopId: "loop_001",
          pendingApproval: makePending({ tool_name: "delete_rule" }),
        },
      });

      const btn = screen.getByTestId("approval-approve");
      await user.click(btn);

      // While the request is in flight, the visible label is
      // "Approving…" — aria-label must agree.
      expect(btn).toHaveTextContent("Approving");
      expect(btn).toHaveAttribute("aria-label", "Approving delete_rule");

      // Cleanly resolve so the test doesn't leak a pending promise.
      resolve?.({
        loop_id: "loop_001",
        decision: "approve",
        accepted: true,
        timestamp: "2026-04-29T11:00:00Z",
      });
    });
  });

  describe("approve / reject paths", () => {
    it("approve click POSTs decision=approve with the loopId", async () => {
      const user = userEvent.setup();
      render(PendingApprovalSection, {
        props: { loopId: "loop_xyz", pendingApproval: makePending() },
      });

      await user.click(screen.getByTestId("approval-approve"));

      expect(agentApi.submitApproval).toHaveBeenCalledWith("loop_xyz", {
        decision: "approve",
      });
    });

    it("reject click POSTs decision=reject", async () => {
      const user = userEvent.setup();
      render(PendingApprovalSection, {
        props: { loopId: "loop_xyz", pendingApproval: makePending() },
      });

      await user.click(screen.getByTestId("approval-reject"));

      expect(agentApi.submitApproval).toHaveBeenCalledWith("loop_xyz", {
        decision: "reject",
      });
    });

    it("forwards the optional reason field when populated", async () => {
      const user = userEvent.setup();
      render(PendingApprovalSection, {
        props: { loopId: "loop_xyz", pendingApproval: makePending() },
      });

      await user.type(
        screen.getByTestId("approval-reason-input"),
        "looks safe",
      );
      await user.click(screen.getByTestId("approval-approve"));

      expect(agentApi.submitApproval).toHaveBeenCalledWith("loop_xyz", {
        decision: "approve",
        reason: "looks safe",
      });
    });
  });

  describe("modify path", () => {
    it("Edit toggles the JSON editor pre-filled with current arguments", async () => {
      const user = userEvent.setup();
      render(PendingApprovalSection, {
        props: { loopId: "loop_xyz", pendingApproval: makePending() },
      });

      expect(screen.queryByTestId("approval-args-editor")).not.toBeInTheDocument();
      await user.click(screen.getByTestId("approval-edit-args"));

      const editor = screen.getByTestId("approval-args-editor") as HTMLTextAreaElement;
      expect(editor).toBeInTheDocument();
      expect(editor.value).toContain("high-temperature-alert");
      // Approve / Reject collapse while editing.
      expect(screen.queryByTestId("approval-approve")).not.toBeInTheDocument();
      expect(screen.getByTestId("approval-submit-modify")).toBeInTheDocument();
      expect(screen.getByTestId("approval-cancel-edit")).toBeInTheDocument();
    });

    it("Submit modified POSTs decision=modify with parsed arguments", async () => {
      const user = userEvent.setup();
      render(PendingApprovalSection, {
        props: { loopId: "loop_xyz", pendingApproval: makePending() },
      });

      await user.click(screen.getByTestId("approval-edit-args"));
      const editor = screen.getByTestId("approval-args-editor");
      await user.clear(editor);
      await user.type(editor, '{{"name": "narrowed"}');
      await user.click(screen.getByTestId("approval-submit-modify"));

      expect(agentApi.submitApproval).toHaveBeenCalledWith("loop_xyz", {
        decision: "modify",
        modified_arguments: { name: "narrowed" },
      });
    });

    it("rejects malformed JSON without firing the request", async () => {
      const user = userEvent.setup();
      render(PendingApprovalSection, {
        props: { loopId: "loop_xyz", pendingApproval: makePending() },
      });

      await user.click(screen.getByTestId("approval-edit-args"));
      const editor = screen.getByTestId("approval-args-editor");
      await user.clear(editor);
      await user.type(editor, "not-json");
      await user.click(screen.getByTestId("approval-submit-modify"));

      expect(screen.getByTestId("approval-args-error")).toHaveTextContent(
        /Invalid JSON/,
      );
      expect(agentApi.submitApproval).not.toHaveBeenCalled();
    });

    it("rejects a non-object JSON value (array, scalar) without firing", async () => {
      const user = userEvent.setup();
      render(PendingApprovalSection, {
        props: { loopId: "loop_xyz", pendingApproval: makePending() },
      });

      await user.click(screen.getByTestId("approval-edit-args"));
      const editor = screen.getByTestId("approval-args-editor");
      await user.clear(editor);
      // user-event treats unescaped `[` as a key descriptor; double-
      // square-brackets escapes to a literal `[`.
      await user.type(editor, "[[1, 2]");
      await user.click(screen.getByTestId("approval-submit-modify"));

      expect(screen.getByTestId("approval-args-error")).toHaveTextContent(
        /must be a JSON object/,
      );
      expect(agentApi.submitApproval).not.toHaveBeenCalled();
    });

    it("Cancel returns to the approve / reject layout without firing", async () => {
      const user = userEvent.setup();
      render(PendingApprovalSection, {
        props: { loopId: "loop_xyz", pendingApproval: makePending() },
      });

      await user.click(screen.getByTestId("approval-edit-args"));
      await user.click(screen.getByTestId("approval-cancel-edit"));

      expect(screen.queryByTestId("approval-args-editor")).not.toBeInTheDocument();
      expect(screen.getByTestId("approval-approve")).toBeInTheDocument();
      expect(screen.getByTestId("approval-reject")).toBeInTheDocument();
      expect(agentApi.submitApproval).not.toHaveBeenCalled();
    });
  });

  describe("error handling", () => {
    it("surfaces a 409 with a user-friendly stale-state message", async () => {
      vi.mocked(agentApi.submitApproval).mockRejectedValueOnce(
        new AgentApiError("conflict", 409),
      );
      const user = userEvent.setup();
      render(PendingApprovalSection, {
        props: { loopId: "loop_xyz", pendingApproval: makePending() },
      });

      await user.click(screen.getByTestId("approval-approve"));

      const err = await screen.findByTestId("approval-submit-error");
      expect(err).toHaveTextContent(/no longer awaiting approval/);
    });

    it("surfaces non-409 status codes with the original message", async () => {
      vi.mocked(agentApi.submitApproval).mockRejectedValueOnce(
        new AgentApiError("nats publish failed", 500),
      );
      const user = userEvent.setup();
      render(PendingApprovalSection, {
        props: { loopId: "loop_xyz", pendingApproval: makePending() },
      });

      await user.click(screen.getByTestId("approval-approve"));

      const err = await screen.findByTestId("approval-submit-error");
      expect(err).toHaveTextContent(/Approval failed \(500\)/);
      expect(err).toHaveTextContent(/nats publish failed/);
    });
  });
});
