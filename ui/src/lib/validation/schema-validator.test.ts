import { describe, it, expect } from "vitest";
import type { PropertySchema, ConfigSchema } from "$lib/types/schema";
import { validateField, validateConfig } from "./schema-validator";

describe("schema-validator", () => {
  // T112: Validation consistency with backend - comprehensive test
  describe("validation consistency with backend", () => {
    it("should produce same error codes as backend for all validation types", () => {
      const schema: ConfigSchema = {
        properties: {
          port: {
            type: "int",
            description: "Port number",
            minimum: 1,
            maximum: 65535,
          },
          protocol: {
            type: "enum",
            description: "Protocol type",
            enum: ["tcp", "udp"],
          },
          enabled: {
            type: "bool",
            description: "Enabled flag",
          },
        },
        required: ["port", "protocol"],
      };

      // Test 1: Required field missing → code: "required"
      const config1 = { protocol: "tcp" }; // port missing
      const errors1 = validateConfig(config1, schema);
      const portError = errors1.find((e) => e.field === "port");
      expect(portError).toBeDefined();
      expect(portError?.code).toBe("required");

      // Test 2: Value exceeds max → code: "max"
      const config2 = { port: 99999, protocol: "tcp" };
      const errors2 = validateConfig(config2, schema);
      const maxError = errors2.find((e) => e.field === "port");
      expect(maxError).toBeDefined();
      expect(maxError?.code).toBe("max");

      // Test 3: Value below min → code: "min"
      const config3 = { port: 0, protocol: "tcp" };
      const errors3 = validateConfig(config3, schema);
      const minError = errors3.find((e) => e.field === "port");
      expect(minError).toBeDefined();
      expect(minError?.code).toBe("min");

      // Test 4: Invalid enum value → code: "enum"
      const config4 = { port: 8080, protocol: "http" };
      const errors4 = validateConfig(config4, schema);
      const enumError = errors4.find((e) => e.field === "protocol");
      expect(enumError).toBeDefined();
      expect(enumError?.code).toBe("enum");

      // Test 5: Type mismatch (string for int) → code: "type"
      const config5 = { port: "not-a-number", protocol: "tcp" };
      const errors5 = validateConfig(config5, schema);
      const typeError = errors5.find((e) => e.field === "port");
      expect(typeError).toBeDefined();
      expect(typeError?.code).toBe("type");

      // Test 6: Type mismatch (number for bool) → code: "type"
      const config6 = { port: 8080, protocol: "tcp", enabled: 1 };
      const errors6 = validateConfig(config6, schema);
      const boolTypeError = errors6.find((e) => e.field === "enabled");
      expect(boolTypeError).toBeDefined();
      expect(boolTypeError?.code).toBe("type");
    });
  });

  // T033: Component test client-side validation - required fields
  describe("required field validation", () => {
    it("should return error for missing required field", () => {
      const schema: PropertySchema = {
        type: "string",
        description: "Port number",
      };

      const error = validateField("port", undefined, schema, true);

      expect(error).not.toBeNull();
      expect(error?.field).toBe("port");
      expect(error?.message).toContain("required");
      expect(error?.code).toBe("required");
    });

    it("should return error for empty string in required field", () => {
      const schema: PropertySchema = {
        type: "string",
        description: "Port number",
      };

      const error = validateField("port", "", schema, true);

      expect(error).not.toBeNull();
      expect(error?.code).toBe("required");
    });

    it("should return null for filled required field", () => {
      const schema: PropertySchema = {
        type: "string",
        description: "Port number",
      };

      const error = validateField("port", "14550", schema, true);

      expect(error).toBeNull();
    });

    it("should return null for empty optional field", () => {
      const schema: PropertySchema = {
        type: "string",
        description: "Optional field",
      };

      const error = validateField("optional", undefined, schema, false);

      expect(error).toBeNull();
    });
  });

  // T034: Component test client-side validation - min/max
  describe("min/max validation", () => {
    it("should return error for value exceeding max", () => {
      const schema: PropertySchema = {
        type: "int",
        description: "Port number",
        minimum: 1,
        maximum: 65535,
      };

      const error = validateField("port", 99999, schema, false);

      expect(error).not.toBeNull();
      expect(error?.field).toBe("port");
      expect(error?.message).toContain("65535");
      expect(error?.code).toBe("max");
    });

    it("should return error for value below min", () => {
      const schema: PropertySchema = {
        type: "int",
        description: "Port number",
        minimum: 1,
        maximum: 65535,
      };

      const error = validateField("port", 0, schema, false);

      expect(error).not.toBeNull();
      expect(error?.message).toContain("1");
      expect(error?.code).toBe("min");
    });

    it("should return null for value within range", () => {
      const schema: PropertySchema = {
        type: "int",
        description: "Port number",
        minimum: 1,
        maximum: 65535,
      };

      const error = validateField("port", 14550, schema, false);

      expect(error).toBeNull();
    });

    it("should return null for value at min boundary", () => {
      const schema: PropertySchema = {
        type: "int",
        description: "Port number",
        minimum: 1,
        maximum: 65535,
      };

      const error = validateField("port", 1, schema, false);

      expect(error).toBeNull();
    });

    it("should return null for value at max boundary", () => {
      const schema: PropertySchema = {
        type: "int",
        description: "Port number",
        minimum: 1,
        maximum: 65535,
      };

      const error = validateField("port", 65535, schema, false);

      expect(error).toBeNull();
    });
  });

  // T035: Component test client-side validation - enum values
  describe("enum validation", () => {
    it("should return error for value not in enum", () => {
      const schema: PropertySchema = {
        type: "enum",
        description: "Log level",
        enum: ["debug", "info", "warn", "error"],
      };

      const error = validateField("logLevel", "invalid", schema, false);

      expect(error).not.toBeNull();
      expect(error?.field).toBe("logLevel");
      expect(error?.code).toBe("enum");
      expect(error?.message).toContain("debug");
      expect(error?.message).toContain("info");
    });

    it("should return null for valid enum value", () => {
      const schema: PropertySchema = {
        type: "enum",
        description: "Log level",
        enum: ["debug", "info", "warn", "error"],
      };

      const error = validateField("logLevel", "info", schema, false);

      expect(error).toBeNull();
    });

    it("should handle empty enum array", () => {
      const schema: PropertySchema = {
        type: "enum",
        description: "Empty enum",
        enum: [],
      };

      const error = validateField("field", "any", schema, false);

      expect(error).not.toBeNull();
      expect(error?.code).toBe("enum");
    });
  });

  // T112: Validation consistency - type validation
  describe("type validation", () => {
    it("should return error for invalid number type", () => {
      const schema: PropertySchema = {
        type: "int",
        description: "Port number",
      };

      const error = validateField("port", "not-a-number", schema, false);

      expect(error).not.toBeNull();
      expect(error?.field).toBe("port");
      expect(error?.code).toBe("type");
      expect(error?.message).toContain("number");
    });

    it("should return error for invalid boolean type", () => {
      const schema: PropertySchema = {
        type: "bool",
        description: "Enabled flag",
      };

      const error = validateField("enabled", "not-a-boolean", schema, false);

      expect(error).not.toBeNull();
      expect(error?.code).toBe("type");
    });

    it("should return null for valid types", () => {
      const intSchema: PropertySchema = {
        type: "int",
        description: "Port",
      };

      const boolSchema: PropertySchema = {
        type: "bool",
        description: "Enabled",
      };

      expect(validateField("port", 123, intSchema, false)).toBeNull();
      expect(validateField("enabled", true, boolSchema, false)).toBeNull();
    });
  });

  // Edge cases
  describe("edge cases", () => {
    it("should handle undefined schema", () => {
      const schema: PropertySchema = {
        type: "string",
        description: "Test",
      };

      const error = validateField("field", null, schema, false);

      expect(error).toBeNull(); // null is valid for optional field
    });

    it("should handle number as string", () => {
      const schema: PropertySchema = {
        type: "int",
        description: "Port",
        minimum: 1,
        maximum: 65535,
      };

      const error = validateField("port", "14550", schema, false);

      // Should convert string to number for validation
      expect(error).toBeNull();
    });

    it("should handle boolean fields", () => {
      const schema: PropertySchema = {
        type: "bool",
        description: "Enabled flag",
      };

      const error = validateField("enabled", true, schema, false);

      expect(error).toBeNull();
    });

    it("should handle float fields", () => {
      const schema: PropertySchema = {
        type: "float",
        description: "Timeout",
        minimum: 0.1,
        maximum: 60.0,
      };

      const error = validateField("timeout", 5.5, schema, false);

      expect(error).toBeNull();
    });
  });
});
