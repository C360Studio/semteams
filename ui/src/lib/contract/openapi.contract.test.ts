import { describe, it, expect } from "vitest";
import { readFileSync, existsSync } from "fs";
import { join } from "path";
import YAML from "yaml";

/**
 * Contract tests for OpenAPI specification
 * These tests validate that the UI can load and use the committed OpenAPI spec
 * without needing the backend to be running
 */

// OpenAPI spec path must be provided via environment variable
// No default path - this enforces explicit configuration
const OPENAPI_SPEC_PATH = process.env.OPENAPI_SPEC_PATH || "";

const SCHEMAS_DIR = process.env.SCHEMAS_DIR || "";

/**
 * OpenAPI specification structure
 */
interface OpenAPISpec {
  openapi: string;
  info: {
    title: unknown;
    version: unknown;
    [key: string]: unknown;
  };
  paths: Record<
    string,
    {
      get?: {
        responses: Record<string, unknown>;
        [key: string]: unknown;
      };
      [key: string]: unknown;
    }
  >;
  components?: {
    schemas?: Record<
      string,
      {
        type?: unknown;
        properties?: {
          [key: string]: {
            oneOf?: Array<{ $ref?: string; [key: string]: unknown }>;
            [key: string]: unknown;
          };
        };
        required?: string[];
        [key: string]: unknown;
      }
    >;
    [key: string]: unknown;
  };
}

/**
 * Helper to read OpenAPI spec with clear error messages
 * Returns null if spec not found (tests will be skipped)
 */
function loadOpenAPISpec(): OpenAPISpec | null {
  try {
    if (!existsSync(OPENAPI_SPEC_PATH)) {
      console.log(
        `
⚠️  Skipping contract tests: OpenAPI spec not found at ${OPENAPI_SPEC_PATH}

These tests are optional. They validate the UI against a backend's OpenAPI spec.

To enable these tests:
  1. Set OPENAPI_SPEC_PATH to your backend's spec:
     export OPENAPI_SPEC_PATH=/path/to/your-backend/specs/openapi.v3.yaml
     npm test

  2. In a monorepo setup:
     export OPENAPI_SPEC_PATH=/path/to/backend/specs/openapi.v3.yaml
     npm test
			`.trim(),
      );
      return null; // Skip tests gracefully
    }

    const yamlContent = readFileSync(OPENAPI_SPEC_PATH, "utf8");
    return YAML.parse(yamlContent) as OpenAPISpec;
  } catch (err) {
    if (
      err instanceof Error &&
      err.message.includes("OpenAPI spec not found")
    ) {
      throw err; // Re-throw our custom error
    }
    const errorMessage = err instanceof Error ? err.message : String(err);
    throw new Error(`Failed to parse OpenAPI spec: ${errorMessage}`);
  }
}

/**
 * Helper to load schema file with clear error messages
 */
function loadSchemaFile(filename: string): Record<string, unknown> {
  const schemaPath = join(SCHEMAS_DIR, filename);

  try {
    if (!existsSync(schemaPath)) {
      throw new Error(
        `
Schema file not found: ${schemaPath}

Expected schemas directory: ${SCHEMAS_DIR}

Solutions:
  1. Ensure semstreams/schemas/ directory exists
  2. Run: task schema:generate (in semstreams repo)
  3. Set environment variable:
     export SCHEMAS_DIR=/path/to/schemas

For more info, see your backend's documentation on schema generation
			`.trim(),
      );
    }

    const schemaContent = readFileSync(schemaPath, "utf8");
    return JSON.parse(schemaContent) as Record<string, unknown>;
  } catch (err) {
    if (err instanceof Error && err.message.includes("Schema file not found")) {
      throw err; // Re-throw our custom error
    }
    const errorMessage = err instanceof Error ? err.message : String(err);
    throw new Error(`Failed to parse schema ${filename}: ${errorMessage}`);
  }
}

describe.skipIf(!existsSync(OPENAPI_SPEC_PATH))(
  "OpenAPI Specification Contract Tests",
  () => {
    let openapiSpec: OpenAPISpec | null;

    it("should load and parse the committed OpenAPI spec", () => {
      expect(() => {
        openapiSpec = loadOpenAPISpec();
      }).not.toThrow();

      expect(openapiSpec).toBeDefined();
      expect(openapiSpec).toBeTypeOf("object");
    });

    it("should have valid OpenAPI 3.0 structure", () => {
      openapiSpec = loadOpenAPISpec();
      if (!openapiSpec) {
        throw new Error("openapiSpec is null");
      }

      expect(openapiSpec.openapi).toBe("3.0.3");
      expect(openapiSpec.info).toBeDefined();
      expect(openapiSpec.info.title).toBeTruthy();
      expect(openapiSpec.info.version).toBeTruthy();
    });

    it("should have paths section", () => {
      openapiSpec = loadOpenAPISpec();
      if (!openapiSpec) {
        throw new Error("openapiSpec is null");
      }

      expect(openapiSpec.paths).toBeDefined();
      expect(Object.keys(openapiSpec.paths).length).toBeGreaterThan(0);
    });

    it("should have components section with schemas", () => {
      openapiSpec = loadOpenAPISpec();

      expect(openapiSpec?.components).toBeDefined();
      expect(openapiSpec?.components?.schemas).toBeDefined();
      expect(
        Object.keys(openapiSpec?.components?.schemas ?? {}).length,
      ).toBeGreaterThan(0);
    });

    it("should have ComponentType schema with component references", () => {
      openapiSpec = loadOpenAPISpec();
      if (!openapiSpec?.components?.schemas) {
        throw new Error("components.schemas is not defined");
      }

      const componentType = openapiSpec.components.schemas.ComponentType;
      expect(componentType).toBeDefined();
      expect(componentType?.type).toBe("object");
      expect(componentType?.properties).toBeDefined();
      expect(componentType?.properties?.schema).toBeDefined();
      expect(componentType?.properties?.schema?.oneOf).toBeDefined();
      expect(Array.isArray(componentType?.properties?.schema?.oneOf)).toBe(
        true,
      );
      expect(componentType?.properties?.schema?.oneOf?.length).toBeGreaterThan(
        0,
      );
    });

    it("should have required API paths for component management", () => {
      openapiSpec = loadOpenAPISpec();
      if (!openapiSpec) {
        throw new Error("openapiSpec is null");
      }

      const requiredPaths = [
        "/components/types",
        "/components/types/{id}",
        "/components/status/{name}",
        "/components/flowgraph",
        "/components/validate",
      ];

      for (const path of requiredPaths) {
        expect(openapiSpec.paths[path]).toBeDefined();
      }
    });

    it("should have GET method for /components/types", () => {
      openapiSpec = loadOpenAPISpec();
      if (!openapiSpec) {
        throw new Error("openapiSpec is null");
      }

      const typesPath = openapiSpec.paths["/components/types"];
      expect(typesPath).toBeDefined();
      expect(typesPath?.get).toBeDefined();
      expect(typesPath?.get?.responses["200"]).toBeDefined();
    });

    it("should reference component schemas in oneOf", () => {
      openapiSpec = loadOpenAPISpec();
      if (!openapiSpec?.components?.schemas) {
        throw new Error("components.schemas is not defined");
      }

      const componentType = openapiSpec.components.schemas.ComponentType;
      const oneOfRefs = componentType?.properties?.schema?.oneOf;
      if (!oneOfRefs) {
        throw new Error("oneOf refs not found");
      }

      // Each oneOf item should have a $ref to a component schema
      for (const item of oneOfRefs) {
        expect(item?.$ref).toBeDefined();
        expect(item?.$ref).toMatch(/\.\.\/schemas\/.*\.v1\.json$/);
      }
    });

    it("should have metadata fields in ComponentType", () => {
      openapiSpec = loadOpenAPISpec();
      if (!openapiSpec?.components?.schemas) {
        throw new Error("components.schemas is not defined");
      }

      const componentType = openapiSpec.components.schemas.ComponentType;
      expect(componentType?.properties?.id).toBeDefined();
      expect(componentType?.properties?.name).toBeDefined();
      expect(componentType?.properties?.type).toBeDefined();
      expect(componentType?.properties?.description).toBeDefined();
      expect(componentType?.properties?.protocol).toBeDefined();
      expect(componentType?.properties?.domain).toBeDefined();
      expect(componentType?.properties?.version).toBeDefined();
    });

    it("should have required fields specified in ComponentType", () => {
      openapiSpec = loadOpenAPISpec();
      if (!openapiSpec?.components?.schemas) {
        throw new Error("components.schemas is not defined");
      }

      const componentType = openapiSpec.components.schemas.ComponentType;
      expect(componentType?.required).toBeDefined();
      expect(Array.isArray(componentType?.required)).toBe(true);
      expect(componentType?.required).toContain("id");
      expect(componentType?.required).toContain("name");
      expect(componentType?.required).toContain("type");
    });
  },
);

describe.skipIf(!existsSync(OPENAPI_SPEC_PATH))(
  "OpenAPI Schema References",
  () => {
    it("should have all schema references pointing to existing files", () => {
      const openapiSpec = loadOpenAPISpec();
      if (!openapiSpec?.components?.schemas) {
        throw new Error("components.schemas is not defined");
      }

      const componentType = openapiSpec.components.schemas.ComponentType;
      const oneOfRefs = componentType?.properties?.schema?.oneOf;
      if (!oneOfRefs) {
        throw new Error("oneOf refs not found");
      }

      for (const item of oneOfRefs) {
        const ref = item?.$ref;
        if (!ref) continue;
        // ref is like "../schemas/udp.v1.json" relative to OpenAPI spec location
        const schemaFilename = ref.split("/").pop();
        if (!schemaFilename) continue;

        // loadSchemaFile will throw clear error if file doesn't exist
        expect(() => {
          loadSchemaFile(schemaFilename);
        }).not.toThrow();
      }
    });

    it("should have all referenced schemas be valid JSON", () => {
      const openapiSpec = loadOpenAPISpec();
      if (!openapiSpec?.components?.schemas) {
        throw new Error("components.schemas is not defined");
      }

      const componentType = openapiSpec.components.schemas.ComponentType;
      const oneOfRefs = componentType?.properties?.schema?.oneOf;
      if (!oneOfRefs) {
        throw new Error("oneOf refs not found");
      }

      for (const item of oneOfRefs) {
        const ref = item?.$ref;
        if (!ref) continue;
        const schemaFilename = ref.split("/").pop();
        if (!schemaFilename) continue;

        // loadSchemaFile parses JSON and throws clear error if invalid
        expect(() => {
          const schema = loadSchemaFile(schemaFilename);
          expect(schema).toBeDefined();
        }).not.toThrow();
      }
    });
  },
);
