# Chat Redesign: General-Purpose Contextual Assistant

ADR-003 | Status: **Proposed** | Date: 2026-03-09

## Decision

Pivot the chat system from a flow-generation-only assistant to a general-purpose
contextual assistant with slash commands, context chips, and page-aware tool
selection. Flow generation remains as one capability among many.

## Context

The current chat system is hardcoded for flow creation:

- Server endpoint has a hardcoded system prompt about flow generation
- `ChatRequest.currentFlow` is mandatory
- `ChatMessage.flow` and `markFlowApplied()` assume every response may produce a flow
- The removed `NlqSearchBar` left a gap in graph search/exploration UX

The chat components themselves (ChatInput, ChatMessage, ChatMessageList) are
~95% generic already. The coupling is concentrated in types, the store, the API
client, and the server endpoint.

## Architecture Overview

```
                   +------------------+
                   |    ChatInput     |  <-- slash command parsing + context chips
                   +--------+---------+
                            |
                   +--------v---------+
                   | SlashCommandReg  |  <-- client-side dispatch
                   | /search /flow ...|
                   +--------+---------+
                            |
              +-------------v--------------+
              |        chatStore           |  <-- generic: messages + context + attachments
              |  (context chips, page ctx) |
              +-------------+--------------+
                            |
              +-------------v--------------+
              |         chatApi            |  <-- generic request envelope
              | POST /api/ai/chat          |
              +-------------+--------------+
                            |
              +-------------v--------------+
              |    Server: +server.ts      |
              |  ToolRegistry + Contexts   |
              |  (configurable per intent) |
              +----------------------------+
```

---

## 1. Type Definitions

### 1.1 Chat Context (replaces hardcoded `currentFlow`)

```typescript
// src/lib/types/chat.ts

/**
 * A context chip is a reference to a domain object the user has pinned
 * into the conversation. The AI sees these as structured context.
 */
export interface ContextChip {
  id: string;
  kind: ContextChipKind;
  label: string; // Human-readable short label
  value: string; // Machine-readable ID (entity ID, node ID, flow ID)
  metadata?: Record<string, unknown>; // Extra data sent to server
}

export type ContextChipKind =
  | "entity" // Graph entity (6-part ID)
  | "component" // Flow component/node
  | "flow" // Entire flow reference
  | "community" // Graph community
  | "custom"; // Extension point

/**
 * Page-level context that the chat system receives from the host page.
 * Each page provides its own context shape.
 */
export type ChatPageContext = FlowBuilderContext | DataViewContext;

export interface FlowBuilderContext {
  page: "flow-builder";
  flowId: string;
  flowName: string;
  nodes: FlowNode[];
  connections: FlowConnection[];
}

export interface DataViewContext {
  page: "data-view";
  flowId: string;
  entityCount: number;
  selectedEntityId: string | null;
  filters: GraphFilters;
}
```

### 1.2 Generalized ChatMessage

```typescript
// src/lib/types/chat.ts

export interface ChatMessage {
  id: string;
  role: "user" | "assistant" | "system";
  content: string;
  timestamp: Date;

  // Attachments: zero or more typed payloads on assistant messages.
  // Replaces the old flow?/flowDiff?/applied? fields.
  attachments?: MessageAttachment[];

  // Context chips that were active when this message was sent.
  // Stored for conversation history replay.
  chips?: ContextChip[];
}

export type MessageAttachment =
  | FlowAttachment
  | SearchResultAttachment
  | EntityDetailAttachment
  | ErrorAttachment;

export interface FlowAttachment {
  kind: "flow";
  flow: Partial<Flow>;
  diff?: FlowDiff;
  applied?: boolean;
  validationResult?: unknown;
}

export interface SearchResultAttachment {
  kind: "search-result";
  query: string;
  entityIds: string[];
  count: number;
  durationMs: number;
  communitySummaries?: Array<{ communityId: string; text: string }>;
}

export interface EntityDetailAttachment {
  kind: "entity-detail";
  entityId: string;
  summary: string;
  propertyCount: number;
  relationshipCount: number;
}

export interface ErrorAttachment {
  kind: "error";
  code: string;
  message: string;
}
```

### 1.3 Generalized ChatRequest/ChatResponse

```typescript
// src/lib/types/chat.ts

export interface ChatRequest {
  messages: Array<{
    role: "user" | "assistant";
    content: string;
  }>;
  context: ChatPageContext;
  chips: ContextChip[];

  // Optional intent hint from slash command parsing.
  // Server uses this to select tools and system prompt sections.
  intent?: ChatIntent;
}

export type ChatIntent =
  | "general" // Default: answer questions
  | "flow-create" // /flow: create/modify flows
  | "search" // /search: graph search
  | "explain" // /explain: explain entity or component
  | "debug" // /debug: diagnose runtime issues
  | "health"; // /health: system health summary
```

### 1.4 Slash Command Types

```typescript
// src/lib/types/slashCommand.ts

export interface SlashCommand {
  name: string; // e.g., "search"
  aliases: string[]; // e.g., ["s", "find"]
  description: string; // Shown in autocomplete
  usage: string; // e.g., "/search <query>"
  intent: ChatIntent; // Maps to server-side intent
  availableOn: PageKind[]; // Which pages this command is available on
  parse: (args: string) => SlashCommandResult;
}

export type PageKind = "flow-builder" | "data-view";

export interface SlashCommandResult {
  intent: ChatIntent;
  content: string; // Cleaned user message (without the slash prefix)
  params: Record<string, unknown>; // Parsed parameters
}

export interface SlashCommandMatch {
  command: SlashCommand;
  result: SlashCommandResult;
}
```

---

## 2. Component Architecture

### 2.1 What Stays (unchanged or minor props change)

| Component         | Change                                                   |
| ----------------- | -------------------------------------------------------- |
| `ChatMessageList` | Add `onChipClick` prop for entity references in messages |
| `ChatToolbar`     | Add context-aware actions (stays generic)                |
| `ValidationBadge` | No change                                                |

### 2.2 What Changes

**ChatInput** -- Add slash command autocomplete and context chip display.

```svelte
<!-- src/lib/components/chat/ChatInput.svelte -->
<script lang="ts">
  import type { ContextChip, SlashCommand } from "$lib/types/chat";

  interface Props {
    onSubmit: (content: string) => void;
    onCancel?: () => void;
    isStreaming?: boolean;
    disabled?: boolean;
    chips?: ContextChip[];                // Active context chips
    onRemoveChip?: (chipId: string) => void;
    placeholder?: string;                  // Context-aware placeholder
    commands?: SlashCommand[];             // Available slash commands for autocomplete
  }
</script>
```

Key behavior:

- When user types `/`, show autocomplete dropdown filtered by current page
- Chips render as removable pills above the textarea
- Placeholder changes based on page context ("Ask about this flow..." vs
  "Search the knowledge graph...")

**ChatMessage** -- Render attachments by kind instead of checking `message.flow`.

```svelte
<!-- src/lib/components/chat/ChatMessage.svelte -->
<script lang="ts">
  import type { ChatMessage, MessageAttachment } from "$lib/types/chat";

  interface Props {
    message: ChatMessage;
    onApplyFlow?: (messageId: string) => void;
    onViewEntity?: (entityId: string) => void;
    onChipClick?: (chip: ContextChip) => void;
  }
</script>

<!-- Template: iterate message.attachments, render by kind -->
{#each message.attachments ?? [] as attachment}
  {#if attachment.kind === "flow"}
    <FlowDiffSummary diff={attachment.diff} />
    <button disabled={attachment.applied}>Apply to Canvas</button>
  {:else if attachment.kind === "search-result"}
    <SearchResultSummary result={attachment} />
  {:else if attachment.kind === "entity-detail"}
    <EntityDetailCard detail={attachment} />
  {/if}
{/each}
```

**ChatPanel** -- Accept generic callbacks, delegate to page integration.

```svelte
<!-- src/lib/components/chat/ChatPanel.svelte -->
<script lang="ts">
  import type { ChatMessage, ContextChip, SlashCommand } from "$lib/types/chat";

  interface Props {
    messages: ChatMessage[];
    isStreaming?: boolean;
    streamingContent?: string;
    error?: string | null;
    chips?: ContextChip[];
    commands?: SlashCommand[];
    placeholder?: string;
    emptyStateMessage?: string;
    onSubmit: (content: string) => void;
    onCancel?: () => void;
    onApplyFlow?: (messageId: string) => void;
    onViewEntity?: (entityId: string) => void;
    onRemoveChip?: (chipId: string) => void;
    onChipClick?: (chip: ContextChip) => void;
    onLoadJson?: (data: unknown) => void;
    onExportJson?: () => void;
    onNewChat: () => void;
    toolbarActions?: ToolbarAction[];  // Page-specific toolbar actions
  }
</script>
```

### 2.3 New Components

**ContextChipPill** -- Renders a single context chip with remove button.

```svelte
<!-- src/lib/components/chat/ContextChipPill.svelte -->
<script lang="ts">
  import type { ContextChip } from "$lib/types/chat";

  interface Props {
    chip: ContextChip;
    removable?: boolean;
    onclick?: (chip: ContextChip) => void;
    onRemove?: (chipId: string) => void;
  }
</script>

<span class="chip" data-kind={chip.kind} role="button" tabindex="0">
  <span class="chip-icon">{kindIcon(chip.kind)}</span>
  <span class="chip-label">{chip.label}</span>
  {#if removable}
    <button class="chip-remove" aria-label="Remove {chip.label}">x</button>
  {/if}
</span>
```

**ContextChipBar** -- Renders the row of active chips above the input.

```svelte
<!-- src/lib/components/chat/ContextChipBar.svelte -->
<script lang="ts">
  import type { ContextChip } from "$lib/types/chat";
  import ContextChipPill from "./ContextChipPill.svelte";

  interface Props {
    chips: ContextChip[];
    onRemove: (chipId: string) => void;
    onClick?: (chip: ContextChip) => void;
  }
</script>

<div class="chip-bar" data-testid="context-chip-bar" role="list">
  {#each chips as chip (chip.id)}
    <ContextChipPill {chip} removable onclick={onClick} onRemove={onRemove} />
  {/each}
</div>
```

**SlashCommandMenu** -- Autocomplete dropdown shown when user types `/`.

```svelte
<!-- src/lib/components/chat/SlashCommandMenu.svelte -->
<script lang="ts">
  import type { SlashCommand } from "$lib/types/slashCommand";

  interface Props {
    commands: SlashCommand[];
    filter: string;            // Current text after "/"
    onSelect: (command: SlashCommand) => void;
    onDismiss: () => void;
  }

  let filtered = $derived(
    commands.filter(c =>
      c.name.startsWith(filter) || c.aliases.some(a => a.startsWith(filter))
    )
  );
</script>

<div class="slash-menu" role="listbox" data-testid="slash-command-menu">
  {#each filtered as cmd (cmd.name)}
    <button role="option" onclick={() => onSelect(cmd)}>
      <span class="cmd-name">/{cmd.name}</span>
      <span class="cmd-desc">{cmd.description}</span>
    </button>
  {/each}
</div>
```

**SearchResultSummary** -- Renders search result attachment in a message.

```svelte
<!-- src/lib/components/chat/SearchResultSummary.svelte -->
<script lang="ts">
  import type { SearchResultAttachment } from "$lib/types/chat";

  interface Props {
    result: SearchResultAttachment;
    onViewEntity?: (entityId: string) => void;
  }
</script>
```

**EntityDetailCard** -- Renders entity detail attachment in a message.

```svelte
<!-- src/lib/components/chat/EntityDetailCard.svelte -->
<script lang="ts">
  import type { EntityDetailAttachment } from "$lib/types/chat";

  interface Props {
    detail: EntityDetailAttachment;
    onViewEntity?: (entityId: string) => void;
  }
</script>
```

---

## 3. Store Changes

### 3.1 Generalized chatStore

The store drops flow-specific methods and gains context chip management.

```typescript
// src/lib/stores/chatStore.svelte.ts

import type {
  ChatMessage,
  ContextChip,
  MessageAttachment,
} from "$lib/types/chat";

function createChatStore() {
  let messages = $state<ChatMessage[]>([]);
  let isStreaming = $state(false);
  let streamingContent = $state("");
  let error = $state<string | null>(null);
  let chips = $state<ContextChip[]>([]);

  function makeMessage(
    role: ChatMessage["role"],
    content: string,
    attachments?: MessageAttachment[],
    messageChips?: ContextChip[],
  ): ChatMessage {
    return {
      id: crypto.randomUUID(),
      role,
      content,
      timestamp: new Date(),
      attachments,
      chips: messageChips,
    };
  }

  return {
    // --- Existing (unchanged) ---
    get messages() {
      return messages;
    },
    get isStreaming() {
      return isStreaming;
    },
    get streamingContent() {
      return streamingContent;
    },
    get error() {
      return error;
    },

    addUserMessage(content: string): ChatMessage {
      // Snapshot current chips into the message for history
      const msg = makeMessage("user", content, undefined, [...chips]);
      messages = [...messages, msg];
      return msg;
    },

    addAssistantMessage(
      content: string,
      attachments?: MessageAttachment[],
    ): ChatMessage {
      const msg = makeMessage("assistant", content, attachments);
      messages = [...messages, msg];
      return msg;
    },

    addSystemMessage(content: string): ChatMessage {
      const msg = makeMessage("system", content);
      messages = [...messages, msg];
      return msg;
    },

    setStreaming(streaming: boolean) {
      isStreaming = streaming;
      if (!streaming) streamingContent = "";
    },

    appendStreamContent(chunk: string) {
      streamingContent = streamingContent + chunk;
    },

    finalizeStream(
      fullContent: string,
      attachments?: MessageAttachment[],
    ): ChatMessage {
      const msg = makeMessage("assistant", fullContent, attachments);
      messages = [...messages, msg];
      isStreaming = false;
      streamingContent = "";
      return msg;
    },

    clearConversation() {
      messages = [];
      error = null;
      // Chips persist across conversation clear (they are page context)
    },

    setError(err: string | null) {
      error = err;
    },

    // --- New: Attachment mutation ---

    /**
     * Update a specific attachment on a message.
     * Used for marking flows as applied, etc.
     */
    updateAttachment(
      messageId: string,
      attachmentKind: string,
      update: Partial<MessageAttachment>,
    ) {
      const idx = messages.findIndex((m) => m.id === messageId);
      if (idx === -1) return;
      const msg = messages[idx];
      const attachments = (msg.attachments ?? []).map((a) =>
        a.kind === attachmentKind ? { ...a, ...update } : a,
      );
      const updated = [...messages];
      updated[idx] = { ...msg, attachments };
      messages = updated;
    },

    // --- New: Context chip management ---

    get chips() {
      return chips;
    },

    addChip(chip: ContextChip) {
      // Prevent duplicates by value
      if (chips.some((c) => c.kind === chip.kind && c.value === chip.value))
        return;
      chips = [...chips, chip];
    },

    removeChip(chipId: string) {
      chips = chips.filter((c) => c.id !== chipId);
    },

    clearChips() {
      chips = [];
    },

    /**
     * Replace all chips. Used when page context changes
     * (e.g., user selects a different entity in DataView).
     */
    setChips(newChips: ContextChip[]) {
      chips = newChips;
    },
  };
}

export const chatStore = createChatStore();
```

### 3.2 Backward Compatibility

The old `markFlowApplied(messageId)` becomes:

```typescript
chatStore.updateAttachment(messageId, "flow", { applied: true });
```

The old `finalizeStream(content, flow)` becomes:

```typescript
const attachments: MessageAttachment[] = [];
if (flow) {
  attachments.push({ kind: "flow", flow, diff: computeDiff(...) });
}
if (searchResult) {
  attachments.push({ kind: "search-result", ...searchResult });
}
chatStore.finalizeStream(content, attachments);
```

---

## 4. API Client Changes

### 4.1 Generic streamChat

```typescript
// src/lib/services/chatApi.ts

import type { ChatRequest, MessageAttachment } from "$lib/types/chat";

export interface ChatStreamCallbacks {
  onText: (content: string) => void;
  onDone: (data: { attachments?: MessageAttachment[] }) => void;
  onError: (error: string) => void;
}

export async function streamChat(
  request: ChatRequest,
  callbacks: ChatStreamCallbacks,
  signal?: AbortSignal,
): Promise<void> {
  // ... same SSE fetch logic, but request body is now ChatRequest
  // ... onDone receives attachments[] instead of flow?
}
```

The SSE event protocol gains an `attachment` event type alongside the existing
`text` and `done` events. This lets the server stream attachment metadata
(search results, entity details) as they become available, before the final
`done` event.

```
event: text
data: {"content": "I found 3 entities matching..."}

event: attachment
data: {"kind": "search-result", "query": "drones", "entityIds": [...], "count": 3}

event: text
data: {"content": "\n\nHere are the details..."}

event: done
data: {}
```

The client accumulates attachments during streaming:

```typescript
let pendingAttachments: MessageAttachment[] = [];

// In the SSE parsing loop:
if (eventName === "attachment") {
  pendingAttachments.push(parsed as MessageAttachment);
} else if (eventName === "done") {
  callbacks.onDone({ attachments: pendingAttachments });
}
```

---

## 5. Server Endpoint Changes

### 5.1 Request Validation

`currentFlow` is no longer required. The server validates `context` instead:

```typescript
// src/routes/api/ai/chat/+server.ts

interface ServerChatRequest {
  messages: Array<{ role: "user" | "assistant"; content: string }>;
  context: ChatPageContext;
  chips: ContextChip[];
  intent?: ChatIntent;
}

function validateChatRequest(
  body: unknown,
): ValidationResult<ServerChatRequest> {
  // messages: required, non-empty array (unchanged)
  // context: required, must have context.page
  // context.page: must be "flow-builder" or "data-view"
  // chips: optional array, defaults to []
  // intent: optional string, defaults to "general"
}
```

### 5.2 Tool Registry

Replace the hardcoded `create_flow` tool array with a registry that returns
tools based on intent and page context.

```typescript
// src/lib/server/ai/toolRegistry.ts

import type { AiTool } from "$lib/server/ai/provider";
import type { ChatIntent, ChatPageContext } from "$lib/types/chat";

export interface ToolRegistryConfig {
  backendUrl: string;
}

/**
 * Returns the tools available for a given intent + page combination.
 * The AI model only sees tools relevant to the current request.
 */
export function getToolsForContext(
  config: ToolRegistryConfig,
  intent: ChatIntent,
  context: ChatPageContext,
): AiTool[] {
  const tools: AiTool[] = [];

  // Flow tools: available on flow-builder page with flow-create intent
  if (context.page === "flow-builder" && intent === "flow-create") {
    tools.push(createFlowTool());
  }

  // Search tools: available on data-view page, or with search intent
  if (context.page === "data-view" || intent === "search") {
    tools.push(graphSearchTool());
    tools.push(entityLookupTool());
  }

  // Explain tool: always available when entities are in context
  if (intent === "explain") {
    tools.push(entityLookupTool());
  }

  // Debug/health tools: available when runtime data exists
  if (intent === "debug" || intent === "health") {
    tools.push(componentHealthTool(config));
    tools.push(flowStatusTool(config));
  }

  // General intent gets a curated set based on page
  if (intent === "general") {
    if (context.page === "flow-builder") {
      tools.push(createFlowTool());
      tools.push(flowStatusTool(config));
    } else if (context.page === "data-view") {
      tools.push(graphSearchTool());
      tools.push(entityLookupTool());
    }
  }

  return tools;
}
```

Tool definitions (new tools):

```typescript
function graphSearchTool(): AiTool {
  return {
    name: "graph_search",
    description:
      "Search the knowledge graph using natural language or entity ID prefix",
    parameters: {
      type: "object",
      properties: {
        query: {
          type: "string",
          description: "Search query or entity ID prefix",
        },
        mode: {
          type: "string",
          enum: ["nlq", "prefix"],
          description:
            "Search mode: nlq for natural language, prefix for ID matching",
        },
        limit: { type: "number", description: "Max results (default 20)" },
      },
      required: ["query"],
    },
  };
}

function entityLookupTool(): AiTool {
  return {
    name: "entity_lookup",
    description: "Get detailed information about a specific graph entity by ID",
    parameters: {
      type: "object",
      properties: {
        entityId: { type: "string", description: "Full 6-part entity ID" },
        includeNeighbors: {
          type: "boolean",
          description: "Also return 1-hop neighbors (default false)",
        },
      },
      required: ["entityId"],
    },
  };
}

function componentHealthTool(config: ToolRegistryConfig): AiTool {
  return {
    name: "component_health",
    description: "Get health status of flow components",
    parameters: {
      type: "object",
      properties: {
        flowId: { type: "string", description: "Flow ID" },
        componentId: {
          type: "string",
          description:
            "Specific component ID (optional, returns all if omitted)",
        },
      },
      required: ["flowId"],
    },
  };
}

function flowStatusTool(config: ToolRegistryConfig): AiTool {
  return {
    name: "flow_status",
    description: "Get current runtime status of a flow",
    parameters: {
      type: "object",
      properties: {
        flowId: { type: "string", description: "Flow ID" },
      },
      required: ["flowId"],
    },
  };
}
```

### 5.3 Configurable System Prompt

Replace the monolithic `buildSystemPrompt` with composable sections:

```typescript
// src/lib/server/ai/systemPrompt.ts

import type { ChatIntent, ChatPageContext, ContextChip } from "$lib/types/chat";

export function buildSystemPrompt(
  intent: ChatIntent,
  context: ChatPageContext,
  chips: ContextChip[],
  componentCatalog?: unknown[],
): string {
  const sections: string[] = [];

  // Base identity
  sections.push(
    "You are a contextual assistant for SemStreams, a semantic stream processing platform. " +
      "You help users build flows, explore knowledge graphs, debug runtime issues, and " +
      "understand their data.",
  );

  // Page-specific context
  if (context.page === "flow-builder") {
    sections.push(buildFlowBuilderSection(context, componentCatalog));
  } else if (context.page === "data-view") {
    sections.push(buildDataViewSection(context));
  }

  // Intent-specific instructions
  if (intent === "flow-create" && componentCatalog) {
    sections.push(buildFlowCreationInstructions(componentCatalog));
  } else if (intent === "search") {
    sections.push(buildSearchInstructions());
  } else if (intent === "debug") {
    sections.push(buildDebugInstructions());
  }

  // Context chips
  if (chips.length > 0) {
    sections.push(buildChipsSection(chips));
  }

  return sections.join("\n\n");
}

function buildFlowBuilderSection(
  context: FlowBuilderContext,
  catalog?: unknown[],
): string {
  let section = `The user is on the Flow Builder page editing flow "${context.flowName}" (ID: ${context.flowId}).`;
  section += `\nCurrent flow has ${context.nodes.length} node(s) and ${context.connections.length} connection(s).`;
  if (context.nodes.length > 0) {
    section += `\nExisting nodes: ${JSON.stringify(context.nodes)}`;
  }
  return section;
}

function buildDataViewSection(context: DataViewContext): string {
  let section = `The user is on the Data View page for flow "${context.flowId}".`;
  section += `\nThe knowledge graph currently has ${context.entityCount} entities loaded.`;
  if (context.selectedEntityId) {
    section += `\nCurrently selected entity: ${context.selectedEntityId}`;
  }
  return section;
}

function buildChipsSection(chips: ContextChip[]): string {
  const lines = chips.map((c) => `- [${c.kind}] ${c.label}: ${c.value}`);
  return `The user has pinned the following items as context:\n${lines.join("\n")}`;
}

function buildFlowCreationInstructions(catalog: unknown[]): string {
  // The existing flow generation prompt content, extracted verbatim
  // from the current buildSystemPrompt() in +server.ts
  return `When creating or modifying flows, use the create_flow tool. Follow these guidelines:
1. Use only component types from the catalog
2. Create meaningful component names that describe their purpose
3. Position nodes in a left-to-right flow layout (x: 0-1000, y: 0-600)
4. Connect components using their input/output ports
5. Ensure all connections reference valid node IDs and port names
6. Use connection IDs in format: conn_<source>_<target>_<port>
7. Configure components with sensible default values
8. Consider data flow patterns: sources -> processors -> sinks`;
}

function buildSearchInstructions(): string {
  return (
    `When searching the knowledge graph, use the graph_search tool. ` +
    `Entity IDs use 6-part dotted notation: org.platform.domain.system.type.instance. ` +
    `For natural language queries, use mode "nlq". For ID-based lookups, use mode "prefix".`
  );
}

function buildDebugInstructions(): string {
  return (
    `When debugging, check component health first using the component_health tool, ` +
    `then examine flow status. Report issues with specific component names and error messages.`
  );
}
```

### 5.4 Tool Execution in the Server Handler

The server handler routes tool_use events to actual backend calls:

```typescript
// In +server.ts POST handler, replace the hardcoded create_flow handling:

const toolExecutors: Record<
  string,
  (input: Record<string, unknown>) => Promise<MessageAttachment>
> = {
  create_flow: async (input) => {
    const flow = input as Partial<Flow>;
    const validationResult = await mcpServer.validateFlow(tempId, flow);
    return { kind: "flow", flow, validationResult } as FlowAttachment;
  },

  graph_search: async (input) => {
    const { query, mode, limit } = input as {
      query: string;
      mode?: string;
      limit?: number;
    };
    if (mode === "prefix") {
      const data = await graphqlQuery(
        graphqlEndpoint,
        ENTITIES_BY_PREFIX_QUERY,
        { prefix: query, limit: limit ?? 20 },
      );
      return {
        kind: "search-result",
        query,
        entityIds: data.entitiesByPrefix.map((e: { id: string }) => e.id),
        count: data.entitiesByPrefix.length,
        durationMs: 0,
      };
    }
    const data = await graphqlQuery(graphqlEndpoint, GLOBAL_SEARCH_QUERY, {
      query,
      limit: limit ?? 20,
    });
    return {
      kind: "search-result",
      query,
      entityIds: data.globalSearch.entities.map((e: { id: string }) => e.id),
      count: data.globalSearch.count,
      durationMs: data.globalSearch.duration_ms,
      communitySummaries: data.globalSearch.community_summaries,
    };
  },

  entity_lookup: async (input) => {
    const { entityId } = input as { entityId: string };
    const data = await graphqlQuery(graphqlEndpoint, ENTITY_QUERY, {
      id: entityId,
    });
    return {
      kind: "entity-detail",
      entityId,
      summary: `Entity ${entityId} has ${data.entity.triples.length} properties`,
      propertyCount: data.entity.triples.length,
      relationshipCount: 0,
    };
  },

  component_health: async (_input) => {
    // Phase 6: wire to runtime WebSocket store or backend health endpoint
    return {
      kind: "error",
      code: "NOT_IMPLEMENTED",
      message: "Health tool coming in Phase 6",
    };
  },

  flow_status: async (_input) => {
    // Phase 6: fetch from backend
    return {
      kind: "error",
      code: "NOT_IMPLEMENTED",
      message: "Status tool coming in Phase 6",
    };
  },
};

// In the streaming loop:
for await (const event of provider.streamChat(
  messages,
  systemPrompt,
  tools,
  signal,
)) {
  if (event.type === "text") {
    chunks.push(buildEvent("text", { content: event.content }));
  } else if (event.type === "tool_use") {
    const executor = toolExecutors[event.name];
    if (executor) {
      const attachment = await executor(event.input);
      chunks.push(buildEvent("attachment", attachment));
    }
  } else if (event.type === "done") {
    chunks.push(buildEvent("done", {}));
  }
}
```

### 5.5 Server-Side GraphQL Client

The server endpoint needs to call GraphQL directly (not through the browser
client). Add a lightweight server-side GraphQL helper:

```typescript
// src/lib/server/graphql/client.ts

export async function graphqlQuery<T>(
  endpoint: string,
  query: string,
  variables: Record<string, unknown>,
): Promise<T> {
  const response = await fetch(endpoint, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ query, variables }),
  });
  if (!response.ok) throw new Error(`GraphQL ${response.status}`);
  const { data, errors } = await response.json();
  if (errors?.length) throw new Error(errors[0].message);
  return data;
}
```

The GraphQL endpoint URL comes from an environment variable (`GRAPHQL_HOST`),
matching the existing Caddy/Docker Compose setup. The server handler resolves
it as:

```typescript
const graphqlEndpoint = `http://${env.GRAPHQL_HOST || "localhost:8080"}/graphql`;
```

---

## 6. Slash Command System

### 6.1 Command Registry

```typescript
// src/lib/services/slashCommands.ts

import type {
  SlashCommand,
  SlashCommandMatch,
  PageKind,
} from "$lib/types/slashCommand";

const COMMANDS: SlashCommand[] = [
  {
    name: "search",
    aliases: ["s", "find"],
    description: "Search the knowledge graph",
    usage: "/search <query>",
    intent: "search",
    availableOn: ["flow-builder", "data-view"],
    parse: (args) => ({
      intent: "search",
      content: args,
      params: { query: args },
    }),
  },
  {
    name: "flow",
    aliases: ["f", "create"],
    description: "Create or modify a flow",
    usage: "/flow <description>",
    intent: "flow-create",
    availableOn: ["flow-builder"],
    parse: (args) => ({
      intent: "flow-create",
      content: args,
      params: {},
    }),
  },
  {
    name: "explain",
    aliases: ["e", "what"],
    description: "Explain an entity or component",
    usage: "/explain [entity-id or name]",
    intent: "explain",
    availableOn: ["flow-builder", "data-view"],
    parse: (args) => ({
      intent: "explain",
      content: args || "Explain the selected item",
      params: { target: args },
    }),
  },
  {
    name: "debug",
    aliases: ["d"],
    description: "Diagnose runtime issues",
    usage: "/debug [component-name]",
    intent: "debug",
    availableOn: ["flow-builder"],
    parse: (args) => ({
      intent: "debug",
      content: args || "Check for runtime issues",
      params: { component: args },
    }),
  },
  {
    name: "health",
    aliases: ["h", "status"],
    description: "Show system health summary",
    usage: "/health",
    intent: "health",
    availableOn: ["flow-builder", "data-view"],
    parse: (args) => ({
      intent: "health",
      content: "Show system health",
      params: {},
    }),
  },
];

/**
 * Parse user input for slash commands.
 * Returns null if input is not a slash command.
 */
export function parseSlashCommand(
  input: string,
  page: PageKind,
): SlashCommandMatch | null {
  const trimmed = input.trim();
  if (!trimmed.startsWith("/")) return null;

  const spaceIdx = trimmed.indexOf(" ");
  const cmdName = (
    spaceIdx === -1 ? trimmed.slice(1) : trimmed.slice(1, spaceIdx)
  ).toLowerCase();
  const args = spaceIdx === -1 ? "" : trimmed.slice(spaceIdx + 1).trim();

  const command = COMMANDS.find(
    (c) =>
      (c.name === cmdName || c.aliases.includes(cmdName)) &&
      c.availableOn.includes(page),
  );

  if (!command) return null;

  return {
    command,
    result: command.parse(args),
  };
}

/**
 * Get commands available on a given page.
 * Used for autocomplete in ChatInput.
 */
export function getCommandsForPage(page: PageKind): SlashCommand[] {
  return COMMANDS.filter((c) => c.availableOn.includes(page));
}
```

### 6.2 Integration with ChatInput

```typescript
// In the page's handleChatSubmit:

function handleChatSubmit(content: string) {
  const match = parseSlashCommand(content, currentPage);
  const intent = match?.result.intent ?? "general";

  chatStore.addUserMessage(content); // Show original input including slash
  chatStore.setStreaming(true);

  streamChat(
    {
      messages: chatStore.messages
        .filter((m) => m.role !== "system")
        .map((m) => ({
          role: m.role as "user" | "assistant",
          content: m.content,
        })),
      context: buildPageContext(),
      chips: chatStore.chips,
      intent,
    },
    { onText, onDone, onError },
    signal,
  );
}
```

---

## 7. Context Chip System

### 7.1 How Chips Get Created

Chips are created by user actions in the UI, not by the chat system itself.

| Source                    | Action                                         | Chip Created                                                                       |
| ------------------------- | ---------------------------------------------- | ---------------------------------------------------------------------------------- |
| DataView: entity select   | User clicks entity in graph                    | `{ kind: "entity", label: "drone-001", value: "c360.ops.robotics.gcs.drone.001" }` |
| DataView: entity detail   | User clicks "+" on entity in detail panel      | Same as above                                                                      |
| FlowBuilder: node select  | User clicks "+" on node in properties panel    | `{ kind: "component", label: "HTTP Input", value: "node-123" }`                    |
| Search result             | User clicks entity in search result attachment | `{ kind: "entity", ... }`                                                          |
| Message: entity reference | User clicks an entity ID in assistant message  | `{ kind: "entity", ... }`                                                          |

### 7.2 How Chips Are Passed Through

```
User adds chip via UI action
  --> chatStore.addChip(chip)
  --> chips render in ContextChipBar above ChatInput
  --> On next message, chips[] included in ChatRequest
  --> Server includes chips in system prompt context section
  --> Chips snapshot saved on the ChatMessage for history
```

### 7.3 Creating Chips from Page Components

Each page is responsible for calling `chatStore.addChip()` when the user
performs a chip-worthy action.

**DataView integration:**

```typescript
// In DataView.svelte or a wrapper:

function handleEntitySelect(entityId: string | null) {
  graphStore.selectEntity(entityId);

  // Auto-add selected entity as a chip (user can remove it)
  if (entityId) {
    const entity = graphStore.entities.get(entityId);
    if (entity) {
      chatStore.addChip({
        id: crypto.randomUUID(),
        kind: "entity",
        label: entity.idParts.instance || entityId,
        value: entityId,
        metadata: {
          type: entity.idParts.type,
          domain: entity.idParts.domain,
        },
      });
    }
  }
}
```

**FlowBuilder integration (explicit, not auto):**

```typescript
// In PropertiesPanel.svelte, add a button:

function handleAddToChat() {
  if (!node) return;
  chatStore.addChip({
    id: crypto.randomUUID(),
    kind: "component",
    label: node.name,
    value: node.id,
    metadata: { component: node.component, type: node.type },
  });
}
```

### 7.4 Chip Display in Detail Panels

Add a small "Add to chat" button in GraphDetailPanel and PropertiesPanel:

```svelte
<!-- In GraphDetailPanel.svelte -->
<button
  class="add-to-chat"
  aria-label="Add {entity.id} to chat context"
  onclick={() => onAddChip?.({
    id: crypto.randomUUID(),
    kind: "entity",
    label: getEntityLabel(entity),
    value: entity.id,
  })}
>
  + Chat
</button>
```

This is the explicit "+" pattern from semdragon. Users click to add, click "x"
on the chip to remove. No auto-adding on the flow builder page. On the data
view page, entity selection auto-adds because the user clearly intends to focus
on that entity (and can remove the chip if they do not want it in context).

### 7.5 Chip Limits

Cap at 10 chips maximum. When the limit is hit, show a toast/message and
refuse the add. The user must remove an existing chip first. This prevents
context window bloat.

---

## 8. Page Integration Patterns

### 8.1 Flow Builder Page (`/flows/[id]/+page.svelte`)

The flow builder page provides `FlowBuilderContext` and wires up flow-specific
callbacks.

```svelte
<script lang="ts">
  import { chatStore } from "$lib/stores/chatStore.svelte";
  import { streamChat } from "$lib/services/chatApi";
  import { parseSlashCommand, getCommandsForPage } from "$lib/services/slashCommands";
  import type { FlowBuilderContext, MessageAttachment, FlowAttachment } from "$lib/types/chat";

  // Build page context for every chat request
  function buildPageContext(): FlowBuilderContext {
    return {
      page: "flow-builder",
      flowId: backendFlow.id,
      flowName: backendFlow.name,
      nodes: flowNodes,
      connections: flowConnections,
    };
  }

  const availableCommands = $derived(getCommandsForPage("flow-builder"));

  function handleChatSubmit(content: string) {
    const match = parseSlashCommand(content, "flow-builder");
    chatStore.setError(null);
    chatStore.addUserMessage(content);
    chatStore.setStreaming(true);

    chatAbortController = new AbortController();

    streamChat(
      {
        messages: chatStore.messages
          .filter(m => m.role !== "system")
          .map(m => ({ role: m.role as "user" | "assistant", content: m.content })),
        context: buildPageContext(),
        chips: chatStore.chips,
        intent: match?.result.intent ?? "general",
      },
      {
        onText: (chunk) => chatStore.appendStreamContent(chunk),
        onDone: ({ attachments }) => {
          chatStore.finalizeStream(chatStore.streamingContent, attachments);
          chatAbortController = null;
        },
        onError: (err) => {
          chatStore.setStreaming(false);
          chatStore.setError(err);
          chatAbortController = null;
        },
      },
      chatAbortController.signal,
    );
  }

  function handleApplyFlow(messageId: string) {
    const msg = chatStore.messages.find(m => m.id === messageId);
    const flowAtt = msg?.attachments?.find(a => a.kind === "flow") as FlowAttachment | undefined;
    if (!flowAtt?.flow) return;

    flowHistory.push({ ...backendFlow, nodes: flowNodes, connections: flowConnections });

    if (flowAtt.flow.nodes) flowNodes = [...flowAtt.flow.nodes];
    if (flowAtt.flow.connections) flowConnections = [...flowAtt.flow.connections];

    dirty = true;
    saveState = { ...saveState, status: "dirty" };
    chatStore.updateAttachment(messageId, "flow", { applied: true });
  }
</script>

<!-- In the left panel snippet: -->
<ChatPanel
  messages={chatStore.messages}
  isStreaming={chatStore.isStreaming}
  streamingContent={chatStore.streamingContent}
  error={chatStore.error}
  chips={chatStore.chips}
  commands={availableCommands}
  placeholder="Ask about this flow or type / for commands..."
  emptyStateMessage="I can help you build flows, debug components, or search your knowledge graph."
  onSubmit={handleChatSubmit}
  onCancel={handleChatCancel}
  onApplyFlow={handleApplyFlow}
  onRemoveChip={(id) => chatStore.removeChip(id)}
  onLoadJson={handleLoadJson}
  onExportJson={handleExportJson}
  onNewChat={handleNewChat}
/>
```

### 8.2 Data View Chat Integration

DataView currently lives inside the flow page behind a view switcher. Chat
needs to be available there too.

**Phase 1 approach: shared chat, context switches based on view mode.**

The flow page already has the ChatPanel in the left panel. When the user
switches to data view, the ChatPanel stays visible but the context changes.
This requires modifying the data view layout to include a chat sidebar:

```svelte
<!-- In +page.svelte, replace the simple DataView with a layout that includes chat -->
{#if panelLayout.state.viewMode === "data" && isFlowRunning}
  <div class="data-view-with-chat">
    <aside class="data-chat-panel">
      <ChatPanel
        messages={chatStore.messages}
        isStreaming={chatStore.isStreaming}
        streamingContent={chatStore.streamingContent}
        error={chatStore.error}
        chips={chatStore.chips}
        commands={getCommandsForPage("data-view")}
        placeholder="Search the graph or ask a question..."
        emptyStateMessage="I can search the knowledge graph, explain entities, and explore connections."
        onSubmit={handleDataViewChatSubmit}
        onCancel={handleChatCancel}
        onViewEntity={handleViewEntityFromChat}
        onRemoveChip={(id) => chatStore.removeChip(id)}
        onNewChat={handleNewChat}
      />
    </aside>
    <DataView flowId={backendFlow.id} />
  </div>
{:else}
  <!-- Existing ThreePanelLayout with ChatPanel in left panel -->
{/if}
```

The `buildPageContext` function switches based on view mode:

```typescript
function buildPageContext(): ChatPageContext {
  if (panelLayout.state.viewMode === "data") {
    return {
      page: "data-view",
      flowId: backendFlow.id,
      entityCount: graphStore.entities.size,
      selectedEntityId: graphStore.selectedEntityId,
      filters: graphStore.filters,
    };
  }
  return {
    page: "flow-builder",
    flowId: backendFlow.id,
    flowName: backendFlow.name,
    nodes: flowNodes,
    connections: flowConnections,
  };
}
```

**Phase 2+: standalone DataView route.**

If DataView becomes its own route (e.g., `/flows/[id]/data`), it would have
its own ChatPanel instance. The `chatStore` is a singleton so conversation
persists across view changes within the same page session. For separate routes,
clear the conversation on route change.

---

## 9. Migration Path

### Phase 1: Generalize Types and Store (no UI changes)

**Goal:** Remove flow-only assumptions from types, store, and API client.
Existing flow functionality must continue to work identically.

1. Update `ChatMessage` type: replace `flow?/flowDiff?/applied?` with
   `attachments?: MessageAttachment[]`
2. Update `ChatRequest` type: replace `currentFlow` with `context: ChatPageContext`,
   add `chips: ContextChip[]` and `intent?: ChatIntent`
3. Update `chatStore`: replace `markFlowApplied` with `updateAttachment`,
   add chip management methods, update `finalizeStream` signature
4. Update `chatApi.streamChat`: accept new `ChatRequest`, handle `attachment`
   SSE events, return `attachments[]` in `onDone`
5. Update `+server.ts`: accept new request format, backward-compatible
   (if `context` is missing, treat `currentFlow` as `FlowBuilderContext`)
6. Update page integration in `+page.svelte`: build `FlowBuilderContext`,
   adapt `handleApplyFlow` to use attachments

**Tests:**

- All existing chat tests must pass (update assertions for new shapes)
- New tests for attachment-based message rendering
- New tests for backward-compatible request handling

**Files touched:**

- `src/lib/types/chat.ts`
- `src/lib/stores/chatStore.svelte.ts`
- `src/lib/services/chatApi.ts`
- `src/routes/api/ai/chat/+server.ts`
- `src/routes/flows/[id]/+page.svelte`
- `src/lib/components/chat/ChatMessage.svelte`
- `src/lib/components/chat/ChatMessageList.svelte`
- All chat test files (assertion updates)

### Phase 2: Slash Commands and Tool Registry

**Goal:** Add slash command parsing and server-side tool selection.

1. Create `src/lib/types/slashCommand.ts`
2. Create `src/lib/services/slashCommands.ts` (registry + parser)
3. Create `src/lib/server/ai/toolRegistry.ts`
4. Create `src/lib/server/ai/systemPrompt.ts` (extract from `+server.ts`)
5. Update `+server.ts` to use tool registry and composable system prompt
6. Update ChatInput: add `commands` prop, show SlashCommandMenu on `/`
7. Create `SlashCommandMenu.svelte`

**Tests:**

- Unit tests for slash command parsing (every command + aliases + edge cases)
- Unit tests for tool registry (correct tools per intent/page combination)
- Unit tests for system prompt builder
- Component tests for SlashCommandMenu

**Files created:**

- `src/lib/types/slashCommand.ts`
- `src/lib/services/slashCommands.ts`
- `src/lib/server/ai/toolRegistry.ts`
- `src/lib/server/ai/systemPrompt.ts`
- `src/lib/server/graphql/client.ts`
- `src/lib/components/chat/SlashCommandMenu.svelte`

### Phase 3: Context Chips

**Goal:** Users can pin entities and components into chat context.

1. Create `ContextChipPill.svelte` and `ContextChipBar.svelte`
2. Update ChatInput to render chips above textarea
3. Update ChatPanel to pass chips through
4. Wire GraphDetailPanel "+" button to create chips
5. Wire PropertiesPanel "+" button to create chips
6. Wire DataView entity selection to auto-chip
7. Wire FlowBuilder node click to chip (explicit button, not auto)

**Tests:**

- Component tests for ContextChipPill, ContextChipBar
- Integration tests for chip creation from detail panels
- Integration tests for chip snapshot in messages
- Store tests for chip add/remove/limit/dedup

**Files created:**

- `src/lib/components/chat/ContextChipPill.svelte`
- `src/lib/components/chat/ContextChipBar.svelte`

**Files modified:**

- `src/lib/components/chat/ChatInput.svelte`
- `src/lib/components/chat/ChatPanel.svelte`
- `src/lib/components/runtime/GraphDetailPanel.svelte`
- `src/lib/components/PropertiesPanel.svelte`
- `src/lib/components/DataView.svelte`

### Phase 4: Search and Graph Tools (Server-Side)

**Goal:** AI can search the graph and look up entities via tools.

1. Create server-side GraphQL client (`src/lib/server/graphql/client.ts`)
2. Implement `graph_search` tool executor (calls globalSearch or entitiesByPrefix)
3. Implement `entity_lookup` tool executor (calls entity query)
4. Create `SearchResultSummary.svelte` attachment renderer
5. Create `EntityDetailCard.svelte` attachment renderer
6. Update ChatMessage to render search-result and entity-detail attachments
7. Wire "view entity" actions from search results to DataView/graphStore

**Tests:**

- Server-side unit tests for GraphQL client
- Server-side unit tests for tool executors (mock fetch)
- Component tests for SearchResultSummary, EntityDetailCard
- E2E test: `/search drones` returns results in chat

### Phase 5: Data View Chat Integration

**Goal:** Chat panel available and useful on the data view.

1. Add ChatPanel to DataView layout (sidebar)
2. Wire DataView entity selection to chip creation
3. Context-switch between flow-builder and data-view page contexts
4. Add graph-specific empty state and placeholder
5. Wire "view entity" from chat to graphStore.selectEntity

### Phase 6: Debug and Health Tools

**Goal:** AI can diagnose runtime issues using health and metrics data.

1. Implement `component_health` tool executor (fetch from backend or runtimeStore)
2. Implement `flow_status` tool executor (fetch flow runtime state)
3. Wire to runtimeStore data or backend health endpoints
4. Add runtime-specific system prompt sections
5. Create attachment renderers for health/status data if needed

---

## Non-Goals (Out of Scope)

- **Multi-turn tool use**: The AI calls one tool per turn. Multi-step
  orchestration (call search, then lookup each result) is not in scope.
  The AI can do this across turns naturally via conversation.
- **Persistent chat history**: Conversations are session-scoped. No database
  storage of chat history.
- **Collaborative chat**: Single-user per session. No multi-user shared context.
- **Custom user-defined commands**: Slash commands are hardcoded in the registry.
  Extension point exists (`custom` chip kind) but no UI for defining new commands.
- **Inline entity rendering**: Parsing entity IDs out of assistant message text
  and rendering them as clickable links is desirable but deferred. Phase 4+
  search results give structured entity references instead.

---

## File Inventory

New files to create:

| File                                                 | Phase | Purpose                           |
| ---------------------------------------------------- | ----- | --------------------------------- |
| `src/lib/types/slashCommand.ts`                      | 2     | Slash command type definitions    |
| `src/lib/services/slashCommands.ts`                  | 2     | Command registry and parser       |
| `src/lib/server/ai/toolRegistry.ts`                  | 2     | Intent-based tool selection       |
| `src/lib/server/ai/systemPrompt.ts`                  | 2     | Composable system prompt builder  |
| `src/lib/server/graphql/client.ts`                   | 4     | Server-side GraphQL client        |
| `src/lib/components/chat/SlashCommandMenu.svelte`    | 2     | Autocomplete dropdown             |
| `src/lib/components/chat/ContextChipPill.svelte`     | 3     | Single chip pill                  |
| `src/lib/components/chat/ContextChipBar.svelte`      | 3     | Chip row container                |
| `src/lib/components/chat/SearchResultSummary.svelte` | 4     | Search result attachment renderer |
| `src/lib/components/chat/EntityDetailCard.svelte`    | 4     | Entity detail attachment renderer |

Existing files modified:

| File                                                 | Phase | Change                                         |
| ---------------------------------------------------- | ----- | ---------------------------------------------- |
| `src/lib/types/chat.ts`                              | 1     | Generalize types (attachments, context, chips) |
| `src/lib/stores/chatStore.svelte.ts`                 | 1     | Generic store with chip management             |
| `src/lib/services/chatApi.ts`                        | 1     | Generic request/response, attachment events    |
| `src/routes/api/ai/chat/+server.ts`                  | 1,2,4 | Context-aware, tool registry, executors        |
| `src/routes/flows/[id]/+page.svelte`                 | 1,3,5 | Page context builder, chip wiring              |
| `src/lib/components/chat/ChatInput.svelte`           | 2,3   | Slash autocomplete, chip display               |
| `src/lib/components/chat/ChatMessage.svelte`         | 1,4   | Attachment-based rendering                     |
| `src/lib/components/chat/ChatMessageList.svelte`     | 1     | Pass through new props                         |
| `src/lib/components/chat/ChatPanel.svelte`           | 1,2,3 | Generic props, commands, chips                 |
| `src/lib/components/chat/ChatToolbar.svelte`         | 1     | Context-aware toolbar actions                  |
| `src/lib/components/DataView.svelte`                 | 5     | Chat panel sidebar integration                 |
| `src/lib/components/runtime/GraphDetailPanel.svelte` | 3     | "Add to chat" button                           |
| `src/lib/components/PropertiesPanel.svelte`          | 3     | "Add to chat" button                           |

---

## Risks and Mitigations

| Risk                                          | Impact                             | Mitigation                                                                   |
| --------------------------------------------- | ---------------------------------- | ---------------------------------------------------------------------------- |
| System prompt bloat with full flow JSON       | Token cost, slow responses         | Truncate to node IDs + names when > 20 nodes                                 |
| Slash commands conflict with natural language | User types "/search" literally     | Only match registered command names; unmatched `/foo` sent as-is             |
| Context chips accumulate unbounded            | Large context window, confusing UI | Cap at 10 chips; refuse add at limit; clear button in chip bar               |
| Tool execution latency on GraphQL calls       | Slow streaming response            | Stream text first; send attachment event when tool completes                 |
| Breaking ChatMessage shape change             | 108 test failures                  | Phase 1 updates all tests atomically before new features                     |
| Server needs GraphQL access                   | New network dependency             | Use existing `GRAPHQL_HOST` env var; graceful fallback on connection failure |
