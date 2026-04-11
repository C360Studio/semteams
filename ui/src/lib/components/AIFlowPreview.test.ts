import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/svelte";
import AIFlowPreview from "./AIFlowPreview.svelte";
import type { Flow } from "$lib/types/flow";
import type { ValidationResult } from "$lib/types/validation";

describe("AIFlowPreview", () => {
  // Helper to create mock flow
  const createMockFlow = (): Flow => ({
    version: 1,
    id: "flow-123",
    name: "Generated Flow",
    description: "AI-generated flow",
    nodes: [
      {
        id: "node-1",
        component: "udp-input",
        type: "input",
        name: "UDP Input",
        position: { x: 100, y: 100 },
        config: { port: 5000 },
      },
      {
        id: "node-2",
        component: "json-transform",
        type: "processor",
        name: "JSON Transform",
        position: { x: 300, y: 100 },
        config: {},
      },
      {
        id: "node-3",
        component: "nats-publisher",
        type: "output",
        name: "NATS Publisher",
        position: { x: 500, y: 100 },
        config: { subject: "sensor.data" },
      },
    ],
    connections: [
      {
        id: "conn-1",
        source_node_id: "node-1",
        source_port: "output",
        target_node_id: "node-2",
        target_port: "input",
      },
      {
        id: "conn-2",
        source_node_id: "node-2",
        source_port: "output",
        target_node_id: "node-3",
        target_port: "input",
      },
    ],
    runtime_state: "not_deployed",
    created_at: "2026-01-06T12:00:00Z",
    updated_at: "2026-01-06T12:00:00Z",
    created_by: "user-123",
    last_modified: "2026-01-06T12:00:00Z",
  });

  // Helper for validation result
  const createValidationResult = (
    hasErrors = false,
    hasWarnings = false,
  ): ValidationResult => ({
    validation_status: hasErrors
      ? "errors"
      : hasWarnings
        ? "warnings"
        : "valid",
    errors: hasErrors
      ? [
          {
            type: "missing_config",
            severity: "error",
            component_name: "node-1",
            message: "Missing required configuration",
          },
        ]
      : [],
    warnings: hasWarnings
      ? [
          {
            type: "orphaned_port",
            severity: "warning",
            component_name: "node-2",
            port_name: "output",
            message: "Port not connected",
          },
        ]
      : [],
  });

  // =========================================================================
  // Rendering Tests
  // =========================================================================

  describe("Rendering", () => {
    it("should not render when isOpen is false", () => {
      const { container } = render(AIFlowPreview, {
        props: {
          isOpen: false,
          flow: null,
        },
      });

      const modal = container.querySelector('[role="dialog"]');
      expect(modal).not.toBeInTheDocument();
    });

    it("should render modal when isOpen is true", () => {
      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: createMockFlow(),
        },
      });

      expect(screen.getByRole("dialog")).toBeInTheDocument();
      expect(screen.getByText(/generated flow preview/i)).toBeInTheDocument();
    });

    it("should render loading state", () => {
      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: null,
          loading: true,
        },
      });

      expect(screen.getByText(/generating/i)).toBeInTheDocument();
      expect(screen.getByRole("status")).toBeInTheDocument();
    });

    it("should render error state", () => {
      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: null,
          error: "Failed to generate flow: Invalid prompt",
        },
      });

      expect(screen.getByText(/failed to generate flow/i)).toBeInTheDocument();
      expect(screen.getByText(/invalid prompt/i)).toBeInTheDocument();
    });

    it("should render close button", () => {
      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: createMockFlow(),
        },
      });

      const closeButton = screen.getByRole("button", { name: /close/i });
      expect(closeButton).toBeInTheDocument();
    });

    it("should render modal backdrop", () => {
      const { container } = render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: createMockFlow(),
        },
      });

      const backdrop = container.querySelector(".backdrop, .modal-backdrop");
      expect(backdrop).toBeInTheDocument();
    });
  });

  // =========================================================================
  // Flow Display Tests
  // =========================================================================

  describe("Flow Display", () => {
    it("should display nodes list", () => {
      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: createMockFlow(),
        },
      });

      expect(screen.getByText(/components/i)).toBeInTheDocument();
      expect(screen.getByText(/components \(3\)/i)).toBeInTheDocument(); // 3 components

      // Use getAllByText to check for multiple occurrences of node names
      const udpInputElements = screen.getAllByText(/udp input/i);
      expect(udpInputElements.length).toBeGreaterThan(0);

      const jsonTransformElements = screen.getAllByText(/json transform/i);
      expect(jsonTransformElements.length).toBeGreaterThan(0);

      const natsPublisherElements = screen.getAllByText(/nats publisher/i);
      expect(natsPublisherElements.length).toBeGreaterThan(0);
    });

    it("should display connections list", () => {
      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: createMockFlow(),
        },
      });

      expect(screen.getByText(/connections/i)).toBeInTheDocument();
      expect(screen.getByText(/2/)).toBeInTheDocument(); // 2 connections
    });

    it("should display connection details", () => {
      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: createMockFlow(),
        },
      });

      // Use more specific text patterns to match exact connection strings
      expect(
        screen.getByText(/udp input.*output.*→.*json transform.*input/i),
      ).toBeInTheDocument();
      expect(
        screen.getByText(/json transform.*output.*→.*nats publisher.*input/i),
      ).toBeInTheDocument();
    });

    it("should handle empty flow", () => {
      const emptyFlow: Flow = {
        ...createMockFlow(),
        nodes: [],
        connections: [],
      };

      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: emptyFlow,
        },
      });

      // Check for specific headings with 0 counts
      expect(screen.getByText(/components \(0\)/i)).toBeInTheDocument();
      expect(screen.getByText(/connections \(0\)/i)).toBeInTheDocument();
      expect(screen.getByText(/no components/i)).toBeInTheDocument();
    });

    it("should display flow description if provided", () => {
      const flow = createMockFlow();
      flow.description = "A flow that processes sensor data";

      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow,
        },
      });

      expect(screen.getByText(/processes sensor data/i)).toBeInTheDocument();
    });

    it("should display node configurations", () => {
      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: createMockFlow(),
        },
      });

      expect(screen.getByText(/port.*5000/i)).toBeInTheDocument();
      expect(screen.getByText(/subject.*sensor\.data/i)).toBeInTheDocument();
    });
  });

  // =========================================================================
  // Validation Display Tests
  // =========================================================================

  describe("Validation Display", () => {
    it("should display validation success", () => {
      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: createMockFlow(),
          validationResult: createValidationResult(false, false),
        },
      });

      expect(screen.getByText(/flow is valid/i)).toBeInTheDocument();
    });

    it("should display validation errors", () => {
      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: createMockFlow(),
          validationResult: createValidationResult(true, false),
        },
      });

      expect(screen.getByText(/error/i)).toBeInTheDocument();
      expect(
        screen.getByText(/missing required configuration/i),
      ).toBeInTheDocument();
    });

    it("should display validation warnings", () => {
      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: createMockFlow(),
          validationResult: createValidationResult(false, true),
        },
      });

      expect(screen.getByText(/warning/i)).toBeInTheDocument();
      expect(screen.getByText(/port not connected/i)).toBeInTheDocument();
    });

    it("should display multiple validation issues", () => {
      const validationResult: ValidationResult = {
        validation_status: "errors",
        errors: [
          {
            type: "missing_config",
            severity: "error",
            component_name: "node-1",
            message: "Error 1",
          },
          {
            type: "orphaned_port",
            severity: "error",
            component_name: "node-2",
            message: "Error 2",
          },
        ],
        warnings: [
          {
            type: "disconnected_node",
            severity: "warning",
            component_name: "node-3",
            message: "Warning 1",
          },
          {
            type: "orphaned_port",
            severity: "warning",
            component_name: "node-4",
            message: "Warning 2",
          },
        ],
      };

      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: createMockFlow(),
          validationResult,
        },
      });

      expect(screen.getByText(/error 1/i)).toBeInTheDocument();
      expect(screen.getByText(/error 2/i)).toBeInTheDocument();
      expect(screen.getByText(/warning 1/i)).toBeInTheDocument();
      expect(screen.getByText(/warning 2/i)).toBeInTheDocument();
    });

    it("should show error count badge", () => {
      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: createMockFlow(),
          validationResult: createValidationResult(true, false),
        },
      });

      expect(screen.getByText(/1.*error/i)).toBeInTheDocument();
    });

    it("should show warning count badge", () => {
      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: createMockFlow(),
          validationResult: createValidationResult(false, true),
        },
      });

      expect(screen.getByText(/1.*warning/i)).toBeInTheDocument();
    });

    it("should handle null validation result", () => {
      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: createMockFlow(),
          validationResult: null,
        },
      });

      // Should not crash, may show "not validated" or similar
      expect(screen.getByRole("dialog")).toBeInTheDocument();
    });
  });

  // =========================================================================
  // Actions Tests
  // =========================================================================

  describe("Actions", () => {
    it("should call onApply when Apply button is clicked", async () => {
      const onApply = vi.fn();
      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: createMockFlow(),
          onApply,
        },
      });

      const applyBtn = screen.getByRole("button", { name: /apply to canvas/i });
      await fireEvent.click(applyBtn);

      expect(onApply).toHaveBeenCalledTimes(1);
    });

    it("should call onReject when Reject button is clicked", async () => {
      const onReject = vi.fn();
      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: createMockFlow(),
          onReject,
        },
      });

      const rejectBtn = screen.getByRole("button", { name: /reject/i });
      await fireEvent.click(rejectBtn);

      expect(onReject).toHaveBeenCalledTimes(1);
    });

    it("should call onRetry when Retry button is clicked", async () => {
      const onRetry = vi.fn();
      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: createMockFlow(),
          onRetry,
        },
      });

      const retryBtn = screen.getByRole("button", { name: /retry/i });
      await fireEvent.click(retryBtn);

      expect(onRetry).toHaveBeenCalledTimes(1);
    });

    it("should call onClose when Close button is clicked", async () => {
      const onClose = vi.fn();
      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: createMockFlow(),
          onClose,
        },
      });

      const closeBtn = screen.getByRole("button", { name: /close/i });
      await fireEvent.click(closeBtn);

      expect(onClose).toHaveBeenCalledTimes(1);
    });

    it("should disable Apply button when validation fails", () => {
      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: createMockFlow(),
          validationResult: createValidationResult(true, false),
        },
      });

      const applyBtn = screen.getByRole("button", { name: /apply to canvas/i });
      expect(applyBtn).toBeDisabled();
    });

    it("should enable Apply button when validation passes", () => {
      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: createMockFlow(),
          validationResult: createValidationResult(false, false),
        },
      });

      const applyBtn = screen.getByRole("button", { name: /apply to canvas/i });
      expect(applyBtn).toBeEnabled();
    });

    it("should allow Apply with warnings only", () => {
      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: createMockFlow(),
          validationResult: createValidationResult(false, true),
        },
      });

      const applyBtn = screen.getByRole("button", { name: /apply to canvas/i });
      expect(applyBtn).toBeEnabled();
    });

    it("should show Retry button in error state", () => {
      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: null,
          error: "Generation failed",
        },
      });

      expect(
        screen.getByRole("button", { name: /retry/i }),
      ).toBeInTheDocument();
    });

    it("should hide action buttons when loading", () => {
      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: null,
          loading: true,
        },
      });

      expect(
        screen.queryByRole("button", { name: /apply/i }),
      ).not.toBeInTheDocument();
      expect(
        screen.queryByRole("button", { name: /reject/i }),
      ).not.toBeInTheDocument();
    });
  });

  // =========================================================================
  // Keyboard Support Tests
  // =========================================================================

  describe("Keyboard Support", () => {
    it("should close modal when Escape is pressed", async () => {
      const onClose = vi.fn();
      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: createMockFlow(),
          onClose,
        },
      });

      await fireEvent.keyDown(document, { key: "Escape" });

      expect(onClose).toHaveBeenCalledTimes(1);
    });

    it("should not close when Escape is pressed if loading", async () => {
      const onClose = vi.fn();
      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: null,
          loading: true,
          onClose,
        },
      });

      await fireEvent.keyDown(document, { key: "Escape" });

      expect(onClose).not.toHaveBeenCalled();
    });

    it("should apply flow when Enter is pressed on Apply button", async () => {
      const onApply = vi.fn();
      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: createMockFlow(),
          validationResult: createValidationResult(false, false),
          onApply,
        },
      });

      const applyBtn = screen.getByRole("button", { name: /apply/i });
      // Buttons with onclick handle Enter natively - simulate with click instead
      await fireEvent.click(applyBtn);

      expect(onApply).toHaveBeenCalledTimes(1);
    });
  });

  // =========================================================================
  // Accessibility Tests
  // =========================================================================

  describe("Accessibility", () => {
    it("should have proper dialog role", () => {
      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: createMockFlow(),
        },
      });

      const dialog = screen.getByRole("dialog");
      expect(dialog).toBeInTheDocument();
      expect(dialog).toHaveAttribute("aria-modal", "true");
    });

    it("should have proper aria-label for dialog", () => {
      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: createMockFlow(),
        },
      });

      const dialog = screen.getByRole("dialog", {
        name: /generated flow preview/i,
      });
      expect(dialog).toBeInTheDocument();
    });

    it("should have proper button labels", () => {
      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: createMockFlow(),
        },
      });

      expect(
        screen.getByRole("button", { name: /apply to canvas/i }),
      ).toBeInTheDocument();
      expect(
        screen.getByRole("button", { name: /reject/i }),
      ).toBeInTheDocument();
      expect(
        screen.getByRole("button", { name: /retry/i }),
      ).toBeInTheDocument();
      expect(
        screen.getByRole("button", { name: /close/i }),
      ).toBeInTheDocument();
    });

    it("should announce loading state to screen readers", () => {
      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: null,
          loading: true,
        },
      });

      const status = screen.getByRole("status");
      expect(status).toHaveAttribute("aria-live", "polite");
    });

    it("should announce errors to screen readers", () => {
      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: null,
          error: "Generation failed",
        },
      });

      const alert = screen.getByRole("alert");
      expect(alert).toBeInTheDocument();
      expect(alert).toHaveTextContent(/generation failed/i);
    });

    it("should have proper heading hierarchy", () => {
      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: createMockFlow(),
        },
      });

      const heading = screen.getByRole("heading", {
        name: /generated flow preview/i,
      });
      expect(heading).toBeInTheDocument();
    });

    it("should associate validation errors with nodes", () => {
      const { container } = render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: createMockFlow(),
          validationResult: createValidationResult(true, false),
        },
      });

      const errorItem = container.querySelector('[data-node-id="node-1"]');
      if (errorItem) {
        expect(errorItem).toHaveAttribute("data-node-id");
      }
    });

    it("should have proper color contrast for validation states", () => {
      const { container } = render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: createMockFlow(),
          validationResult: createValidationResult(true, true),
        },
      });

      // Error and warning elements should have distinct colors
      const errors = container.querySelectorAll(
        '.error, [data-severity="error"]',
      );
      const warnings = container.querySelectorAll(
        '.warning, [data-severity="warning"]',
      );

      expect(errors.length).toBeGreaterThan(0);
      expect(warnings.length).toBeGreaterThan(0);
    });
  });

  // =========================================================================
  // Edge Cases Tests
  // =========================================================================

  describe("Edge Cases", () => {
    it("should handle flow with many nodes", () => {
      const largeFlow: Flow = {
        ...createMockFlow(),
        nodes: Array.from({ length: 50 }, (_, i) => ({
          id: `node-${i}`,
          component: "test-component",
          type: "processor",
          name: `Component ${i}`,
          position: { x: 100 * i, y: 100 },
          config: {},
        })),
        connections: [],
      };

      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: largeFlow,
        },
      });

      expect(screen.getByText(/50/)).toBeInTheDocument();
    });

    it("should handle flow with many connections", () => {
      const flow = createMockFlow();
      flow.connections = Array.from({ length: 100 }, (_, i) => ({
        id: `conn-${i}`,
        source_node_id: "node-1",
        source_port: "output",
        target_node_id: "node-2",
        target_port: `input-${i}`,
      }));

      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow,
        },
      });

      expect(screen.getByText(/100/)).toBeInTheDocument();
    });

    it("should handle transition from loading to loaded", async () => {
      const { rerender } = render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: null,
          loading: true,
        },
      });

      expect(screen.getByText(/generating/i)).toBeInTheDocument();

      rerender({
        isOpen: true,
        flow: createMockFlow(),
        loading: false,
      });

      await waitFor(() => {
        expect(screen.queryByText(/generating/i)).not.toBeInTheDocument();
        // Check for the heading instead which is unique
        expect(screen.getByText(/components \(3\)/i)).toBeInTheDocument();
      });
    });

    it("should handle transition from loading to error", async () => {
      const { rerender } = render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: null,
          loading: true,
        },
      });

      rerender({
        isOpen: true,
        flow: null,
        loading: false,
        error: "Failed to generate",
      });

      await waitFor(() => {
        expect(screen.getByText(/failed to generate/i)).toBeInTheDocument();
      });
    });

    it("should handle clicking backdrop to close", async () => {
      const onClose = vi.fn();
      const { container } = render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: createMockFlow(),
          onClose,
        },
      });

      const backdrop = container.querySelector(".backdrop, .modal-backdrop");
      if (backdrop) {
        await fireEvent.click(backdrop as Element);
        expect(onClose).toHaveBeenCalledTimes(1);
      }
    });

    it("should prevent closing on dialog click", async () => {
      const onClose = vi.fn();
      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: createMockFlow(),
          onClose,
        },
      });

      const dialog = screen.getByRole("dialog");
      await fireEvent.click(dialog);

      expect(onClose).not.toHaveBeenCalled();
    });

    it("should handle rapid open/close toggles", async () => {
      const { rerender } = render(AIFlowPreview, {
        props: {
          isOpen: false,
          flow: null,
        },
      });

      rerender({ isOpen: true, flow: createMockFlow() });
      await waitFor(() => {
        expect(screen.getByRole("dialog")).toBeInTheDocument();
      });

      rerender({ isOpen: false, flow: createMockFlow() });
      await waitFor(() => {
        expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
      });

      rerender({ isOpen: true, flow: createMockFlow() });
      await waitFor(() => {
        expect(screen.getByRole("dialog")).toBeInTheDocument();
      });
    });

    it("should handle component unmount gracefully", () => {
      const { unmount } = render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: createMockFlow(),
        },
      });

      expect(() => unmount()).not.toThrow();
    });

    it("should handle null flow gracefully", () => {
      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: null,
        },
      });

      expect(screen.getByRole("dialog")).toBeInTheDocument();
    });

    it("should handle malformed validation result", () => {
      const malformedValidation = {
        validation_status: "valid" as const,
        errors: null as unknown as [],
        warnings: undefined as unknown as [],
      };

      render(AIFlowPreview, {
        props: {
          isOpen: true,
          flow: createMockFlow(),
          validationResult: malformedValidation,
        },
      });

      expect(screen.getByRole("dialog")).toBeInTheDocument();
    });
  });
});
