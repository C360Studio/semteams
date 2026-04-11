/**
 * Flow History Store
 *
 * Manages undo/redo history for flow canvas operations using Svelte 5 runes.
 * Stores snapshots of flow state with configurable stack size limit.
 *
 * Features:
 * - Push flow snapshots to history stack
 * - Pop/undo to previous state
 * - Redo functionality
 * - Configurable stack size limit (default 10)
 * - Clear history
 * - Get current state
 * - Check undo/redo availability
 *
 * Usage:
 * ```typescript
 * import { createFlowHistoryStore } from '$lib/stores/flowHistory.svelte';
 *
 * const history = createFlowHistoryStore(10);
 *
 * // Push a flow state
 * history.push(currentFlow);
 *
 * // Undo to previous state
 * const previousFlow = history.pop();
 *
 * // Redo to next state
 * const nextFlow = history.redo();
 *
 * // Check availability
 * if (history.canUndo()) {
 *   // Undo is available
 * }
 * ```
 */

import type { Flow } from "$lib/types/flow";

/**
 * Flow history store interface
 */
export interface FlowHistoryStore {
  /** Current history stack */
  readonly history: Flow[];

  /** Current position in history (for redo support) */
  readonly currentIndex: number;

  /** Maximum history stack size */
  readonly maxSize: number;

  /** Push new flow state to history */
  push: (flow: Flow) => void;

  /** Pop/undo to previous state */
  pop: () => Flow | undefined;

  /** Redo to next state (if available) */
  redo: () => Flow | undefined;

  /** Get current flow state */
  getCurrent: () => Flow | undefined;

  /** Check if undo is available */
  canUndo: () => boolean;

  /** Check if redo is available */
  canRedo: () => boolean;

  /** Clear history */
  clear: () => void;

  /** Get history size */
  size: () => number;
}

/**
 * Deep clone helper using JSON serialization
 * This avoids issues with Svelte $state proxies
 */
function deepClone<T>(obj: T): T {
  return JSON.parse(JSON.stringify(obj));
}

/**
 * Create flow history store using Svelte 5 runes
 *
 * @param maxSize Maximum number of history states to keep (default: 10)
 * @returns Flow history store instance
 */
export function createFlowHistoryStore(maxSize = 10): FlowHistoryStore {
  // Use plain arrays to avoid $state proxy issues with structuredClone
  let historyStack: Flow[] = [];
  let index = -1;

  return {
    // Getters for history state
    get history() {
      return historyStack;
    },
    get currentIndex() {
      return index;
    },
    get maxSize() {
      return maxSize;
    },

    /**
     * Push a new flow state to history
     *
     * - Removes any forward history (for branching after undo)
     * - Adds new state at current position
     * - Enforces max size by removing oldest states
     * - Deep clones the flow to preserve immutability
     */
    push(flow: Flow) {
      // Remove any history after current index (for branching)
      historyStack = historyStack.slice(0, index + 1);

      // Deep clone the flow to preserve immutability
      const clonedFlow = deepClone(flow);

      // Add new state
      historyStack.push(clonedFlow);
      index++;

      // Enforce max size (keep most recent states)
      if (historyStack.length > maxSize) {
        historyStack.shift();
        index--;
      }
    },

    /**
     * Pop/undo to previous state
     *
     * @returns Previous flow state (deep cloned), or undefined if at start
     */
    pop(): Flow | undefined {
      if (index <= 0) {
        return undefined;
      }

      index--;
      return deepClone(historyStack[index]);
    },

    /**
     * Redo to next state
     *
     * @returns Next flow state (deep cloned), or undefined if at end
     */
    redo(): Flow | undefined {
      if (index >= historyStack.length - 1) {
        return undefined;
      }

      index++;
      return deepClone(historyStack[index]);
    },

    /**
     * Get current flow state
     *
     * @returns Current flow state (deep cloned), or undefined if none
     */
    getCurrent(): Flow | undefined {
      if (index < 0 || index >= historyStack.length) {
        return undefined;
      }
      return deepClone(historyStack[index]);
    },

    /**
     * Check if undo is available
     *
     * @returns True if there is a previous state to undo to
     */
    canUndo(): boolean {
      return index > 0;
    },

    /**
     * Check if redo is available
     *
     * @returns True if there is a next state to redo to
     */
    canRedo(): boolean {
      return index < historyStack.length - 1;
    },

    /**
     * Clear all history
     *
     * Resets the history stack and current index to initial state.
     */
    clear() {
      historyStack = [];
      index = -1;
    },

    /**
     * Get current history size
     *
     * @returns Number of states in history
     */
    size(): number {
      return historyStack.length;
    },
  };
}

/**
 * Default singleton flow history store instance
 *
 * This provides a shared history store that can be imported directly
 * for simple use cases where a single history store is sufficient.
 */
export const flowHistory = createFlowHistoryStore(10);
