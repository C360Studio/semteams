import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/svelte";
import { tick } from "svelte";
import ConfigPanel from "./ConfigPanel.svelte";
import type { ComponentInstance } from "$lib/types/flow";

// Mock fetch globally
const mockFetch = vi.fn<typeof fetch>();
globalThis.fetch = mockFetch;

describe("ConfigPanel (Prop-Based Architecture)", () => {
  const mockComponent: ComponentInstance = {
    id: "node-1",
    component: "udp-input",
    type: "input",
    name: "UDP Input 1",
    position: { x: 100, y: 100 },
    config: {
      port: 14550,
      bind_address: "0.0.0.0",
    },
    health: {
      status: "healthy",
      lastUpdated: new Date().toISOString(),
    },
  };

  beforeEach(() => {
    vi.clearAllMocks();
    mockFetch.mockClear();

    // Default: mock 404 response (no schema available - fallback to JSON editor)
    mockFetch.mockResolvedValue({
      ok: false,
      status: 404,
    } as Response);
  });

  afterEach(() => {
    mockFetch.mockReset();
  });

  // ========================================================================
  // Prop Acceptance Tests
  // ========================================================================

  describe("Prop Acceptance", () => {
    it("should accept component prop", () => {
      render(ConfigPanel, { props: { component: mockComponent } });

      expect(screen.getByText(/Configure: udp-input/i)).toBeInTheDocument();
    });

    it("should accept onSave callback prop", () => {
      const onSave = vi.fn();

      render(ConfigPanel, { props: { component: mockComponent, onSave } });

      expect(onSave).toBeDefined();
    });

    it("should not render anything when component is null", () => {
      const { container } = render(ConfigPanel, { props: { component: null } });

      expect(container.querySelector(".config-panel")).not.toBeInTheDocument();
    });
  });

  // ========================================================================
  // Config Display Tests
  // ========================================================================

  describe("Config Display", () => {
    it("should display component name", () => {
      render(ConfigPanel, { props: { component: mockComponent } });

      expect(screen.getByText("UDP Input 1")).toBeInTheDocument();
    });

    it("should display component type", () => {
      render(ConfigPanel, { props: { component: mockComponent } });

      // Type appears in both header and info section, so use getAllByText
      const typeElements = screen.getAllByText(/udp-input/i);
      expect(typeElements.length).toBeGreaterThan(0);
    });

    it("should display config as JSON in textarea when schema unavailable", async () => {
      render(ConfigPanel, { props: { component: mockComponent } });

      // Wait for schema fetch to complete (404 response)
      await waitFor(() => {
        const textarea = screen.queryByRole("textbox", {
          name: /configuration/i,
        });
        expect(textarea).toBeInTheDocument();
      });

      const textarea = screen.getByRole("textbox", { name: /configuration/i });
      expect(textarea).toHaveValue(
        JSON.stringify(mockComponent.config, null, 2),
      );
    });
  });

  // ========================================================================
  // Config Editing Tests
  // ========================================================================

  describe("Config Editing", () => {
    it("should call onSave when Save button is clicked", async () => {
      const onSave = vi.fn();

      render(ConfigPanel, { props: { component: mockComponent, onSave } });

      // Wait for schema fetch and JSON editor to appear
      await waitFor(() => {
        expect(
          screen.queryByRole("textbox", { name: /configuration/i }),
        ).toBeInTheDocument();
      });

      const textarea = screen.getByRole("textbox", { name: /configuration/i });
      const newConfig = { port: 14551, bind_address: "127.0.0.1" };
      await fireEvent.input(textarea, {
        target: { value: JSON.stringify(newConfig, null, 2) },
      });

      const saveButton = screen.getByRole("button", { name: /apply/i });
      await fireEvent.click(saveButton);

      expect(onSave).toHaveBeenCalledWith("node-1", newConfig);
    });

    it("should call onClose when Save is successful", async () => {
      const onSave = vi.fn();
      const onClose = vi.fn();

      render(ConfigPanel, {
        props: { component: mockComponent, onSave, onClose },
      });

      // Wait for JSON editor to appear
      await waitFor(() => {
        expect(
          screen.queryByRole("button", { name: /apply/i }),
        ).toBeInTheDocument();
      });

      const saveButton = screen.getByRole("button", { name: /apply/i });
      await fireEvent.click(saveButton);

      expect(onClose).toHaveBeenCalled();
    });

    it("should handle JSON editor for complex config", async () => {
      const complexComponent: ComponentInstance = {
        ...mockComponent,
        config: {
          port: 14550,
          nested: { key1: "value1", key2: "value2" },
        },
      };

      const onSave = vi.fn();

      render(ConfigPanel, { props: { component: complexComponent, onSave } });

      // Wait for JSON editor to appear
      await waitFor(() => {
        expect(
          screen.queryByRole("textbox", { name: /configuration/i }),
        ).toBeInTheDocument();
      });

      const jsonEditor = screen.getByRole("textbox", {
        name: /configuration/i,
      });
      expect(jsonEditor).toBeInTheDocument();

      const newConfig = {
        port: 14550,
        nested: { key1: "updated", key2: "value2" },
      };

      await fireEvent.input(jsonEditor, {
        target: { value: JSON.stringify(newConfig) },
      });

      const saveButton = screen.getByRole("button", { name: /apply/i });
      await fireEvent.click(saveButton);

      expect(onSave).toHaveBeenCalledWith("node-1", newConfig);
    });
  });

  // ========================================================================
  // Validation Tests
  // ========================================================================

  describe("Validation", () => {
    it("should display validation error for invalid JSON", async () => {
      const onSave = vi.fn();

      render(ConfigPanel, { props: { component: mockComponent, onSave } });

      // Wait for JSON editor to appear
      await waitFor(() => {
        expect(
          screen.queryByRole("textbox", { name: /configuration/i }),
        ).toBeInTheDocument();
      });

      const jsonEditor = screen.getByRole("textbox", {
        name: /configuration/i,
      });
      await fireEvent.input(jsonEditor, { target: { value: "invalid{json" } });

      // Wait for error to appear in JsonEditor
      await waitFor(() => {
        const errorElement = screen.queryByText(/unexpected token/i);
        expect(errorElement).toBeInTheDocument();
      });
    });

    it("should not call onSave when validation fails", async () => {
      const onSave = vi.fn();

      render(ConfigPanel, { props: { component: mockComponent, onSave } });

      // Wait for JSON editor to appear
      await waitFor(() => {
        expect(
          screen.queryByRole("textbox", { name: /configuration/i }),
        ).toBeInTheDocument();
      });

      const jsonEditor = screen.getByRole("textbox", {
        name: /configuration/i,
      });
      await fireEvent.input(jsonEditor, { target: { value: "{invalid}" } });

      const saveButton = screen.getByRole("button", { name: /apply/i });
      await fireEvent.click(saveButton);

      // Should NOT emit event for invalid JSON
      expect(onSave).not.toHaveBeenCalled();
    });

    it("should clear error when valid JSON is entered", async () => {
      const onSave = vi.fn();

      render(ConfigPanel, { props: { component: mockComponent, onSave } });

      // Wait for JSON editor to appear
      await waitFor(() => {
        expect(
          screen.queryByRole("textbox", { name: /configuration/i }),
        ).toBeInTheDocument();
      });

      const jsonEditor = screen.getByRole("textbox", {
        name: /configuration/i,
      });

      // First, enter invalid JSON
      await fireEvent.input(jsonEditor, { target: { value: "invalid{json" } });

      // Wait for error to appear
      await waitFor(() => {
        expect(screen.queryByText(/unexpected token/i)).toBeInTheDocument();
      });

      // Then enter valid JSON
      await fireEvent.input(jsonEditor, {
        target: { value: '{"port": 14551}' },
      });

      const saveButton = screen.getByRole("button", { name: /apply/i });
      await fireEvent.click(saveButton);

      // Error should be cleared
      expect(screen.queryByText(/unexpected token/i)).not.toBeInTheDocument();
      expect(onSave).toHaveBeenCalled();
    });
  });

  // ========================================================================
  // Edge Cases
  // ========================================================================

  describe("Edge Cases", () => {
    it("should handle component with empty config", async () => {
      const emptyComponent: ComponentInstance = {
        ...mockComponent,
        config: {},
      };

      render(ConfigPanel, { props: { component: emptyComponent } });

      // Should render without crashing
      expect(screen.getByText("UDP Input 1")).toBeInTheDocument();

      // Wait for JSON editor to appear
      await waitFor(() => {
        expect(
          screen.queryByRole("textbox", { name: /configuration/i }),
        ).toBeInTheDocument();
      });

      const textarea = screen.getByRole("textbox", { name: /configuration/i });
      expect(textarea).toHaveValue("{}");
    });

    it("should handle component switching", async () => {
      const { rerender } = render(ConfigPanel, {
        props: { component: mockComponent },
      });

      expect(screen.getByText("UDP Input 1")).toBeInTheDocument();

      const newComponent: ComponentInstance = {
        id: "node-2",
        component: "websocket-output",
        type: "output",
        name: "WebSocket Output 1",
        position: { x: 200, y: 200 },
        config: { port: 8080 },
        health: {
          status: "healthy",
          lastUpdated: new Date().toISOString(),
        },
      };

      await rerender({ component: newComponent });
      await tick();

      // Wait for new component to load
      await waitFor(() => {
        expect(screen.queryByText("WebSocket Output 1")).toBeInTheDocument();
      });

      // Wait for textarea to update with new config
      await waitFor(() => {
        const textarea = screen.queryByRole("textbox", {
          name: /configuration/i,
        });
        if (textarea) {
          expect((textarea as HTMLTextAreaElement).value).toBe(
            JSON.stringify({ port: 8080 }, null, 2),
          );
        }
      });
    });

    it("should clear panel when component is deselected", async () => {
      const { rerender, container } = render(ConfigPanel, {
        props: { component: mockComponent },
      });

      expect(screen.getByText("UDP Input 1")).toBeInTheDocument();

      await rerender({ component: null });

      expect(container.querySelector(".config-panel")).not.toBeInTheDocument();
      expect(screen.queryByText("UDP Input 1")).not.toBeInTheDocument();
    });

    it("should call onClose when Cancel button is clicked", async () => {
      const onClose = vi.fn();

      render(ConfigPanel, { props: { component: mockComponent, onClose } });

      // Wait for Cancel button to appear
      await waitFor(() => {
        expect(
          screen.queryByRole("button", { name: /cancel/i }),
        ).toBeInTheDocument();
      });

      const cancelButton = screen.getByRole("button", { name: /cancel/i });
      await fireEvent.click(cancelButton);

      expect(onClose).toHaveBeenCalled();
    });

    it("should reset config when Cancel is clicked after editing", async () => {
      const onClose = vi.fn();

      render(ConfigPanel, { props: { component: mockComponent, onClose } });

      // Wait for JSON editor to appear
      await waitFor(() => {
        expect(
          screen.queryByRole("textbox", { name: /configuration/i }),
        ).toBeInTheDocument();
      });

      const textarea = screen.getByRole("textbox", {
        name: /configuration/i,
      }) as HTMLTextAreaElement;
      const originalValue = textarea.value;

      // Edit config
      await fireEvent.input(textarea, { target: { value: '{"port": 99999}' } });

      // Click Cancel
      const cancelButton = screen.getByRole("button", { name: /cancel/i });
      await fireEvent.click(cancelButton);

      // Config should be reset
      expect(textarea).toHaveValue(originalValue);
      expect(onClose).toHaveBeenCalled();
    });
  });

  // ========================================================================
  // Accessibility Tests
  // ========================================================================

  describe("Accessibility", () => {
    it("should have proper label for textarea", async () => {
      render(ConfigPanel, { props: { component: mockComponent } });

      // Wait for JSON editor to appear
      await waitFor(() => {
        expect(screen.queryByLabelText(/configuration/i)).toBeInTheDocument();
      });
    });

    it("should have proper ARIA labels for buttons", async () => {
      render(ConfigPanel, { props: { component: mockComponent } });

      // Wait for buttons to appear (after schema load)
      await waitFor(() => {
        expect(
          screen.queryByRole("button", { name: /cancel/i }),
        ).toBeInTheDocument();
      });

      expect(
        screen.getByRole("button", { name: /apply/i }),
      ).toBeInTheDocument();
      expect(
        screen.getByRole("button", { name: /cancel/i }),
      ).toBeInTheDocument();
      expect(
        screen.getByRole("button", { name: /close/i }),
      ).toBeInTheDocument();
    });
  });

  // ========================================================================
  // Schema Support Tests (Feature 011)
  // ========================================================================

  describe("Schema Caching (T040)", () => {
    it("should cache schema to avoid redundant fetches", async () => {
      // Mock successful schema fetch
      mockFetch.mockResolvedValue({
        ok: true,
        json: async () => ({
          componentType: "udp-input",
          schema: {
            properties: {
              port: { type: "int", description: "Port", category: "basic" },
            },
            required: ["port"],
          },
          version: "1.0.0",
        }),
      } as Response);

      const { rerender } = render(ConfigPanel, {
        props: { component: mockComponent },
      });

      // Wait for initial schema fetch
      await waitFor(() => {
        expect(mockFetch).toHaveBeenCalledWith("/components/types/udp-input");
      });

      // const initialFetchCount = mockFetch.mock.calls.length;

      // Switch to different component type
      const otherComponent: ComponentInstance = {
        id: "node-2",
        component: "websocket-output",
        type: "output",
        name: "WebSocket Output",
        position: { x: 200, y: 200 },
        config: {},
        health: { status: "healthy", lastUpdated: new Date().toISOString() },
      };

      await rerender({ component: otherComponent });
      await tick();

      // Wait for websocket-output schema fetch
      await waitFor(
        () => {
          const websocketCalls = mockFetch.mock.calls.filter((call) => {
            const url =
              typeof call[0] === "string"
                ? call[0]
                : call[0] instanceof URL
                  ? call[0].toString()
                  : "";
            return url.includes("websocket-output");
          });
          expect(websocketCalls.length).toBeGreaterThan(0);
        },
        { timeout: 1000 },
      );

      // Switch back to original component (same type - should use cache)
      await rerender({ component: mockComponent });
      await tick();

      // Wait a bit to ensure no additional fetch happens
      await new Promise((resolve) => setTimeout(resolve, 100));

      // Should only have fetched udp-input once (cached on second render)
      const udpInputCalls = mockFetch.mock.calls.filter((call) => {
        const url =
          typeof call[0] === "string"
            ? call[0]
            : call[0] instanceof URL
              ? call[0].toString()
              : "";
        return url.includes("udp-input");
      });
      expect(udpInputCalls.length).toBe(1);
    });
  });

  describe("Schema Fallback (T041)", () => {
    it("should show JSON editor with warning when schema missing", async () => {
      // Default mock already returns 404
      render(ConfigPanel, { props: { component: mockComponent } });

      // Wait for schema fetch attempt and JSON editor to appear
      await waitFor(() => {
        const textarea = screen.queryByRole("textbox", {
          name: /configuration/i,
        });
        expect(textarea).toBeInTheDocument();
      });

      // Should show warning
      await waitFor(() => {
        const warning = screen.queryByText(/schema not available/i);
        expect(warning).toBeInTheDocument();
      });
    });
  });

  // T004: Test for schema extraction from full API response
  describe("Schema Extraction from API Response (T004)", () => {
    it("should handle full backend API response format", async () => {
      // Mock full API response (matches backend pkg/service/component_manager_http.go)
      const mockApiResponse = {
        id: "udp-input",
        name: "UDP Input",
        type: "input",
        protocol: "nats",
        description: "UDP packet input",
        version: "1.0.0",
        category: "input",
        schema: {
          properties: {
            port: {
              type: "int",
              default: 14550,
              category: "basic",
              description: "Port number",
            },
          },
          required: ["port"],
        },
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockApiResponse,
      } as Response);

      render(ConfigPanel, { props: { component: mockComponent } });

      // Wait for schema to load
      await waitFor(() => {
        expect(mockFetch).toHaveBeenCalledWith("/components/types/udp-input");
      });

      // Wait for component to process schema and render SchemaForm
      // The component stores the full response object and extracts .schema for SchemaForm
      await waitFor(() => {
        // SchemaForm should render with port field from schema
        const saveButton = screen.queryByRole("button", { name: /^save$/i });
        expect(saveButton).toBeInTheDocument();
      });
    });
  });

  describe("Dirty State Preservation (T043)", () => {
    it("should preserve dirty state when switching components", async () => {
      const { rerender } = render(ConfigPanel, {
        props: { component: mockComponent },
      });

      // Wait for JSON editor to appear
      await waitFor(() => {
        expect(
          screen.queryByRole("textbox", { name: /configuration/i }),
        ).toBeInTheDocument();
      });

      // Edit config (make it dirty)
      const textarea = screen.getByRole("textbox", {
        name: /configuration/i,
      }) as HTMLTextAreaElement;
      const dirtyConfig = {
        port: 8080,
        bind_address: "127.0.0.1",
        custom_field: "test",
      };
      await fireEvent.input(textarea, {
        target: { value: JSON.stringify(dirtyConfig, null, 2) },
      });

      const dirtyValue = textarea.value;

      // Wait for dirty state to be saved (effects need to run)
      await tick();
      await new Promise((resolve) => setTimeout(resolve, 50));

      // Switch to different component
      const otherComponent: ComponentInstance = {
        id: "node-2",
        component: "websocket-output",
        type: "output",
        name: "WebSocket Output",
        position: { x: 200, y: 200 },
        config: { port: 3000 },
        health: { status: "healthy", lastUpdated: new Date().toISOString() },
      };

      await rerender({ component: otherComponent });
      await tick();

      // Switch back to original component
      await rerender({ component: mockComponent });
      await tick();

      // Wait for textarea to appear again
      await waitFor(() => {
        expect(
          screen.queryByRole("textbox", { name: /configuration/i }),
        ).toBeInTheDocument();
      });

      // Dirty state should be preserved
      const textareaAfter = screen.getByRole("textbox", {
        name: /configuration/i,
      }) as HTMLTextAreaElement;
      expect(textareaAfter.value).toBe(dirtyValue);
    });

    it("should clear dirty state after successful save", async () => {
      const onSave = vi.fn();
      render(ConfigPanel, { props: { component: mockComponent, onSave } });

      // Wait for JSON editor to appear
      await waitFor(() => {
        expect(
          screen.queryByRole("textbox", { name: /configuration/i }),
        ).toBeInTheDocument();
      });

      // Edit config
      const textarea = screen.getByRole("textbox", { name: /configuration/i });
      const newConfig = { port: 8080 };
      await fireEvent.input(textarea, {
        target: { value: JSON.stringify(newConfig) },
      });

      // Save
      const saveButton = screen.getByRole("button", { name: /apply/i });
      await fireEvent.click(saveButton);

      expect(onSave).toHaveBeenCalledWith("node-1", newConfig);

      // Dirty state should be cleared (config now matches saved state)
      // Verify by checking if textarea value matches the saved config
      const textareaAfter = screen.getByRole("textbox", {
        name: /configuration/i,
      }) as HTMLTextAreaElement;
      expect(JSON.parse(textareaAfter.value)).toEqual(newConfig);
    });
  });
});
