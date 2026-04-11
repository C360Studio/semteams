import { describe, it, expect, beforeEach, vi, type Mock } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/svelte";
import LogsTab from "./LogsTab.svelte";
import type {
  RuntimeStoreState,
  LogEntry,
} from "$lib/stores/runtimeStore.svelte";

/**
 * LogsTab Component Tests
 * Tests for store-based log display with filtering
 */

// Mutable state controlled by tests
let mockState: RuntimeStoreState;
let mockClearLogs: Mock;

// Mock the runtimeStore module — expose reactive-like getters over mockState
vi.mock("$lib/stores/runtimeStore.svelte", () => {
  const clearLogsMock = vi.fn();

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
      clearLogs: clearLogsMock,
    },
    get __mockClearLogs() {
      return clearLogsMock;
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

function createLogEntry(overrides: Partial<LogEntry> = {}): LogEntry {
  return {
    id: `log-${Date.now()}-${Math.random()}`,
    timestamp: Date.now(),
    level: "INFO",
    source: "test-source",
    message: "Test message",
    ...overrides,
  };
}

describe("LogsTab", () => {
  beforeEach(async () => {
    vi.clearAllMocks();

    // Reset mock state for each test
    mockState = createDefaultState();

    // Get reference to mock clearLogs function
    const module = await import("$lib/stores/runtimeStore.svelte");
    mockClearLogs = (module as unknown as { __mockClearLogs: Mock })
      .__mockClearLogs;
  });

  describe("Connection Status", () => {
    it("should show connecting status when not connected", () => {
      mockState = {
        ...createDefaultState(),
        connected: false,
      };

      render(LogsTab, { flowId: "test-flow", isActive: true });

      const connectingStatus = screen.getByTestId("connection-connecting");
      expect(connectingStatus).toBeTruthy();
      expect(connectingStatus.textContent).toContain(
        "Connecting to runtime stream",
      );
    });

    it("should hide connecting status when connected", async () => {
      mockState = {
        ...createDefaultState(),
        connected: true,
      };

      render(LogsTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        const connectingStatus = screen.queryByTestId("connection-connecting");
        expect(connectingStatus).toBeNull();
      });
    });

    it("should show error message when store has error", async () => {
      mockState = {
        ...createDefaultState(),
        connected: false,
        error: "Connection failed",
      };

      render(LogsTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        const errorStatus = screen.getByTestId("connection-error");
        expect(errorStatus).toBeTruthy();
        expect(errorStatus.textContent).toContain("Connection failed");
      });
    });
  });

  describe("Log Display", () => {
    it("should display logs from store", async () => {
      const logs: LogEntry[] = [
        createLogEntry({
          id: "log-1",
          timestamp: 1705412345234,
          level: "INFO",
          source: "udp-source",
          message: "Listening on port 8080",
        }),
      ];

      mockState = {
        ...createDefaultState(),
        connected: true,
        logs,
      };

      render(LogsTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        const logEntries = screen.getAllByTestId("log-entry");
        expect(logEntries).toHaveLength(1);
        expect(logEntries[0].textContent).toContain("INFO");
        expect(logEntries[0].textContent).toContain("[udp-source]");
        expect(logEntries[0].textContent).toContain("Listening on port 8080");
      });
    });

    it("should format timestamps correctly", async () => {
      // Use a timestamp that creates predictable local time
      const timestamp = new Date(2025, 10, 17, 14, 23, 45, 678).getTime();

      mockState = {
        ...createDefaultState(),
        connected: true,
        logs: [
          createLogEntry({
            id: "log-1",
            timestamp,
            level: "DEBUG",
            source: "processor",
            message: "Test message",
          }),
        ],
      };

      render(LogsTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        const logEntry = screen.getByTestId("log-entry");
        // Timestamp should be formatted as HH:MM:SS.mmm
        expect(logEntry.textContent).toMatch(/\d{2}:\d{2}:\d{2}\.\d{3}/);
      });
    });

    it("should handle multi-line log messages", async () => {
      mockState = {
        ...createDefaultState(),
        connected: true,
        logs: [
          createLogEntry({
            id: "log-1",
            level: "ERROR",
            source: "processor",
            message:
              "Failed to process message\n  at processor.go:45\n  invalid JSON syntax",
          }),
        ],
      };

      render(LogsTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        const logEntry = screen.getByTestId("log-entry");
        expect(logEntry.textContent).toContain("Failed to process message");
        expect(logEntry.textContent).toContain("at processor.go:45");
        expect(logEntry.textContent).toContain("invalid JSON syntax");
      });
    });

    it("should display multiple log entries in order", async () => {
      const logs: LogEntry[] = [
        createLogEntry({
          id: "log-1",
          level: "INFO",
          source: "source",
          message: "First",
        }),
        createLogEntry({
          id: "log-2",
          level: "WARN",
          source: "processor",
          message: "Second",
        }),
        createLogEntry({
          id: "log-3",
          level: "DEBUG",
          source: "sink",
          message: "Third",
        }),
      ];

      mockState = {
        ...createDefaultState(),
        connected: true,
        logs,
      };

      render(LogsTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        const logEntries = screen.getAllByTestId("log-entry");
        expect(logEntries).toHaveLength(3);
        expect(logEntries[0].textContent).toContain("First");
        expect(logEntries[1].textContent).toContain("Second");
        expect(logEntries[2].textContent).toContain("Third");
      });
    });

    it("should display all logs provided by store", async () => {
      // Verifies that the component renders all log entries from the store.
      // Dynamic re-rendering is handled by Svelte's runes system; here we
      // validate that multiple logs are correctly displayed on initial render.
      mockState = {
        ...createDefaultState(),
        connected: true,
        logs: [
          createLogEntry({ id: "log-1", message: "First log" }),
          createLogEntry({ id: "log-2", message: "Second log" }),
        ],
      };

      render(LogsTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        const logEntries = screen.getAllByTestId("log-entry");
        expect(logEntries).toHaveLength(2);
        expect(logEntries[0].textContent).toContain("First log");
        expect(logEntries[1].textContent).toContain("Second log");
      });
    });
  });

  describe("Filtering", () => {
    const setupLogsForFiltering = () => {
      const logs: LogEntry[] = [
        createLogEntry({
          id: "log-1",
          level: "DEBUG",
          source: "source",
          message: "Debug message",
        }),
        createLogEntry({
          id: "log-2",
          level: "INFO",
          source: "processor",
          message: "Info message",
        }),
        createLogEntry({
          id: "log-3",
          level: "WARN",
          source: "source",
          message: "Warning message",
        }),
        createLogEntry({
          id: "log-4",
          level: "ERROR",
          source: "sink",
          message: "Error message",
        }),
      ];

      mockState = {
        ...createDefaultState(),
        connected: true,
        logs,
      };
    };

    it("should filter logs by level", async () => {
      setupLogsForFiltering();
      render(LogsTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        expect(screen.getAllByTestId("log-entry")).toHaveLength(4);
      });

      const levelFilter = screen.getByTestId(
        "level-filter",
      ) as HTMLSelectElement;
      await fireEvent.change(levelFilter, { target: { value: "ERROR" } });

      await waitFor(() => {
        const logEntries = screen.getAllByTestId("log-entry");
        expect(logEntries).toHaveLength(1);
        expect(logEntries[0].textContent).toContain("ERROR");
        expect(logEntries[0].textContent).toContain("Error message");
      });
    });

    it("should filter logs by source/component", async () => {
      setupLogsForFiltering();
      render(LogsTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        expect(screen.getAllByTestId("log-entry")).toHaveLength(4);
      });

      const componentFilter = screen.getByTestId(
        "component-filter",
      ) as HTMLSelectElement;
      await fireEvent.change(componentFilter, { target: { value: "source" } });

      await waitFor(() => {
        const logEntries = screen.getAllByTestId("log-entry");
        expect(logEntries).toHaveLength(2);
        expect(logEntries[0].textContent).toContain("[source]");
        expect(logEntries[1].textContent).toContain("[source]");
      });
    });

    it("should populate component filter with unique sources", async () => {
      setupLogsForFiltering();
      render(LogsTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        expect(screen.getAllByTestId("log-entry")).toHaveLength(4);
      });

      const componentFilter = screen.getByTestId(
        "component-filter",
      ) as HTMLSelectElement;
      const options = Array.from(componentFilter.options).map((o) => o.value);

      expect(options).toContain("all");
      expect(options).toContain("source");
      expect(options).toContain("processor");
      expect(options).toContain("sink");
    });

    it("should combine level and component filters", async () => {
      setupLogsForFiltering();
      render(LogsTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        expect(screen.getAllByTestId("log-entry")).toHaveLength(4);
      });

      const levelFilter = screen.getByTestId(
        "level-filter",
      ) as HTMLSelectElement;
      const componentFilter = screen.getByTestId(
        "component-filter",
      ) as HTMLSelectElement;

      await fireEvent.change(levelFilter, { target: { value: "WARN" } });
      await fireEvent.change(componentFilter, { target: { value: "source" } });

      await waitFor(() => {
        const logEntries = screen.getAllByTestId("log-entry");
        expect(logEntries).toHaveLength(1);
        expect(logEntries[0].textContent).toContain("WARN");
        expect(logEntries[0].textContent).toContain("[source]");
      });
    });

    it("should show empty state when no logs match filters", async () => {
      setupLogsForFiltering();
      render(LogsTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        expect(screen.getAllByTestId("log-entry")).toHaveLength(4);
      });

      // Filter by ERROR level (only ERROR log exists) and source "processor"
      // (which has INFO level, not ERROR), so no logs match
      const levelFilter = screen.getByTestId(
        "level-filter",
      ) as HTMLSelectElement;
      await fireEvent.change(levelFilter, { target: { value: "ERROR" } });

      const componentFilter = screen.getByTestId(
        "component-filter",
      ) as HTMLSelectElement;
      await fireEvent.change(componentFilter, {
        target: { value: "processor" },
      });

      await waitFor(() => {
        const logEntries = screen.queryAllByTestId("log-entry");
        expect(logEntries).toHaveLength(0);
        expect(screen.getByText(/No logs match current filters/)).toBeTruthy();
      });
    });

    it("should use minimum level filtering (WARN shows WARN and ERROR)", async () => {
      setupLogsForFiltering();
      render(LogsTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        expect(screen.getAllByTestId("log-entry")).toHaveLength(4);
      });

      const levelFilter = screen.getByTestId(
        "level-filter",
      ) as HTMLSelectElement;
      await fireEvent.change(levelFilter, { target: { value: "WARN" } });

      await waitFor(() => {
        const logEntries = screen.getAllByTestId("log-entry");
        expect(logEntries).toHaveLength(2);
        // Should show WARN and ERROR (both >= WARN level)
        const levels = logEntries.map((e) => e.textContent);
        expect(levels.some((l) => l?.includes("WARN"))).toBe(true);
        expect(levels.some((l) => l?.includes("ERROR"))).toBe(true);
      });
    });
  });

  describe("Controls", () => {
    it("should call store clearLogs when Clear button is clicked", async () => {
      mockState = {
        ...createDefaultState(),
        connected: true,
        logs: [createLogEntry({ id: "log-1", message: "Test message" })],
      };

      render(LogsTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        expect(screen.getAllByTestId("log-entry")).toHaveLength(1);
      });

      const clearButton = screen.getByTestId("clear-logs-button");
      await fireEvent.click(clearButton);

      expect(mockClearLogs).toHaveBeenCalled();
    });

    it("should reset filters when clearing logs", async () => {
      mockState = {
        ...createDefaultState(),
        connected: true,
        logs: [
          createLogEntry({ id: "log-1", level: "ERROR", message: "Test" }),
        ],
      };

      render(LogsTab, { flowId: "test-flow", isActive: true });

      const levelFilter = screen.getByTestId(
        "level-filter",
      ) as HTMLSelectElement;
      await fireEvent.change(levelFilter, { target: { value: "ERROR" } });

      expect(levelFilter.value).toBe("ERROR");

      const clearButton = screen.getByTestId("clear-logs-button");
      await fireEvent.click(clearButton);

      await waitFor(() => {
        expect(levelFilter.value).toBe("all");
      });
    });

    it("should toggle auto-scroll checkbox", async () => {
      mockState = {
        ...createDefaultState(),
        connected: true,
      };

      render(LogsTab, { flowId: "test-flow", isActive: true });

      const autoScrollToggle = screen.getByTestId(
        "auto-scroll-toggle",
      ) as HTMLInputElement;
      expect(autoScrollToggle.checked).toBe(true);

      await fireEvent.click(autoScrollToggle);

      await waitFor(() => {
        expect(autoScrollToggle.checked).toBe(false);
      });
    });
  });

  describe("Empty States", () => {
    it("should show empty state when no logs received yet", () => {
      mockState = {
        ...createDefaultState(),
        connected: true,
        logs: [],
      };

      render(LogsTab, { flowId: "test-flow", isActive: true });

      expect(
        screen.getByText(/No logs yet. Waiting for runtime events/),
      ).toBeTruthy();
    });
  });

  describe("Message Logger Exclusion", () => {
    it("should exclude message-logger entries from logs display", async () => {
      const logs: LogEntry[] = [
        createLogEntry({
          id: "log-1",
          source: "udp-source",
          message: "Regular log",
        }),
        createLogEntry({
          id: "log-2",
          source: "message-logger",
          message: "NATS message",
        }),
        createLogEntry({
          id: "log-3",
          source: "processor",
          message: "Another log",
        }),
      ];

      mockState = {
        ...createDefaultState(),
        connected: true,
        logs,
      };

      render(LogsTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        const logEntries = screen.getAllByTestId("log-entry");
        // Should only show 2 logs (excluding message-logger)
        expect(logEntries).toHaveLength(2);
        expect(logEntries[0].textContent).toContain("Regular log");
        expect(logEntries[1].textContent).toContain("Another log");
      });
    });

    it("should not include message-logger in source filter options", async () => {
      const logs: LogEntry[] = [
        createLogEntry({ id: "log-1", source: "udp-source", message: "Log 1" }),
        createLogEntry({
          id: "log-2",
          source: "message-logger",
          message: "NATS message",
        }),
        createLogEntry({ id: "log-3", source: "processor", message: "Log 2" }),
      ];

      mockState = {
        ...createDefaultState(),
        connected: true,
        logs,
      };

      render(LogsTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        const componentFilter = screen.getByTestId(
          "component-filter",
        ) as HTMLSelectElement;
        const options = Array.from(componentFilter.options).map((o) => o.value);

        expect(options).toContain("udp-source");
        expect(options).toContain("processor");
        expect(options).not.toContain("message-logger");
      });
    });

    it("should show empty state when only message-logger logs exist", async () => {
      const logs: LogEntry[] = [
        createLogEntry({
          id: "log-1",
          source: "message-logger",
          message: "NATS message 1",
        }),
        createLogEntry({
          id: "log-2",
          source: "message-logger",
          message: "NATS message 2",
        }),
      ];

      mockState = {
        ...createDefaultState(),
        connected: true,
        logs,
      };

      render(LogsTab, { flowId: "test-flow", isActive: true });

      expect(
        screen.getByText(/No logs yet. Waiting for runtime events/),
      ).toBeTruthy();
    });
  });

  describe("Accessibility", () => {
    it("should have proper ARIA labels on controls", () => {
      mockState = {
        ...createDefaultState(),
        connected: true,
      };

      render(LogsTab, { flowId: "test-flow", isActive: true });

      const clearButton = screen.getByTestId("clear-logs-button");
      expect(clearButton.getAttribute("aria-label")).toBe("Clear all logs");
    });

    it("should use aria-live region for log entries when logs exist", async () => {
      mockState = {
        ...createDefaultState(),
        connected: true,
        logs: [createLogEntry({ id: "log-1", message: "Test message" })],
      };

      render(LogsTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        const logContainer = screen.getByRole("log");
        expect(logContainer.getAttribute("aria-live")).toBe("polite");
      });
    });
  });
});
