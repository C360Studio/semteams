// UI State Types for Visual Flow Builder
// Feature: spec 005-we-just-completed

import type { ValidationResult } from "./port";

/**
 * Save status for flow persistence
 * - clean: No unsaved changes, flow is valid
 * - dirty: Local changes not yet persisted to backend
 * - draft: Saved with validation errors (allows work-in-progress)
 * - saving: Save operation in progress
 * - error: Save operation failed
 */
export type SaveStatus = "clean" | "dirty" | "draft" | "saving" | "error";

/**
 * Save state tracking
 */
export interface SaveState {
  /** Current save status */
  status: SaveStatus;

  /** Timestamp of last successful save to server */
  lastSaved: Date | null;

  /** Error message if save failed */
  error: string | null;

  /** Validation result from last save (if any) */
  validationResult?: ValidationResult | null;
}

/**
 * Runtime state for flow execution
 * Matches backend RuntimeState from pkg/flowstore/flow.go
 * - not_deployed: Flow not deployed (initial state)
 * - deployed_stopped: Flow deployed but not running
 * - running: Flow actively processing data
 * - error: Runtime failure (component crash, validation error)
 */
export type RuntimeState =
  | "not_deployed"
  | "deployed_stopped"
  | "running"
  | "error";

/**
 * Runtime state information
 */
export interface RuntimeStateInfo {
  /** Current runtime state */
  state: RuntimeState;

  /** Error message if state === 'error' */
  message: string | null;

  /** When state last changed */
  lastTransition: Date | null;
}

/**
 * Navigation guard state for preventing navigation with unsaved changes
 * Used by NavigationGuard component to manage user choices
 */
export interface NavigationGuardState {
  /** Has unsaved changes */
  isDirty: boolean;

  /** Currently blocking navigation (dialog shown) */
  isBlocking: boolean;

  /** Destination URL if navigation was blocked */
  pendingNavigation: string | null;

  /** User's choice from the dialog */
  userChoice: "save" | "discard" | "cancel" | null;
}

/**
 * User preferences for flow editor
 * Currently placeholder for future theme support
 */
export type UserPreferences = Record<string, never>;

/**
 * Default user preferences
 */
export const DEFAULT_PREFERENCES: UserPreferences = {};

/**
 * Explorer panel tab types
 */
export type ExplorerTab = "components" | "palette";

/**
 * Properties panel display mode
 * - empty: Nothing selected, show placeholder
 * - type-preview: Hovering/selecting in Palette, show component type info
 * - edit: Editing a flow node's configuration
 */
export type PropertiesPanelMode = "empty" | "type-preview" | "edit";

/**
 * View mode for the flow page
 * - flow: Flow editor canvas (default)
 * - data: Knowledge graph visualization (only when flow is running)
 */
export type ViewMode = "flow" | "data";

/**
 * Panel layout state for three-panel VS Code-style layout
 * Manages panel visibility, widths, and responsive behavior
 */
export interface PanelLayoutState {
  /** Left panel (Explorer) visibility */
  leftPanelOpen: boolean;

  /** Right panel (Properties) visibility */
  rightPanelOpen: boolean;

  /** Left panel width in pixels */
  leftPanelWidth: number;

  /** Right panel width in pixels */
  rightPanelWidth: number;

  /** Whether left panel was auto-collapsed due to viewport size (vs user action) */
  autoCollapsedLeft: boolean;

  /** Whether right panel was auto-collapsed due to viewport size (vs user action) */
  autoCollapsedRight: boolean;

  /** Currently active Explorer tab */
  explorerTab: ExplorerTab;

  /** Monitor mode - full-screen runtime panel view */
  monitorMode: boolean;

  /** Current view mode (flow editor or data visualization) */
  viewMode: ViewMode;
}

/**
 * Default panel layout state
 */
export const DEFAULT_PANEL_LAYOUT: PanelLayoutState = {
  leftPanelOpen: true,
  rightPanelOpen: true,
  leftPanelWidth: 280,
  rightPanelWidth: 320,
  autoCollapsedLeft: false,
  autoCollapsedRight: false,
  explorerTab: "components",
  monitorMode: false,
  viewMode: "flow",
};

/**
 * Panel layout responsive breakpoints
 */
export const PANEL_BREAKPOINTS = {
  /** Full three-panel layout */
  FULL: 1200,
  /** Right panel auto-collapses */
  MEDIUM: 900,
  /** Both panels auto-collapse */
  SMALL: 600,
} as const;
