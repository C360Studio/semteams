import type { Flow, FlowNode, FlowConnection } from "./flow";
import type { GraphFilters } from "./graph";
import type { AgentLoopState } from "./agent";

// ---------------------------------------------------------------------------
// FlowDiff — shared between FlowAttachment and legacy usage
// ---------------------------------------------------------------------------

export interface FlowDiff {
  nodesAdded: string[];
  nodesRemoved: string[];
  nodesModified: string[];
  connectionsAdded: number;
  connectionsRemoved: number;
}

// ---------------------------------------------------------------------------
// MessageAttachment — discriminated union
// Replaces the old top-level flow?/flowDiff?/applied? fields on ChatMessage.
// ---------------------------------------------------------------------------

export type MessageAttachment =
  | FlowAttachment
  | SearchResultAttachment
  | EntityDetailAttachment
  | ErrorAttachment
  | HealthAttachment
  | FlowStatusAttachment
  | AgentLoopAttachment
  | ApprovalAttachment
  | ToolCallAttachment
  | RuleDiffAttachment;

export interface FlowAttachment {
  kind: "flow";
  flow: Partial<Flow>;
  diff?: FlowDiff;
  applied?: boolean;
  validationResult?: unknown;
}

export interface SearchResultEntity {
  id: string;
  label: string;
  type: string;
  domain: string;
  score?: number;
}

export interface SearchResultAttachment {
  kind: "search-result";
  query: string;

  // Phase 4 shape
  results?: SearchResultEntity[];
  totalCount?: number;

  // Phase 1 shape (kept for backward compatibility with existing tests/server code)
  entityIds?: string[];
  count?: number;
  durationMs?: number;
  communitySummaries?: Array<{ communityId: string; text: string }>;
}

export interface EntityProperty {
  predicate: string;
  value: unknown;
}

export interface EntityRelationship {
  predicate: string;
  targetId: string;
}

export interface EntityDetail {
  id: string;
  label: string;
  type: string;
  domain: string;
  properties: EntityProperty[];
  relationships: EntityRelationship[];
}

export interface EntityDetailAttachment {
  kind: "entity-detail";

  // Phase 4 shape
  entity?: EntityDetail;

  // Phase 1 shape (kept for backward compatibility with existing tests/server code)
  entityId?: string;
  summary?: string;
  propertyCount?: number;
  relationshipCount?: number;
}

export interface ErrorAttachment {
  kind: "error";
  code: string;
  message: string;
}

export interface HealthAttachment {
  kind: "health";
  componentName: string;
  status: "healthy" | "degraded" | "unhealthy" | "unknown";
  message?: string;
  metrics?: Record<string, number>;
  lastCheck?: string;
}

export interface FlowStatusAttachment {
  kind: "flow-status";
  flowId: string;
  flowName: string;
  state: string;
  nodeCount: number;
  connectionCount: number;
  warnings?: string[];
}

export interface AgentLoopAttachment {
  kind: "agent-loop";
  loopId: string;
  state: AgentLoopState;
  role: string;
  iterations: number;
  maxIterations: number;
  parentLoopId?: string;
}

export interface ApprovalAttachment {
  kind: "approval";
  loopId: string;
  toolName: string;
  toolArgs: Record<string, unknown>;
  question: string;
  resolved?: boolean;
  resolution?: "approved" | "rejected";
}

export interface ToolCallAttachment {
  kind: "tool-call";
  toolName: string;
  args: Record<string, unknown>;
  result?: string;
  error?: string;
  status: "pending" | "running" | "complete" | "error";
  durationMs?: number;
}

export interface RuleDiffAttachment {
  kind: "rule-diff";
  ruleId: string;
  ruleName: string;
  operation: "create" | "update" | "delete";
  before?: Record<string, unknown>;
  after?: Record<string, unknown>;
}

// ---------------------------------------------------------------------------
// ContextChip — domain object pinned into the conversation
// ---------------------------------------------------------------------------

export type ContextChipKind =
  | "entity"
  | "component"
  | "flow"
  | "community"
  | "custom";

export interface ContextChip {
  id: string;
  kind: ContextChipKind;
  label: string;
  value: string;
  metadata?: Record<string, unknown>;
}

// ---------------------------------------------------------------------------
// ChatPageContext — page-level context (discriminated union)
// Replaces the old currentFlow field in ChatRequest.
// ---------------------------------------------------------------------------

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
  // Partial allows callers to pass {} for testing or provide only the filters they care about
  filters: Partial<GraphFilters>;
}

// ---------------------------------------------------------------------------
// ChatIntent — maps to server-side tool selection
// ---------------------------------------------------------------------------

export type ChatIntent =
  | "general"
  | "flow-create"
  | "flow-modify"
  | "search"
  | "explain"
  | "debug"
  | "health"
  | "agent-control";

// ---------------------------------------------------------------------------
// ChatMessage — generalized shape with attachments and chips
// ---------------------------------------------------------------------------

export interface ChatMessage {
  id: string;
  role: "user" | "assistant" | "system";
  content: string;
  timestamp: Date;

  // Attachments: zero or more typed payloads on assistant messages.
  // Replaces the old flow?/flowDiff?/applied? fields.
  attachments?: MessageAttachment[];

  // Context chips snapshotted at message send time.
  chips?: ContextChip[];

  // Index signature allows runtime field access (e.g., testing old-field absence).
  [key: string]: unknown;
}

// ---------------------------------------------------------------------------
// ChatRequest — generalized request shape
// ---------------------------------------------------------------------------

export interface ChatRequest {
  messages: Array<{
    role: "user" | "assistant";
    content: string;
  }>;
  context: ChatPageContext;
  chips: ContextChip[];
  intent?: ChatIntent;

  // Index signature allows runtime field access (e.g., testing old-field absence).
  [key: string]: unknown;
}

// ---------------------------------------------------------------------------
// ChatStreamCallbacks — updated onDone signature
// ---------------------------------------------------------------------------

export interface ChatStreamCallbacks {
  onText: (content: string) => void;
  onDone: (data: { attachments: MessageAttachment[] }) => void;
  onError: (error: string) => void;
}
