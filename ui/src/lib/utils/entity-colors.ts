/**
 * Entity Color Mapping for Knowledge Graph Visualization
 *
 * Maps entity domains and types to colors for the graph visualization.
 * Entity IDs have format: org.platform.domain.system.type.instance
 * Colors are assigned based on domain (primary) and type (secondary).
 */

import type { EntityIdParts, GraphCommunity } from "$lib/types/graph";

// =============================================================================
// Domain Colors (Primary categorization)
// =============================================================================

/**
 * Domain-to-color mapping using CSS variables.
 * Domains are the 3rd part of the 6-part entity ID.
 */
export const DOMAIN_COLORS: Record<string, string> = {
  // Core domains
  robotics: "var(--graph-domain-robotics, #3b82f6)", // Blue
  network: "var(--graph-domain-network, #8b5cf6)", // Purple
  semantic: "var(--graph-domain-semantic, #06b6d4)", // Cyan
  sensor: "var(--graph-domain-sensor, #22c55e)", // Green
  fleet: "var(--graph-domain-fleet, #f59e0b)", // Amber
  geo: "var(--graph-domain-geo, #ef4444)", // Red
  iot: "var(--graph-domain-iot, #ec4899)", // Pink
  system: "var(--graph-domain-system, #6b7280)", // Gray

  // Fallback
  unknown: "var(--ui-border-subtle, #9ca3af)",
};

/**
 * Get color for an entity's domain.
 */
export function getDomainColor(domain: string | undefined): string {
  if (!domain) return DOMAIN_COLORS.unknown;
  return DOMAIN_COLORS[domain.toLowerCase()] || DOMAIN_COLORS.unknown;
}

// =============================================================================
// Type Colors (Secondary categorization within domain)
// =============================================================================

/**
 * Type-to-color mapping for entity types (5th part of ID).
 * Used as primary node fill color.
 */
export const TYPE_COLORS: Record<string, string> = {
  // Robotics types
  drone: "#60a5fa", // Blue
  vehicle: "#34d399", // Emerald
  robot: "#a78bfa", // Violet
  sensor: "#fbbf24", // Amber

  // Network types
  gateway: "#c084fc", // Purple
  router: "#818cf8", // Indigo
  endpoint: "#22d3ee", // Cyan

  // Data types
  stream: "#4ade80", // Green
  store: "#f472b6", // Pink
  cache: "#fb923c", // Orange

  // Fleet/logistics types (used by mock data)
  fleet: "#3b82f6", // Blue
  driver: "#8b5cf6", // Purple
  location: "#ef4444", // Red

  // Fallback
  unknown: "#9ca3af",
};

/**
 * Get color for an entity type.
 */
export function getTypeColor(type: string | undefined): string {
  if (!type) return TYPE_COLORS.unknown;
  return TYPE_COLORS[type.toLowerCase()] || TYPE_COLORS.unknown;
}

// =============================================================================
// Community Colors (Cluster assignment)
// =============================================================================

/**
 * Color palette for community clusters.
 * Communities are assigned colors in order.
 */
export const COMMUNITY_PALETTE: string[] = [
  "#f87171", // Red
  "#fb923c", // Orange
  "#fbbf24", // Amber
  "#a3e635", // Lime
  "#4ade80", // Green
  "#2dd4bf", // Teal
  "#22d3ee", // Cyan
  "#60a5fa", // Blue
  "#a78bfa", // Violet
  "#f472b6", // Pink
  "#fb7185", // Rose
  "#94a3b8", // Slate (fallback)
];

/**
 * Assign a color to a community based on its index.
 */
export function getCommunityColor(index: number): string {
  return COMMUNITY_PALETTE[index % COMMUNITY_PALETTE.length];
}

/**
 * Assign colors to a list of communities.
 */
export function assignCommunityColors(
  communities: GraphCommunity[],
): GraphCommunity[] {
  return communities.map((community, index) => ({
    ...community,
    color: getCommunityColor(index),
  }));
}

// =============================================================================
// Confidence Colors (Edge opacity/color)
// =============================================================================

/**
 * Get opacity value based on confidence score.
 * Maps 0.0-1.0 confidence to 0.3-1.0 opacity (never fully invisible).
 */
export function getConfidenceOpacity(confidence: number): number {
  // Clamp to 0-1 range
  const clamped = Math.max(0, Math.min(1, confidence));
  // Map to 0.3-1.0 range
  return 0.3 + clamped * 0.7;
}

/**
 * Get a color with confidence-based opacity.
 */
export function getColorWithConfidence(
  baseColor: string,
  confidence: number,
): string {
  const opacity = getConfidenceOpacity(confidence);
  // If it's a CSS variable, we can't easily modify opacity
  // Return an rgba approximation for the most common colors
  if (baseColor.startsWith("var(")) {
    // Extract fallback value if present
    const match = baseColor.match(/var\([^,]+,\s*([^)]+)\)/);
    if (match) {
      return hexToRgba(match[1], opacity);
    }
    // Can't modify CSS variable opacity directly
    return baseColor;
  }
  return hexToRgba(baseColor, opacity);
}

/**
 * Convert hex color to rgba with given opacity.
 */
function hexToRgba(hex: string, opacity: number): string {
  // Remove # if present
  hex = hex.replace("#", "");

  // Parse RGB values
  let r: number, g: number, b: number;

  if (hex.length === 3) {
    r = parseInt(hex[0] + hex[0], 16);
    g = parseInt(hex[1] + hex[1], 16);
    b = parseInt(hex[2] + hex[2], 16);
  } else if (hex.length === 6) {
    r = parseInt(hex.substring(0, 2), 16);
    g = parseInt(hex.substring(2, 4), 16);
    b = parseInt(hex.substring(4, 6), 16);
  } else {
    // Invalid hex, return gray
    return `rgba(156, 163, 175, ${opacity})`;
  }

  return `rgba(${r}, ${g}, ${b}, ${opacity})`;
}

// =============================================================================
// Entity Color Assignment
// =============================================================================

/**
 * Get the primary color for an entity based on its ID parts.
 * Uses TYPE (5th part of ID) for distinct colors per entity type.
 */
export function getEntityColor(idParts: EntityIdParts): string {
  // Primary color is based on type (e.g., fleet, vehicle, driver, location)
  return getTypeColor(idParts.type);
}

/**
 * Get the accent color for an entity based on its type.
 */
export function getEntityAccentColor(idParts: EntityIdParts): string {
  return getTypeColor(idParts.type);
}

/**
 * Get complete color scheme for an entity.
 */
export function getEntityColorScheme(
  idParts: EntityIdParts,
  community?: GraphCommunity,
): {
  fill: string;
  stroke: string;
  accent: string;
  communityBorder?: string;
} {
  return {
    fill: getEntityColor(idParts),
    stroke: getEntityAccentColor(idParts),
    accent: getTypeColor(idParts.type),
    communityBorder: community?.color,
  };
}

// =============================================================================
// Predicate Colors (Relationship types)
// =============================================================================

/**
 * Color mapping for relationship predicates by category.
 */
export const PREDICATE_COLORS: Record<string, string> = {
  // Membership/grouping
  membership: "#f59e0b",
  contains: "#f59e0b",
  belongs: "#f59e0b",

  // Location
  location: "#ef4444",
  position: "#ef4444",
  geo: "#ef4444",

  // Data flow
  sends: "#3b82f6",
  receives: "#3b82f6",
  publishes: "#3b82f6",
  subscribes: "#3b82f6",

  // Control
  controls: "#8b5cf6",
  commands: "#8b5cf6",
  manages: "#8b5cf6",

  // State
  status: "#22c55e",
  state: "#22c55e",
  health: "#22c55e",

  // Default
  default: "#6b7280",
};

/**
 * Get color for a relationship predicate.
 */
export function getPredicateColor(predicate: string): string {
  // Extract first part of dotted predicate (e.g., "fleet.membership.current" -> "membership")
  const parts = predicate.split(".");
  const category = parts[1] || parts[0]; // Use second part if available (domain.category.property)

  return PREDICATE_COLORS[category.toLowerCase()] || PREDICATE_COLORS.default;
}
