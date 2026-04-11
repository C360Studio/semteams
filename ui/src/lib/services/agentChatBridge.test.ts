import { describe, it, expect, beforeEach, vi } from "vitest";
import type { AgentLoop } from "$lib/types/agent";
import type { AgentLoopAttachment, ApprovalAttachment } from "$lib/types/chat";

// Mock chatStore before importing the module under test
vi.mock("$lib/stores/chatStore.svelte", () => ({
  chatStore: {
    addAssistantMessage: vi.fn(),
    addSystemMessage: vi.fn(),
  },
}));

import { handleLoopStateChange } from "./agentChatBridge";
import { chatStore } from "$lib/stores/chatStore.svelte";

const mockedChatStore = vi.mocked(chatStore);

describe("agentChatBridge", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  // Helper to create a mock loop
  const createMockLoop = (overrides?: Partial<AgentLoop>): AgentLoop => ({
    loop_id: "loop-1",
    task_id: "task-1",
    state: "exploring",
    role: "architect",
    iterations: 1,
    max_iterations: 10,
    user_id: "user-1",
    channel_type: "chat",
    parent_loop_id: "",
    outcome: "",
    error: "",
    ...overrides,
  });

  // =========================================================================
  // New loop (previousState = null)
  // =========================================================================

  describe("new loop (previousState = null)", () => {
    it("should call addAssistantMessage with agent-loop attachment", () => {
      const loop = createMockLoop({
        state: "exploring",
        role: "architect",
      });

      handleLoopStateChange(loop, null);

      expect(mockedChatStore.addAssistantMessage).toHaveBeenCalledTimes(1);
      const [message, attachments] =
        mockedChatStore.addAssistantMessage.mock.calls[0];
      expect(message).toContain("Agent loop started");
      expect(message).toContain("architect");

      expect(attachments).toHaveLength(1);
      const attachment = attachments![0] as AgentLoopAttachment;
      expect(attachment.kind).toBe("agent-loop");
      expect(attachment.loopId).toBe("loop-1");
      expect(attachment.state).toBe("exploring");
      expect(attachment.role).toBe("architect");
      expect(attachment.iterations).toBe(1);
      expect(attachment.maxIterations).toBe(10);
    });

    it("should include parentLoopId when present", () => {
      const loop = createMockLoop({
        parent_loop_id: "parent-1",
      });

      handleLoopStateChange(loop, null);

      const [, attachments] = mockedChatStore.addAssistantMessage.mock.calls[0];
      const attachment = attachments![0] as AgentLoopAttachment;
      expect(attachment.parentLoopId).toBe("parent-1");
    });

    it("should omit parentLoopId when empty string", () => {
      const loop = createMockLoop({
        parent_loop_id: "",
      });

      handleLoopStateChange(loop, null);

      const [, attachments] = mockedChatStore.addAssistantMessage.mock.calls[0];
      const attachment = attachments![0] as AgentLoopAttachment;
      expect(attachment.parentLoopId).toBeUndefined();
    });
  });

  // =========================================================================
  // State unchanged — should skip
  // =========================================================================

  describe("state unchanged", () => {
    it("should NOT call any chatStore method when state has not changed", () => {
      const loop = createMockLoop({ state: "exploring" });

      handleLoopStateChange(loop, "exploring");

      expect(mockedChatStore.addAssistantMessage).not.toHaveBeenCalled();
      expect(mockedChatStore.addSystemMessage).not.toHaveBeenCalled();
    });
  });

  // =========================================================================
  // Transition to awaiting_approval
  // =========================================================================

  describe("awaiting_approval", () => {
    it("should inject approval prompt with approval attachment", () => {
      const loop = createMockLoop({
        state: "awaiting_approval",
        role: "builder",
      });

      handleLoopStateChange(loop, "exploring");

      expect(mockedChatStore.addAssistantMessage).toHaveBeenCalledTimes(1);
      const [message, attachments] =
        mockedChatStore.addAssistantMessage.mock.calls[0];
      expect(message).toContain("waiting for approval");
      expect(message).toContain("builder");

      expect(attachments).toHaveLength(1);
      const attachment = attachments![0] as ApprovalAttachment;
      expect(attachment.kind).toBe("approval");
      expect(attachment.loopId).toBe("loop-1");
      expect(attachment.question).toContain("builder");
    });
  });

  // =========================================================================
  // Transition to complete
  // =========================================================================

  describe("complete", () => {
    it("should inject completion message with loop attachment", () => {
      const loop = createMockLoop({
        state: "complete",
        role: "architect",
        outcome: "Generated API specification",
      });

      handleLoopStateChange(loop, "executing");

      expect(mockedChatStore.addAssistantMessage).toHaveBeenCalledTimes(1);
      const [message, attachments] =
        mockedChatStore.addAssistantMessage.mock.calls[0];
      expect(message).toContain("Agent loop completed");
      expect(message).toContain("architect");
      expect(message).toContain("Generated API specification");

      const attachment = attachments![0] as AgentLoopAttachment;
      expect(attachment.kind).toBe("agent-loop");
      expect(attachment.state).toBe("complete");
    });

    it("should handle completion without outcome", () => {
      const loop = createMockLoop({
        state: "complete",
        outcome: "",
      });

      handleLoopStateChange(loop, "executing");

      const [message] = mockedChatStore.addAssistantMessage.mock.calls[0];
      expect(message).toContain("Agent loop completed");
      // Should not have trailing " — "
      expect(message).not.toMatch(/— $/);
    });
  });

  // =========================================================================
  // Transition to failed
  // =========================================================================

  describe("failed", () => {
    it("should inject failure message with error detail", () => {
      const loop = createMockLoop({
        state: "failed",
        role: "builder",
        error: "Compilation error in main.go",
      });

      handleLoopStateChange(loop, "executing");

      expect(mockedChatStore.addAssistantMessage).toHaveBeenCalledTimes(1);
      const [message, attachments] =
        mockedChatStore.addAssistantMessage.mock.calls[0];
      expect(message).toContain("Agent loop failed");
      expect(message).toContain("builder");
      expect(message).toContain("Compilation error in main.go");

      const attachment = attachments![0] as AgentLoopAttachment;
      expect(attachment.kind).toBe("agent-loop");
      expect(attachment.state).toBe("failed");
    });

    it("should handle failure without error detail", () => {
      const loop = createMockLoop({
        state: "failed",
        error: "",
      });

      handleLoopStateChange(loop, "executing");

      const [message] = mockedChatStore.addAssistantMessage.mock.calls[0];
      expect(message).toContain("Agent loop failed");
      expect(message).not.toMatch(/— $/);
    });
  });

  // =========================================================================
  // Transition to cancelled
  // =========================================================================

  describe("cancelled", () => {
    it("should inject cancellation message with loop attachment", () => {
      const loop = createMockLoop({
        state: "cancelled",
        role: "reviewer",
      });

      handleLoopStateChange(loop, "paused");

      expect(mockedChatStore.addAssistantMessage).toHaveBeenCalledTimes(1);
      const [message, attachments] =
        mockedChatStore.addAssistantMessage.mock.calls[0];
      expect(message).toContain("Agent loop cancelled");
      expect(message).toContain("reviewer");

      const attachment = attachments![0] as AgentLoopAttachment;
      expect(attachment.kind).toBe("agent-loop");
      expect(attachment.state).toBe("cancelled");
    });
  });

  // =========================================================================
  // Other state transitions (no specific message)
  // =========================================================================

  describe("other transitions", () => {
    it("should not inject a message for exploring -> planning", () => {
      const loop = createMockLoop({ state: "planning" });

      handleLoopStateChange(loop, "exploring");

      expect(mockedChatStore.addAssistantMessage).not.toHaveBeenCalled();
    });

    it("should not inject a message for planning -> executing", () => {
      const loop = createMockLoop({ state: "executing" });

      handleLoopStateChange(loop, "planning");

      expect(mockedChatStore.addAssistantMessage).not.toHaveBeenCalled();
    });

    it("should not inject a message for exploring -> paused", () => {
      const loop = createMockLoop({ state: "paused" });

      handleLoopStateChange(loop, "exploring");

      expect(mockedChatStore.addAssistantMessage).not.toHaveBeenCalled();
    });
  });
});
