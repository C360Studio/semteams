import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/svelte";
import RuntimePanel from "./RuntimePanel.svelte";

describe("RuntimePanel", () => {
  // Mock EventSource for LogsTab
  interface MockEventSource {
    addEventListener: (event: string, handler: () => void) => void;
    close: () => void;
    readyState: number;
  }

  let mockEventSource: MockEventSource;
  let eventListeners: Record<string, Array<() => void>> = {};

  beforeEach(() => {
    eventListeners = {};
    mockEventSource = {
      addEventListener: vi.fn((event: string, handler: () => void) => {
        if (!eventListeners[event]) {
          eventListeners[event] = [];
        }
        eventListeners[event].push(handler);
      }),
      close: vi.fn(),
      readyState: 0,
    };
    globalThis.EventSource = vi.fn(
      () => mockEventSource,
    ) as unknown as typeof EventSource;
  });

  describe("Panel Visibility", () => {
    it("should not render when isOpen is false", () => {
      render(RuntimePanel, { props: { isOpen: false, flowId: "test-flow" } });

      expect(screen.queryByTestId("runtime-panel")).not.toBeInTheDocument();
    });

    it("should render when isOpen is true", () => {
      render(RuntimePanel, { props: { isOpen: true, flowId: "test-flow" } });

      expect(screen.getByTestId("runtime-panel")).toBeInTheDocument();
    });

    it("should display panel title", () => {
      render(RuntimePanel, { props: { isOpen: true, flowId: "test-flow" } });

      expect(screen.getByText("Runtime Debugging")).toBeInTheDocument();
    });
  });

  describe("Tab Navigation", () => {
    it("should display all three tabs", () => {
      render(RuntimePanel, { props: { isOpen: true, flowId: "test-flow" } });

      expect(screen.getByTestId("tab-logs")).toBeInTheDocument();
      expect(screen.getByTestId("tab-metrics")).toBeInTheDocument();
      expect(screen.getByTestId("tab-health")).toBeInTheDocument();
    });

    it("should have Logs tab active by default", () => {
      render(RuntimePanel, { props: { isOpen: true, flowId: "test-flow" } });

      const logsTab = screen.getByTestId("tab-logs");
      expect(logsTab).toHaveClass("active");
      expect(logsTab.getAttribute("aria-selected")).toBe("true");
    });

    it("should show Logs panel by default", () => {
      render(RuntimePanel, { props: { isOpen: true, flowId: "test-flow" } });

      expect(screen.getByTestId("logs-panel")).toBeInTheDocument();
    });

    it("should have Metrics tab enabled (Phase 3)", () => {
      render(RuntimePanel, { props: { isOpen: true, flowId: "test-flow" } });

      const metricsTab = screen.getByTestId("tab-metrics");
      expect(metricsTab).not.toBeDisabled();
    });

    it("should have Health tab enabled (Phase 4 complete)", () => {
      render(RuntimePanel, { props: { isOpen: true, flowId: "test-flow" } });

      const healthTab = screen.getByTestId("tab-health");
      expect(healthTab).not.toBeDisabled();
    });

    it("should switch to Metrics tab when clicked", async () => {
      // Mock fetch for MetricsTab
      global.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({
          timestamp: "2025-11-17T14:23:05.123Z",
          components: [],
        }),
      });

      render(RuntimePanel, { props: { isOpen: true, flowId: "test-flow" } });

      const metricsTab = screen.getByTestId("tab-metrics");
      await fireEvent.click(metricsTab);

      expect(metricsTab).toHaveClass("active");
      expect(metricsTab.getAttribute("aria-selected")).toBe("true");
      expect(screen.getByTestId("metrics-panel")).toBeInTheDocument();
    });

    it("should switch to Health tab when clicked", async () => {
      // Mock fetch for HealthTab
      global.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({
          timestamp: "2025-11-17T14:23:05.123Z",
          overall: { status: "healthy", healthyCount: 2, totalCount: 2 },
          components: [],
        }),
      });

      render(RuntimePanel, { props: { isOpen: true, flowId: "test-flow" } });

      const healthTab = screen.getByTestId("tab-health");
      await fireEvent.click(healthTab);

      expect(healthTab).toHaveClass("active");
      expect(healthTab.getAttribute("aria-selected")).toBe("true");
      expect(screen.getByTestId("health-panel")).toBeInTheDocument();
    });

    it("should have proper ARIA attributes for tabs", () => {
      render(RuntimePanel, { props: { isOpen: true, flowId: "test-flow" } });

      const logsTab = screen.getByTestId("tab-logs");
      expect(logsTab.getAttribute("role")).toBe("tab");
      expect(logsTab.getAttribute("aria-controls")).toBe("logs-panel");
    });

    it("should have proper ARIA attributes for tab panels", () => {
      render(RuntimePanel, { props: { isOpen: true, flowId: "test-flow" } });

      const logsPanel = screen.getByTestId("logs-panel");
      expect(logsPanel.getAttribute("role")).toBe("tabpanel");
      expect(logsPanel.getAttribute("aria-labelledby")).toBe("tab-logs");
    });
  });

  describe("Panel Height", () => {
    it("should use default height of 300px", () => {
      render(RuntimePanel, { props: { isOpen: true, flowId: "test-flow" } });

      const panel = screen.getByTestId("runtime-panel");
      expect(panel).toHaveStyle({ height: "300px" });
    });

    it("should use custom height when provided", () => {
      render(RuntimePanel, {
        props: { isOpen: true, height: 400, flowId: "test-flow" },
      });

      const panel = screen.getByTestId("runtime-panel");
      expect(panel).toHaveStyle({ height: "400px" });
    });
  });

  describe("Close Functionality", () => {
    it("should have close button", () => {
      render(RuntimePanel, { props: { isOpen: true, flowId: "test-flow" } });

      expect(
        screen.getByRole("button", { name: /close runtime panel/i }),
      ).toBeInTheDocument();
    });

    it("should call onClose when close button clicked", async () => {
      const onClose = vi.fn();
      render(RuntimePanel, {
        props: { isOpen: true, onClose, flowId: "test-flow" },
      });

      const closeButton = screen.getByRole("button", {
        name: /close runtime panel/i,
      });
      await fireEvent.click(closeButton);

      expect(onClose).toHaveBeenCalledOnce();
    });

    it("should call onClose when Esc key pressed", async () => {
      const onClose = vi.fn();
      render(RuntimePanel, {
        props: { isOpen: true, onClose, flowId: "test-flow" },
      });

      await fireEvent.keyDown(window, { key: "Escape" });

      expect(onClose).toHaveBeenCalledOnce();
    });

    it("should not call onClose when Esc pressed and panel is closed", async () => {
      const onClose = vi.fn();
      render(RuntimePanel, {
        props: { isOpen: false, onClose, flowId: "test-flow" },
      });

      await fireEvent.keyDown(window, { key: "Escape" });

      expect(onClose).not.toHaveBeenCalled();
    });

    it("should not call onClose when other keys pressed", async () => {
      const onClose = vi.fn();
      render(RuntimePanel, {
        props: { isOpen: true, onClose, flowId: "test-flow" },
      });

      await fireEvent.keyDown(window, { key: "Enter" });
      await fireEvent.keyDown(window, { key: "Tab" });

      expect(onClose).not.toHaveBeenCalled();
    });
  });

  describe("Accessibility", () => {
    it("should have proper aria-label on close button", () => {
      render(RuntimePanel, { props: { isOpen: true, flowId: "test-flow" } });

      const closeButton = screen.getByRole("button", {
        name: /close runtime panel/i,
      });
      expect(closeButton).toHaveAttribute("aria-label", "Close runtime panel");
    });

    it("should have tablist role on tab navigation", () => {
      render(RuntimePanel, { props: { isOpen: true, flowId: "test-flow" } });

      const tablist = screen.getByRole("tablist");
      expect(tablist).toBeInTheDocument();
      expect(tablist.getAttribute("aria-label")).toBe("Runtime debugging tabs");
    });
  });
});
