/**
 * Component Type to Domain Category Mapping
 *
 * Maps component type strings to their domain categories for color/styling.
 * Used by ComponentCard and other UI elements to determine domain colors.
 */

import type { DomainType } from "./domain-colors";

/**
 * Map component type to domain category
 *
 * @param type - Component type (e.g., "udp-input", "mavlink-decoder")
 * @returns Domain category for color styling
 */
export function getComponentDomain(type: string): DomainType {
  // Normalize type to lowercase for matching
  const normalizedType = type.toLowerCase();

  // Robotics domain (MAVLink, autonomous vehicles)
  if (
    normalizedType.includes("mavlink") ||
    normalizedType.includes("telemetry") ||
    normalizedType.includes("drone") ||
    normalizedType.includes("vehicle")
  ) {
    return "robotics";
  }

  // Network I/O domain (UDP, WebSocket, HTTP, TCP)
  if (
    normalizedType.includes("udp") ||
    normalizedType.includes("websocket") ||
    normalizedType.includes("http") ||
    normalizedType.includes("tcp") ||
    normalizedType.includes("input") ||
    normalizedType.includes("output")
  ) {
    return "network";
  }

  // Semantic processing domain (transforms, processors, rules)
  if (
    normalizedType.includes("json") ||
    normalizedType.includes("transform") ||
    normalizedType.includes("processor") ||
    normalizedType.includes("filter") ||
    normalizedType.includes("parser") ||
    normalizedType.includes("semantic") ||
    normalizedType.includes("rule")
  ) {
    return "semantic";
  }

  // Storage domain (database, files, persistence)
  if (
    normalizedType.includes("storage") ||
    normalizedType.includes("database") ||
    normalizedType.includes("writer") ||
    normalizedType.includes("reader") ||
    normalizedType.includes("persist")
  ) {
    return "storage";
  }

  // Geospatial domain
  if (
    normalizedType.includes("geo") ||
    normalizedType.includes("map") ||
    normalizedType.includes("location") ||
    normalizedType.includes("gis")
  ) {
    return "geospatial";
  }

  // Media domain
  if (
    normalizedType.includes("video") ||
    normalizedType.includes("audio") ||
    normalizedType.includes("image") ||
    normalizedType.includes("media")
  ) {
    return "media";
  }

  // Integration domain
  if (
    normalizedType.includes("api") ||
    normalizedType.includes("connector") ||
    normalizedType.includes("integration")
  ) {
    return "integration";
  }

  // Default to network for unknown types
  return "network";
}
