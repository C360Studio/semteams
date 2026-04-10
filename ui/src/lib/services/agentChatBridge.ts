// Agent chat bridge — translates agent loop state changes into chat messages
// This is a plain module (no runes) that bridges agentStore events to chatStore.

import { chatStore } from "$lib/stores/chatStore.svelte";
import type { AgentLoopAttachment, ApprovalAttachment } from "$lib/types/chat";
import type { AgentLoop, AgentLoopState } from "$lib/types/agent";

function makeLoopAttachment(loop: AgentLoop): AgentLoopAttachment {
  return {
    kind: "agent-loop",
    loopId: loop.loop_id,
    state: loop.state,
    role: loop.role,
    iterations: loop.iterations,
    maxIterations: loop.max_iterations,
    parentLoopId: loop.parent_loop_id || undefined,
  };
}

/**
 * Handle a loop state change by injecting appropriate chat messages.
 *
 * Called when the agentStore receives a loop_update SSE event.
 * Only injects messages for meaningful state transitions:
 * - New loop started (previousState = null)
 * - Transition to awaiting_approval
 * - Transition to complete, failed, or cancelled
 *
 * Intermediate transitions (exploring -> planning, etc.) are silent.
 */
export function handleLoopStateChange(
  loop: AgentLoop,
  previousState: AgentLoopState | null,
): void {
  // New loop started
  if (previousState === null) {
    chatStore.addAssistantMessage(`Agent loop started: ${loop.role}`, [
      makeLoopAttachment(loop),
    ]);
    return;
  }

  // State didn't change — skip (iteration updates don't need chat messages)
  if (previousState === loop.state) return;

  // Awaiting approval — inject approval prompt
  if (loop.state === "awaiting_approval") {
    const attachment: ApprovalAttachment = {
      kind: "approval",
      loopId: loop.loop_id,
      toolName: "", // Will be populated when we have tool-level detail
      toolArgs: {},
      question: `Agent "${loop.role}" needs approval to proceed`,
    };
    chatStore.addAssistantMessage(
      `Agent "${loop.role}" is waiting for approval`,
      [attachment],
    );
    return;
  }

  // Completed
  if (loop.state === "complete") {
    chatStore.addAssistantMessage(
      `Agent loop completed: ${loop.role}${loop.outcome ? ` \u2014 ${loop.outcome}` : ""}`,
      [makeLoopAttachment(loop)],
    );
    return;
  }

  // Failed
  if (loop.state === "failed") {
    chatStore.addAssistantMessage(
      `Agent loop failed: ${loop.role}${loop.error ? ` \u2014 ${loop.error}` : ""}`,
      [makeLoopAttachment(loop)],
    );
    return;
  }

  // Cancelled
  if (loop.state === "cancelled") {
    chatStore.addAssistantMessage(`Agent loop cancelled: ${loop.role}`, [
      makeLoopAttachment(loop),
    ]);
    return;
  }

  // All other transitions (exploring -> planning, etc.) are silent
}
