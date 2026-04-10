/**
 * MCP Server
 *
 * Model Context Protocol (MCP) server for AI-assisted flow generation.
 * Orchestrates tool registration, execution, and caching.
 *
 * Features:
 * - Register MCP tools (get_component_catalog, validate_flow)
 * - Execute tool calls from Claude
 * - Cache component catalog (session-scoped)
 * - Orchestrate flow generation workflow
 *
 * Usage:
 * ```typescript
 * import { MCPServer } from '$lib/server/mcp/server';
 *
 * const server = new MCPServer({ backendUrl: 'http://localhost:8080' });
 *
 * // Register tools
 * server.registerTools();
 *
 * // Execute a tool
 * const result = await server.executeTool('get_component_catalog', {});
 * ```
 */

import type { MCPTool, ToolConfig } from "./tools";
import { createMCPTools } from "./tools";
import type { ComponentType } from "$lib/types/component";

/**
 * MCP Server configuration
 */
export interface MCPServerConfig {
  backendUrl: string;
  cacheEnabled?: boolean;
}

/**
 * MCP Server
 *
 * Manages MCP tool registration, execution, and caching.
 */
export class MCPServer {
  private tools = new Map<string, MCPTool>();
  private cache = new Map<string, unknown>();
  private config: MCPServerConfig;

  /**
   * Create a new MCP server
   *
   * @param config Server configuration
   */
  constructor(config: MCPServerConfig) {
    this.config = {
      ...config,
      cacheEnabled: config.cacheEnabled !== false, // Default to true
    };
  }

  /**
   * Register a single tool
   *
   * @param tool MCP tool to register
   */
  registerTool(tool: MCPTool): void {
    this.tools.set(tool.name, tool);
  }

  /**
   * Register all available tools
   *
   * Creates and registers all MCP tools with the server configuration.
   */
  registerTools(): void {
    const toolConfig: ToolConfig = {
      backendUrl: this.config.backendUrl,
    };

    const tools = createMCPTools(toolConfig);
    tools.forEach((tool) => this.registerTool(tool));
  }

  /**
   * Execute a tool by name
   *
   * @param name Tool name
   * @param input Tool input parameters
   * @returns Tool execution result
   */
  async executeTool(
    name: string,
    input: Record<string, unknown>,
  ): Promise<unknown> {
    const tool = this.tools.get(name);

    if (!tool) {
      throw new Error(`Tool not found: ${name}`);
    }

    // Special handling for get_component_catalog with caching
    if (name === "get_component_catalog" && this.config.cacheEnabled) {
      return await this.executeWithCache("component_catalog", async () => {
        return await tool.execute(input);
      });
    }

    return await tool.execute(input);
  }

  /**
   * Get all registered tools
   *
   * @returns Array of registered tools
   */
  getRegisteredTools(): MCPTool[] {
    return Array.from(this.tools.values());
  }

  /**
   * Get a tool by name
   *
   * @param name Tool name
   * @returns MCP tool or undefined if not found
   */
  getTool(name: string): MCPTool | undefined {
    return this.tools.get(name);
  }

  /**
   * Clear all caches
   *
   * Removes all cached data from the server.
   */
  clearCache(): void {
    this.cache.clear();
  }

  /**
   * Get cached value
   *
   * @param key Cache key
   * @returns Cached value or undefined
   */
  getCachedValue(key: string): unknown {
    return this.cache.get(key);
  }

  /**
   * Set cached value
   *
   * @param key Cache key
   * @param value Value to cache
   */
  setCachedValue(key: string, value: unknown): void {
    this.cache.set(key, value);
  }

  /**
   * Execute with cache
   *
   * Executes a function and caches the result.
   * Returns cached value if available.
   *
   * @param cacheKey Cache key
   * @param fn Function to execute
   * @returns Function result (from cache or fresh execution)
   */
  private async executeWithCache<T>(
    cacheKey: string,
    fn: () => Promise<T>,
  ): Promise<T> {
    const cached = this.cache.get(cacheKey);
    if (cached !== undefined) {
      return cached as T;
    }

    const result = await fn();
    this.cache.set(cacheKey, result);
    return result;
  }

  /**
   * Get component catalog (with caching)
   *
   * Fetches component catalog and caches the result.
   *
   * @returns Array of component types
   */
  async getComponentCatalog(): Promise<ComponentType[]> {
    return (await this.executeTool(
      "get_component_catalog",
      {},
    )) as ComponentType[];
  }

  /**
   * Validate flow
   *
   * Validates a flow configuration.
   *
   * @param flowId Flow ID
   * @param flow Flow configuration
   * @returns Validation result
   */
  async validateFlow(flowId: string, flow: unknown): Promise<unknown> {
    return await this.executeTool("validate_flow", { flowId, flow });
  }
}

/**
 * Create an MCP server instance
 *
 * Factory function for creating an MCP server.
 *
 * @param config Server configuration
 * @returns MCP server instance
 */
export function createMCPServer(config: MCPServerConfig): MCPServer {
  const server = new MCPServer(config);
  server.registerTools();
  return server;
}

/**
 * Session-scoped MCP server
 *
 * Creates a new MCP server instance for each session.
 * Used for maintaining separate caches per user session.
 */
export class SessionMCPServer extends MCPServer {
  private sessionId: string;

  constructor(config: MCPServerConfig, sessionId: string) {
    super(config);
    this.sessionId = sessionId;
  }

  /**
   * Get session ID
   *
   * @returns Session ID
   */
  getSessionId(): string {
    return this.sessionId;
  }
}

/**
 * Create a session-scoped MCP server
 *
 * Factory function for creating a session-scoped MCP server.
 *
 * @param config Server configuration
 * @param sessionId Session ID
 * @returns Session MCP server instance
 */
export function createSessionMCPServer(
  config: MCPServerConfig,
  sessionId: string,
): SessionMCPServer {
  const server = new SessionMCPServer(config, sessionId);
  server.registerTools();
  return server;
}
