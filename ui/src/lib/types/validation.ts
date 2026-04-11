/**
 * Validation Type Definitions
 *
 * Matches backend schema from pkg/flowengine/validator.go
 * These types represent the structured validation errors returned by FlowGraph validation
 */

/**
 * Validation status indicating overall validation result
 */
export type ValidationStatus = "valid" | "warnings" | "errors";

/**
 * Severity level of a validation issue
 */
export type ValidationSeverity = "error" | "warning";

/**
 * Type of validation issue detected
 */
export type ValidationIssueType =
  | "orphaned_port" // Port with no connections
  | "disconnected_node" // Component not connected to flow
  | "unknown_component" // Component type not in registry
  | "cycle_detected" // Circular dependency detected
  | "missing_config" // Required configuration missing
  | "graph_build_error"; // Error building flow graph

/**
 * ValidationIssue represents a single validation problem
 */
export interface ValidationIssue {
  /** Type of validation issue */
  type: ValidationIssueType;

  /** Severity level (error blocks deployment, warning does not) */
  severity: ValidationSeverity;

  /** Name of the component with the issue */
  component_name: string;

  /** Name of the port with the issue (if applicable) */
  port_name?: string;

  /** Human-readable description of the issue */
  message: string;

  /** Actionable suggestions for fixing the issue */
  suggestions?: string[];
}

/**
 * ValidationResult contains the complete validation results
 */
export interface ValidationResult {
  /** Overall validation status */
  validation_status: ValidationStatus;

  /** List of validation errors (block deployment) */
  errors: ValidationIssue[];

  /** List of validation warnings (do not block deployment) */
  warnings: ValidationIssue[];
}
