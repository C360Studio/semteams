/**
 * Flow fixture generators for E2E tests
 * Each function returns a fresh instance of a flow configuration
 */

/**
 * Flow configuration structure
 */
export interface FlowConfig {
  name: string;
  description?: string;
  nodes: Array<{
    id: string;
    type: string;
    name: string;
    config: Record<string, unknown>;
  }>;
  connections: Array<{
    id: string;
    source_node_id: string;
    source_port: string;
    target_node_id: string;
    target_port: string;
  }>;
}

/**
 * Create a minimal valid flow with a single UDP input component
 * This flow should pass backend validation
 *
 * @returns Fresh flow configuration instance
 */
export function createMinimalValidFlow(): FlowConfig {
  return {
    name: "E2E Test - Minimal Flow",
    description: "Minimal valid flow for testing",
    nodes: [
      {
        id: "minimal-udp-input-1",
        type: "udp-input",
        name: "UDP Input",
        config: {
          port: 5000,
          host: "0.0.0.0",
        },
      },
    ],
    connections: [],
  };
}

/**
 * Create a flow with all major component types
 * Includes input, processor, and output components
 *
 * @returns Fresh flow configuration instance
 */
export function createFlowWithAllComponentTypes(): FlowConfig {
  return {
    name: "E2E Test - All Component Types",
    description: "Flow demonstrating all component types",
    nodes: [
      {
        id: "all-types-udp-input-1",
        type: "udp-input",
        name: "UDP Input Source",
        config: {
          port: 5001,
          host: "0.0.0.0",
        },
      },
      {
        id: "all-types-robotics-processor-1",
        type: "robotics-processor",
        name: "Robotics Data Processor",
        config: {
          processing_mode: "realtime",
          buffer_size: 1024,
        },
      },
      {
        id: "all-types-log-output-1",
        type: "log-output",
        name: "Log Output Sink",
        config: {
          log_level: "info",
          format: "json",
        },
      },
    ],
    connections: [],
  };
}

/**
 * Create a complex flow with multiple nodes and connections
 * Represents a realistic data pipeline with input -> processor(s) -> output
 *
 * @returns Fresh flow configuration instance
 */
export function createComplexFlow(): FlowConfig {
  return {
    name: "E2E Test - Complex Flow",
    description: "Complex flow with multiple components and connections",
    nodes: [
      {
        id: "complex-udp-input-1",
        type: "udp-input",
        name: "UDP Data Source",
        config: {
          port: 5002,
          host: "0.0.0.0",
        },
      },
      {
        id: "complex-robotics-processor-1",
        type: "robotics-processor",
        name: "First Processor",
        config: {
          processing_mode: "realtime",
          buffer_size: 2048,
        },
      },
      {
        id: "complex-robotics-processor-2",
        type: "robotics-processor",
        name: "Second Processor",
        config: {
          processing_mode: "batch",
          buffer_size: 4096,
        },
      },
      {
        id: "complex-udp-output-1",
        type: "udp-output",
        name: "UDP Output",
        config: {
          host: "localhost",
          port: 6000,
        },
      },
      {
        id: "complex-log-output-1",
        type: "log-output",
        name: "Logging Output",
        config: {
          log_level: "debug",
          format: "json",
        },
      },
    ],
    connections: [
      {
        id: "complex-conn-1",
        source_node_id: "complex-udp-input-1",
        source_port: "out",
        target_node_id: "complex-robotics-processor-1",
        target_port: "in",
      },
      {
        id: "complex-conn-2",
        source_node_id: "complex-robotics-processor-1",
        source_port: "out",
        target_node_id: "complex-robotics-processor-2",
        target_port: "in",
      },
      {
        id: "complex-conn-3",
        source_node_id: "complex-robotics-processor-2",
        source_port: "out",
        target_node_id: "complex-udp-output-1",
        target_port: "in",
      },
      {
        id: "complex-conn-4",
        source_node_id: "complex-robotics-processor-2",
        source_port: "out",
        target_node_id: "complex-log-output-1",
        target_port: "in",
      },
    ],
  };
}

/**
 * Create a flow that will fail backend validation
 * Components have empty configs that should trigger validation errors
 *
 * @returns Fresh flow configuration instance with validation errors
 */
export function createInvalidFlow(): FlowConfig {
  return {
    name: "E2E Test - Invalid Flow for Validation",
    description: "Flow with invalid configuration to test error handling",
    nodes: [
      {
        id: "invalid-udp-input-1",
        type: "udp-input",
        name: "Unconfigured UDP Input",
        config: {}, // Empty config - will fail validation
      },
      {
        id: "invalid-robotics-processor-1",
        type: "robotics-processor",
        name: "Unconfigured Processor",
        config: {}, // Empty config - will fail validation
      },
      {
        id: "invalid-log-output-1",
        type: "log-output",
        name: "Unconfigured Log Output",
        config: {}, // Empty config - will fail validation
      },
    ],
    connections: [],
  };
}
