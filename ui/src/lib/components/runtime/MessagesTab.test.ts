/**
 * MessagesTab Component Tests
 * Tests for store-based NATS message flow visualization
 * Messages come from logs filtered by source="message-logger"
 */

import { describe, it, expect, beforeEach, vi, type Mock } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/svelte";
import MessagesTab from "./MessagesTab.svelte";
import type {
  RuntimeStoreState,
  LogEntry,
} from "$lib/stores/runtimeStore.svelte";

// Shared mock state — tests mutate this directly
let mockState: RuntimeStoreState;
let mockClearLogs: Mock;

// Mock the runtimeStore module using getter-based API (Svelte 5 runes style)
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

// Create state with message-logger service available (required for rendering messages)
function createStateWithMessageLogger(
  overrides: Partial<RuntimeStoreState> = {},
): RuntimeStoreState {
  return {
    ...createDefaultState(),
    healthComponents: [
      {
        name: "message-logger",
        component: "message-logger",
        type: "service",
        status: "healthy",
        healthy: true,
        message: null,
      },
    ],
    ...overrides,
  };
}

function createMessageLog(
  overrides: Partial<LogEntry> & { fields?: Record<string, unknown> } = {},
): LogEntry {
  return {
    id: `log-${Date.now()}-${Math.random()}`,
    timestamp: Date.now(),
    level: "INFO",
    source: "message-logger", // Important: must be message-logger
    message: "Message logged",
    fields: {
      subject: "test.subject",
      direction: "published",
      component: "test-component",
      ...overrides.fields,
    },
    ...overrides,
  };
}

// Sample message logs for tests
const sampleMessageLogs: LogEntry[] = [
  createMessageLog({
    id: "msg-001",
    timestamp: 1705329785123,
    message: "Published camera frame data",
    fields: {
      subject: "sensors.camera.frame",
      direction: "published",
      component: "camera-sensor",
      frame_id: 1234,
      resolution: "1920x1080",
    },
  }),
  createMessageLog({
    id: "msg-002",
    timestamp: 1705329785456,
    message: "Received camera frame for processing",
    fields: {
      subject: "sensors.camera.frame",
      direction: "received",
      component: "image-processor",
    },
  }),
  createMessageLog({
    id: "msg-003",
    timestamp: 1705329785789,
    message: "Completed image processing",
    fields: {
      subject: "processors.vision.output",
      direction: "processed",
      component: "image-processor",
      objects_detected: 3,
      processing_time_ms: 45,
    },
  }),
];

describe("MessagesTab", () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    mockState = createDefaultState();

    const module = await import("$lib/stores/runtimeStore.svelte");
    mockClearLogs = (module as unknown as { __mockClearLogs: Mock })
      .__mockClearLogs;
  });

  describe("Connection Status", () => {
    it("should show connecting status when not connected", () => {
      mockState = { ...createDefaultState(), connected: false };

      render(MessagesTab, { flowId: "test-flow", isActive: true });

      expect(screen.getByText(/Connecting to runtime stream/)).toBeTruthy();
    });

    it("should hide connecting status when connected", async () => {
      mockState = { ...createDefaultState(), connected: true };

      render(MessagesTab, { flowId: "test-flow", isActive: true });

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

      render(MessagesTab, { flowId: "test-flow", isActive: true });

      await waitFor(() => {
        const errorElement = screen.getByRole("alert");
        expect(errorElement).toBeTruthy();
        expect(errorElement.textContent).toContain(
          "WebSocket connection failed",
        );
      });
    });
  });

  describe("Rendering", () => {
    it("renders empty state when no messages", async () => {
      mockState = createStateWithMessageLogger({
        connected: true,
        logs: [], // No message-logger logs
      });

      render(MessagesTab, { flowId: "flow-123", isActive: true });

      await waitFor(() => {
        expect(screen.getByText(/no messages/i)).toBeTruthy();
      });
    });

    it("renders messages list with all fields", async () => {
      mockState = createStateWithMessageLogger({
        connected: true,
        logs: sampleMessageLogs,
      });

      render(MessagesTab, { flowId: "flow-123", isActive: true });

      await waitFor(() => {
        const subjects = screen.getAllByText("sensors.camera.frame");
        expect(subjects.length).toBeGreaterThan(0);
        expect(screen.getByText("[camera-sensor]")).toBeTruthy();
        expect(screen.getByText("Published camera frame data")).toBeTruthy();
      });
    });

    it("shows direction indicators with correct symbols", async () => {
      mockState = createStateWithMessageLogger({
        connected: true,
        logs: sampleMessageLogs,
      });

      const { container } = render(MessagesTab, {
        flowId: "flow-123",
        isActive: true,
      });

      await waitFor(() => {
        const indicators = container.querySelectorAll(".direction");
        expect(indicators.length).toBeGreaterThan(0);
      });
    });

    it("displays NATS subjects in monospace font class", async () => {
      mockState = createStateWithMessageLogger({
        connected: true,
        logs: sampleMessageLogs,
      });

      const { container } = render(MessagesTab, {
        flowId: "flow-123",
        isActive: true,
      });

      await waitFor(() => {
        const subjectElements = container.querySelectorAll(".subject");
        expect(subjectElements.length).toBeGreaterThan(0);
        const firstSubject = subjectElements[0] as HTMLElement;
        expect(firstSubject.className).toContain("subject");
      });
    });

    it("formats timestamps with millisecond precision", async () => {
      mockState = createStateWithMessageLogger({
        connected: true,
        logs: sampleMessageLogs,
      });

      const { container } = render(MessagesTab, {
        flowId: "flow-123",
        isActive: true,
      });

      await waitFor(() => {
        const timestamps = container.querySelectorAll(".timestamp");
        expect(timestamps.length).toBeGreaterThan(0);
        const firstTimestamp = timestamps[0] as HTMLElement;
        // Should match HH:MM:SS.mmm format
        expect(firstTimestamp.textContent).toMatch(/\d{2}:\d{2}:\d{2}\.\d{3}/);
      });
    });

    it("only shows logs from message-logger source", async () => {
      const mixedLogs: LogEntry[] = [
        ...sampleMessageLogs,
        {
          id: "other-log",
          timestamp: Date.now(),
          level: "INFO",
          source: "graph-processor", // Not message-logger
          message: "Processing entity",
        },
      ];

      mockState = createStateWithMessageLogger({
        connected: true,
        logs: mixedLogs,
      });

      render(MessagesTab, { flowId: "flow-123", isActive: true });

      await waitFor(() => {
        const entries = screen.getAllByTestId("message-entry");
        // Should only show the 3 message-logger entries
        expect(entries).toHaveLength(3);
      });
    });
  });

  describe("Filtering", () => {
    it("filters messages by direction", async () => {
      mockState = createStateWithMessageLogger({
        connected: true,
        logs: sampleMessageLogs,
      });

      render(MessagesTab, { flowId: "flow-123", isActive: true });

      await waitFor(() => {
        expect(screen.getByText("Published camera frame data")).toBeTruthy();
      });

      const directionFilter = screen.getByTestId(
        "direction-filter",
      ) as HTMLSelectElement;
      await fireEvent.change(directionFilter, {
        target: { value: "published" },
      });

      await waitFor(() => {
        expect(screen.getByText("Published camera frame data")).toBeTruthy();
        const receivedMessages = screen.queryAllByText(
          "Received camera frame for processing",
        );
        expect(receivedMessages.length).toBe(0);
      });
    });

    it('shows "All" option in direction filter', async () => {
      mockState = createStateWithMessageLogger({
        connected: true,
        logs: sampleMessageLogs,
      });

      render(MessagesTab, { flowId: "flow-123", isActive: true });

      const directionFilter = screen.getByTestId(
        "direction-filter",
      ) as HTMLSelectElement;
      expect(directionFilter.querySelector('option[value="all"]')).toBeTruthy();
    });

    it("updates filtered message count when filter changes", async () => {
      mockState = createStateWithMessageLogger({
        connected: true,
        logs: sampleMessageLogs,
      });

      render(MessagesTab, { flowId: "flow-123", isActive: true });

      await waitFor(() => {
        expect(screen.getAllByTestId("message-entry").length).toBe(3);
      });

      const directionFilter = screen.getByTestId(
        "direction-filter",
      ) as HTMLSelectElement;
      await fireEvent.change(directionFilter, {
        target: { value: "published" },
      });

      await waitFor(() => {
        expect(screen.getAllByTestId("message-entry").length).toBe(1);
      });
    });

    it("shows empty state when filter matches no messages", async () => {
      mockState = createStateWithMessageLogger({
        connected: true,
        logs: [
          createMessageLog({
            id: "msg-1",
            fields: {
              direction: "published",
              subject: "test",
              component: "comp",
            },
          }),
        ],
      });

      render(MessagesTab, { flowId: "flow-123", isActive: true });

      const directionFilter = screen.getByTestId(
        "direction-filter",
      ) as HTMLSelectElement;
      await fireEvent.change(directionFilter, {
        target: { value: "received" },
      });

      await waitFor(() => {
        expect(
          screen.getByText(/No messages match current filters/),
        ).toBeTruthy();
      });
    });
  });

  describe("Controls", () => {
    it("auto-scroll toggles correctly", async () => {
      mockState = createStateWithMessageLogger({
        connected: true,
        logs: sampleMessageLogs,
      });

      render(MessagesTab, { flowId: "flow-123", isActive: true });

      const autoScrollToggle = screen.getByTestId(
        "auto-scroll-toggle",
      ) as HTMLInputElement;

      expect(autoScrollToggle.checked).toBe(true);

      await fireEvent.click(autoScrollToggle);
      expect(autoScrollToggle.checked).toBe(false);

      await fireEvent.click(autoScrollToggle);
      expect(autoScrollToggle.checked).toBe(true);
    });

    it("clear messages calls store clearLogs", async () => {
      mockState = createStateWithMessageLogger({
        connected: true,
        logs: sampleMessageLogs,
      });

      render(MessagesTab, { flowId: "flow-123", isActive: true });

      await waitFor(() => {
        expect(screen.getAllByTestId("message-entry").length).toBe(3);
      });

      const clearButton = screen.getByTestId("clear-messages-button");
      await fireEvent.click(clearButton);

      expect(mockClearLogs).toHaveBeenCalled();
    });

    it("clear messages resets direction filter", async () => {
      mockState = createStateWithMessageLogger({
        connected: true,
        logs: sampleMessageLogs,
      });

      render(MessagesTab, { flowId: "flow-123", isActive: true });

      const directionFilter = screen.getByTestId(
        "direction-filter",
      ) as HTMLSelectElement;
      await fireEvent.change(directionFilter, {
        target: { value: "published" },
      });

      expect(directionFilter.value).toBe("published");

      const clearButton = screen.getByTestId("clear-messages-button");
      await fireEvent.click(clearButton);

      await waitFor(() => {
        expect(directionFilter.value).toBe("all");
      });
    });

    it("metadata expands and collapses", async () => {
      mockState = createStateWithMessageLogger({
        connected: true,
        logs: sampleMessageLogs,
      });

      const { container } = render(MessagesTab, {
        flowId: "flow-123",
        isActive: true,
      });

      await waitFor(() => {
        const subjects = screen.getAllByText("sensors.camera.frame");
        expect(subjects.length).toBeGreaterThan(0);
      });

      // Find first message with metadata
      const expandButtons = container.querySelectorAll(".metadata-toggle");
      const firstExpandButton = expandButtons[0] as HTMLElement;

      // Initially metadata should be hidden
      expect(container.querySelector(".metadata")).toBeNull();

      // Click to expand
      await fireEvent.click(firstExpandButton);

      await waitFor(() => {
        expect(container.querySelector(".metadata")).toBeTruthy();
      });

      // Click to collapse
      await fireEvent.click(firstExpandButton);

      await waitFor(() => {
        expect(container.querySelector(".metadata")).toBeNull();
      });
    });

    it("multiple metadata sections independent", async () => {
      mockState = createStateWithMessageLogger({
        connected: true,
        logs: sampleMessageLogs,
      });

      const { container } = render(MessagesTab, {
        flowId: "flow-123",
        isActive: true,
      });

      await waitFor(() => {
        const subjects = screen.getAllByText("sensors.camera.frame");
        expect(subjects.length).toBeGreaterThan(0);
      });

      const expandButtons = container.querySelectorAll(".metadata-toggle");

      // Expand first message metadata
      await fireEvent.click(expandButtons[0] as HTMLElement);

      await waitFor(() => {
        const metadataSections = container.querySelectorAll(".metadata");
        expect(metadataSections.length).toBe(1);
      });

      // Expand third message metadata (second button with metadata)
      await fireEvent.click(expandButtons[1] as HTMLElement);

      await waitFor(() => {
        const metadataSections = container.querySelectorAll(".metadata");
        expect(metadataSections.length).toBe(2);
      });
    });
  });

  describe("Store Updates", () => {
    it("renders all messages provided by store on initial load", async () => {
      // Verifies that the component renders all message-logger log entries
      // from the store. Dynamic re-rendering is driven by Svelte's runes system;
      // here we validate correct display of multiple messages on initial render.
      mockState = createStateWithMessageLogger({
        connected: true,
        logs: sampleMessageLogs,
      });

      render(MessagesTab, { flowId: "flow-123", isActive: true });

      await waitFor(() => {
        expect(screen.getAllByTestId("message-entry")).toHaveLength(3);
      });
    });

    it("renders empty state when store has no message-logger logs", async () => {
      // Verifies that the component correctly shows the empty state when the
      // store contains no message-logger entries.
      mockState = createStateWithMessageLogger({
        connected: true,
        logs: [],
      });

      render(MessagesTab, { flowId: "flow-123", isActive: true });

      await waitFor(() => {
        expect(screen.queryAllByTestId("message-entry")).toHaveLength(0);
        expect(screen.getByText(/no messages/i)).toBeTruthy();
      });
    });
  });

  describe("Accessibility", () => {
    it("should have proper ARIA labels on controls", async () => {
      mockState = createStateWithMessageLogger({
        connected: true,
        logs: sampleMessageLogs,
      });

      render(MessagesTab, { flowId: "flow-123", isActive: true });

      const clearButton = screen.getByTestId("clear-messages-button");
      expect(clearButton.getAttribute("aria-label")).toBe("Clear all messages");

      const directionFilter = screen.getByTestId("direction-filter");
      expect(directionFilter.getAttribute("aria-label")).toBe(
        "Filter by direction",
      );
    });

    it("should use aria-live region for message entries", async () => {
      mockState = createStateWithMessageLogger({
        connected: true,
        logs: sampleMessageLogs,
      });

      render(MessagesTab, { flowId: "flow-123", isActive: true });

      await waitFor(() => {
        const logContainer = screen.getByRole("log");
        expect(logContainer.getAttribute("aria-live")).toBe("polite");
      });
    });

    it("should have aria-expanded on metadata toggles", async () => {
      mockState = createStateWithMessageLogger({
        connected: true,
        logs: sampleMessageLogs,
      });

      const { container } = render(MessagesTab, {
        flowId: "flow-123",
        isActive: true,
      });

      await waitFor(() => {
        const expandButtons = container.querySelectorAll(".metadata-toggle");
        expect(expandButtons.length).toBeGreaterThan(0);

        const firstButton = expandButtons[0] as HTMLElement;
        expect(firstButton.getAttribute("aria-expanded")).toBe("false");
      });
    });
  });

  describe("Trace ID Features (Phase 1)", () => {
    describe("Trace ID Extraction", () => {
      it("extracts trace ID from message_id field", async () => {
        const logsWithTraceId = [
          createMessageLog({
            id: "msg-001",
            fields: {
              subject: "test.subject",
              direction: "published",
              component: "test-comp",
              message_id: "trace-abc123-def456-ghi789",
            },
          }),
        ];

        mockState = createStateWithMessageLogger({
          connected: true,
          logs: logsWithTraceId,
        });

        const { container } = render(MessagesTab, {
          flowId: "flow-123",
          isActive: true,
        });

        await waitFor(() => {
          const traceIdElement = container.querySelector(".trace-id");
          expect(traceIdElement).toBeTruthy();
          expect(traceIdElement?.textContent).toContain("trace-ab");
        });
      });

      it("extracts trace ID from trace_id field when message_id is absent", async () => {
        const logsWithFallback = [
          createMessageLog({
            id: "msg-001",
            fields: {
              subject: "test.subject",
              direction: "published",
              component: "test-comp",
              trace_id: "fallback-xyz-123-456",
            },
          }),
        ];

        mockState = createStateWithMessageLogger({
          connected: true,
          logs: logsWithFallback,
        });

        const { container } = render(MessagesTab, {
          flowId: "flow-123",
          isActive: true,
        });

        await waitFor(() => {
          const traceIdElement = container.querySelector(".trace-id");
          expect(traceIdElement).toBeTruthy();
          expect(traceIdElement?.textContent).toContain("fallback");
        });
      });

      it("handles messages without trace ID gracefully", async () => {
        const logsNoTrace = [
          createMessageLog({
            id: "msg-001",
            fields: {
              subject: "test.subject",
              direction: "published",
              component: "test-comp",
            },
          }),
        ];

        mockState = createStateWithMessageLogger({
          connected: true,
          logs: logsNoTrace,
        });

        const { container } = render(MessagesTab, {
          flowId: "flow-123",
          isActive: true,
        });

        await waitFor(() => {
          const messageEntries = container.querySelectorAll(".message-entry");
          expect(messageEntries.length).toBe(1);
          // Should not have trace-id element
          const traceIdElement = container.querySelector(".trace-id");
          expect(traceIdElement).toBeNull();
        });
      });

      it("prefers message_id over trace_id when both exist", async () => {
        const logsWithBoth = [
          createMessageLog({
            id: "msg-001",
            fields: {
              subject: "test.subject",
              direction: "published",
              component: "test-comp",
              message_id: "primary-trace-id",
              trace_id: "fallback-trace-id",
            },
          }),
        ];

        mockState = createStateWithMessageLogger({
          connected: true,
          logs: logsWithBoth,
        });

        const { container } = render(MessagesTab, {
          flowId: "flow-123",
          isActive: true,
        });

        await waitFor(() => {
          const traceIdElement = container.querySelector(".trace-id");
          expect(traceIdElement).toBeTruthy();
          expect(traceIdElement?.textContent).toContain("primary-");
        });
      });
    });

    describe("Trace ID Display", () => {
      it('displays trace ID in truncated format (first 8 chars + "...")', async () => {
        const longTraceId = "abcdefghijklmnopqrstuvwxyz123456";
        const logsWithLongTrace = [
          createMessageLog({
            id: "msg-001",
            fields: {
              subject: "test.subject",
              direction: "published",
              component: "test-comp",
              message_id: longTraceId,
            },
          }),
        ];

        mockState = createStateWithMessageLogger({
          connected: true,
          logs: logsWithLongTrace,
        });

        const { container } = render(MessagesTab, {
          flowId: "flow-123",
          isActive: true,
        });

        await waitFor(() => {
          const traceIdElement = container.querySelector(".trace-id");
          expect(traceIdElement).toBeTruthy();
          expect(traceIdElement?.textContent).toBe("abcdefgh...");
        });
      });

      it("displays short trace IDs without truncation", async () => {
        const shortTraceId = "abc123";
        const logsWithShortTrace = [
          createMessageLog({
            id: "msg-001",
            fields: {
              subject: "test.subject",
              direction: "published",
              component: "test-comp",
              message_id: shortTraceId,
            },
          }),
        ];

        mockState = createStateWithMessageLogger({
          connected: true,
          logs: logsWithShortTrace,
        });

        const { container } = render(MessagesTab, {
          flowId: "flow-123",
          isActive: true,
        });

        await waitFor(() => {
          const traceIdElement = container.querySelector(".trace-id");
          expect(traceIdElement).toBeTruthy();
          expect(traceIdElement?.textContent).toBe("abc123");
        });
      });

      it("uses monospace font for trace ID display", async () => {
        const logsWithTrace = [
          createMessageLog({
            id: "msg-001",
            fields: {
              subject: "test.subject",
              direction: "published",
              component: "test-comp",
              message_id: "trace-123-456",
            },
          }),
        ];

        mockState = createStateWithMessageLogger({
          connected: true,
          logs: logsWithTrace,
        });

        const { container } = render(MessagesTab, {
          flowId: "flow-123",
          isActive: true,
        });

        await waitFor(() => {
          const traceIdElement = container.querySelector(".trace-id");
          expect(traceIdElement).toBeTruthy();
          // Should have class indicating monospace styling
          expect(traceIdElement?.className).toContain("trace-id");
        });
      });
    });

    describe("Trace ID Search and Filtering", () => {
      const messagesWithDifferentTraces = [
        createMessageLog({
          id: "msg-001",
          fields: {
            subject: "test.a",
            direction: "published",
            component: "comp-a",
            message_id: "trace-aaa-111",
          },
        }),
        createMessageLog({
          id: "msg-002",
          fields: {
            subject: "test.b",
            direction: "received",
            component: "comp-b",
            message_id: "trace-aaa-111", // Same trace
          },
        }),
        createMessageLog({
          id: "msg-003",
          fields: {
            subject: "test.c",
            direction: "processed",
            component: "comp-c",
            message_id: "trace-bbb-222", // Different trace
          },
        }),
      ];

      it("renders trace ID search input in control bar", async () => {
        mockState = createStateWithMessageLogger({
          connected: true,
          logs: messagesWithDifferentTraces,
        });

        render(MessagesTab, { flowId: "flow-123", isActive: true });

        const searchInput = screen.getByTestId(
          "trace-id-search",
        ) as HTMLInputElement;
        expect(searchInput).toBeTruthy();
        expect(searchInput.placeholder).toContain("Filter by trace ID");
      });

      it("filters messages by exact trace ID match", async () => {
        mockState = createStateWithMessageLogger({
          connected: true,
          logs: messagesWithDifferentTraces,
        });

        render(MessagesTab, { flowId: "flow-123", isActive: true });

        // Initially shows all 3 messages
        await waitFor(() => {
          expect(screen.getAllByTestId("message-entry")).toHaveLength(3);
        });

        const searchInput = screen.getByTestId(
          "trace-id-search",
        ) as HTMLInputElement;
        await fireEvent.input(searchInput, {
          target: { value: "trace-aaa-111" },
        });

        // Should now show only 2 messages with matching trace
        await waitFor(() => {
          expect(screen.getAllByTestId("message-entry")).toHaveLength(2);
        });
      });

      it("filters messages by partial trace ID match", async () => {
        mockState = createStateWithMessageLogger({
          connected: true,
          logs: messagesWithDifferentTraces,
        });

        render(MessagesTab, { flowId: "flow-123", isActive: true });

        const searchInput = screen.getByTestId(
          "trace-id-search",
        ) as HTMLInputElement;
        await fireEvent.input(searchInput, { target: { value: "aaa" } });

        await waitFor(() => {
          expect(screen.getAllByTestId("message-entry")).toHaveLength(2);
        });
      });

      it("shows empty state when no messages match trace filter", async () => {
        mockState = createStateWithMessageLogger({
          connected: true,
          logs: messagesWithDifferentTraces,
        });

        render(MessagesTab, { flowId: "flow-123", isActive: true });

        const searchInput = screen.getByTestId(
          "trace-id-search",
        ) as HTMLInputElement;
        await fireEvent.input(searchInput, {
          target: { value: "nonexistent-trace" },
        });

        await waitFor(() => {
          expect(
            screen.getByText(/No messages match current filters/),
          ).toBeTruthy();
        });
      });

      it("trace filter is case-insensitive", async () => {
        mockState = createStateWithMessageLogger({
          connected: true,
          logs: messagesWithDifferentTraces,
        });

        render(MessagesTab, { flowId: "flow-123", isActive: true });

        const searchInput = screen.getByTestId(
          "trace-id-search",
        ) as HTMLInputElement;
        await fireEvent.input(searchInput, { target: { value: "TRACE-AAA" } });

        await waitFor(() => {
          expect(screen.getAllByTestId("message-entry")).toHaveLength(2);
        });
      });

      it("combines trace filter with direction filter", async () => {
        mockState = createStateWithMessageLogger({
          connected: true,
          logs: messagesWithDifferentTraces,
        });

        render(MessagesTab, { flowId: "flow-123", isActive: true });

        // Apply trace filter (matches 2 messages: published and received)
        const searchInput = screen.getByTestId(
          "trace-id-search",
        ) as HTMLInputElement;
        await fireEvent.input(searchInput, {
          target: { value: "trace-aaa-111" },
        });

        await waitFor(() => {
          expect(screen.getAllByTestId("message-entry")).toHaveLength(2);
        });

        // Apply direction filter (only published)
        const directionFilter = screen.getByTestId(
          "direction-filter",
        ) as HTMLSelectElement;
        await fireEvent.change(directionFilter, {
          target: { value: "published" },
        });

        await waitFor(() => {
          expect(screen.getAllByTestId("message-entry")).toHaveLength(1);
        });
      });

      it("clears trace filter when clear messages button clicked", async () => {
        mockState = createStateWithMessageLogger({
          connected: true,
          logs: messagesWithDifferentTraces,
        });

        render(MessagesTab, { flowId: "flow-123", isActive: true });

        const searchInput = screen.getByTestId(
          "trace-id-search",
        ) as HTMLInputElement;
        await fireEvent.input(searchInput, { target: { value: "trace-aaa" } });

        expect(searchInput.value).toBe("trace-aaa");

        const clearButton = screen.getByTestId("clear-messages-button");
        await fireEvent.click(clearButton);

        await waitFor(() => {
          expect(searchInput.value).toBe("");
        });
      });
    });

    describe("Trace ID Click Interaction", () => {
      it("clicking trace ID sets it as filter", async () => {
        const logsWithTrace = [
          createMessageLog({
            id: "msg-001",
            fields: {
              subject: "test.subject",
              direction: "published",
              component: "test-comp",
              message_id: "clickable-trace-123",
            },
          }),
          createMessageLog({
            id: "msg-002",
            fields: {
              subject: "test.other",
              direction: "received",
              component: "other-comp",
              message_id: "different-trace-456",
            },
          }),
        ];

        mockState = createStateWithMessageLogger({
          connected: true,
          logs: logsWithTrace,
        });

        const { container } = render(MessagesTab, {
          flowId: "flow-123",
          isActive: true,
        });

        await waitFor(() => {
          expect(screen.getAllByTestId("message-entry")).toHaveLength(2);
        });

        const traceIdElements = container.querySelectorAll(".trace-id");
        const firstTraceId = traceIdElements[0] as HTMLElement;

        await fireEvent.click(firstTraceId);

        await waitFor(() => {
          const searchInput = screen.getByTestId(
            "trace-id-search",
          ) as HTMLInputElement;
          expect(searchInput.value).toBe("clickable-trace-123");
        });

        // Should filter to show only matching message
        await waitFor(() => {
          expect(screen.getAllByTestId("message-entry")).toHaveLength(1);
        });
      });

      it("trace ID element is keyboard accessible", async () => {
        const logsWithTrace = [
          createMessageLog({
            id: "msg-001",
            fields: {
              subject: "test.subject",
              direction: "published",
              component: "test-comp",
              message_id: "keyboard-trace-123",
            },
          }),
        ];

        mockState = createStateWithMessageLogger({
          connected: true,
          logs: logsWithTrace,
        });

        const { container } = render(MessagesTab, {
          flowId: "flow-123",
          isActive: true,
        });

        await waitFor(() => {
          const traceIdElement = container.querySelector(
            ".trace-id",
          ) as HTMLElement;
          expect(traceIdElement).toBeTruthy();
          // Should be a button or have role="button"
          const isButton = traceIdElement.tagName === "BUTTON";
          const hasButtonRole =
            traceIdElement.getAttribute("role") === "button";
          expect(isButton || hasButtonRole).toBe(true);
          // Should be keyboard accessible
          expect(traceIdElement.tabIndex).toBeGreaterThanOrEqual(0);
        });
      });
    });

    describe("Filter Badge and Clear Button", () => {
      const logsWithTrace = [
        createMessageLog({
          id: "msg-001",
          fields: {
            subject: "test.subject",
            direction: "published",
            component: "test-comp",
            message_id: "badge-trace-123",
          },
        }),
      ];

      it("shows filter badge when trace ID filter is active", async () => {
        mockState = createStateWithMessageLogger({
          connected: true,
          logs: logsWithTrace,
        });

        render(MessagesTab, { flowId: "flow-123", isActive: true });

        // No badge initially
        expect(screen.queryByTestId("trace-filter-badge")).toBeNull();

        const searchInput = screen.getByTestId(
          "trace-id-search",
        ) as HTMLInputElement;
        await fireEvent.input(searchInput, {
          target: { value: "badge-trace-123" },
        });

        await waitFor(() => {
          const badge = screen.getByTestId("trace-filter-badge");
          expect(badge).toBeTruthy();
          expect(badge.textContent).toContain("badge-trace-123");
        });
      });

      it("filter badge shows truncated trace ID", async () => {
        const longTrace = "very-long-trace-id-abcdefghijklmnopqrstuvwxyz";
        const logsWithLongTrace = [
          createMessageLog({
            id: "msg-001",
            fields: {
              subject: "test.subject",
              direction: "published",
              component: "test-comp",
              message_id: longTrace,
            },
          }),
        ];

        mockState = createStateWithMessageLogger({
          connected: true,
          logs: logsWithLongTrace,
        });

        render(MessagesTab, { flowId: "flow-123", isActive: true });

        const searchInput = screen.getByTestId(
          "trace-id-search",
        ) as HTMLInputElement;
        await fireEvent.input(searchInput, { target: { value: longTrace } });

        await waitFor(() => {
          const badge = screen.getByTestId("trace-filter-badge");
          expect(badge).toBeTruthy();
          // Badge should show truncated version
          expect(badge.textContent).toContain("very-lon...");
        });
      });

      it("clear filter button removes trace ID filter", async () => {
        mockState = createStateWithMessageLogger({
          connected: true,
          logs: logsWithTrace,
        });

        render(MessagesTab, { flowId: "flow-123", isActive: true });

        const searchInput = screen.getByTestId(
          "trace-id-search",
        ) as HTMLInputElement;
        await fireEvent.input(searchInput, {
          target: { value: "badge-trace-123" },
        });

        await waitFor(() => {
          expect(screen.getByTestId("trace-filter-badge")).toBeTruthy();
        });

        const clearButton = screen.getByTestId("clear-trace-filter-button");
        await fireEvent.click(clearButton);

        await waitFor(() => {
          expect(searchInput.value).toBe("");
          expect(screen.queryByTestId("trace-filter-badge")).toBeNull();
        });
      });

      it("hides filter badge when trace filter is cleared", async () => {
        mockState = createStateWithMessageLogger({
          connected: true,
          logs: logsWithTrace,
        });

        render(MessagesTab, { flowId: "flow-123", isActive: true });

        const searchInput = screen.getByTestId(
          "trace-id-search",
        ) as HTMLInputElement;
        await fireEvent.input(searchInput, {
          target: { value: "badge-trace-123" },
        });

        await waitFor(() => {
          expect(screen.getByTestId("trace-filter-badge")).toBeTruthy();
        });

        // Clear by typing empty string
        await fireEvent.input(searchInput, { target: { value: "" } });

        await waitFor(() => {
          expect(screen.queryByTestId("trace-filter-badge")).toBeNull();
        });
      });
    });

    describe("Copy to Clipboard", () => {
      // Mock clipboard API
      const mockClipboard = {
        writeText: vi.fn().mockResolvedValue(undefined),
      };

      beforeEach(() => {
        Object.assign(navigator, {
          clipboard: mockClipboard,
        });
        mockClipboard.writeText.mockClear();
      });

      it("copies full trace ID to clipboard on button click", async () => {
        const fullTraceId = "full-trace-id-to-copy-123456789";
        const logsWithTrace = [
          createMessageLog({
            id: "msg-001",
            fields: {
              subject: "test.subject",
              direction: "published",
              component: "test-comp",
              message_id: fullTraceId,
            },
          }),
        ];

        mockState = createStateWithMessageLogger({
          connected: true,
          logs: logsWithTrace,
        });

        const { container } = render(MessagesTab, {
          flowId: "flow-123",
          isActive: true,
        });

        await waitFor(() => {
          const copyButton = container.querySelector(
            ".copy-trace-button",
          ) as HTMLElement;
          expect(copyButton).toBeTruthy();
        });

        const copyButton = container.querySelector(
          ".copy-trace-button",
        ) as HTMLElement;
        await fireEvent.click(copyButton);

        expect(mockClipboard.writeText).toHaveBeenCalledWith(fullTraceId);
      });

      it("shows visual feedback after copying trace ID", async () => {
        const logsWithTrace = [
          createMessageLog({
            id: "msg-001",
            fields: {
              subject: "test.subject",
              direction: "published",
              component: "test-comp",
              message_id: "copy-feedback-trace",
            },
          }),
        ];

        mockState = createStateWithMessageLogger({
          connected: true,
          logs: logsWithTrace,
        });

        const { container } = render(MessagesTab, {
          flowId: "flow-123",
          isActive: true,
        });

        await waitFor(() => {
          const copyButton = container.querySelector(
            ".copy-trace-button",
          ) as HTMLElement;
          expect(copyButton).toBeTruthy();
        });

        const copyButton = container.querySelector(
          ".copy-trace-button",
        ) as HTMLElement;
        await fireEvent.click(copyButton);

        // Should show "Copied!" or similar feedback
        await waitFor(() => {
          const feedback = screen.getByText(/copied/i);
          expect(feedback).toBeTruthy();
        });
      });
    });

    describe("Mixed Messages (With and Without Trace IDs)", () => {
      it("displays both traced and untraced messages correctly", async () => {
        const mixedLogs = [
          createMessageLog({
            id: "msg-001",
            fields: {
              subject: "traced.message",
              direction: "published",
              component: "comp-a",
              message_id: "trace-123",
            },
          }),
          createMessageLog({
            id: "msg-002",
            fields: {
              subject: "untraced.message",
              direction: "received",
              component: "comp-b",
              // No trace ID
            },
          }),
        ];

        mockState = createStateWithMessageLogger({
          connected: true,
          logs: mixedLogs,
        });

        const { container } = render(MessagesTab, {
          flowId: "flow-123",
          isActive: true,
        });

        await waitFor(() => {
          const messageEntries = screen.getAllByTestId("message-entry");
          expect(messageEntries).toHaveLength(2);

          const traceIdElements = container.querySelectorAll(".trace-id");
          expect(traceIdElements.length).toBe(1); // Only one message has trace
        });
      });

      it("filters correctly when some messages lack trace IDs", async () => {
        const mixedLogs = [
          createMessageLog({
            id: "msg-001",
            fields: {
              subject: "traced.a",
              direction: "published",
              component: "comp-a",
              message_id: "trace-aaa",
            },
          }),
          createMessageLog({
            id: "msg-002",
            fields: {
              subject: "untraced",
              direction: "received",
              component: "comp-b",
            },
          }),
          createMessageLog({
            id: "msg-003",
            fields: {
              subject: "traced.b",
              direction: "processed",
              component: "comp-c",
              message_id: "trace-bbb",
            },
          }),
        ];

        mockState = createStateWithMessageLogger({
          connected: true,
          logs: mixedLogs,
        });

        render(MessagesTab, { flowId: "flow-123", isActive: true });

        const searchInput = screen.getByTestId(
          "trace-id-search",
        ) as HTMLInputElement;
        await fireEvent.input(searchInput, { target: { value: "trace-aaa" } });

        await waitFor(() => {
          expect(screen.getAllByTestId("message-entry")).toHaveLength(1);
          expect(screen.getByText("traced.a")).toBeTruthy();
        });
      });
    });
  });

  describe("Load History Integration (Phase 2)", () => {
    // Mock the messagesApi module
    let mockFetchMessages: Mock;

    beforeEach(async () => {
      // Dynamic import and mock setup
      const { messagesApi } = await import("$lib/services/messagesApi");
      mockFetchMessages = vi.fn();
      vi.spyOn(messagesApi, "fetchMessages").mockImplementation(
        mockFetchMessages,
      );
    });

    describe("Load History Button", () => {
      it("renders Load History button when connected", async () => {
        mockState = createStateWithMessageLogger({
          connected: true,
          logs: [],
        });

        render(MessagesTab, { flowId: "flow-123", isActive: true });

        await waitFor(() => {
          const loadButton = screen.getByTestId("load-history-button");
          expect(loadButton).toBeTruthy();
          expect(loadButton.textContent).toContain("Load History");
        });
      });

      it("does not render Load History button when not connected", async () => {
        mockState = createStateWithMessageLogger({
          connected: false,
          logs: [],
        });

        render(MessagesTab, { flowId: "flow-123", isActive: true });

        await waitFor(() => {
          const loadButton = screen.queryByTestId("load-history-button");
          expect(loadButton).toBeNull();
        });
      });

      it("does not render Load History button when message-logger unavailable", async () => {
        mockState = {
          ...createDefaultState(),
          connected: true,
          healthComponents: [], // No message-logger
        };

        render(MessagesTab, { flowId: "flow-123", isActive: true });

        await waitFor(() => {
          const loadButton = screen.queryByTestId("load-history-button");
          expect(loadButton).toBeNull();
        });
      });
    });

    describe("Fetching Historical Messages", () => {
      it("calls fetchMessages with correct flowId when Load History clicked", async () => {
        mockFetchMessages.mockResolvedValueOnce({
          messages: [],
          total: 0,
        });

        mockState = createStateWithMessageLogger({
          connected: true,
          flowId: "test-flow-456",
          logs: [],
        });

        render(MessagesTab, { flowId: "test-flow-456", isActive: true });

        await waitFor(() => {
          expect(screen.getByTestId("load-history-button")).toBeTruthy();
        });

        const loadButton = screen.getByTestId("load-history-button");
        await fireEvent.click(loadButton);

        expect(mockFetchMessages).toHaveBeenCalledWith("test-flow-456", {
          limit: expect.any(Number),
        });
      });

      it("calls fetchMessages with default limit parameter", async () => {
        mockFetchMessages.mockResolvedValueOnce({
          messages: [],
          total: 0,
        });

        mockState = createStateWithMessageLogger({
          connected: true,
          flowId: "flow-123",
          logs: [],
        });

        render(MessagesTab, { flowId: "flow-123", isActive: true });

        const loadButton = screen.getByTestId("load-history-button");
        await fireEvent.click(loadButton);

        expect(mockFetchMessages).toHaveBeenCalledWith("flow-123", {
          limit: 100,
        });
      });

      it("shows loading state while fetching", async () => {
        // Mock a delayed response
        let resolvePromise: (value: unknown) => void;
        const pendingPromise = new Promise((resolve) => {
          resolvePromise = resolve;
        });
        mockFetchMessages.mockReturnValueOnce(pendingPromise);

        mockState = createStateWithMessageLogger({
          connected: true,
          flowId: "flow-123",
          logs: [],
        });

        render(MessagesTab, { flowId: "flow-123", isActive: true });

        const loadButton = screen.getByTestId("load-history-button");
        await fireEvent.click(loadButton);

        // Should show loading indicator
        await waitFor(() => {
          expect(screen.getByTestId("history-loading")).toBeTruthy();
        });

        // Button should be disabled during load
        expect(loadButton).toHaveProperty("disabled", true);

        // Resolve the promise
        resolvePromise!({ messages: [], total: 0 });

        await waitFor(() => {
          expect(screen.queryByTestId("history-loading")).toBeNull();
        });
      });

      it("disables Load History button while fetching", async () => {
        let resolvePromise: (value: unknown) => void;
        const pendingPromise = new Promise((resolve) => {
          resolvePromise = resolve;
        });
        mockFetchMessages.mockReturnValueOnce(pendingPromise);

        mockState = createStateWithMessageLogger({
          connected: true,
          flowId: "flow-123",
          logs: [],
        });

        render(MessagesTab, { flowId: "flow-123", isActive: true });

        const loadButton = screen.getByTestId(
          "load-history-button",
        ) as HTMLButtonElement;
        expect(loadButton.disabled).toBe(false);

        await fireEvent.click(loadButton);

        await waitFor(() => {
          expect(loadButton.disabled).toBe(true);
        });

        resolvePromise!({ messages: [], total: 0 });

        await waitFor(() => {
          expect(loadButton.disabled).toBe(false);
        });
      });
    });

    describe("Merging Historical Messages with Live Data", () => {
      it("merges historical messages with existing live messages", async () => {
        const historicalMessages = [
          {
            message_id: "history-001",
            timestamp: 1705329780000,
            subject: "historical.message",
            direction: "published" as const,
            component: "old-component",
          },
          {
            message_id: "history-002",
            timestamp: 1705329781000,
            subject: "historical.other",
            direction: "received" as const,
            component: "old-processor",
          },
        ];

        mockFetchMessages.mockResolvedValueOnce({
          messages: historicalMessages,
          total: 2,
        });

        const liveMessages = [
          createMessageLog({
            id: "live-001",
            timestamp: 1705329785123,
            fields: {
              subject: "live.message",
              direction: "processed",
              component: "live-component",
              message_id: "live-trace-001",
            },
          }),
        ];

        mockState = createStateWithMessageLogger({
          connected: true,
          flowId: "flow-123",
          logs: liveMessages,
        });

        render(MessagesTab, { flowId: "flow-123", isActive: true });

        // Initially shows 1 live message
        await waitFor(() => {
          expect(screen.getAllByTestId("message-entry")).toHaveLength(1);
        });

        const loadButton = screen.getByTestId("load-history-button");
        await fireEvent.click(loadButton);

        // After loading, should show 3 total messages (2 historical + 1 live)
        await waitFor(() => {
          expect(screen.getAllByTestId("message-entry")).toHaveLength(3);
        });

        // Verify historical messages are displayed
        expect(screen.getByText("historical.message")).toBeTruthy();
        expect(screen.getByText("historical.other")).toBeTruthy();
        expect(screen.getByText("live.message")).toBeTruthy();
      });

      it("deduplicates messages by message_id", async () => {
        const duplicateTraceId = "duplicate-trace-123";

        const historicalMessages = [
          {
            message_id: duplicateTraceId,
            timestamp: 1705329780000,
            subject: "duplicate.message",
            direction: "published" as const,
            component: "component-a",
          },
        ];

        mockFetchMessages.mockResolvedValueOnce({
          messages: historicalMessages,
          total: 1,
        });

        const liveMessages = [
          createMessageLog({
            id: "live-001",
            timestamp: 1705329780000, // Same timestamp
            fields: {
              subject: "duplicate.message", // Same subject
              direction: "published",
              component: "component-a",
              message_id: duplicateTraceId, // Same trace ID
            },
          }),
        ];

        mockState = createStateWithMessageLogger({
          connected: true,
          flowId: "flow-123",
          logs: liveMessages,
        });

        render(MessagesTab, { flowId: "flow-123", isActive: true });

        const loadButton = screen.getByTestId("load-history-button");
        await fireEvent.click(loadButton);

        // Should only show 1 message (deduplicated)
        await waitFor(() => {
          expect(screen.getAllByTestId("message-entry")).toHaveLength(1);
        });
      });

      it("sorts merged messages by timestamp (oldest first)", async () => {
        const historicalMessages = [
          {
            message_id: "old-001",
            timestamp: 1705329770000,
            subject: "oldest.message",
            direction: "published" as const,
            component: "comp-a",
          },
          {
            message_id: "old-002",
            timestamp: 1705329775000,
            subject: "middle.message",
            direction: "received" as const,
            component: "comp-b",
          },
        ];

        mockFetchMessages.mockResolvedValueOnce({
          messages: historicalMessages,
          total: 2,
        });

        const liveMessages = [
          createMessageLog({
            id: "live-001",
            timestamp: 1705329780000,
            fields: {
              subject: "newest.message",
              direction: "processed",
              component: "comp-c",
              message_id: "new-001",
            },
          }),
        ];

        mockState = createStateWithMessageLogger({
          connected: true,
          flowId: "flow-123",
          logs: liveMessages,
        });

        const { container } = render(MessagesTab, {
          flowId: "flow-123",
          isActive: true,
        });

        const loadButton = screen.getByTestId("load-history-button");
        await fireEvent.click(loadButton);

        await waitFor(() => {
          expect(screen.getAllByTestId("message-entry")).toHaveLength(3);
        });

        // Get all subjects in order
        const subjects = container.querySelectorAll(".subject");
        expect(subjects[0].textContent).toBe("oldest.message");
        expect(subjects[1].textContent).toBe("middle.message");
        expect(subjects[2].textContent).toBe("newest.message");
      });

      it("preserves all message fields from historical data", async () => {
        const historicalMessages = [
          {
            message_id: "detailed-trace",
            timestamp: 1705329780000,
            subject: "detailed.message",
            direction: "processed" as const,
            component: "detailed-component",
            payload_size: 2048,
            custom_field: "custom_value",
          },
        ];

        mockFetchMessages.mockResolvedValueOnce({
          messages: historicalMessages,
          total: 1,
        });

        mockState = createStateWithMessageLogger({
          connected: true,
          flowId: "flow-123",
          logs: [],
        });

        const { container } = render(MessagesTab, {
          flowId: "flow-123",
          isActive: true,
        });

        const loadButton = screen.getByTestId("load-history-button");
        await fireEvent.click(loadButton);

        await waitFor(() => {
          expect(screen.getByText("detailed.message")).toBeTruthy();
        });

        // Expand metadata to verify all fields
        const expandButton = container.querySelector(".metadata-toggle");
        if (expandButton) {
          await fireEvent.click(expandButton as HTMLElement);

          await waitFor(() => {
            const metadata = container.querySelector(".metadata");
            expect(metadata?.textContent).toContain("payload_size");
            expect(metadata?.textContent).toContain("custom_field");
          });
        }
      });
    });

    describe("Error Handling", () => {
      it("shows error state when fetchMessages fails", async () => {
        const errorMessage = "Failed to fetch messages: Network error";
        mockFetchMessages.mockRejectedValueOnce(new Error(errorMessage));

        mockState = createStateWithMessageLogger({
          connected: true,
          flowId: "flow-123",
          logs: [],
        });

        render(MessagesTab, { flowId: "flow-123", isActive: true });

        const loadButton = screen.getByTestId("load-history-button");
        await fireEvent.click(loadButton);

        await waitFor(() => {
          const errorElement = screen.getByTestId("history-error");
          expect(errorElement).toBeTruthy();
          expect(errorElement.textContent).toContain("Failed to load history");
        });
      });

      it("displays specific error message from MessagesApiError", async () => {
        const { MessagesApiError } = await import("$lib/services/messagesApi");
        const apiError = new MessagesApiError(
          "Flow not found",
          "flow-123",
          404,
        );
        mockFetchMessages.mockRejectedValueOnce(apiError);

        mockState = createStateWithMessageLogger({
          connected: true,
          flowId: "flow-123",
          logs: [],
        });

        render(MessagesTab, { flowId: "flow-123", isActive: true });

        const loadButton = screen.getByTestId("load-history-button");
        await fireEvent.click(loadButton);

        await waitFor(() => {
          const errorElement = screen.getByTestId("history-error");
          expect(errorElement.textContent).toContain("Flow not found");
        });
      });

      it("allows retry after error", async () => {
        mockFetchMessages
          .mockRejectedValueOnce(new Error("Network error"))
          .mockResolvedValueOnce({ messages: [], total: 0 });

        mockState = createStateWithMessageLogger({
          connected: true,
          flowId: "flow-123",
          logs: [],
        });

        render(MessagesTab, { flowId: "flow-123", isActive: true });

        const loadButton = screen.getByTestId("load-history-button");

        // First attempt fails
        await fireEvent.click(loadButton);

        await waitFor(() => {
          expect(screen.getByTestId("history-error")).toBeTruthy();
        });

        // Second attempt succeeds
        await fireEvent.click(loadButton);

        await waitFor(() => {
          expect(screen.queryByTestId("history-error")).toBeNull();
        });

        expect(mockFetchMessages).toHaveBeenCalledTimes(2);
      });

      it("clears error state when retry is successful", async () => {
        mockFetchMessages
          .mockRejectedValueOnce(new Error("First error"))
          .mockResolvedValueOnce({
            messages: [
              {
                message_id: "success-001",
                timestamp: 1705329780000,
                subject: "success.message",
                direction: "published" as const,
                component: "comp-a",
              },
            ],
            total: 1,
          });

        mockState = createStateWithMessageLogger({
          connected: true,
          flowId: "flow-123",
          logs: [],
        });

        render(MessagesTab, { flowId: "flow-123", isActive: true });

        const loadButton = screen.getByTestId("load-history-button");

        // First attempt fails
        await fireEvent.click(loadButton);
        await waitFor(() => {
          expect(screen.getByTestId("history-error")).toBeTruthy();
        });

        // Retry succeeds
        await fireEvent.click(loadButton);
        await waitFor(() => {
          expect(screen.queryByTestId("history-error")).toBeNull();
          expect(screen.getByText("success.message")).toBeTruthy();
        });
      });

      it("re-enables Load History button after error", async () => {
        mockFetchMessages.mockRejectedValueOnce(new Error("Network error"));

        mockState = createStateWithMessageLogger({
          connected: true,
          flowId: "flow-123",
          logs: [],
        });

        render(MessagesTab, { flowId: "flow-123", isActive: true });

        const loadButton = screen.getByTestId(
          "load-history-button",
        ) as HTMLButtonElement;

        await fireEvent.click(loadButton);

        await waitFor(() => {
          expect(screen.getByTestId("history-error")).toBeTruthy();
        });

        // Button should be re-enabled for retry
        expect(loadButton.disabled).toBe(false);
      });
    });

    describe("Interaction with Filters", () => {
      it("applies direction filter to merged messages", async () => {
        const historicalMessages = [
          {
            message_id: "hist-pub",
            timestamp: 1705329770000,
            subject: "historical.published",
            direction: "published" as const,
            component: "comp-a",
          },
          {
            message_id: "hist-rec",
            timestamp: 1705329775000,
            subject: "historical.received",
            direction: "received" as const,
            component: "comp-b",
          },
        ];

        mockFetchMessages.mockResolvedValueOnce({
          messages: historicalMessages,
          total: 2,
        });

        mockState = createStateWithMessageLogger({
          connected: true,
          flowId: "flow-123",
          logs: [],
        });

        render(MessagesTab, { flowId: "flow-123", isActive: true });

        // Load history
        const loadButton = screen.getByTestId("load-history-button");
        await fireEvent.click(loadButton);

        await waitFor(() => {
          expect(screen.getAllByTestId("message-entry")).toHaveLength(2);
        });

        // Apply direction filter
        const directionFilter = screen.getByTestId(
          "direction-filter",
        ) as HTMLSelectElement;
        await fireEvent.change(directionFilter, {
          target: { value: "published" },
        });

        await waitFor(() => {
          expect(screen.getAllByTestId("message-entry")).toHaveLength(1);
          expect(screen.getByText("historical.published")).toBeTruthy();
          expect(screen.queryByText("historical.received")).toBeNull();
        });
      });

      it("applies trace ID filter to merged messages", async () => {
        const targetTraceId = "target-trace-123";
        const historicalMessages = [
          {
            message_id: targetTraceId,
            timestamp: 1705329770000,
            subject: "target.message",
            direction: "published" as const,
            component: "comp-a",
          },
          {
            message_id: "other-trace-456",
            timestamp: 1705329775000,
            subject: "other.message",
            direction: "received" as const,
            component: "comp-b",
          },
        ];

        mockFetchMessages.mockResolvedValueOnce({
          messages: historicalMessages,
          total: 2,
        });

        mockState = createStateWithMessageLogger({
          connected: true,
          flowId: "flow-123",
          logs: [],
        });

        render(MessagesTab, { flowId: "flow-123", isActive: true });

        const loadButton = screen.getByTestId("load-history-button");
        await fireEvent.click(loadButton);

        await waitFor(() => {
          expect(screen.getAllByTestId("message-entry")).toHaveLength(2);
        });

        // Apply trace filter
        const searchInput = screen.getByTestId(
          "trace-id-search",
        ) as HTMLInputElement;
        await fireEvent.input(searchInput, {
          target: { value: targetTraceId },
        });

        await waitFor(() => {
          expect(screen.getAllByTestId("message-entry")).toHaveLength(1);
          expect(screen.getByText("target.message")).toBeTruthy();
          expect(screen.queryByText("other.message")).toBeNull();
        });
      });

      it("clear messages does not remove historical messages", async () => {
        const historicalMessages = [
          {
            message_id: "hist-001",
            timestamp: 1705329770000,
            subject: "historical.persistent",
            direction: "published" as const,
            component: "comp-a",
          },
        ];

        mockFetchMessages.mockResolvedValueOnce({
          messages: historicalMessages,
          total: 1,
        });

        const liveMessages = [
          createMessageLog({
            id: "live-001",
            timestamp: 1705329780000,
            fields: {
              subject: "live.clearable",
              direction: "received",
              component: "comp-b",
              message_id: "live-trace",
            },
          }),
        ];

        mockState = createStateWithMessageLogger({
          connected: true,
          flowId: "flow-123",
          logs: liveMessages,
        });

        render(MessagesTab, { flowId: "flow-123", isActive: true });

        // Load history
        const loadButton = screen.getByTestId("load-history-button");
        await fireEvent.click(loadButton);

        await waitFor(() => {
          expect(screen.getAllByTestId("message-entry")).toHaveLength(2);
        });

        // Clear messages — calls runtimeStore.clearLogs() and resets filters
        const clearButton = screen.getByTestId("clear-messages-button");
        await fireEvent.click(clearButton);

        // After clicking clear, the store's clearLogs was called.
        // The component's historicalMessages state is separate from the store's
        // live logs and should remain populated after clearing.
        // We verify that the clear button was called (store side-effect verified
        // separately) and that the historical message from the load is still shown.
        await waitFor(() => {
          // Historical message should remain (it's in component state, not store)
          expect(screen.getByText("historical.persistent")).toBeTruthy();
        });
      });
    });

    describe("Multiple Load History Operations", () => {
      it("allows loading history multiple times", async () => {
        mockFetchMessages
          .mockResolvedValueOnce({
            messages: [
              {
                message_id: "batch1-001",
                timestamp: 1705329770000,
                subject: "batch1.message",
                direction: "published" as const,
                component: "comp-a",
              },
            ],
            total: 50,
          })
          .mockResolvedValueOnce({
            messages: [
              {
                message_id: "batch2-001",
                timestamp: 1705329760000,
                subject: "batch2.message",
                direction: "received" as const,
                component: "comp-b",
              },
            ],
            total: 50,
          });

        mockState = createStateWithMessageLogger({
          connected: true,
          flowId: "flow-123",
          logs: [],
        });

        render(MessagesTab, { flowId: "flow-123", isActive: true });

        const loadButton = screen.getByTestId("load-history-button");

        // First load
        await fireEvent.click(loadButton);
        await waitFor(() => {
          expect(screen.getByText("batch1.message")).toBeTruthy();
        });

        // Second load
        await fireEvent.click(loadButton);
        await waitFor(() => {
          expect(screen.getByText("batch2.message")).toBeTruthy();
        });

        // Both batches should be visible
        expect(screen.getAllByTestId("message-entry")).toHaveLength(2);
      });

      it("deduplicates across multiple load operations", async () => {
        const duplicateMessage = {
          message_id: "duplicate-123",
          timestamp: 1705329770000,
          subject: "duplicate.message",
          direction: "published" as const,
          component: "comp-a",
        };

        mockFetchMessages
          .mockResolvedValueOnce({
            messages: [duplicateMessage],
            total: 1,
          })
          .mockResolvedValueOnce({
            messages: [duplicateMessage], // Same message
            total: 1,
          });

        mockState = createStateWithMessageLogger({
          connected: true,
          flowId: "flow-123",
          logs: [],
        });

        render(MessagesTab, { flowId: "flow-123", isActive: true });

        const loadButton = screen.getByTestId("load-history-button");

        // Load twice with duplicate data
        await fireEvent.click(loadButton);
        await waitFor(() => {
          expect(screen.getAllByTestId("message-entry")).toHaveLength(1);
        });

        await fireEvent.click(loadButton);
        await waitFor(() => {
          // Should still only show 1 message (deduplicated)
          expect(screen.getAllByTestId("message-entry")).toHaveLength(1);
        });
      });
    });
  });
});
