// See https://svelte.dev/docs/kit/types#app.d.ts
// for information about these interfaces
declare global {
  namespace App {
    // interface Error {}
    // interface Locals {}
    // interface PageData {}
    // interface PageState {}
    // interface Platform {}
  }

  interface Window {
    /** E2E test seam: select a graph entity by ID without WebGL canvas interaction. */
    __e2eSelectEntity?: (entityId: string) => void;
    /** E2E test seam: hover a graph entity by ID (pass null to clear). */
    __e2eHoverEntity?: (entityId: string | null) => void;
    /** E2E test seam: set graph filters programmatically (partial update). */
    __e2eSetFilters?: (
      filters: Partial<import("$lib/types/graph").GraphFilters>,
    ) => void;
    /** E2E test seam: expand entity neighbors programmatically. */
    __e2eExpandEntity?: (entityId: string) => Promise<void>;
  }
}

export {};
