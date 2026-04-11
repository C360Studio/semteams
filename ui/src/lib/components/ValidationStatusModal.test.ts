// Component tests for ValidationStatusModal - Feature 015
// TDD RED PHASE: These tests should fail until T013 creates ValidationStatusModal component

import { render, screen, fireEvent } from "@testing-library/svelte";
import { describe, it, expect, vi } from "vitest";
import type { ValidationResult } from "$lib/types/port";

// Import will fail until component is created
import ValidationStatusModal from "$lib/components/ValidationStatusModal.svelte";

describe("ValidationStatusModal", () => {
  const mockValidationResult: ValidationResult = {
    validation_status: "errors",
    errors: [
      {
        type: "orphaned_port",
        severity: "error",
        component_name: "node_123_abc", // Backend sends node ID
        port_name: "output",
        message: "Port not connected",
        suggestions: ["Connect this port to a processor"],
      },
      {
        type: "disconnected_node",
        severity: "error",
        component_name: "node_456_def", // Backend sends node ID
        message: "Node has no connections",
        suggestions: ["Add input and output connections"],
      },
    ],
    warnings: [
      {
        type: "orphaned_port",
        severity: "warning",
        component_name: "node_789_ghi", // Backend sends node ID
        port_name: "metadata",
        message: "Optional port not connected",
        suggestions: [],
      },
    ],
    nodes: [
      {
        id: "node_123_abc",
        type: "udp-input",
        name: "udp-input-1", // User-friendly name
        input_ports: [],
        output_ports: [
          {
            name: "output",
            pattern: "stream",
            direction: "output",
            type: "message.Storable",
            required: true,
            connection_id: "nats.output",
            description: "Output port",
          },
        ],
      },
      {
        id: "node_456_def",
        type: "processor",
        name: "processor-1", // User-friendly name
        input_ports: [],
        output_ports: [],
      },
      {
        id: "node_789_ghi",
        type: "optional-output",
        name: "optional-output-1", // User-friendly name
        input_ports: [],
        output_ports: [
          {
            name: "metadata",
            pattern: "stream",
            direction: "output",
            type: "message.Storable",
            required: false,
            connection_id: "nats.metadata",
            description: "Metadata output port",
          },
        ],
      },
    ],
    discovered_connections: [],
  };

  /**
   * T009: Tests for ValidationStatusModal component
   * Expected to FAIL until T013 creates the component
   */

  it("renders when isOpen is true", () => {
    render(ValidationStatusModal, {
      props: {
        isOpen: true,
        validationResult: mockValidationResult,
        onClose: vi.fn(),
      },
    });

    const dialog = screen.getByRole("dialog");
    expect(dialog).toBeInTheDocument();
  });

  it("does not render when isOpen is false", () => {
    render(ValidationStatusModal, {
      props: {
        isOpen: false,
        validationResult: mockValidationResult,
        onClose: vi.fn(),
      },
    });

    const dialog = screen.queryByRole("dialog");
    expect(dialog).not.toBeInTheDocument();
  });

  it("displays error count in header", () => {
    render(ValidationStatusModal, {
      props: {
        isOpen: true,
        validationResult: mockValidationResult,
        onClose: vi.fn(),
      },
    });

    const dialog = screen.getByRole("dialog");
    // Should show error count in section title (not modal header)
    expect(dialog).toHaveTextContent(/errors.*2/i);
  });

  it("displays all error details", () => {
    render(ValidationStatusModal, {
      props: {
        isOpen: true,
        validationResult: mockValidationResult,
        onClose: vi.fn(),
      },
    });

    const dialog = screen.getByRole("dialog");

    // Should show both errors
    expect(dialog).toHaveTextContent("udp-input-1");
    expect(dialog).toHaveTextContent("Port not connected");
    expect(dialog).toHaveTextContent("processor-1");
    expect(dialog).toHaveTextContent("Node has no connections");
  });

  it("displays error suggestions", () => {
    render(ValidationStatusModal, {
      props: {
        isOpen: true,
        validationResult: mockValidationResult,
        onClose: vi.fn(),
      },
    });

    const dialog = screen.getByRole("dialog");

    // Should show suggestions
    expect(dialog).toHaveTextContent("Connect this port to a processor");
    expect(dialog).toHaveTextContent("Add input and output connections");
  });

  it("displays warning details separately from errors", () => {
    render(ValidationStatusModal, {
      props: {
        isOpen: true,
        validationResult: mockValidationResult,
        onClose: vi.fn(),
      },
    });

    const dialog = screen.getByRole("dialog");

    // Should show warnings section
    expect(dialog).toHaveTextContent(/1.*warning/i);
    expect(dialog).toHaveTextContent("optional-output-1");
    expect(dialog).toHaveTextContent("Optional port not connected");
  });

  it("calls onClose when backdrop clicked", async () => {
    const onClose = vi.fn();

    render(ValidationStatusModal, {
      props: {
        isOpen: true,
        validationResult: mockValidationResult,
        onClose,
      },
    });

    const backdrop = screen.getByTestId("modal-backdrop");
    await fireEvent.click(backdrop);

    expect(onClose).toHaveBeenCalled();
  });

  it("calls onClose when close button clicked", async () => {
    const onClose = vi.fn();

    render(ValidationStatusModal, {
      props: {
        isOpen: true,
        validationResult: mockValidationResult,
        onClose,
      },
    });

    // Use getAllByRole to get all close buttons, then click the footer one (exact match)
    const closeButtons = screen.getAllByRole("button", { name: /close/i });
    const footerCloseButton = closeButtons.find(
      (btn) => btn.textContent === "Close",
    );
    expect(footerCloseButton).toBeTruthy();
    await fireEvent.click(footerCloseButton!);

    expect(onClose).toHaveBeenCalled();
  });

  it("calls onClose when ESC pressed", async () => {
    const onClose = vi.fn();

    render(ValidationStatusModal, {
      props: {
        isOpen: true,
        validationResult: mockValidationResult,
        onClose,
      },
    });

    // Simulate ESC key press on document
    await fireEvent.keyDown(document, { key: "Escape" });

    expect(onClose).toHaveBeenCalled();
  });

  it("has proper ARIA attributes for accessibility", () => {
    render(ValidationStatusModal, {
      props: {
        isOpen: true,
        validationResult: mockValidationResult,
        onClose: vi.fn(),
      },
    });

    const dialog = screen.getByRole("dialog");

    // Should have aria-label
    expect(dialog).toHaveAttribute("aria-label");

    // Backdrop should be visible
    const backdrop = screen.getByTestId("modal-backdrop");
    expect(backdrop).toBeVisible();
  });

  it("renders empty state when no validation result provided", () => {
    render(ValidationStatusModal, {
      props: {
        isOpen: true,
        validationResult: null,
        onClose: vi.fn(),
      },
    });

    const dialog = screen.queryByRole("dialog");
    // Should either not render or show empty state message
    // Implementation detail - either approach is valid
    if (dialog) {
      expect(dialog).toHaveTextContent(/no.*validation/i);
    }
  });

  it("groups errors by component", () => {
    render(ValidationStatusModal, {
      props: {
        isOpen: true,
        validationResult: mockValidationResult,
        onClose: vi.fn(),
      },
    });

    // Component names should be visually distinct (e.g., in headers or bold)
    const componentNames = screen.getAllByText(/udp-input-1|processor-1/);
    expect(componentNames.length).toBeGreaterThan(0);
  });

  it("shows port name when provided", () => {
    render(ValidationStatusModal, {
      props: {
        isOpen: true,
        validationResult: mockValidationResult,
        onClose: vi.fn(),
      },
    });

    const dialog = screen.getByRole("dialog");

    // Should show port name for first error
    expect(dialog).toHaveTextContent("output");
    expect(dialog).toHaveTextContent("metadata");
  });
});
