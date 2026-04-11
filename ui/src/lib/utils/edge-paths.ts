/**
 * Edge path utilities for flow visualization
 *
 * Generates SVG path data for edges between nodes.
 * Uses cubic Bezier curves for smooth connections.
 */

import type { LayoutEdge } from "./d3-layout";

/** Path styling options */
export interface PathStyle {
  /** Stroke color */
  stroke: string;
  /** Stroke width */
  strokeWidth: number;
  /** Dash pattern (empty for solid) */
  strokeDasharray: string;
  /** Whether to show arrow marker */
  showArrow: boolean;
}

/** Default path styles by validation state */
export const PATH_STYLES = {
  valid: {
    stroke: "var(--ui-interactive-primary)",
    strokeWidth: 2,
    strokeDasharray: "",
    showArrow: true,
  },
  error: {
    stroke: "var(--status-error)",
    strokeWidth: 2,
    strokeDasharray: "5,5",
    showArrow: true,
  },
  warning: {
    stroke: "var(--status-warning)",
    strokeWidth: 2,
    strokeDasharray: "5,5",
    showArrow: true,
  },
  auto: {
    stroke: "var(--ui-interactive-secondary)",
    strokeWidth: 2,
    strokeDasharray: "3,3",
    showArrow: true,
  },
} as const;

/**
 * Generate a cubic Bezier curve path between two points
 *
 * Creates a smooth horizontal-first curve suitable for left-to-right flow diagrams.
 */
export function generateBezierPath(
  sourceX: number,
  sourceY: number,
  targetX: number,
  targetY: number,
): string {
  // Control point offset (how far the curve extends horizontally)
  const dx = Math.abs(targetX - sourceX);
  const controlOffset = Math.min(dx * 0.5, 100);

  // Source control point: extends right from source
  const c1x = sourceX + controlOffset;
  const c1y = sourceY;

  // Target control point: extends left from target
  const c2x = targetX - controlOffset;
  const c2y = targetY;

  return `M ${sourceX} ${sourceY} C ${c1x} ${c1y}, ${c2x} ${c2y}, ${targetX} ${targetY}`;
}

/**
 * Generate path data for a layout edge
 */
export function edgeToPath(edge: LayoutEdge): string {
  return generateBezierPath(
    edge.sourceX,
    edge.sourceY,
    edge.targetX,
    edge.targetY,
  );
}

/**
 * Get path style based on edge properties
 */
export function getPathStyle(edge: LayoutEdge): PathStyle {
  const conn = edge.original;

  // Check validation state
  if (conn.validationState === "error") {
    return PATH_STYLES.error;
  }
  if (conn.validationState === "warning") {
    return PATH_STYLES.warning;
  }

  // Check if auto-discovered
  if (conn.source === "auto") {
    return PATH_STYLES.auto;
  }

  return PATH_STYLES.valid;
}

/**
 * Generate arrow marker definition for SVG defs
 *
 * Returns the SVG markup for an arrow marker that can be referenced by edges.
 */
export function generateArrowMarker(
  id: string,
  color: string = "var(--ui-interactive-primary)",
): string {
  return `
		<marker
			id="${id}"
			viewBox="0 0 10 10"
			refX="9"
			refY="5"
			markerWidth="6"
			markerHeight="6"
			orient="auto-start-reverse"
		>
			<path d="M 0 0 L 10 5 L 0 10 z" fill="${color}" />
		</marker>
	`;
}

/**
 * Calculate the midpoint of a Bezier curve (approximate)
 *
 * Useful for placing labels on edges.
 */
export function getBezierMidpoint(
  sourceX: number,
  sourceY: number,
  targetX: number,
  targetY: number,
): { x: number; y: number } {
  // Simple midpoint (good enough for most cases)
  return {
    x: (sourceX + targetX) / 2,
    y: (sourceY + targetY) / 2,
  };
}

/**
 * Check if a point is near an edge path
 *
 * Useful for hit testing (e.g., clicking on an edge).
 */
export function isPointNearEdge(
  px: number,
  py: number,
  edge: LayoutEdge,
  threshold: number = 10,
): boolean {
  // Sample points along the Bezier curve and check distance
  const samples = 20;

  for (let i = 0; i <= samples; i++) {
    const t = i / samples;
    const point = getBezierPoint(
      edge.sourceX,
      edge.sourceY,
      edge.targetX,
      edge.targetY,
      t,
    );

    const distance = Math.sqrt(
      Math.pow(px - point.x, 2) + Math.pow(py - point.y, 2),
    );
    if (distance <= threshold) {
      return true;
    }
  }

  return false;
}

/**
 * Get a point along a Bezier curve at parameter t (0 to 1)
 */
function getBezierPoint(
  sourceX: number,
  sourceY: number,
  targetX: number,
  targetY: number,
  t: number,
): { x: number; y: number } {
  const dx = Math.abs(targetX - sourceX);
  const controlOffset = Math.min(dx * 0.5, 100);

  const c1x = sourceX + controlOffset;
  const c1y = sourceY;
  const c2x = targetX - controlOffset;
  const c2y = targetY;

  // Cubic Bezier formula
  const mt = 1 - t;
  const mt2 = mt * mt;
  const mt3 = mt2 * mt;
  const t2 = t * t;
  const t3 = t2 * t;

  return {
    x: mt3 * sourceX + 3 * mt2 * t * c1x + 3 * mt * t2 * c2x + t3 * targetX,
    y: mt3 * sourceY + 3 * mt2 * t * c1y + 3 * mt * t2 * c2y + t3 * targetY,
  };
}
