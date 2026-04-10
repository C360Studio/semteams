// TypeScript schema types matching backend Go types
// Must match: pkg/component/discovery.go PropertySchema and ConfigSchema

/**
 * PropertySchema defines the configuration schema for a single property/field.
 * Matches backend: pkg/component/discovery.go PropertySchema
 */
/**
 * Valid default values for PropertySchema fields based on type
 */
export type PropertyDefaultValue =
  | string
  | number
  | boolean
  | string[]
  | Record<string, unknown>
  | null
  | undefined;

export interface PropertySchema {
  /** Field data type */
  type:
    | "string"
    | "int"
    | "bool"
    | "float"
    | "enum"
    | "object"
    | "array"
    | "ports"
    | "cache";

  /** Human-readable description */
  description: string;

  /** Default value for the field */
  default?: PropertyDefaultValue;

  /** Valid enum values (for type: 'enum') */
  enum?: string[];

  /** Minimum value (for numeric types) */
  minimum?: number;

  /** Maximum value (for numeric types) */
  maximum?: number;

  /** Field category: 'basic' (shown first) or 'advanced' (collapsible section) */
  category?: "basic" | "advanced";

  /**
   * Port field metadata (for type: 'ports' only)
   * Describes which PortDefinition fields are editable vs read-only
   * Matches backend: pkg/component/discovery.go PropertySchema.PortFields
   */
  portFields?: Record<string, PortFieldInfo>;

  /**
   * Cache field metadata (for type: 'cache' only)
   * Describes which cache.Config fields are editable and their constraints
   * Matches backend: pkg/component/discovery.go PropertySchema.CacheFields
   */
  cacheFields?: Record<string, CacheFieldInfo>;
}

/**
 * PortFieldInfo describes metadata for a single PortDefinition field.
 * Used to determine UI rendering for port configuration forms.
 * Matches backend: pkg/component/schema_tags.go PortFieldInfo
 */
export interface PortFieldInfo {
  /** Field data type ('string', 'int', 'bool', etc.) */
  type: string;

  /** Whether users can modify this field in the UI */
  editable: boolean;
}

/**
 * CacheFieldInfo describes metadata for a single cache.Config field.
 * Used to determine UI rendering for cache configuration forms.
 * Matches backend: pkg/component/schema_tags.go CacheFieldInfo
 */
export interface CacheFieldInfo {
  /** Field data type ('string', 'int', 'bool', 'enum', etc.) */
  type: string;

  /** Whether users can modify this field in the UI */
  editable: boolean;

  /** Valid enum values (for strategy field) */
  enum?: string[];

  /** Minimum value (for numeric fields like max_size) */
  min?: number;
}

/**
 * ConfigSchema defines the complete configuration schema for a component type.
 * Matches backend: pkg/component/discovery.go ConfigSchema
 */
export interface ConfigSchema {
  /** Map of field name to PropertySchema */
  properties: Record<string, PropertySchema>;

  /** Array of required field names */
  required: string[];
}

/**
 * SchemaResponse is the API response structure for schema endpoints.
 * Matches backend: pkg/service/component_manager_http.go handleComponentTypeByID
 * Response structure (lines 413-422)
 */
export interface SchemaResponse {
  /** Component type identifier (e.g., 'udp-input', 'websocket', etc.) */
  id: string;

  /** Human-readable display name (e.g., 'UDP Input', 'WebSocket Output') */
  name: string;

  /** Component category: 'input', 'processor', 'output', 'storage' */
  type: string;

  /** Communication protocol (e.g., 'nats', 'http', etc.) */
  protocol: string;

  /** Human-readable component description */
  description: string;

  /** Component version (e.g., '1.0.0') */
  version: string;

  /** Category for frontend (same as type, for compatibility) */
  category: string;

  /** The configuration schema definition */
  schema: ConfigSchema;
}

/**
 * ValidationError represents a field-specific validation failure.
 * Matches backend: pkg/component/schema.go ValidationError
 */
export interface ValidationError {
  /** Field name that failed validation */
  field: string;

  /** Human-readable error message */
  message: string;

  /** Error code for programmatic handling */
  code: "required" | "min" | "max" | "pattern" | "enum" | "type";
}
