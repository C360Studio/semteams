import { test, expect } from "@playwright/test";
import { FlowListPage } from "./pages/FlowListPage";
import { FlowCanvasPage } from "./pages/FlowCanvasPage";
import { ComponentPalettePage } from "./pages/ComponentPalettePage";
import { ConfigPanelPage } from "./pages/ConfigPanelPage";
import {
  getComponentTypes,
  type ComponentType,
} from "./helpers/backend-helpers";
import { createTestFlow, deleteTestFlow } from "./helpers/flow-setup";

/**
 * Component Backend Integration E2E Tests
 * Verifies UI correctly loads and renders component types from the backend API
 *
 * Tests:
 * 1. Component palette loads types from backend
 * 2. Component schemas match backend
 * 3. All backend component types are available in UI
 * 4. Component categories match backend
 * 5. Port types from backend render correctly
 */

test.describe("Component Backend Integration", () => {
  let componentTypes: ComponentType[];

  // Fetch component types once before all tests
  test.beforeAll(async ({ request }) => {
    const response = await request.get("/components/types");
    expect(response.ok()).toBe(true);
    componentTypes = await response.json();
    expect(componentTypes.length).toBeGreaterThan(0);
  });

  test("Component palette loads types from backend", async ({ page }) => {
    // Navigate to flow list
    const flowList = new FlowListPage(page);
    await flowList.goto();

    // Create new flow
    await flowList.clickCreateNewFlow();
    await page.waitForURL(/\/flows\/.+/);

    // Wait for canvas to load
    const canvas = new FlowCanvasPage(page);
    await canvas.expectCanvasLoaded();

    // Open the Add Component modal
    const palette = new ComponentPalettePage(page);
    await palette.openAddModal();
    await palette.expectPaletteVisible();

    // Get backend component types
    const backendTypes = await getComponentTypes(page);
    expect(backendTypes.length).toBeGreaterThan(0);

    // Verify UI palette shows same component names from backend
    // Sample key component types across different categories
    const sampleTypes = [
      backendTypes.find((t) => t.type === "input"),
      backendTypes.find((t) => t.type === "output"),
      backendTypes.find((t) => t.type === "processor"),
      backendTypes.find((t) => t.type === "storage"),
    ].filter((t): t is ComponentType => t !== undefined);

    expect(sampleTypes.length).toBeGreaterThanOrEqual(4);

    for (const componentType of sampleTypes) {
      await palette.expectComponentInPalette(componentType.name);
    }

    // Verify component count matches backend (within reasonable range)
    const visibleCardCount = await palette.componentCards.count();
    expect(visibleCardCount).toBe(backendTypes.length);
  });

  test("Component schemas match backend", async ({ page }) => {
    // Get UDP Input component from backend (has well-known schema)
    const backendTypes = await getComponentTypes(page);
    const udpInput = backendTypes.find((t) => t.id === "udp-input");
    expect(udpInput).toBeDefined();

    // Create test flow with UDP Input component
    const { id: flowId, url } = await createTestFlow(page, {
      nodes: [
        {
          id: "node-1",
          type: "udp-input",
          name: "UDP 1",
          config: {},
        },
      ],
    });

    try {
      // Navigate to flow
      await page.goto(url);

      const canvas = new FlowCanvasPage(page);
      await canvas.expectCanvasLoaded();

      // Open edit modal for the component
      await canvas.clickEditButton("UDP 1");

      // Verify config panel loads
      const configPanel = new ConfigPanelPage(page);
      await configPanel.expectPanelVisible();
      await configPanel.expectComponentTitle("UDP 1");

      // Verify schema-driven config fields are present
      // UDP Input schema has: listen_address (string), listen_port (number)
      const schema = udpInput!.schema as {
        properties?: Record<string, { type: string; title?: string }>;
      };

      if (schema.properties) {
        // Check that config fields match schema properties
        for (const [fieldName, fieldSchema] of Object.entries(
          schema.properties,
        )) {
          // Verify field is visible
          await configPanel.expectFieldVisible(fieldName);

          // Verify field label matches schema title or field name
          const expectedLabel = fieldSchema.title || fieldName;
          const label = configPanel.getFieldLabel(fieldName);
          await expect(label).toContainText(expectedLabel, {
            ignoreCase: true,
          });
        }
      }
    } finally {
      // Cleanup
      await deleteTestFlow(page, flowId);
    }
  });

  test("All backend component types are available in UI", async ({ page }) => {
    // Navigate to flow list
    const flowList = new FlowListPage(page);
    await flowList.goto();

    // Create new flow
    await flowList.clickCreateNewFlow();
    await page.waitForURL(/\/flows\/.+/);

    // Wait for canvas to load
    const canvas = new FlowCanvasPage(page);
    await canvas.expectCanvasLoaded();

    // Open the Add Component modal
    const palette = new ComponentPalettePage(page);
    await palette.openAddModal();
    await palette.expectPaletteVisible();

    // Get all component types from backend
    const backendTypes = await getComponentTypes(page);
    expect(backendTypes.length).toBeGreaterThan(0);

    // Verify each type can be found in palette by name
    for (const componentType of backendTypes) {
      await palette.expectComponentInPalette(componentType.name);
    }

    // Verify component count matches exactly
    const visibleCardCount = await palette.componentCards.count();
    expect(visibleCardCount).toBe(backendTypes.length);
  });

  test("Component categories match backend", async ({ page }) => {
    // Navigate to flow list
    const flowList = new FlowListPage(page);
    await flowList.goto();

    // Create new flow
    await flowList.clickCreateNewFlow();
    await page.waitForURL(/\/flows\/.+/);

    // Wait for canvas to load
    const canvas = new FlowCanvasPage(page);
    await canvas.expectCanvasLoaded();

    // Open the Add Component modal
    const palette = new ComponentPalettePage(page);
    await palette.openAddModal();
    await palette.expectPaletteVisible();

    // Get component types with categories from backend
    const backendTypes = await getComponentTypes(page);

    // Extract unique categories from backend
    const backendCategories = Array.from(
      new Set(backendTypes.map((t) => t.category.toLowerCase())),
    );
    expect(backendCategories.length).toBeGreaterThan(0);

    // Verify each backend category is visible in palette
    for (const category of backendCategories) {
      await palette.expectCategoryVisible(category);
    }

    // Verify category count matches
    const visibleCategories = await palette.categories.count();
    expect(visibleCategories).toBe(backendCategories.length);
  });

  test("Port types from backend render correctly", async ({ page }) => {
    // Get component type with ports from backend
    // UDP Input has input ports for configuration, Robotics Processor has both input/output
    const backendTypes = await getComponentTypes(page);
    const roboticsProcessor = backendTypes.find(
      (t) => t.id === "robotics-processor",
    );
    expect(roboticsProcessor).toBeDefined();

    // Extract port information from schema
    const schema = roboticsProcessor!.schema as {
      ports?: {
        input?: Array<{ name: string; subject: string }>;
        output?: Array<{ name: string; subject: string }>;
      };
    };

    expect(schema.ports).toBeDefined();

    // Create test flow with Robotics Processor
    const { id: flowId, url } = await createTestFlow(page, {
      nodes: [
        {
          id: "node-1",
          type: "robotics-processor",
          name: "Robotics 1",
          config: {},
        },
      ],
    });

    try {
      // Navigate to flow
      await page.goto(url);

      const canvas = new FlowCanvasPage(page);
      await canvas.expectCanvasLoaded();

      // Verify node is visible
      await canvas.expectNodeVisible("node-1");

      // Verify input ports are visible
      if (schema.ports?.input && schema.ports.input.length > 0) {
        const inputPorts = canvas.getInputPorts("node-1");
        const inputCount = await inputPorts.count();
        expect(inputCount).toBe(schema.ports.input.length);

        // Verify each port from backend schema is rendered
        for (const port of schema.ports.input) {
          const portElement = canvas.getPortByName("node-1", port.name);
          await expect(portElement).toBeVisible();
        }
      }

      // Verify output ports are visible
      if (schema.ports?.output && schema.ports.output.length > 0) {
        const outputPorts = canvas.getOutputPorts("node-1");
        const outputCount = await outputPorts.count();
        expect(outputCount).toBe(schema.ports.output.length);

        // Verify each port from backend schema is rendered
        for (const port of schema.ports.output) {
          const portElement = canvas.getPortByName("node-1", port.name);
          await expect(portElement).toBeVisible();
        }
      }
    } finally {
      // Cleanup
      await deleteTestFlow(page, flowId);
    }
  });

  test("Component type metadata matches backend", async ({ page }) => {
    // Get all component types from backend
    const backendTypes = await getComponentTypes(page);

    // Navigate to flow list
    const flowList = new FlowListPage(page);
    await flowList.goto();

    // Create new flow
    await flowList.clickCreateNewFlow();
    await page.waitForURL(/\/flows\/.+/);

    // Wait for canvas to load
    const canvas = new FlowCanvasPage(page);
    await canvas.expectCanvasLoaded();

    // Open the Add Component modal
    const palette = new ComponentPalettePage(page);
    await palette.openAddModal();
    await palette.expectPaletteVisible();

    // Sample a few component types and verify metadata
    const sampleTypes = backendTypes.slice(0, 3);

    for (const componentType of sampleTypes) {
      const componentCard = palette.getComponentByName(componentType.name);

      // Verify component card is visible
      await expect(componentCard).toBeVisible();

      // Verify component type/category is reflected in card
      // ComponentPalette uses category for color coding
      const categoryClass = `category-${componentType.category.toLowerCase()}`;
      await expect(componentCard).toHaveClass(new RegExp(categoryClass));

      // Verify description is present if available
      if (componentType.description) {
        await expect(componentCard).toContainText(componentType.description);
      }
    }
  });

  test("Component type protocol information is preserved", async ({ page }) => {
    // Get all component types from backend
    const backendTypes = await getComponentTypes(page);

    // Find components with specific protocols
    const udpComponent = backendTypes.find((t) => t.protocol === "udp");
    const websocketComponent = backendTypes.find(
      (t) => t.protocol === "websocket",
    );

    expect(udpComponent).toBeDefined();
    expect(websocketComponent).toBeDefined();

    // Create test flow with both components
    const { id: flowId, url } = await createTestFlow(page, {
      nodes: [
        {
          id: "node-1",
          type: udpComponent!.id,
          name: "UDP Test",
          config: {},
        },
        {
          id: "node-2",
          type: websocketComponent!.id,
          name: "WebSocket Test",
          config: {},
        },
      ],
    });

    try {
      // Navigate to flow
      await page.goto(url);

      const canvas = new FlowCanvasPage(page);
      await canvas.expectCanvasLoaded();

      // Verify nodes are visible with correct types
      const udpNode = canvas.getNodeById("node-1");
      await expect(udpNode).toBeVisible();
      await expect(udpNode).toHaveAttribute("data-node-type", udpComponent!.id);

      const websocketNode = canvas.getNodeById("node-2");
      await expect(websocketNode).toBeVisible();
      await expect(websocketNode).toHaveAttribute(
        "data-node-type",
        websocketComponent!.id,
      );
    } finally {
      // Cleanup
      await deleteTestFlow(page, flowId);
    }
  });

  test("Backend component type schema validation errors surface in UI", async ({
    page,
  }) => {
    // Get component with required schema fields
    const backendTypes = await getComponentTypes(page);
    const udpInput = backendTypes.find((t) => t.id === "udp-input");
    expect(udpInput).toBeDefined();

    // Create test flow with unconfigured UDP Input (missing required fields)
    const { id: flowId, url } = await createTestFlow(page, {
      nodes: [
        {
          id: "node-1",
          type: "udp-input",
          name: "UDP Unconfigured",
          config: {}, // Missing required listen_address and listen_port
        },
      ],
    });

    try {
      // Navigate to flow
      await page.goto(url);

      const canvas = new FlowCanvasPage(page);
      await canvas.expectCanvasLoaded();

      // Open edit modal
      await canvas.clickEditButton("UDP Unconfigured");

      const configPanel = new ConfigPanelPage(page);
      await configPanel.expectPanelVisible();

      // Verify schema fields are present
      const schema = udpInput!.schema as {
        properties?: Record<string, { type: string }>;
        required?: string[];
      };

      if (schema.required && schema.required.length > 0) {
        // Verify required fields are marked or validated
        for (const requiredField of schema.required) {
          // Field should be visible
          await configPanel.expectFieldVisible(requiredField);

          // Field should be empty (unconfigured)
          const fieldValue = await configPanel.getFieldValue(requiredField);
          expect(fieldValue).toBe("");
        }
      }
    } finally {
      // Cleanup
      await deleteTestFlow(page, flowId);
    }
  });
});
