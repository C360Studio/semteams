import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/svelte";
import HealthTab from "./HealthTab.svelte";
import type {
  RuntimeStoreState,
  ComponentHealth,
  HealthOverall,
} from "$lib/stores/runtimeStore.svelte";

/**
 * HealthTab Component Tests
 * Tests for store-based health display with component status
 */

// Mutable state controlled by tests
let mockState: RuntimeStoreState;

// Mock the runtimeStore module — expose reactive-like getters over mockState
vi.mock("$lib/stores/runtimeStore.svelte", () => {
  return {
    runtimeStore: {
      get connected() {
        return mockState.connected;
      },
      get error() {
        return mockState.error;
      },
      get flowId() {
        return mockState.flowId;
      },
      get flowStatus() {
        return mockState.flowStatus;
      },
      get healthOverall() {
        return mockState.healthOverall;
      },
      get healthComponents() {
        return mockState.healthComponents;
      },
      get logs() {
        return mockState.logs;
      },
      get metricsRaw() {
        return mockState.metricsRaw;
      },
      get metricsRates() {
        return mockState.metricsRates;
      },
      get lastMetricsTimestamp() {
        return mockState.lastMetricsTimestamp;
      },
    },
  };
});

function createDefaultState(): RuntimeStoreState {
  return {
    connected: false,
    error: null,
    flowId: null,
    flowStatus: null,
    healthOverall: null,
    healthComponents: [],
    logs: [],
    metricsRaw: new Map(),
    metricsRates: new Map(),
    lastMetricsTimestamp: null,
  };
}

function createHealthComponent(
  overrides: Partial<ComponentHealth> = {},
): ComponentHealth {
  return {
    name: "test-component",
    component: "test",
    type: "processor",
    status: "healthy",
    healthy: true,
    message: null,
    ...overrides,
  };
}

function createHealthOverall(
  overrides: Partial<HealthOverall> = {},
): HealthOverall {
  return {
    status: "healthy",
    counts: {
      healthy: 3,
      degraded: 0,
      error: 0,
    },
    ...overrides,
  };
}

describe("HealthTab", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockState = createDefaultState();
  });

  describe("Connection Status", () => {
    it("should show connecting status when not connected", () => {
      mockState = {
        ...createDefaultState(),
        connected: false,
      };

      render(HealthTab, { flowId: "test-flow", isActive: true });

      expect(screen.getByText(/Connecting to runtime stream/)).toBeTruthy();
    });

    it("should hide connecting status when connected", async () => {
      mockState = {
        ...createDefaultState(),
        connected: true,
      };

      render(HealthTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        expect(screen.queryByText(/Connecting to runtime stream/)).toBeNull();
      });
    });

    it("should show error message when store has error", async () => {
      mockState = {
        ...createDefaultState(),
        connected: false,
        error: "WebSocket connection failed",
      };

      render(HealthTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        const errorElement = screen.getByTestId("health-error");
        expect(errorElement).toBeTruthy();
        expect(errorElement.textContent).toContain(
          "WebSocket connection failed",
        );
      });
    });
  });

  describe("Health Summary Display", () => {
    it("should display overall health status", async () => {
      mockState = {
        ...createDefaultState(),
        connected: true,
        healthOverall: createHealthOverall(),
        healthComponents: [createHealthComponent()],
      };

      render(HealthTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        expect(screen.getByTestId("health-summary")).toBeTruthy();
        expect(screen.getByText("System Health:")).toBeTruthy();
      });
    });

    it("should show correct health count", async () => {
      mockState = {
        ...createDefaultState(),
        connected: true,
        healthOverall: createHealthOverall({
          counts: { healthy: 2, degraded: 1, error: 0 },
        }),
        healthComponents: [
          createHealthComponent({ name: "comp1", status: "healthy" }),
          createHealthComponent({ name: "comp2", status: "healthy" }),
          createHealthComponent({ name: "comp3", status: "degraded" }),
        ],
      };

      render(HealthTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        expect(screen.getByText("2/3 components healthy")).toBeTruthy();
      });
    });

    it("should display healthy status with green indicator", async () => {
      mockState = {
        ...createDefaultState(),
        connected: true,
        healthOverall: createHealthOverall({ status: "healthy" }),
        healthComponents: [createHealthComponent()],
      };

      render(HealthTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        expect(screen.getByText("🟢")).toBeTruthy();
      });
    });

    it("should display degraded status with yellow indicator", async () => {
      mockState = {
        ...createDefaultState(),
        connected: true,
        healthOverall: createHealthOverall({ status: "degraded" }),
        healthComponents: [createHealthComponent({ status: "degraded" })],
      };

      render(HealthTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        expect(screen.getByText("🟡")).toBeTruthy();
      });
    });

    it("should display error status with red indicator", async () => {
      mockState = {
        ...createDefaultState(),
        connected: true,
        healthOverall: createHealthOverall({ status: "error" }),
        healthComponents: [createHealthComponent({ status: "error" })],
      };

      render(HealthTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        expect(screen.getByText("🔴")).toBeTruthy();
      });
    });
  });

  describe("Component Health Table", () => {
    it("should render health table with correct columns", async () => {
      mockState = {
        ...createDefaultState(),
        connected: true,
        healthOverall: createHealthOverall(),
        healthComponents: [createHealthComponent()],
      };

      render(HealthTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        expect(screen.getByText("Component")).toBeTruthy();
        expect(screen.getByText("Type")).toBeTruthy();
        expect(screen.getByText("Status")).toBeTruthy();
      });
    });

    it("should display all component names", async () => {
      mockState = {
        ...createDefaultState(),
        connected: true,
        healthOverall: createHealthOverall(),
        healthComponents: [
          createHealthComponent({ name: "udp-source", type: "input" }),
          createHealthComponent({ name: "json-processor", type: "processor" }),
          createHealthComponent({ name: "nats-sink", type: "output" }),
        ],
      };

      render(HealthTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        expect(screen.getByText("udp-source")).toBeTruthy();
        expect(screen.getByText("json-processor")).toBeTruthy();
        expect(screen.getByText("nats-sink")).toBeTruthy();
      });
    });

    it("should sort components alphabetically", async () => {
      mockState = {
        ...createDefaultState(),
        connected: true,
        healthOverall: createHealthOverall(),
        healthComponents: [
          createHealthComponent({ name: "udp-source" }),
          createHealthComponent({ name: "json-processor" }),
          createHealthComponent({ name: "nats-sink" }),
        ],
      };

      render(HealthTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        const rows = screen.getAllByTestId("health-row");
        expect(rows[0]).toHaveTextContent("json-processor");
        expect(rows[1]).toHaveTextContent("nats-sink");
        expect(rows[2]).toHaveTextContent("udp-source");
      });
    });

    it("should display status indicators for each component", async () => {
      mockState = {
        ...createDefaultState(),
        connected: true,
        healthOverall: createHealthOverall(),
        healthComponents: [
          createHealthComponent({ name: "comp1" }),
          createHealthComponent({ name: "comp2" }),
          createHealthComponent({ name: "comp3" }),
        ],
      };

      render(HealthTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        const statusIndicators = screen.getAllByTestId("status-indicator");
        expect(statusIndicators).toHaveLength(3);
      });
    });

    it("should show healthy status with green indicator", async () => {
      mockState = {
        ...createDefaultState(),
        connected: true,
        healthOverall: createHealthOverall(),
        healthComponents: [createHealthComponent({ status: "healthy" })],
      };

      render(HealthTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        const indicators = screen.getAllByTestId("status-indicator");
        const healthyIndicators = indicators.filter(
          (el) => el.textContent === "●",
        );
        expect(healthyIndicators.length).toBeGreaterThan(0);
      });
    });

    it("should show degraded status with warning icon", async () => {
      mockState = {
        ...createDefaultState(),
        connected: true,
        healthOverall: createHealthOverall({ status: "degraded" }),
        healthComponents: [createHealthComponent({ status: "degraded" })],
      };

      render(HealthTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        const indicators = screen.getAllByTestId("status-indicator");
        const warningIndicators = indicators.filter(
          (el) => el.textContent === "⚠",
        );
        expect(warningIndicators.length).toBeGreaterThan(0);
      });
    });

    it("should show empty state when no components", async () => {
      mockState = {
        ...createDefaultState(),
        connected: true,
        healthOverall: null,
        healthComponents: [],
      };

      render(HealthTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        expect(screen.getByText("No health data available")).toBeTruthy();
      });
    });

    it("should display component types", async () => {
      mockState = {
        ...createDefaultState(),
        connected: true,
        healthOverall: createHealthOverall(),
        healthComponents: [
          createHealthComponent({ name: "udp-source", type: "input" }),
          createHealthComponent({ name: "my-processor", type: "processor" }),
          createHealthComponent({ name: "sink", type: "output" }),
        ],
      };

      render(HealthTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        // Types are displayed (CSS capitalizes them visually)
        const rows = screen.getAllByTestId("health-row");
        expect(rows.length).toBe(3);
        // Check that type cells contain the expected values
        expect(rows[0].textContent).toContain("processor"); // my-processor sorted first
        expect(rows[1].textContent).toContain("output"); // sink sorted second
        expect(rows[2].textContent).toContain("input"); // udp-source sorted third
      });
    });
  });

  describe("Expandable Details", () => {
    it("should show expand button for components with messages", async () => {
      mockState = {
        ...createDefaultState(),
        connected: true,
        healthOverall: createHealthOverall(),
        healthComponents: [
          createHealthComponent({ name: "healthy-comp", message: null }),
          createHealthComponent({
            name: "degraded-comp",
            status: "degraded",
            message: "Slow acks",
          }),
        ],
      };

      render(HealthTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        const expandButtons = screen.getAllByTestId("expand-button");
        expect(expandButtons).toHaveLength(1); // Only degraded-comp has message
      });
    });

    it("should not show expand button for healthy components without messages", async () => {
      mockState = {
        ...createDefaultState(),
        connected: true,
        healthOverall: createHealthOverall(),
        healthComponents: [
          createHealthComponent({ name: "comp1", message: null }),
          createHealthComponent({ name: "comp2", message: null }),
        ],
      };

      render(HealthTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        expect(screen.getByText("comp1")).toBeTruthy();
        const expandButtons = screen.queryAllByTestId("expand-button");
        expect(expandButtons).toHaveLength(0);
      });
    });

    it("should expand details when button is clicked", async () => {
      mockState = {
        ...createDefaultState(),
        connected: true,
        healthOverall: createHealthOverall(),
        healthComponents: [
          createHealthComponent({
            name: "nats-sink",
            status: "degraded",
            message: "Slow acks (>100ms)",
          }),
        ],
      };

      render(HealthTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        expect(screen.getByTestId("expand-button")).toBeTruthy();
      });

      const expandButton = screen.getByTestId("expand-button");
      await fireEvent.click(expandButton);

      await waitFor(() => {
        expect(screen.getByTestId("details-row")).toBeTruthy();
        expect(screen.getByText("Slow acks (>100ms)")).toBeTruthy();
      });
    });

    it("should collapse details when button is clicked again", async () => {
      mockState = {
        ...createDefaultState(),
        connected: true,
        healthOverall: createHealthOverall(),
        healthComponents: [
          createHealthComponent({
            name: "comp",
            status: "degraded",
            message: "Warning message",
          }),
        ],
      };

      render(HealthTab, { flowId: "test-flow", isActive: true });

      const expandButton = await screen.findByTestId("expand-button");

      // Expand
      await fireEvent.click(expandButton);
      await waitFor(() => {
        expect(screen.getByTestId("details-row")).toBeTruthy();
      });

      // Collapse
      await fireEvent.click(expandButton);
      await waitFor(() => {
        expect(screen.queryByTestId("details-row")).toBeNull();
      });
    });

    it("should display warning severity correctly", async () => {
      mockState = {
        ...createDefaultState(),
        connected: true,
        healthOverall: createHealthOverall(),
        healthComponents: [
          createHealthComponent({
            name: "comp",
            status: "degraded",
            message: "Warning message",
          }),
        ],
      };

      render(HealthTab, { flowId: "test-flow", isActive: true });

      const expandButton = await screen.findByTestId("expand-button");
      await fireEvent.click(expandButton);

      await waitFor(() => {
        expect(screen.getByText("WARNING:")).toBeTruthy();
      });
    });

    it("should display error severity correctly", async () => {
      mockState = {
        ...createDefaultState(),
        connected: true,
        healthOverall: createHealthOverall({ status: "error" }),
        healthComponents: [
          createHealthComponent({
            name: "comp",
            status: "error",
            message: "Connection failed",
          }),
        ],
      };

      render(HealthTab, { flowId: "test-flow", isActive: true });

      const expandButton = await screen.findByTestId("expand-button");
      await fireEvent.click(expandButton);

      await waitFor(() => {
        expect(screen.getByText("ERROR:")).toBeTruthy();
      });
    });

    it("should update aria-expanded attribute", async () => {
      mockState = {
        ...createDefaultState(),
        connected: true,
        healthOverall: createHealthOverall(),
        healthComponents: [
          createHealthComponent({
            name: "comp",
            status: "degraded",
            message: "Message",
          }),
        ],
      };

      render(HealthTab, { flowId: "test-flow", isActive: true });

      const expandButton = await screen.findByTestId("expand-button");

      // Initially collapsed
      expect(expandButton.getAttribute("aria-expanded")).toBe("false");

      // Click to expand
      await fireEvent.click(expandButton);
      await waitFor(() => {
        expect(expandButton.getAttribute("aria-expanded")).toBe("true");
      });

      // Click to collapse
      await fireEvent.click(expandButton);
      await waitFor(() => {
        expect(expandButton.getAttribute("aria-expanded")).toBe("false");
      });
    });
  });

  describe("Store Updates", () => {
    it("should display multiple components from store", async () => {
      // Verifies that the component displays all health components provided
      // by the store — dynamic re-rendering is covered by the Svelte runes
      // system; here we validate the render output for a multi-component state.
      mockState = {
        ...createDefaultState(),
        connected: true,
        healthOverall: createHealthOverall(),
        healthComponents: [
          createHealthComponent({ name: "comp1" }),
          createHealthComponent({ name: "comp2" }),
        ],
      };

      render(HealthTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        expect(screen.getByText("comp1")).toBeTruthy();
        expect(screen.getByText("comp2")).toBeTruthy();
      });
    });

    it("should display degraded overall status", async () => {
      // Verifies that the component correctly renders degraded health state.
      mockState = {
        ...createDefaultState(),
        connected: true,
        healthOverall: createHealthOverall({ status: "degraded" }),
        healthComponents: [createHealthComponent()],
      };

      render(HealthTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        expect(screen.getByText("🟡")).toBeTruthy();
      });
    });
  });

  describe("Accessibility", () => {
    it("should have proper table structure", async () => {
      mockState = {
        ...createDefaultState(),
        connected: true,
        healthOverall: createHealthOverall(),
        healthComponents: [createHealthComponent()],
      };

      render(HealthTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        const table = screen.getByRole("table");
        expect(table).toBeTruthy();
      });
    });

    it("should have column headers with scope attributes", async () => {
      mockState = {
        ...createDefaultState(),
        connected: true,
        healthOverall: createHealthOverall(),
        healthComponents: [createHealthComponent()],
      };

      render(HealthTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        const headers = screen.getAllByRole("columnheader");
        expect(headers.length).toBe(3); // Component, Type, Status
        headers.forEach((header) => {
          expect(header.getAttribute("scope")).toBe("col");
        });
      });
    });

    it("should have accessible status indicators with aria-labels", async () => {
      mockState = {
        ...createDefaultState(),
        connected: true,
        healthOverall: createHealthOverall(),
        healthComponents: [createHealthComponent()],
      };

      render(HealthTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        const indicators = screen.getAllByTestId("status-indicator");
        indicators.forEach((indicator) => {
          expect(indicator.getAttribute("aria-label")).toBeTruthy();
        });
      });
    });

    it("should have accessible expand buttons with aria-labels", async () => {
      mockState = {
        ...createDefaultState(),
        connected: true,
        healthOverall: createHealthOverall(),
        healthComponents: [
          createHealthComponent({
            name: "nats-sink",
            status: "degraded",
            message: "Warning",
          }),
        ],
      };

      render(HealthTab, { flowId: "test-flow", isActive: true });

      const expandButton = await screen.findByTestId("expand-button");
      expect(expandButton.getAttribute("aria-label")).toBeTruthy();
      expect(expandButton.getAttribute("aria-label")).toContain("nats-sink");
    });

    it("should have accessible error alerts", async () => {
      mockState = {
        ...createDefaultState(),
        connected: false,
        error: "Connection failed",
      };

      render(HealthTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        const alert = screen.getByRole("alert");
        expect(alert).toBeTruthy();
      });
    });

    it("should have accessible overall health summary", async () => {
      mockState = {
        ...createDefaultState(),
        connected: true,
        healthOverall: createHealthOverall(),
        healthComponents: [createHealthComponent()],
      };

      render(HealthTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        const summary = screen.getByTestId("health-summary");
        const statusElement = summary.querySelector('[aria-label*="health"]');
        expect(statusElement).toBeTruthy();
      });
    });
  });
});
