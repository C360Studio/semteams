// Component Type API Service
// Feature: spec 005-we-just-completed
// Fetches component type definitions from backend

import type { ComponentType } from "$lib/types/component";

const API_BASE = import.meta.env.VITE_API_BASE || "http://localhost";

/**
 * Get all available component types from the backend
 * @returns Promise<ComponentType[]> Array of component type definitions
 * @throws Error if request fails
 */
export async function getComponentTypes(): Promise<ComponentType[]> {
  const response = await fetch(`${API_BASE}/components/types`, {
    method: "GET",
    headers: {
      "Content-Type": "application/json",
    },
  });

  if (!response.ok) {
    throw new Error(`Failed to fetch component types: ${response.statusText}`);
  }

  const data = await response.json();
  return data as ComponentType[];
}

/**
 * Get a single component type by ID
 * @param id Component type ID
 * @returns Promise<ComponentType | null> Component type or null if not found
 */
export async function getComponentType(
  id: string,
): Promise<ComponentType | null> {
  try {
    const types = await getComponentTypes();
    return types.find((t) => t.id === id) || null;
  } catch (error) {
    console.error(`Failed to get component type ${id}:`, error);
    return null;
  }
}
