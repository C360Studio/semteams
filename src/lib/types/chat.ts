import type { Flow, FlowNode, FlowConnection } from "./flow";

export interface ChatMessage {
  id: string;
  role: "user" | "assistant" | "system";
  content: string;
  timestamp: Date;
  flow?: Partial<Flow>;
  flowDiff?: FlowDiff;
  applied?: boolean;
}

export interface FlowDiff {
  nodesAdded: string[];
  nodesRemoved: string[];
  nodesModified: string[];
  connectionsAdded: number;
  connectionsRemoved: number;
}

export interface ChatRequest {
  messages: Array<{
    role: "user" | "assistant";
    content: string;
  }>;
  currentFlow: {
    nodes: FlowNode[];
    connections: FlowConnection[];
  };
}
