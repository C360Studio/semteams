/**
 * SemStreams Design System - Color Tokens
 *
 * MIGRATED TO CSS CUSTOM PROPERTIES (colors.css)
 * This file now references CSS variables defined in /src/styles/colors.css
 * for consistent theming across the application.
 *
 * @see /src/styles/colors.css - Three-layer color architecture
 * @see /docs/design-system/COLOR_PALETTE.md - Full design specification
 */

/**
 * Port type colors - Now using CSS custom properties
 *
 * Maps port patterns to CSS variables defined in colors.css.
 * Allows runtime theming and ensures consistency across components.
 */
export const PORT_COLORS = {
  nats_stream: "var(--port-pattern-stream)", // Blue-700
  nats_request: "var(--port-pattern-request)", // Purple-700
  kv_watch: "var(--port-pattern-watch)", // Emerald-700
  network: "var(--port-pattern-api)", // Orange-700
  file: "var(--port-pattern-file)", // Gray-700
} as const;

/**
 * Semantic colors for validation states
 *
 * References Layer 2 (Status Colors) from colors.css.
 * WCAG AA compliant, separation from domain colors maintained.
 */
export const SEMANTIC_COLORS = {
  error: "var(--status-error)", // Red-60
  warning: "var(--status-warning)", // Yellow-30
  success: "var(--status-success)", // Green-50
  info: "var(--status-info)", // Blue-70
  valid: "inherit", // Use port type color for valid state
} as const;

/**
 * Border patterns for required/optional ports
 */
export const BORDER_PATTERNS = {
  required: "solid",
  optional: "dashed",
} as const;

/**
 * Connection line patterns for different interaction types
 */
export const CONNECTION_PATTERNS = {
  stream: "0", // Solid line (continuous data flow)
  request: "8 4", // Dashed line (request/reply)
  watch: "2 3", // Dotted line (periodic updates)
} as const;

/**
 * Heroicon names for port types
 */
export const PORT_ICONS = {
  nats_stream: "arrow-path-rounded-square",
  nats_request: "arrow-path",
  kv_watch: "eye",
  network: "server",
  file: "document-text",
} as const;

/**
 * Surface colors for backgrounds and text
 *
 * References Layer 1 (UI Colors) from colors.css.
 * Supports future dark mode via CSS custom properties.
 */
export const SURFACE_COLORS = {
  background: "var(--ui-surface-primary)",
  foreground: "var(--ui-text-primary)",
  muted: "var(--ui-surface-secondary)",
  border: "var(--ui-border-subtle)",
} as const;

/**
 * Complete theme object
 *
 * Centralized access to all design tokens.
 * Future: Make this runtime-configurable for theming.
 */
export const THEME = {
  port: PORT_COLORS,
  semantic: SEMANTIC_COLORS,
  border: BORDER_PATTERNS,
  connection: CONNECTION_PATTERNS,
  icons: PORT_ICONS,
  surface: SURFACE_COLORS,
} as const;

// Re-export for convenience
export default THEME;
