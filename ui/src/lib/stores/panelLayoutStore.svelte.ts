/**
 * Panel Layout Store
 *
 * Manages the VS Code-style three-panel layout state with:
 * - Panel visibility (open/collapsed)
 * - Panel widths (resizable)
 * - Responsive auto-collapse based on viewport
 * - LocalStorage persistence
 *
 * Usage:
 * ```typescript
 * import { createPanelLayoutStore } from '$lib/stores/panelLayoutStore.svelte';
 *
 * const layout = createPanelLayoutStore();
 *
 * // Read state directly (no $ sigil needed)
 * layout.state.leftPanelOpen
 *
 * // Toggle panels
 * layout.toggleLeft();
 * layout.toggleRight();
 *
 * // Resize panels
 * layout.setLeftWidth(300);
 * layout.setRightWidth(350);
 *
 * // Handle viewport resize
 * layout.handleViewportResize(window.innerWidth);
 * ```
 */

import {
  type PanelLayoutState,
  type ExplorerTab,
  type ViewMode,
  DEFAULT_PANEL_LAYOUT,
  PANEL_BREAKPOINTS,
} from "$lib/types/ui-state";

const STORAGE_KEY = "semstreams-panel-layout";

/**
 * Panel layout store interface - runes-based, state is directly readable
 */
export interface PanelLayoutStore {
  /** Reactive state — read fields directly in templates and $derived/$effect */
  readonly state: PanelLayoutState;

  /** Toggle left panel visibility */
  toggleLeft: () => void;

  /** Toggle right panel visibility */
  toggleRight: () => void;

  /** Toggle both panels (focus mode) */
  toggleBoth: () => void;

  /** Set left panel width */
  setLeftWidth: (width: number) => void;

  /** Set right panel width */
  setRightWidth: (width: number) => void;

  /** Set explorer tab */
  setExplorerTab: (tab: ExplorerTab) => void;

  /** Handle viewport resize for responsive behavior */
  handleViewportResize: (viewportWidth: number) => void;

  /** Toggle monitor mode (full-screen runtime panel) */
  toggleMonitorMode: () => void;

  /** Set monitor mode explicitly */
  setMonitorMode: (enabled: boolean) => void;

  /** Set view mode (flow editor or data view) */
  setViewMode: (mode: ViewMode) => void;

  /** Reset to defaults */
  reset: () => void;

  /** Save current state to localStorage */
  save: () => void;

  /** Load state from localStorage */
  load: () => void;
}

/**
 * Load layout state from localStorage with fallback to defaults
 */
function loadFromStorage(): PanelLayoutState {
  if (typeof window === "undefined") {
    return { ...DEFAULT_PANEL_LAYOUT };
  }

  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored) {
      const parsed = JSON.parse(stored) as Partial<PanelLayoutState>;
      // Merge with defaults to handle missing fields from older versions
      return { ...DEFAULT_PANEL_LAYOUT, ...parsed };
    }
  } catch (e) {
    console.warn("Failed to load panel layout from localStorage:", e);
  }

  return { ...DEFAULT_PANEL_LAYOUT };
}

/**
 * Save layout state to localStorage
 */
function saveToStorage(state: PanelLayoutState): void {
  if (typeof window === "undefined") return;

  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
  } catch (e) {
    console.warn("Failed to save panel layout to localStorage:", e);
  }
}

/**
 * Clamp a value between min and max
 */
function clamp(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), max);
}

/**
 * Create panel layout store
 *
 * Uses Svelte 5 $state rune for fine-grained reactivity.
 *
 * @returns Panel layout store instance
 */
export function createPanelLayoutStore(): PanelLayoutStore {
  // Initialize state from localStorage or defaults
  let state = $state<PanelLayoutState>(loadFromStorage());

  // Track the stored panel visibility before auto-collapse for restoration
  let storedLeftOpen = state.leftPanelOpen;
  let storedRightOpen = state.rightPanelOpen;

  // Helper to save state (strips auto-collapse flags before persisting)
  const saveState = () => {
    const toSave: PanelLayoutState = {
      ...state,
      autoCollapsedLeft: false,
      autoCollapsedRight: false,
    };
    saveToStorage(toSave);
  };

  return {
    get state() {
      return state;
    },

    toggleLeft() {
      state = {
        ...state,
        leftPanelOpen: !state.leftPanelOpen,
        autoCollapsedLeft: false,
      };
      storedLeftOpen = state.leftPanelOpen;
      saveState();
    },

    toggleRight() {
      state = {
        ...state,
        rightPanelOpen: !state.rightPanelOpen,
        autoCollapsedRight: false,
      };
      storedRightOpen = state.rightPanelOpen;
      saveState();
    },

    toggleBoth() {
      const bothOpen = state.leftPanelOpen && state.rightPanelOpen;
      state = {
        ...state,
        leftPanelOpen: !bothOpen,
        rightPanelOpen: !bothOpen,
        autoCollapsedLeft: false,
        autoCollapsedRight: false,
      };
      storedLeftOpen = state.leftPanelOpen;
      storedRightOpen = state.rightPanelOpen;
      saveState();
    },

    setLeftWidth(width: number) {
      state = {
        ...state,
        leftPanelWidth: clamp(width, 200, 400),
      };
      saveState();
    },

    setRightWidth(width: number) {
      state = {
        ...state,
        rightPanelWidth: clamp(width, 240, 480),
      };
      saveState();
    },

    setExplorerTab(tab: ExplorerTab) {
      // Don't save tab state - it's transient
      state = { ...state, explorerTab: tab };
    },

    handleViewportResize(viewportWidth: number) {
      const next = { ...state };

      if (viewportWidth >= PANEL_BREAKPOINTS.FULL) {
        // Full layout - restore panels if they were auto-collapsed
        if (state.autoCollapsedLeft && storedLeftOpen) {
          next.leftPanelOpen = true;
          next.autoCollapsedLeft = false;
        }
        if (state.autoCollapsedRight && storedRightOpen) {
          next.rightPanelOpen = true;
          next.autoCollapsedRight = false;
        }
      } else if (viewportWidth >= PANEL_BREAKPOINTS.MEDIUM) {
        // Medium - auto-collapse right only
        if (state.rightPanelOpen && !state.autoCollapsedRight) {
          next.rightPanelOpen = false;
          next.autoCollapsedRight = true;
        }
        // Restore left if it was auto-collapsed
        if (state.autoCollapsedLeft && storedLeftOpen) {
          next.leftPanelOpen = true;
          next.autoCollapsedLeft = false;
        }
      } else if (viewportWidth >= PANEL_BREAKPOINTS.SMALL) {
        // Small - auto-collapse both
        if (state.leftPanelOpen && !state.autoCollapsedLeft) {
          next.leftPanelOpen = false;
          next.autoCollapsedLeft = true;
        }
        if (state.rightPanelOpen && !state.autoCollapsedRight) {
          next.rightPanelOpen = false;
          next.autoCollapsedRight = true;
        }
      } else {
        // Very small - force both collapsed
        if (state.leftPanelOpen) {
          next.leftPanelOpen = false;
          next.autoCollapsedLeft = true;
        }
        if (state.rightPanelOpen) {
          next.rightPanelOpen = false;
          next.autoCollapsedRight = true;
        }
      }

      // Note: We don't save auto-collapse state to localStorage
      state = next;
    },

    toggleMonitorMode() {
      state = { ...state, monitorMode: !state.monitorMode };
      saveState();
    },

    setMonitorMode(enabled: boolean) {
      state = { ...state, monitorMode: enabled };
      saveState();
    },

    setViewMode(mode: ViewMode) {
      // Don't persist viewMode - it should reset to 'flow' on page reload
      state = { ...state, viewMode: mode };
    },

    reset() {
      const newState = { ...DEFAULT_PANEL_LAYOUT };
      storedLeftOpen = newState.leftPanelOpen;
      storedRightOpen = newState.rightPanelOpen;
      saveState();
      state = newState;
    },

    save() {
      // Not needed externally - save happens in each mutation
      // Kept for interface compatibility
    },

    load() {
      const newState = loadFromStorage();
      storedLeftOpen = newState.leftPanelOpen;
      storedRightOpen = newState.rightPanelOpen;
      state = newState;
    },
  };
}

/**
 * Default singleton panel layout store instance
 */
export const panelLayout = createPanelLayoutStore();
