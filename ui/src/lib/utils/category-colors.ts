/**
 * Component Type Color Mapping
 *
 * Maps backend component types to colors.
 * Types: input, output, processor, gateway, storage
 */

export type ComponentTypeColor =
  | "input"
  | "output"
  | "processor"
  | "gateway"
  | "storage";

/**
 * Type-to-color CSS variable mapping
 */
export const TYPE_COLORS: Record<ComponentTypeColor, string> = {
  input: "var(--category-input)",
  output: "var(--category-output)",
  processor: "var(--category-processor)",
  gateway: "var(--category-gateway)",
  storage: "var(--category-storage)",
};

/**
 * Get color for a component type
 *
 * @param type - Component type from backend (input, output, processor, gateway, storage)
 * @returns CSS variable reference or fallback gray
 */
export function getTypeColor(type: string | undefined): string {
  if (!type) return "var(--ui-border-subtle)";
  return TYPE_COLORS[type as ComponentTypeColor] || "var(--ui-border-subtle)";
}
