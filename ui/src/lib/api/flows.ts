/**
 * Flow API Client
 *
 * Provides client-side methods for flow CRUD operations and lifecycle management.
 * This replaces SvelteKit form actions with direct fetch() calls for SPA-style updates.
 */

import type { Flow, FlowNode, FlowConnection } from "$lib/types/flow";

/**
 * Data structure for updating an existing flow
 */
export interface FlowUpdateData {
  id: string;
  name: string;
  description?: string;
  version: number;
  runtime_state: string;
  nodes: FlowNode[];
  connections: FlowConnection[];
}

/**
 * Validation result from backend schema validation
 */
export interface ValidationResult {
  valid: boolean;
  errors?: Array<{
    field: string;
    message: string;
    code: string;
  }>;
}

/**
 * Standard error response from backend
 */
export interface APIError {
  error: string;
  validation_result?: ValidationResult;
}

/**
 * Custom error class for validation errors
 */
export class ValidationError extends Error {
  constructor(
    message: string,
    public validationResult: ValidationResult,
  ) {
    super(message);
    this.name = "ValidationError";
  }
}

/**
 * Save flow changes to the backend
 *
 * @param flowId - Flow identifier
 * @param data - Updated flow data
 * @returns Updated flow with new version number
 * @throws Error if save fails (network, validation, conflict)
 */
export async function saveFlow(
  flowId: string,
  data: FlowUpdateData,
): Promise<Flow> {
  const response = await fetch(`/flowbuilder/flows/${flowId}`, {
    method: "PUT",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(data),
  });

  if (!response.ok) {
    const error: APIError = await response.json();
    throw new Error(error.error || "Save failed");
  }

  return await response.json();
}

/**
 * Deploy flow to runtime (validates and writes ComponentConfigs to NATS KV)
 *
 * @param flowId - Flow identifier
 * @returns Updated flow with runtime_state = "deployed_stopped"
 * @throws Error with validation details if deployment fails
 */
export async function deployFlow(flowId: string): Promise<Flow> {
  const response = await fetch(`/flowbuilder/deployment/${flowId}/deploy`, {
    method: "POST",
  });

  if (!response.ok) {
    const error: APIError = await response.json();

    // Check if this is a validation error with structured details
    if (error.validation_result) {
      // Create structured error that can be caught and displayed
      const validationError = new ValidationError(
        error.error || "Flow validation failed",
        error.validation_result,
      );
      throw validationError;
    }

    throw new Error(error.error || "Deploy failed");
  }

  return await response.json();
}

/**
 * Start all components in a deployed flow
 *
 * @param flowId - Flow identifier
 * @returns Updated flow with runtime_state = "running"
 * @throws Error if flow not deployed or start fails
 */
export async function startFlow(flowId: string): Promise<Flow> {
  const response = await fetch(`/flowbuilder/deployment/${flowId}/start`, {
    method: "POST",
  });

  if (!response.ok) {
    const error: APIError = await response.json();
    throw new Error(error.error || "Start failed");
  }

  return await response.json();
}

/**
 * Stop all components in a running flow
 *
 * @param flowId - Flow identifier
 * @returns Updated flow with runtime_state = "deployed_stopped"
 * @throws Error if flow not running or stop fails
 */
export async function stopFlow(flowId: string): Promise<Flow> {
  const response = await fetch(`/flowbuilder/deployment/${flowId}/stop`, {
    method: "POST",
  });

  if (!response.ok) {
    const error: APIError = await response.json();
    throw new Error(error.error || "Stop failed");
  }

  return await response.json();
}

/**
 * Type guard for validation errors
 *
 * @param error - Any error object
 * @returns True if error is a ValidationError instance
 */
export function isValidationError(error: unknown): error is ValidationError {
  return error instanceof ValidationError;
}
