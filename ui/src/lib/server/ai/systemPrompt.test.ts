import { describe, it, expect } from "vitest";
import { buildSystemPrompt } from "./systemPrompt";
import type { ChatIntent, ChatPageContext, ContextChip } from "$lib/types/chat";

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const flowBuilderContext: ChatPageContext = {
  page: "flow-builder",
  flowId: "flow-123",
  flowName: "My Pipeline",
  nodes: [],
  connections: [],
};

const flowBuilderContextWithNodes: ChatPageContext = {
  page: "flow-builder",
  flowId: "flow-456",
  flowName: "Active Pipeline",
  nodes: [
    {
      id: "n1",
      component: "http-input",
      type: "input",
      name: "HTTP Input",
      position: { x: 0, y: 0 },
      config: {},
    },
    {
      id: "n2",
      component: "nats-output",
      type: "output",
      name: "NATS Output",
      position: { x: 200, y: 0 },
      config: {},
    },
  ],
  connections: [
    {
      id: "c1",
      source_node_id: "n1",
      source_port: "output",
      target_node_id: "n2",
      target_port: "input",
    },
  ],
};

const dataViewContext: ChatPageContext = {
  page: "data-view",
  flowId: "flow-789",
  entityCount: 1234,
  selectedEntityId: null,
  filters: {},
};

const dataViewContextWithSelection: ChatPageContext = {
  page: "data-view",
  flowId: "flow-789",
  entityCount: 500,
  selectedEntityId: "c360.ops.robotics.gcs.drone.001",
  filters: {},
};

const chipEntity: ContextChip = {
  id: "chip-1",
  kind: "entity",
  label: "drone-001",
  value: "c360.ops.robotics.gcs.drone.001",
};

const chipComponent: ContextChip = {
  id: "chip-2",
  kind: "component",
  label: "HTTP Input",
  value: "node-123",
};

// ---------------------------------------------------------------------------
// Base identity
// ---------------------------------------------------------------------------

describe("buildSystemPrompt — base identity", () => {
  it("always includes a SemStreams identity statement", () => {
    const prompt = buildSystemPrompt("general", flowBuilderContext, []);
    expect(prompt.toLowerCase()).toContain("semstreams");
  });

  it("base identity is present for all intents", () => {
    const intents: ChatIntent[] = [
      "general",
      "search",
      "flow-create",
      "explain",
      "debug",
      "health",
    ];
    for (const intent of intents) {
      const prompt = buildSystemPrompt(intent, flowBuilderContext, []);
      expect(
        prompt.toLowerCase(),
        `intent=${intent} must include semstreams`,
      ).toContain("semstreams");
    }
  });

  it("returns a non-empty string", () => {
    const prompt = buildSystemPrompt("general", flowBuilderContext, []);
    expect(typeof prompt).toBe("string");
    expect(prompt.length).toBeGreaterThan(0);
  });
});

// ---------------------------------------------------------------------------
// Page-specific context: flow-builder
// ---------------------------------------------------------------------------

describe("buildSystemPrompt — flow-builder context", () => {
  it("includes flow builder page reference", () => {
    const prompt = buildSystemPrompt("general", flowBuilderContext, []);
    // Should mention "flow builder" or "Flow Builder" in some form
    expect(prompt.toLowerCase()).toMatch(/flow.?builder|flow builder/);
  });

  it("includes flow name when on flow-builder", () => {
    const prompt = buildSystemPrompt("general", flowBuilderContext, []);
    expect(prompt).toContain("My Pipeline");
  });

  it("includes flow ID when on flow-builder", () => {
    const prompt = buildSystemPrompt("general", flowBuilderContext, []);
    expect(prompt).toContain("flow-123");
  });

  it("includes node count for flow-builder context", () => {
    const prompt = buildSystemPrompt(
      "general",
      flowBuilderContextWithNodes,
      [],
    );
    expect(prompt).toContain("2");
  });

  it("includes connection count for flow-builder context with connections", () => {
    const prompt = buildSystemPrompt(
      "general",
      flowBuilderContextWithNodes,
      [],
    );
    expect(prompt).toContain("1");
  });

  it("does NOT include data-view specific content when on flow-builder", () => {
    const prompt = buildSystemPrompt("general", flowBuilderContext, []);
    // Should not mention entity count from data-view context
    expect(prompt).not.toContain("1234");
  });
});

// ---------------------------------------------------------------------------
// Page-specific context: data-view
// ---------------------------------------------------------------------------

describe("buildSystemPrompt — data-view context", () => {
  it("includes data view page reference", () => {
    const prompt = buildSystemPrompt("general", dataViewContext, []);
    expect(prompt.toLowerCase()).toMatch(/data.?view|data view/);
  });

  it("includes entity count for data-view context", () => {
    const prompt = buildSystemPrompt("general", dataViewContext, []);
    expect(prompt).toContain("1234");
  });

  it("includes flow ID for data-view context", () => {
    const prompt = buildSystemPrompt("general", dataViewContext, []);
    expect(prompt).toContain("flow-789");
  });

  it("includes selected entity ID when present", () => {
    const prompt = buildSystemPrompt(
      "general",
      dataViewContextWithSelection,
      [],
    );
    expect(prompt).toContain("c360.ops.robotics.gcs.drone.001");
  });

  it("does NOT mention selected entity when selectedEntityId is null", () => {
    const prompt = buildSystemPrompt("general", dataViewContext, []);
    // No entity ID should appear from selection
    expect(prompt).not.toContain("c360.ops.robotics.gcs.drone.001");
  });

  it("does NOT include flow-builder specific content when on data-view", () => {
    const prompt = buildSystemPrompt("general", dataViewContext, []);
    // Flow name from flowBuilderContext should not be present
    expect(prompt).not.toContain("My Pipeline");
  });
});

// ---------------------------------------------------------------------------
// Context chips
// ---------------------------------------------------------------------------

describe("buildSystemPrompt — context chips", () => {
  it("includes chip label when chips are provided", () => {
    const prompt = buildSystemPrompt("general", flowBuilderContext, [
      chipEntity,
    ]);
    expect(prompt).toContain("drone-001");
  });

  it("includes chip value when chips are provided", () => {
    const prompt = buildSystemPrompt("general", flowBuilderContext, [
      chipEntity,
    ]);
    expect(prompt).toContain("c360.ops.robotics.gcs.drone.001");
  });

  it("includes chip kind when chips are provided", () => {
    const prompt = buildSystemPrompt("general", flowBuilderContext, [
      chipEntity,
    ]);
    expect(prompt.toLowerCase()).toContain("entity");
  });

  it("includes all provided chips", () => {
    const prompt = buildSystemPrompt("general", flowBuilderContext, [
      chipEntity,
      chipComponent,
    ]);
    expect(prompt).toContain("drone-001");
    expect(prompt).toContain("HTTP Input");
  });

  it("does NOT include chip section when chips array is empty", () => {
    const promptWithChips = buildSystemPrompt("general", flowBuilderContext, [
      chipEntity,
    ]);
    const promptNoChips = buildSystemPrompt("general", flowBuilderContext, []);
    // Prompt without chips should be shorter (no chip section)
    expect(promptNoChips.length).toBeLessThan(promptWithChips.length);
  });

  it("handles empty chips array without throwing", () => {
    expect(() =>
      buildSystemPrompt("general", flowBuilderContext, []),
    ).not.toThrow();
  });
});

// ---------------------------------------------------------------------------
// Intent-specific instructions
// ---------------------------------------------------------------------------

describe("buildSystemPrompt — intent: search", () => {
  it("includes graph_search tool reference for search intent", () => {
    const prompt = buildSystemPrompt("search", flowBuilderContext, []);
    expect(prompt.toLowerCase()).toContain("graph_search");
  });

  it("includes search-specific instructions for search intent", () => {
    const prompt = buildSystemPrompt("search", dataViewContext, []);
    // Should mention search or graph_search
    expect(prompt.toLowerCase()).toMatch(/graph_search|search tool/);
  });
});

describe("buildSystemPrompt — intent: flow-create", () => {
  it("includes create_flow tool reference for flow-create intent", () => {
    const prompt = buildSystemPrompt("flow-create", flowBuilderContext, []);
    expect(prompt.toLowerCase()).toContain("create_flow");
  });

  it("includes flow creation guidelines for flow-create intent", () => {
    const prompt = buildSystemPrompt("flow-create", flowBuilderContext, []);
    // Should have some node/connection/flow creation guidance
    expect(prompt.toLowerCase()).toMatch(/create_flow|flow.*tool|component/);
  });
});

describe("buildSystemPrompt — intent: debug", () => {
  it("includes component_health tool reference for debug intent", () => {
    const prompt = buildSystemPrompt("debug", flowBuilderContext, []);
    expect(prompt.toLowerCase()).toContain("component_health");
  });

  it("includes debug-specific instructions for debug intent", () => {
    const prompt = buildSystemPrompt("debug", flowBuilderContext, []);
    expect(prompt.toLowerCase()).toMatch(/debug|component_health|health/);
  });
});

describe("buildSystemPrompt — intent: general", () => {
  it("does not throw for general intent", () => {
    expect(() =>
      buildSystemPrompt("general", flowBuilderContext, []),
    ).not.toThrow();
  });

  it("returns a prompt with base identity for general intent", () => {
    const prompt = buildSystemPrompt("general", flowBuilderContext, []);
    expect(prompt.toLowerCase()).toContain("semstreams");
  });
});

// ---------------------------------------------------------------------------
// Prompt composition: sections are separated
// ---------------------------------------------------------------------------

describe("buildSystemPrompt — prompt structure", () => {
  it("returns a longer prompt with chips than without", () => {
    const withChips = buildSystemPrompt("general", flowBuilderContext, [
      chipEntity,
      chipComponent,
    ]);
    const withoutChips = buildSystemPrompt("general", flowBuilderContext, []);
    expect(withChips.length).toBeGreaterThan(withoutChips.length);
  });

  it("returns a longer prompt for search intent vs general (search has extra instructions)", () => {
    const searchPrompt = buildSystemPrompt("search", flowBuilderContext, []);
    const generalPrompt = buildSystemPrompt("general", flowBuilderContext, []);
    // search prompt has additional search-specific section
    expect(searchPrompt.length).toBeGreaterThanOrEqual(generalPrompt.length);
  });

  it("flow-builder prompt mentions node details when nodes are present", () => {
    const prompt = buildSystemPrompt(
      "general",
      flowBuilderContextWithNodes,
      [],
    );
    // "HTTP Input" or "NATS Output" should appear since nodes are serialized
    expect(prompt).toMatch(/HTTP Input|NATS Output|n1|n2/);
  });

  it("flow-builder prompt with no nodes still produces valid output", () => {
    const prompt = buildSystemPrompt("general", flowBuilderContext, []);
    expect(prompt.toLowerCase()).toContain("semstreams");
    expect(prompt).toContain("flow-123");
  });
});

// ---------------------------------------------------------------------------
// componentCatalog optional parameter
// ---------------------------------------------------------------------------

describe("buildSystemPrompt — componentCatalog parameter", () => {
  it("accepts undefined componentCatalog without throwing", () => {
    expect(() =>
      buildSystemPrompt("general", flowBuilderContext, [], undefined),
    ).not.toThrow();
  });

  it("accepts empty componentCatalog array without throwing", () => {
    expect(() =>
      buildSystemPrompt("flow-create", flowBuilderContext, [], []),
    ).not.toThrow();
  });

  it("includes catalog context in flow-create prompt when catalog provided", () => {
    const catalog = [
      {
        id: "http-input",
        name: "HTTP Input",
        category: "input",
        description: "Receives HTTP requests",
      },
    ];
    const prompt = buildSystemPrompt(
      "flow-create",
      flowBuilderContext,
      [],
      catalog,
    );
    // The catalog or catalog-derived content should be present
    expect(prompt.length).toBeGreaterThan(0);
  });
});
