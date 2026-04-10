/**
 * Domain Color Utilities
 *
 * Maps component domains to their visual colors and labels.
 * Separates domain colors from status colors (error/warning/success).
 *
 * Based on: /docs/design-system/COLOR_PALETTE.md
 * Part of: Three-layer color architecture (Layer 3: Domain & Data Viz)
 */

export interface DomainMetadata {
  color: string;
  label: string;
  description: string;
}

/**
 * Domain color mapping
 *
 * IMPORTANT: These are domain/category colors, NOT status colors!
 * - Robotics = Purple (NOT red, which is reserved for errors)
 * - Semantic = Teal (NOT blue, which is reserved for info)
 * - Network = Blue
 * - Storage = Green #198038 (DIFFERENT from success green #24a148)
 */
export const DOMAIN_COLORS = {
  robotics: {
    color: "var(--domain-robotics)",
    label: "Robotics",
    description: "Autonomous vehicles and MAVLink processing",
  },
  semantic: {
    color: "var(--domain-semantic)",
    label: "Semantic Processing",
    description: "Knowledge graphs, rules, and reasoning",
  },
  network: {
    color: "var(--domain-network)",
    label: "Network I/O",
    description: "Data ingestion and output (UDP, WebSocket, HTTP)",
  },
  storage: {
    color: "var(--domain-storage)",
    label: "Storage",
    description: "Data persistence and retrieval",
  },
  // Future domains (reserved for expansion)
  telemetry: {
    color: "var(--domain-telemetry)",
    label: "Telemetry",
    description: "Metrics and observability",
  },
  geospatial: {
    color: "var(--domain-geospatial)",
    label: "Geospatial",
    description: "GIS, mapping, and location services",
  },
  media: {
    color: "var(--domain-media)",
    label: "Media",
    description: "Video, audio, and image processing",
  },
  integration: {
    color: "var(--domain-integration)",
    label: "Integration",
    description: "External system connectors",
  },
} as const;

export type DomainType = keyof typeof DOMAIN_COLORS;

/**
 * Get domain color CSS variable reference
 *
 * @param domain - Domain name (e.g., "robotics", "semantic", "network", "storage")
 * @returns CSS variable reference (e.g., "var(--domain-robotics)") or fallback gray
 *
 * @example
 * ```svelte
 * <div style="border-left: 4px solid {getDomainColor('robotics')}">
 *   Purple accent stripe
 * </div>
 * ```
 */
export function getDomainColor(domain: string): string {
  const metadata = DOMAIN_COLORS[domain as DomainType];
  return metadata?.color || "var(--ui-border-subtle)";
}

/**
 * Get human-readable domain label
 *
 * @param domain - Domain name (e.g., "robotics")
 * @returns Display label (e.g., "Robotics") or the raw domain name
 *
 * @example
 * ```svelte
 * <span class="domain-label">{getDomainLabel('semantic')}</span>
 * <!-- Outputs: "Semantic Processing" -->
 * ```
 */
export function getDomainLabel(domain: string): string {
  const metadata = DOMAIN_COLORS[domain as DomainType];
  return metadata?.label || domain;
}

/**
 * Get domain description for tooltips
 *
 * @param domain - Domain name
 * @returns Description text or empty string
 *
 * @example
 * ```svelte
 * <div title={getDomainDescription('network')}>
 *   Outputs: "Data ingestion and output (UDP, WebSocket, HTTP)"
 * </div>
 * ```
 */
export function getDomainDescription(domain: string): string {
  const metadata = DOMAIN_COLORS[domain as DomainType];
  return metadata?.description || "";
}

/**
 * Get all domain metadata for a given domain
 *
 * @param domain - Domain name
 * @returns Domain metadata object or null if not found
 */
export function getDomainMetadata(domain: string): DomainMetadata | null {
  return DOMAIN_COLORS[domain as DomainType] || null;
}

/**
 * Get all available domains
 *
 * @returns Array of domain names
 */
export function getAllDomains(): DomainType[] {
  return Object.keys(DOMAIN_COLORS) as DomainType[];
}

/**
 * Check if a domain is valid
 *
 * @param domain - Domain name to check
 * @returns true if domain exists in DOMAIN_COLORS
 */
export function isValidDomain(domain: string): domain is DomainType {
  return domain in DOMAIN_COLORS;
}
