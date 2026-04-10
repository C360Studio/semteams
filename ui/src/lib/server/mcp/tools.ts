/**
 * MCP Tool Definitions
 *
 * Defines Model Context Protocol (MCP) tools for AI-assisted flow generation.
 * Provides tools for fetching component catalog and validating flows.
 *
 * Tools:
 * - get_component_catalog: Fetches available component types from backend
 * - validate_flow: Validates a flow configuration
 *
 * These tools are used by Claude to understand available components
 * and validate generated flow configurations.
 */

import type { ComponentType } from "$lib/types/component";
import type { ValidationResult } from "$lib/types/validation";

/**
 * MCP tool definition
 */
export interface MCPTool {
  name: string;
  description: string;
  inputSchema: {
    type: string;
    properties: Record<string, unknown>;
    required: string[];
  };
  execute: (input: Record<string, unknown>) => Promise<unknown>;
}

/**
 * Configuration for tool execution
 */
export interface ToolConfig {
  backendUrl: string;
}

/**
 * Get component catalog tool
 *
 * Fetches available component types from the backend component registry.
 * No input parameters required.
 *
 * @param config Tool configuration with backend URL
 * @returns MCP tool definition
 */
export function createGetComponentCatalogTool(config: ToolConfig): MCPTool {
  return {
    name: "get_component_catalog",
    description:
      "Fetches available component types from the backend component registry",
    inputSchema: {
      type: "object",
      properties: {},
      required: [],
    },
    async execute(): Promise<ComponentType[]> {
      const response = await fetch(`${config.backendUrl}/components/types`);

      if (!response.ok) {
        throw new Error(`HTTP ${response.status}: ${response.statusText}`);
      }

      return await response.json();
    },
  };
}

/**
 * Validate flow tool
 *
 * Validates a flow configuration and returns validation results.
 * Requires flowId and flow object as input parameters.
 *
 * @param config Tool configuration with backend URL
 * @returns MCP tool definition
 */
export function createValidateFlowTool(config: ToolConfig): MCPTool {
  return {
    name: "validate_flow",
    description:
      "Validates a flow configuration and returns validation results",
    inputSchema: {
      type: "object",
      properties: {
        flowId: {
          type: "string",
          description: "The ID of the flow to validate",
        },
        flow: {
          type: "object",
          description: "The flow configuration to validate",
        },
      },
      required: ["flowId", "flow"],
    },
    async execute(input: Record<string, unknown>): Promise<ValidationResult> {
      const { flowId, flow } = input;

      if (!flowId || !flow) {
        throw new Error("Missing required parameters");
      }

      const response = await fetch(
        `${config.backendUrl}/flowbuilder/flows/${flowId}/validate`,
        {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
          },
          body: JSON.stringify(flow),
        },
      );

      if (!response.ok) {
        throw new Error(`HTTP ${response.status}: ${response.statusText}`);
      }

      return await response.json();
    },
  };
}

/**
 * Create all MCP tools
 *
 * Factory function to create all available MCP tools with the given configuration.
 *
 * @param config Tool configuration with backend URL
 * @returns Array of MCP tools
 */
export function createMCPTools(config: ToolConfig): MCPTool[] {
  return [
    createGetComponentCatalogTool(config),
    createValidateFlowTool(config),
  ];
}

/**
 * Get tool by name
 *
 * Helper function to retrieve a specific tool by name.
 *
 * @param tools Array of MCP tools
 * @param name Tool name to find
 * @returns MCP tool or undefined if not found
 */
export function getToolByName(
  tools: MCPTool[],
  name: string,
): MCPTool | undefined {
  return tools.find((tool) => tool.name === name);
}
