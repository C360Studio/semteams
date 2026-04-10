/**
 * Port Visualization Types
 *
 * Type definitions for port visualization in the flow canvas editor.
 * Maps backend Port interface to frontend visualization with UI state.
 *
 * @see specs/013-visual-component-wiring/data-model.md
 */

/**
 * Port type discriminator
 * Matches backend Port types from pkg/component/port.go
 */
export type PortType =
  | "nats" // NATS pub/sub
  | "nats-request" // NATS request/reply
  | "jetstream" // NATS JetStream
  | "kvwatch" // KV bucket watch
  | "kvwrite" // KV bucket write
  | "network" // TCP/UDP network binding
  | "file"; // File system access

/**
 * Validation state for ports and connections
 */
export type ValidationState = "valid" | "error" | "warning" | "unknown";

/**
 * XYFlow handle position
 */
export type Position = "top" | "bottom" | "left" | "right";

/**
 * Interface contract for typed ports
 * Ensures data compatibility between connected ports
 */
export interface InterfaceContract {
  type: string; // e.g., "message.Storable"
  version?: string; // e.g., "v1"
  compatible?: string[]; // Also accepts these types
}

/**
 * NATS pub/sub port configuration
 */
export interface NATSPortConfig {
  subject: string; // NATS subject pattern (may include wildcards)
  queue?: string; // Optional queue group for load balancing
  interface?: InterfaceContract;
}

/**
 * NATS request/reply port configuration
 */
export interface NATSRequestPortConfig {
  subject: string; // Request subject
  timeout?: string; // Request timeout duration (e.g., "5s")
  retries?: number; // Number of retry attempts
  interface?: InterfaceContract;
}

/**
 * NATS JetStream port configuration
 */
export interface JetStreamPortConfig {
  // Stream configuration (for outputs)
  stream_name: string; // e.g., "ENTITY_EVENTS"
  subjects: string[]; // e.g., ["events.graph.entity.>"]
  storage?: string; // "file" or "memory", default "file"
  retention_policy?: string; // "limits", "interest", "work_queue"
  retention_days?: number; // Message retention in days
  max_size_gb?: number; // Max stream size in GB
  replicas?: number; // Number of replicas

  // Consumer configuration (for inputs)
  consumer_name?: string; // Durable consumer name
  deliver_policy?: string; // "all", "last", "new"
  ack_policy?: string; // "explicit", "none", "all"
  max_deliver?: number; // Max redelivery attempts

  // Interface contract
  interface?: InterfaceContract;
}

/**
 * KV bucket watch port configuration
 */
export interface KVWatchPortConfig {
  bucket: string; // KV bucket name (e.g., "ENTITY_STATES")
  keys?: string[]; // Keys to watch, empty = all
  history?: boolean; // Include historical values
  interface?: InterfaceContract;
}

/**
 * KV bucket write port configuration
 */
export interface KVWritePortConfig {
  bucket: string; // KV bucket name
  interface?: InterfaceContract;
}

/**
 * Network (TCP/UDP) port configuration
 */
export interface NetworkPortConfig {
  protocol: "tcp" | "udp"; // Network protocol
  host: string; // Bind address or target (e.g., "0.0.0.0")
  port: number; // Port number
}

/**
 * File system port configuration
 */
export interface FilePortConfig {
  path: string; // File path (may include patterns)
  pattern?: string; // Optional pattern for filtering
}

/**
 * Union type of all port configurations
 */
export type PortConfig =
  | NATSPortConfig
  | NATSRequestPortConfig
  | JetStreamPortConfig
  | KVWatchPortConfig
  | KVWritePortConfig
  | NetworkPortConfig
  | FilePortConfig;

/**
 * Frontend representation of a component port with UI state
 *
 * Combines backend Port interface data with frontend visualization state.
 * Created from component type metadata (Discoverable interface) and
 * augmented with validation results from FlowGraph.
 */
export interface PortVisualization {
  // Core port data (from backend Port interface)
  name: string; // Port identifier (e.g., "nats_output", "udp_input")
  direction: "input" | "output"; // Data flow direction
  required: boolean; // Whether port must be connected
  description: string; // Human-readable description

  // Port configuration (from Portable interface)
  type: PortType; // Port type discriminator
  config: PortConfig; // Type-specific configuration

  // UI state
  validationState: ValidationState; // Current validation state
  groupId?: string; // Optional group membership
  position: Position; // XYFlow position (top/bottom/left/right)
}

/**
 * Collapsible grouping of ports
 *
 * Used to manage visual complexity on components with many ports.
 * Collapse state is session-only (not persisted to backend).
 */
export interface PortGroup {
  id: string; // Unique group ID (e.g., "inputs", "outputs")
  label: string; // Display label (e.g., "Input Ports (3)")
  ports: ValidatedPort[]; // Ports in this group (updated for spec 014)
  collapsed: boolean; // Collapse state (session-only)
  position: Position; // Where group is positioned on component
}

/**
 * Validation result from FlowGraph analysis
 *
 * Returned by backend flow validation endpoint.
 * Used to update port and connection visualization states.
 */
export interface ValidationResult {
  validation_status: ValidationStatus; // Backend uses snake_case
  errors: ValidationIssue[];
  warnings: ValidationIssue[];
  nodes: ValidatedNode[]; // Nodes with port information
  discovered_connections: DiscoveredConnection[]; // Backend uses snake_case
}

/**
 * Validated node with port information from backend
 */
export interface ValidatedNode {
  id: string;
  type: string;
  name: string;
  input_ports: ValidatedPort[]; // Backend uses snake_case
  output_ports: ValidatedPort[]; // Backend uses snake_case
}

/**
 * Validated port from backend FlowGraph analysis
 */
export interface ValidatedPort {
  name: string;
  direction: "input" | "output";
  type: string; // Interface contract type (e.g., "message.Storable")
  required: boolean;
  connection_id: string; // NATS subject, network address, etc.
  pattern: string; // stream, request, watch, api
  description: string;
}

/**
 * Overall validation status
 */
export type ValidationStatus = "valid" | "warnings" | "errors";

/**
 * Single validation issue (error or warning)
 */
export interface ValidationIssue {
  type: IssueType;
  severity: "error" | "warning";
  component_name: string; // Backend uses snake_case
  port_name?: string; // Backend uses snake_case
  message: string;
  suggestions: string[];
}

/**
 * Types of validation issues
 */
export type IssueType =
  | "orphaned_port" // Port has no connections
  | "disconnected_node" // Node has no connections at all
  | "unknown_component" // Component type not in registry
  | "invalid_config" // Component config doesn't match schema
  | "port_conflict"; // Multiple components binding to same exclusive port

/**
 * Auto-discovered connection from FlowGraph pattern matching
 */
export interface DiscoveredConnection {
  source_node_id: string; // Backend uses snake_case
  source_port: string; // Backend uses snake_case
  target_node_id: string; // Backend uses snake_case
  target_port: string; // Backend uses snake_case
  connection_id: string; // Connection ID (e.g., NATS subject)
  pattern: string; // Interaction pattern (stream, request, watch, api)
}

// ============================================================================
// Phase 3: Port Visual Styling (Spec 014)
// ============================================================================

/**
 * Visual styling for a port handle
 *
 * Computed from port type and requirement status for visual differentiation.
 * Includes color coding, border patterns, icons, and accessibility labels.
 *
 * @see specs/014-flow-ux-port/data-model.md
 */
export interface PortVisualStyle {
  /** Port type color (hex) - WCAG AA compliant */
  color: string;

  /** Border pattern (solid for required, dashed for optional) */
  borderPattern: "solid" | "dashed";

  /** Heroicon name for port type */
  iconName: string;

  /** ARIA label for screen readers */
  ariaLabel: string;

  /** CSS class names for styling */
  cssClasses: string[];
}

/**
 * Tooltip content for port hover
 *
 * Displays port metadata including name, type, pattern, requirement status,
 * and validation state. Tooltips auto-reposition within viewport bounds.
 *
 * @see specs/014-flow-ux-port/data-model.md
 */
export interface PortTooltipContent {
  /** Port name (e.g., "nats_output") */
  name: string;

  /** Port type (e.g., "nats_stream", "network") */
  type: string;

  /** NATS pattern/subject or network address */
  pattern: string;

  /** Required or optional */
  requirement: "required" | "optional";

  /** Human-readable description */
  description?: string;

  /** Current validation state */
  validationState?: "valid" | "error" | "warning";

  /** Validation message if error/warning */
  validationMessage?: string;
}

/**
 * Collapsible port group state
 *
 * Groups ports by direction and type for components with 10+ ports.
 * Expansion state is session-only (not persisted to backend).
 *
 * @see specs/014-flow-ux-port/data-model.md
 */
export interface PortGroupState {
  /** Unique group ID (e.g., "input-nats-streams") */
  id: string;

  /** Group label (e.g., "Input Ports - NATS Streams (6)") */
  label: string;

  /** Port IDs in this group */
  portIds: string[];

  /** Expanded or collapsed */
  isExpanded: boolean;

  /** Group type for semantic organization */
  groupType:
    | "input"
    | "output"
    | "input-nats"
    | "input-network"
    | "output-nats"
    | "output-kv";
}

/**
 * ValidatedPort with visual enhancements
 *
 * Extends backend ValidatedPort with computed visual styling,
 * tooltip content, and group membership for UI rendering.
 *
 * @see specs/014-flow-ux-port/data-model.md
 */
export interface ValidatedPortWithVisuals extends ValidatedPort {
  /** Visual styling computed from port type and requirement */
  visualStyle?: PortVisualStyle;

  /** Tooltip content derived from port metadata */
  tooltipContent?: PortTooltipContent;

  /** Port group membership (if component has 10+ ports) */
  groupId?: string;
}

/**
 * Port compatibility feedback for connection creation
 *
 * Provides real-time visual feedback during drag-and-drop connection creation.
 * Validates direction compatibility (output to input only) and type matching.
 *
 * @see specs/014-flow-ux-port/data-model.md
 */
export interface CompatibilityFeedback {
  /** Source port identifier */
  sourcePortId: string;

  /** Target port identifier */
  targetPortId: string;

  /** Compatibility status */
  compatibility: "compatible" | "incompatible";

  /** Visual indicator class (green-highlight or red-indicator) */
  indicator: string;

  /** Reason for incompatibility if applicable */
  incompatibilityReason?: string;

  /** CSS classes for feedback styling */
  feedbackClasses: string[];
}
