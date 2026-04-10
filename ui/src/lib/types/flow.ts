// Type definitions for Flow entities
// Matches backend schema from pkg/flowstore/flow.go

import type { RuntimeState, SaveStatus, UserPreferences } from "./ui-state";
import type { ValidationState as PortValidationState } from "./port";

// Re-export for convenience
export type { RuntimeState, SaveStatus, UserPreferences };
export { DEFAULT_PREFERENCES } from "./ui-state";

export interface Flow {
  // Identity
  id: string;
  name: string;
  description?: string;

  // Version for optimistic concurrency
  version: number;

  // Canvas layout (backend calls them 'nodes' not 'components')
  nodes: FlowNode[];
  connections: FlowConnection[];

  // Runtime state
  runtime_state: RuntimeState;
  deployed_at?: string; // ISO 8601, optional
  started_at?: string; // ISO 8601, optional
  stopped_at?: string; // ISO 8601, optional

  // Audit
  created_at: string; // ISO 8601
  updated_at: string; // ISO 8601
  created_by?: string;
  last_modified: string; // ISO 8601
}

// Helper type for UI state (not from backend)
export interface FlowMetadata {
  saveStatus: SaveStatus;
}

// Backend schema: FlowNode
export interface FlowNode {
  id: string;
  component: string; // References ComponentType.id (e.g., "json_filter")
  type: string; // Component type for colors: "input" | "output" | "processor" | "gateway" | "storage"
  name: string;
  position: Position;
  config: Record<string, unknown>; // Component-specific config
}

// UI-only: Extended node with health status
export interface ComponentInstance extends FlowNode {
  health: ComponentHealth;
}

export interface Position {
  x: number;
  y: number;
}

export interface ComponentHealth {
  status: HealthStatus;
  errorMessage?: string;
  lastUpdated: string; // ISO 8601
}

export type HealthStatus = "healthy" | "degraded" | "unhealthy" | "not_running";

/**
 * Connection source type
 * Distinguishes auto-discovered connections (from FlowGraph) vs manually created
 */
export type ConnectionSource = "auto" | "manual";

// Backend schema: FlowConnection (EXTENDED for visual component wiring)
export interface FlowConnection {
  id: string;
  source_node_id: string;
  source_port: string;
  target_node_id: string;
  target_port: string;

  // NEW: Connection source tracking (auto-discovered vs manual)
  // Optional for backward compatibility with existing code
  source?: ConnectionSource;

  // NEW: UI validation state (not persisted to backend)
  validationState?: PortValidationState;
  validationMessage?: string;
}

// UI-only: Extended connection with validation and metrics
export interface Connection extends FlowConnection {
  pattern: InteractionPattern;
  validationError?: string;
  metrics?: ConnectionMetrics;
}

export type InteractionPattern = "stream" | "request" | "watch" | "network";

export interface ConnectionMetrics {
  messageCount: number;
  throughputRate: number; // messages/second
  lastActivity: string; // ISO 8601
}

// ============================================================================
// Phase 3: Connection Visual States (Spec 014)
// ============================================================================

/**
 * Visual styling for connection lines (XYFlow edges)
 *
 * Based on pattern type (stream/request/watch) and validation state.
 * Includes line patterns, colors, error indicators, and tooltips.
 *
 * @see specs/014-flow-ux-port/data-model.md
 */
export interface ConnectionVisualState {
  /** Connection ID (XYFlow edge ID) */
  connectionId: string;

  /** Line style based on pattern type */
  linePattern: "solid" | "dashed" | "dotted";

  /** Line color based on validation state */
  color: string;

  /** Validation state */
  validationState: "valid" | "error" | "warning";

  /** SVG stroke-dasharray value */
  strokeDasharray: string;

  /** Z-index for layering (selected connections on top) */
  zIndex: number;

  /** Optional error icon to display on line */
  errorIcon?: string;

  /** Tooltip content for connection */
  tooltipContent?: string;
}

/**
 * Real-time visual feedback during connection creation
 *
 * Shows port compatibility in real-time as user drags from source port.
 * Compatible ports show green highlight, incompatible show red indicator.
 *
 * @see specs/014-flow-ux-port/data-model.md
 */
export interface CompatibilityFeedback {
  /** Currently dragged source port ID */
  sourcePortId: string;

  /** Target port being hovered over (if any) */
  targetPortId?: string;

  /** Compatibility status */
  compatibility: "compatible" | "incompatible" | "unknown";

  /** Visual indicator to display */
  indicator: "green-highlight" | "red-indicator" | "none";

  /** Reason for incompatibility (if any) */
  incompatibilityReason?: string;

  /** CSS classes for visual feedback */
  feedbackClasses: string[];
}

/**
 * XYFlow edge with visual enhancements
 *
 * Extends XYFlow edge data with connection visual state.
 * Used for pattern-specific styling and error/warning indicators.
 *
 * @see specs/014-flow-ux-port/data-model.md
 */
export interface XYFlowEdgeWithVisuals {
  id: string;
  source: string;
  target: string;
  sourceHandle?: string;
  targetHandle?: string;
  type?: string;
  data?: {
    /** Connection visual state */
    visualState?: ConnectionVisualState;

    /** Pattern type (from backend validation) */
    patternType?: "stream" | "request" | "watch";

    /** Source (auto-discovered vs manual) */
    connectionSource?: "auto" | "manual";
  };
}
