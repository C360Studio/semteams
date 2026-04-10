// Type definitions for Component metadata
// Matches backend component registry schema

export interface ComponentType {
  id: string;
  name: string;
  type: string; // "input", "processor", "output", "storage"
  protocol: string; // "udp", "mavlink", "websocket", etc.
  category: string; // Same as type, for grouping
  domain?: string; // Domain classification (e.g., "network", "graph", "storage")
  description: string;
  version: string;
  ports?: PortDefinition[]; // Optional, for future use
  schema?: ConfigSchema; // Component configuration schema from backend
  icon?: string;
}

export interface PortDefinition {
  id: string;
  name: string;
  direction: "input" | "output" | "bidirectional";
  required: boolean;
  description: string;
  config: PortConfig;
}

export interface PortConfig {
  type:
    | "nats"
    | "nats-request"
    | "jetstream"
    | "kvwatch"
    | "kvwrite"
    | "network"
    | "file";

  // Only one of these will be populated based on type
  nats?: NATSPortConfig;
  natsRequest?: NATSRequestPortConfig;
  jetstream?: JetStreamPortConfig;
  kvwatch?: KVWatchPortConfig;
  kvwrite?: KVWritePortConfig;
  network?: NetworkPortConfig;
  file?: FilePortConfig;
}

export interface NATSPortConfig {
  subject: string;
  queue?: string;
  interface?: InterfaceContract;
}

export interface NATSRequestPortConfig {
  subject: string;
  timeout?: string;
  retries?: number;
  interface?: InterfaceContract;
}

export interface JetStreamPortConfig {
  streamName: string;
  subjects: string[];
  consumerName?: string;
  interface?: InterfaceContract;
}

export interface KVWatchPortConfig {
  bucket: string;
  keys?: string[];
  interface?: InterfaceContract;
}

export interface KVWritePortConfig {
  bucket: string;
  interface?: InterfaceContract;
}

export interface NetworkPortConfig {
  protocol: "tcp" | "udp";
  host: string;
  port: number;
}

export interface FilePortConfig {
  path: string;
  pattern?: string;
}

export interface InterfaceContract {
  type: string;
  version?: string;
  compatible?: string[];
}

export interface ConfigSchema {
  type: "object";
  properties: Record<string, PropertySchema>;
  required: string[];
}

export interface PropertySchema {
  // Backend uses Go-style type names: int, bool, integer
  // Also supports: string, number, boolean, array, object, enum, ports
  type: string;
  description?: string;
  default?: unknown;
  minimum?: number;
  maximum?: number;
  enum?: string[];
  category?: string; // Field category (e.g., "basic", "advanced")
  portFields?: Record<string, PortFieldSchema>; // For ports type
}

export interface PortFieldSchema {
  type: string;
  editable: boolean;
}
