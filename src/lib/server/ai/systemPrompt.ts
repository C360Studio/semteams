import type { ChatIntent, ChatPageContext, ContextChip } from "$lib/types/chat";

// ---------------------------------------------------------------------------
// ComponentCatalogEntry — lightweight catalog item shape for the prompt
// ---------------------------------------------------------------------------

export interface ComponentCatalogEntry {
  id: string;
  name: string;
  category: string;
  description: string;
  [key: string]: unknown;
}

// ---------------------------------------------------------------------------
// buildSystemPrompt — compose a system prompt from intent + context + chips
// ---------------------------------------------------------------------------

export function buildSystemPrompt(
  intent: ChatIntent,
  context: ChatPageContext,
  chips: ContextChip[],
  componentCatalog?: ComponentCatalogEntry[],
): string {
  const sections: string[] = [];

  // 1. Base identity — always present
  sections.push(buildBaseIdentity());

  // 2. Page-specific context
  sections.push(buildPageContext(context));

  // 3. Chip context — only when chips are provided
  if (chips.length > 0) {
    sections.push(buildChipContext(chips));
  }

  // 4. Intent-specific instructions
  const intentSection = buildIntentSection(intent, componentCatalog);
  if (intentSection) {
    sections.push(intentSection);
  }

  return sections.join("\n\n");
}

// ---------------------------------------------------------------------------
// Section builders
// ---------------------------------------------------------------------------

function buildBaseIdentity(): string {
  return `You are a helpful assistant for SemStreams, a real-time data pipeline and knowledge graph platform. You help users build, monitor, and understand their data flows and the entities in their knowledge graph.`;
}

function buildPageContext(context: ChatPageContext): string {
  if (context.page === "flow-builder") {
    const lines: string[] = [
      `## Flow Builder Context`,
      `You are helping the user on the Flow Builder page.`,
      `Flow ID: ${context.flowId}`,
      `Flow Name: ${context.flowName}`,
      `Nodes: ${context.nodes.length}`,
      `Connections: ${context.connections.length}`,
    ];

    if (context.nodes.length > 0) {
      const nodeList = context.nodes
        .map(
          (n) =>
            `  - ${n.name} (${n.id}, type: ${n.type}, component: ${n.component})`,
        )
        .join("\n");
      lines.push(`\nCurrent nodes:\n${nodeList}`);
    }

    if (context.connections.length > 0) {
      const connList = context.connections
        .map(
          (c) =>
            `  - ${c.id}: ${c.source_node_id}:${c.source_port} → ${c.target_node_id}:${c.target_port}`,
        )
        .join("\n");
      lines.push(`\nCurrent connections:\n${connList}`);
    }

    return lines.join("\n");
  }

  // data-view
  const lines: string[] = [
    `## Data View Context`,
    `You are helping the user on the Data View page.`,
    `Flow ID: ${context.flowId}`,
    `Entity Count: ${context.entityCount}`,
  ];

  if (context.selectedEntityId !== null) {
    lines.push(`Selected Entity: ${context.selectedEntityId}`);
  }

  return lines.join("\n");
}

function buildChipContext(chips: ContextChip[]): string {
  const lines = [
    `## Pinned Context`,
    `The user has pinned the following items:`,
  ];

  for (const chip of chips) {
    lines.push(`  - [${chip.kind}] ${chip.label} (${chip.value})`);
  }

  return lines.join("\n");
}

function buildIntentSection(
  intent: ChatIntent,
  componentCatalog?: ComponentCatalogEntry[],
): string | null {
  switch (intent) {
    case "search":
      return [
        `## Search Instructions`,
        `The user wants to search the knowledge graph. Use the graph_search tool to find entities, communities, and relationships.`,
        `When using graph_search, choose an appropriate mode (semantic, keyword, or hybrid) based on the query.`,
        `Present results clearly with entity IDs, labels, and any relevant relationships.`,
      ].join("\n");

    case "flow-create": {
      const lines = [
        `## Flow Creation Instructions`,
        `The user wants to create a new flow pipeline. Use the create_flow tool to generate the flow.`,
        `When building a flow:`,
        `  - Choose appropriate component types for each stage`,
        `  - Connect components using matching port types`,
        `  - Provide sensible default configurations`,
      ];

      if (componentCatalog && componentCatalog.length > 0) {
        lines.push(`\nAvailable components:`);
        for (const entry of componentCatalog) {
          lines.push(`  - ${entry.name} (${entry.id}): ${entry.description}`);
        }
      }

      return lines.join("\n");
    }

    case "explain":
      return [
        `## Explain Instructions`,
        `The user wants to understand an entity, component, or concept. Use the entity_lookup tool to get details.`,
        `Provide a clear, concise explanation in plain language.`,
      ].join("\n");

    case "debug":
      return [
        `## Debug Instructions`,
        `The user wants to diagnose runtime issues. Use component_health and flow_status tools to inspect the pipeline.`,
        `Look for errors, degraded components, or abnormal metrics.`,
        `Suggest actionable fixes for any problems found.`,
      ].join("\n");

    case "health":
      return [
        `## Health Check Instructions`,
        `The user wants a health summary. Use component_health and flow_status tools to check the pipeline state.`,
        `Report on overall flow status, any unhealthy components, and throughput metrics.`,
      ].join("\n");

    case "general":
    case "flow-modify":
    default:
      return null;
  }
}
