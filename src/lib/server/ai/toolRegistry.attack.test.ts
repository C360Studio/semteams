// Attack tests by Reviewer Agent.
// These test adversarial inputs and edge cases.

import { describe, it, expect } from "vitest";
import { getToolsForContext } from "./toolRegistry";
import type { ToolRegistryConfig } from "./toolRegistry";
import type { ChatIntent, ChatPageContext } from "$lib/types/chat";

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const config: ToolRegistryConfig = { backendUrl: "http://localhost:8080" };

const flowBuilderContext: ChatPageContext = {
  page: "flow-builder",
  flowId: "flow-test",
  flowName: "Test Pipeline",
  nodes: [],
  connections: [],
};

const dataViewContext: ChatPageContext = {
  page: "data-view",
  flowId: "flow-test",
  entityCount: 42,
  selectedEntityId: null,
  filters: {},
};

// ---------------------------------------------------------------------------
// Null/undefined context fields
// ---------------------------------------------------------------------------

describe("getToolsForContext — null/undefined config fields", () => {
  it("handles empty backendUrl without throwing", () => {
    const emptyConfig: ToolRegistryConfig = { backendUrl: "" };
    expect(() =>
      getToolsForContext(emptyConfig, "search", flowBuilderContext),
    ).not.toThrow();
  });

  it("handles undefined componentCatalog in config without throwing", () => {
    const configNoCatalog: ToolRegistryConfig = {
      backendUrl: "http://localhost:8080",
      componentCatalog: undefined,
    };
    expect(() =>
      getToolsForContext(configNoCatalog, "search", flowBuilderContext),
    ).not.toThrow();
  });

  it("handles empty componentCatalog array without throwing", () => {
    const configEmptyCatalog: ToolRegistryConfig = {
      backendUrl: "http://localhost:8080",
      componentCatalog: [],
    };
    expect(() =>
      getToolsForContext(configEmptyCatalog, "general", flowBuilderContext),
    ).not.toThrow();
  });
});

// ---------------------------------------------------------------------------
// All ChatIntent values — no throws, always returns an array
// ---------------------------------------------------------------------------

describe("getToolsForContext — all intent values return arrays", () => {
  const allIntents: ChatIntent[] = [
    "general",
    "flow-create",
    "flow-modify",
    "search",
    "explain",
    "debug",
    "health",
  ];

  for (const intent of allIntents) {
    it(`intent="${intent}" on flow-builder returns an array`, () => {
      expect(() =>
        getToolsForContext(config, intent, flowBuilderContext),
      ).not.toThrow();
      const result = getToolsForContext(config, intent, flowBuilderContext);
      expect(Array.isArray(result)).toBe(true);
    });

    it(`intent="${intent}" on data-view returns an array`, () => {
      expect(() =>
        getToolsForContext(config, intent, dataViewContext),
      ).not.toThrow();
      const result = getToolsForContext(config, intent, dataViewContext);
      expect(Array.isArray(result)).toBe(true);
    });
  }

  it("unknown/future intent string returns an array without throwing", () => {
    expect(() =>
      getToolsForContext(config, "unknown-future-intent" as never, flowBuilderContext),
    ).not.toThrow();
    const result = getToolsForContext(
      config,
      "unknown-future-intent" as never,
      flowBuilderContext,
    );
    expect(Array.isArray(result)).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// Tool schema integrity — returned tools are always well-formed
// ---------------------------------------------------------------------------

describe("getToolsForContext — all returned tools are schema-valid", () => {
  const allIntents: ChatIntent[] = [
    "general",
    "flow-create",
    "search",
    "explain",
    "debug",
    "health",
  ];
  const contexts = [flowBuilderContext, dataViewContext];

  for (const intent of allIntents) {
    for (const context of contexts) {
      it(`all tools for intent="${intent}" page="${context.page}" have required fields`, () => {
        const tools = getToolsForContext(config, intent, context);
        for (const tool of tools) {
          expect(typeof tool.name).toBe("string");
          expect(tool.name.length).toBeGreaterThan(0);
          expect(typeof tool.description).toBe("string");
          expect(tool.description.length).toBeGreaterThan(0);
          expect(typeof tool.parameters).toBe("object");
          expect(tool.parameters.type).toBe("object");
          expect(Array.isArray(tool.parameters.required)).toBe(true);
          expect(typeof tool.parameters.properties).toBe("object");
        }
      });
    }
  }
});

// ---------------------------------------------------------------------------
// Tool name uniqueness — no duplicate tool names per call
// ---------------------------------------------------------------------------

describe("getToolsForContext — no duplicate tool names", () => {
  const allIntents: ChatIntent[] = [
    "general",
    "flow-create",
    "search",
    "explain",
    "debug",
    "health",
  ];

  for (const intent of allIntents) {
    it(`no duplicate tool names for intent="${intent}" on flow-builder`, () => {
      const tools = getToolsForContext(config, intent, flowBuilderContext);
      const names = tools.map((t) => t.name);
      const unique = new Set(names);
      expect(unique.size).toBe(names.length);
    });
  }
});

// ---------------------------------------------------------------------------
// Context with extreme field values
// ---------------------------------------------------------------------------

describe("getToolsForContext — extreme context field values", () => {
  it("handles flow-builder context with 10k nodes without throwing", () => {
    const bigContext: ChatPageContext = {
      page: "flow-builder",
      flowId: "x".repeat(10000),
      flowName: "y".repeat(10000),
      nodes: Array.from({ length: 10000 }, (_, i) => ({
        id: `n${i}`,
        component: "http-input",
        type: "input" as const,
        name: `Node ${i}`,
        position: { x: i, y: i },
        config: {},
      })),
      connections: [],
    };
    expect(() =>
      getToolsForContext(config, "general", bigContext),
    ).not.toThrow();
    const result = getToolsForContext(config, "general", bigContext);
    expect(Array.isArray(result)).toBe(true);
  });

  it("handles data-view context with zero entity count without throwing", () => {
    const zeroCtx: ChatPageContext = {
      page: "data-view",
      flowId: "flow-zero",
      entityCount: 0,
      selectedEntityId: null,
      filters: {},
    };
    expect(() => getToolsForContext(config, "search", zeroCtx)).not.toThrow();
  });

  it("handles data-view context with adversarial selectedEntityId without throwing", () => {
    const injectionCtx: ChatPageContext = {
      page: "data-view",
      flowId: "flow-test",
      entityCount: 1,
      selectedEntityId: "'; DROP TABLE entities; --",
      filters: {},
    };
    expect(() =>
      getToolsForContext(config, "explain", injectionCtx),
    ).not.toThrow();
  });
});
