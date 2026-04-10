// Client-side validation matching backend schema validation
// Must match: pkg/component/schema.go ValidateConfig

import type { PropertySchema, ValidationError } from "$lib/types/schema";

/**
 * Validates a single field value against its schema definition.
 * Returns ValidationError if validation fails, null if valid.
 *
 * @param fieldName - The name of the field being validated
 * @param value - The value to validate
 * @param schema - The PropertySchema definition for this field
 * @param isRequired - Whether this field is required (from ConfigSchema.required array)
 * @returns ValidationError if invalid, null if valid
 */
export function validateField(
  fieldName: string,
  value: unknown,
  schema: PropertySchema,
  isRequired: boolean,
): ValidationError | null {
  // T077: Required validation
  if (isRequired && (value === undefined || value === null || value === "")) {
    return {
      field: fieldName,
      message: "This field is required",
      code: "required",
    };
  }

  // Skip further validation if value is empty and not required
  if (value === undefined || value === null || value === "") {
    return null;
  }

  // T078: Min/max validation for numeric types
  if (schema.type === "int" || schema.type === "float") {
    const numValue =
      typeof value === "string" ? parseFloat(value) : Number(value);

    if (isNaN(numValue)) {
      return {
        field: fieldName,
        message: "Must be a valid number",
        code: "type",
      };
    }

    if (schema.minimum !== undefined && numValue < schema.minimum) {
      return {
        field: fieldName,
        message: `Must be >= ${schema.minimum}`,
        code: "min",
      };
    }

    if (schema.maximum !== undefined && numValue > schema.maximum) {
      return {
        field: fieldName,
        message: `Must be <= ${schema.maximum}`,
        code: "max",
      };
    }
  }

  // T079: Enum validation
  if (schema.type === "enum" && schema.enum) {
    const strValue = String(value);
    if (!schema.enum.includes(strValue)) {
      return {
        field: fieldName,
        message: `Must be one of: ${schema.enum.join(", ")}`,
        code: "enum",
      };
    }
  }

  // Type validation for boolean
  if (schema.type === "bool") {
    if (typeof value !== "boolean" && value !== "true" && value !== "false") {
      return {
        field: fieldName,
        message: "Must be true or false",
        code: "type",
      };
    }
  }

  return null;
}

/**
 * Validates all fields in a configuration object against a schema.
 * Returns array of ValidationErrors (empty if all valid).
 *
 * @param config - The configuration object to validate
 * @param schema - The ConfigSchema definition
 * @returns Array of ValidationErrors (empty if valid)
 */
export function validateConfig(
  config: Record<string, unknown>,
  schema: { properties: Record<string, PropertySchema>; required: string[] },
): ValidationError[] {
  const errors: ValidationError[] = [];

  // Validate all properties
  for (const [fieldName, propSchema] of Object.entries(schema.properties)) {
    const isRequired = schema.required.includes(fieldName);
    const value = config[fieldName];
    const error = validateField(fieldName, value, propSchema, isRequired);

    if (error) {
      errors.push(error);
    }
  }

  return errors;
}
