import type {
  PortVisualization,
  PortGroup,
  Position,
  ValidatedPort,
  PortVisualStyle,
  PortTooltipContent,
  CompatibilityFeedback,
} from "$lib/types/port";

/**
 * Port visualization utilities
 *
 * Imports theme colors and constants from centralized design system.
 * @see src/lib/theme/colors.ts
 */

import {
  PORT_COLORS as PORT_TYPE_COLORS,
  SEMANTIC_COLORS as VALIDATION_COLORS,
  BORDER_PATTERNS,
  CONNECTION_PATTERNS,
  PORT_ICONS as PORT_TYPE_ICONS,
} from "$lib/theme/colors";

// Re-export theme constants for backward compatibility
export {
  PORT_TYPE_COLORS,
  VALIDATION_COLORS,
  BORDER_PATTERNS,
  CONNECTION_PATTERNS,
  PORT_TYPE_ICONS,
};

/**
 * Groups ports by direction (inputs vs outputs)
 * Creates collapsible groups with port counts
 * Filters out empty groups
 */
export function groupPorts(ports: ValidatedPort[]): PortGroup[] {
  const inputPorts = ports.filter((p) => p.direction === "input");
  const outputPorts = ports.filter((p) => p.direction === "output");

  const groups: PortGroup[] = [];

  // Create input group if there are input ports
  if (inputPorts.length > 0) {
    groups.push({
      id: "inputs",
      label: `Input Ports (${inputPorts.length})`,
      ports: inputPorts,
      collapsed: false, // Default to expanded
      position: "left" as Position,
    });
  }

  // Create output group if there are output ports
  if (outputPorts.length > 0) {
    groups.push({
      id: "outputs",
      label: `Output Ports (${outputPorts.length})`,
      ports: outputPorts,
      collapsed: false, // Default to expanded
      position: "right" as Position,
    });
  }

  return groups;
}

/**
 * Determines the most severe validation state from a list of ports
 * Priority: error > warning > unknown > valid
 */
export function getPortValidationState(
  ports: PortVisualization[],
): "valid" | "warning" | "error" | "unknown" {
  if (ports.length === 0) return "unknown";

  const hasError = ports.some((p) => p.validationState === "error");
  if (hasError) return "error";

  const hasWarning = ports.some((p) => p.validationState === "warning");
  if (hasWarning) return "warning";

  const hasUnknown = ports.some((p) => p.validationState === "unknown");
  if (hasUnknown) return "unknown";

  return "valid";
}

// ============================================================================
// Phase 3: Port Visual Styling Utilities (Spec 014)
// ============================================================================

/**
 * Compute visual style for a port handle
 *
 * Determines color, border pattern, icon, and ARIA label based on port type
 * and requirement status. Uses constants defined above for consistency.
 *
 * @param port - ValidatedPort from backend
 * @returns PortVisualStyle with computed styling properties
 */
export function computePortVisualStyle(port: ValidatedPort): PortVisualStyle {
  // Map backend pattern to spec 014 port type
  // Backend patterns: "stream", "request", "watch", "api"
  // Spec 014 types: nats_stream, nats_request, kv_watch, network, file
  let portType: keyof typeof PORT_TYPE_COLORS;

  switch (port.pattern) {
    case "stream":
      portType = "nats_stream";
      break;
    case "request":
      portType = "nats_request";
      break;
    case "watch":
      portType = "kv_watch";
      break;
    case "api":
      portType = "network";
      break;
    default:
      // Check connection_id for file:// prefix
      if (port.connection_id?.startsWith("file://")) {
        portType = "file";
      } else {
        portType = "network"; // Default to network for unknown patterns
      }
  }

  const color = PORT_TYPE_COLORS[portType];
  const borderPattern = port.required
    ? BORDER_PATTERNS.required
    : BORDER_PATTERNS.optional;
  const iconName = PORT_TYPE_ICONS[portType];

  // Generate ARIA label for screen readers
  const typeLabel = portType.replace("_", " ");
  const directionLabel = port.direction === "input" ? "Input" : "Output";
  const requirementText = port.required ? "required" : "optional";
  const ariaLabel = `${typeLabel} ${directionLabel}: ${port.name} (${requirementText})`;

  // CSS classes for styling
  const cssClasses = [
    "port-handle",
    `port-${portType}`,
    `port-${borderPattern}`,
  ];

  return {
    color,
    borderPattern,
    iconName,
    ariaLabel,
    cssClasses,
  };
}

/**
 * Extract tooltip content from ValidatedPort
 *
 * Transforms backend port data into tooltip-friendly structure
 * with human-readable labels and optional validation state.
 *
 * @param port - ValidatedPort from backend
 * @returns PortTooltipContent for tooltip display
 */
export function extractTooltipContent(port: ValidatedPort): PortTooltipContent {
  // Map pattern to display type name
  let displayType: string;
  switch (port.pattern) {
    case "stream":
      displayType = "NATS Stream";
      break;
    case "request":
      displayType = "NATS Request";
      break;
    case "watch":
      displayType = "KV Watch";
      break;
    case "api":
      displayType = "Network";
      break;
    default:
      displayType = port.connection_id?.startsWith("file://")
        ? "File"
        : "Network";
  }

  return {
    name: port.name,
    type: displayType,
    pattern: port.connection_id, // Show the actual connection ID (NATS subject, etc.)
    requirement: port.required ? "required" : "optional",
    description: port.description,
    validationState: undefined, // Will be populated from validation results
    validationMessage: undefined, // Will be populated from validation results
  };
}

/**
 * Check compatibility between two ports during connection creation
 *
 * Validates that:
 * 1. Directions are opposite (output → input only)
 * 2. Port types match (nats_stream → nats_stream, etc.)
 *
 * Returns feedback for real-time visual indication during drag-and-drop.
 *
 * @param sourcePort - Port being dragged from
 * @param targetPort - Port being hovered over
 * @returns CompatibilityFeedback with visual indicator
 */
export function checkPortCompatibility(
  sourcePort: ValidatedPort,
  targetPort: ValidatedPort,
): CompatibilityFeedback {
  const feedbackClasses: string[] = ["connection-feedback"];

  // Check direction compatibility (output → input only)
  if (sourcePort.direction === targetPort.direction) {
    return {
      sourcePortId: sourcePort.name,
      targetPortId: targetPort.name,
      compatibility: "incompatible",
      indicator: "red-indicator",
      incompatibilityReason: "Cannot connect ports with same direction",
      feedbackClasses: [...feedbackClasses, "feedback-incompatible"],
    };
  }

  // Check type compatibility (same type required)
  if (sourcePort.type !== targetPort.type) {
    return {
      sourcePortId: sourcePort.name,
      targetPortId: targetPort.name,
      compatibility: "incompatible",
      indicator: "red-indicator",
      incompatibilityReason: "Port types do not match",
      feedbackClasses: [...feedbackClasses, "feedback-incompatible"],
    };
  }

  // Ports are compatible
  return {
    sourcePortId: sourcePort.name,
    targetPortId: targetPort.name,
    compatibility: "compatible",
    indicator: "green-highlight",
    incompatibilityReason: undefined,
    feedbackClasses: [...feedbackClasses, "feedback-compatible"],
  };
}
