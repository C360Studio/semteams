import { describe, it, expect, vi } from "vitest";
import { render, fireEvent } from "@testing-library/svelte";
import { tick } from "svelte";
import ValidationErrorDialog from "./ValidationErrorDialog.svelte";
import type { ValidationResult } from "$lib/types/validation";

describe("ValidationErrorDialog", () => {
  const mockValidationResult: ValidationResult = {
    validation_status: "errors",
    errors: [
      {
        type: "orphaned_port",
        severity: "error",
        component_name: "udp-1",
        port_name: "nats_output",
        message: "Required output port has no subscribers",
        suggestions: [
          "Connect to a processor component",
          "Add WebSocket Output to view data",
        ],
      },
      {
        type: "orphaned_port",
        severity: "error",
        component_name: "graph-proc-1",
        port_name: "storage.*.events",
        message: "Required input port has no publishers",
        suggestions: ["Connect an output from another component"],
      },
    ],
    warnings: [],
  };

  const mockValidationResultWithWarnings: ValidationResult = {
    validation_status: "warnings",
    errors: [],
    warnings: [
      {
        type: "orphaned_port",
        severity: "warning",
        component_name: "robotics-1",
        port_name: "storage_write",
        message: "Optional output port has no subscribers",
        suggestions: ["This port is optional and can remain unconnected"],
      },
    ],
  };

  // Test 1: DOM Output - Renders validation errors correctly
  describe("DOM Output", () => {
    it("should render error messages grouped by component", () => {
      const { container } = render(ValidationErrorDialog, {
        props: {
          isOpen: true,
          validationResult: mockValidationResult,
          onClose: vi.fn(),
        },
      });

      // Verify dialog is visible
      const dialog = container.querySelector('[role="dialog"]');
      expect(dialog).toBeInTheDocument();

      // Verify title
      expect(container.querySelector("#dialog-title")).toHaveTextContent(
        "Flow Validation Failed",
      );

      // Verify error count
      expect(container).toHaveTextContent("Errors (2)");

      // Verify component grouping
      expect(container).toHaveTextContent("udp-1");
      expect(container).toHaveTextContent("graph-proc-1");

      // Verify error messages
      expect(container).toHaveTextContent(
        "Required output port has no subscribers",
      );
      expect(container).toHaveTextContent(
        "Required input port has no publishers",
      );

      // Verify port names displayed
      expect(container).toHaveTextContent("nats_output:");
      expect(container).toHaveTextContent("storage.*.events:");
    });

    it("should render suggestions when present", () => {
      const { container } = render(ValidationErrorDialog, {
        props: {
          isOpen: true,
          validationResult: mockValidationResult,
          onClose: vi.fn(),
        },
      });

      // Verify suggestions section exists
      expect(container).toHaveTextContent("Suggestions:");

      // Verify suggestion content
      expect(container).toHaveTextContent("Connect to a processor component");
      expect(container).toHaveTextContent("Add WebSocket Output to view data");
      expect(container).toHaveTextContent(
        "Connect an output from another component",
      );
    });

    it("should render warnings section when present", () => {
      const { container } = render(ValidationErrorDialog, {
        props: {
          isOpen: true,
          validationResult: mockValidationResultWithWarnings,
          onClose: vi.fn(),
        },
      });

      // Verify warnings section
      expect(container).toHaveTextContent("Warnings (1)");
      expect(container).toHaveTextContent("robotics-1");
      expect(container).toHaveTextContent(
        "Optional output port has no subscribers",
      );
    });

    it("should render both errors and warnings when both present", () => {
      const mixedResult: ValidationResult = {
        validation_status: "errors",
        errors: mockValidationResult.errors,
        warnings: mockValidationResultWithWarnings.warnings,
      };

      const { container } = render(ValidationErrorDialog, {
        props: {
          isOpen: true,
          validationResult: mixedResult,
          onClose: vi.fn(),
        },
      });

      // Verify both sections exist
      expect(container).toHaveTextContent("Errors (2)");
      expect(container).toHaveTextContent("Warnings (1)");
    });
  });

  // Test 2: Interaction - Button callbacks
  describe("User Interactions", () => {
    it("should call onClose when X close button clicked", async () => {
      const onClose = vi.fn();
      const { container } = render(ValidationErrorDialog, {
        props: {
          isOpen: true,
          validationResult: mockValidationResult,
          onClose,
        },
      });

      const closeButton = container.querySelector(".close-button");
      expect(closeButton).toBeInTheDocument();

      await fireEvent.click(closeButton!);

      expect(onClose).toHaveBeenCalledTimes(1);
    });

    it("should call onClose when primary button clicked", async () => {
      const onClose = vi.fn();
      const { container } = render(ValidationErrorDialog, {
        props: {
          isOpen: true,
          validationResult: mockValidationResult,
          onClose,
        },
      });

      const primaryButton = container.querySelector(".primary-button");
      expect(primaryButton).toBeInTheDocument();
      expect(primaryButton).toHaveTextContent("Close and Edit Flow");

      await fireEvent.click(primaryButton!);

      expect(onClose).toHaveBeenCalledTimes(1);
    });

    it("should call onClose when clicking outside dialog (background)", async () => {
      const onClose = vi.fn();
      const { container } = render(ValidationErrorDialog, {
        props: {
          isOpen: true,
          validationResult: mockValidationResult,
          onClose,
        },
      });

      const overlay = container.querySelector(".dialog-overlay");
      expect(overlay).toBeInTheDocument();

      // Click on overlay background (not dialog content)
      await fireEvent.click(overlay!);

      expect(onClose).toHaveBeenCalledTimes(1);
    });

    it("should NOT close when clicking inside dialog content", async () => {
      const onClose = vi.fn();
      const { container } = render(ValidationErrorDialog, {
        props: {
          isOpen: true,
          validationResult: mockValidationResult,
          onClose,
        },
      });

      const dialogContent = container.querySelector(".dialog-content");
      expect(dialogContent).toBeInTheDocument();

      // Click inside dialog content
      await fireEvent.click(dialogContent!);

      // Should not call onClose (event should not bubble)
      expect(onClose).not.toHaveBeenCalled();
    });
  });

  // Test 3: Reactivity - Dialog visibility
  describe("Reactivity", () => {
    it("should not render when isOpen is false", () => {
      const { container } = render(ValidationErrorDialog, {
        props: {
          isOpen: false,
          validationResult: mockValidationResult,
          onClose: vi.fn(),
        },
      });

      expect(
        container.querySelector('[role="dialog"]'),
      ).not.toBeInTheDocument();
    });

    it("should not render when validationResult is null", () => {
      const { container } = render(ValidationErrorDialog, {
        props: {
          isOpen: true,
          validationResult: null,
          onClose: vi.fn(),
        },
      });

      expect(
        container.querySelector('[role="dialog"]'),
      ).not.toBeInTheDocument();
    });

    it("should update when validationResult changes", async () => {
      const { container, rerender } = render(ValidationErrorDialog, {
        props: {
          isOpen: true,
          validationResult: mockValidationResult,
          onClose: vi.fn(),
        },
      });

      // Verify initial state
      expect(container).toHaveTextContent("udp-1");
      expect(container).toHaveTextContent("Errors (2)");

      // Update to warnings only (Svelte 5 pattern - use rerender instead of $set)
      await rerender({ validationResult: mockValidationResultWithWarnings });
      await tick();

      // Verify updated state
      expect(container).toHaveTextContent("robotics-1");
      expect(container).toHaveTextContent("Warnings (1)");
      expect(container).not.toHaveTextContent("Errors");
    });
  });

  // Test 4: Keyboard Navigation
  describe("Keyboard Navigation", () => {
    it("should close dialog when ESC key pressed", async () => {
      const onClose = vi.fn();
      render(ValidationErrorDialog, {
        props: {
          isOpen: true,
          validationResult: mockValidationResult,
          onClose,
        },
      });

      // Simulate ESC key press
      const escEvent = new KeyboardEvent("keydown", { key: "Escape" });
      window.dispatchEvent(escEvent);

      await tick();

      expect(onClose).toHaveBeenCalledTimes(1);
    });

    it("should NOT close when other keys pressed", async () => {
      const onClose = vi.fn();
      render(ValidationErrorDialog, {
        props: {
          isOpen: true,
          validationResult: mockValidationResult,
          onClose,
        },
      });

      // Simulate other key presses
      const enterEvent = new KeyboardEvent("keydown", { key: "Enter" });
      window.dispatchEvent(enterEvent);

      const spaceEvent = new KeyboardEvent("keydown", { key: " " });
      window.dispatchEvent(spaceEvent);

      await tick();

      expect(onClose).not.toHaveBeenCalled();
    });

    it("should NOT trigger ESC handler when dialog is closed", async () => {
      const onClose = vi.fn();
      render(ValidationErrorDialog, {
        props: {
          isOpen: false, // Dialog closed
          validationResult: mockValidationResult,
          onClose,
        },
      });

      // Simulate ESC key press
      const escEvent = new KeyboardEvent("keydown", { key: "Escape" });
      window.dispatchEvent(escEvent);

      await tick();

      // Should not call onClose since dialog is already closed
      expect(onClose).not.toHaveBeenCalled();
    });
  });

  // Test 5: Accessibility
  describe("Accessibility", () => {
    it("should have proper ARIA attributes", () => {
      const { container } = render(ValidationErrorDialog, {
        props: {
          isOpen: true,
          validationResult: mockValidationResult,
          onClose: vi.fn(),
        },
      });

      const dialog = container.querySelector('[role="dialog"]');
      expect(dialog).toHaveAttribute("aria-modal", "true");
      expect(dialog).toHaveAttribute("aria-labelledby", "dialog-title");

      const title = container.querySelector("#dialog-title");
      expect(title).toBeInTheDocument();
      expect(title).toHaveTextContent("Flow Validation Failed");
    });

    it("should have aria-label on close button", () => {
      const { container } = render(ValidationErrorDialog, {
        props: {
          isOpen: true,
          validationResult: mockValidationResult,
          onClose: vi.fn(),
        },
      });

      const closeButton = container.querySelector(".close-button");
      expect(closeButton).toHaveAttribute("aria-label", "Close dialog");
    });

    it("should have proper heading hierarchy", () => {
      const { container } = render(ValidationErrorDialog, {
        props: {
          isOpen: true,
          validationResult: mockValidationResult,
          onClose: vi.fn(),
        },
      });

      // h2 for dialog title
      const h2 = container.querySelector("h2");
      expect(h2).toBeInTheDocument();
      expect(h2).toHaveTextContent("Flow Validation Failed");

      // h3 for section titles
      const h3 = container.querySelector("h3");
      expect(h3).toBeInTheDocument();
      expect(h3).toHaveTextContent("Errors");

      // h4 for component names
      const h4Elements = container.querySelectorAll("h4");
      expect(h4Elements.length).toBeGreaterThan(0);
    });
  });

  // Test 6: Edge Cases
  describe("Edge Cases", () => {
    it("should handle empty errors and warnings arrays", () => {
      const emptyResult: ValidationResult = {
        validation_status: "valid",
        errors: [],
        warnings: [],
      };

      const { container } = render(ValidationErrorDialog, {
        props: {
          isOpen: true,
          validationResult: emptyResult,
          onClose: vi.fn(),
        },
      });

      // Dialog should still render but with no errors/warnings sections
      expect(container.querySelector('[role="dialog"]')).toBeInTheDocument();
      expect(container).not.toHaveTextContent("Errors");
      expect(container).not.toHaveTextContent("Warnings");
    });

    it("should handle issue without suggestions", () => {
      const noSuggestionsResult: ValidationResult = {
        validation_status: "errors",
        errors: [
          {
            type: "unknown_component",
            severity: "error",
            component_name: "unknown-1",
            message: "Unknown component type",
            suggestions: undefined, // No suggestions
          },
        ],
        warnings: [],
      };

      const { container } = render(ValidationErrorDialog, {
        props: {
          isOpen: true,
          validationResult: noSuggestionsResult,
          onClose: vi.fn(),
        },
      });

      // Should render without suggestions section
      expect(container).toHaveTextContent("Unknown component type");
      expect(container).not.toHaveTextContent("Suggestions:");
    });

    it("should handle issue without port_name", () => {
      const noPortResult: ValidationResult = {
        validation_status: "errors",
        errors: [
          {
            type: "disconnected_node",
            severity: "error",
            component_name: "isolated-1",
            message: "Component is not connected to flow",
            suggestions: [],
          },
        ],
        warnings: [],
      };

      const { container } = render(ValidationErrorDialog, {
        props: {
          isOpen: true,
          validationResult: noPortResult,
          onClose: vi.fn(),
        },
      });

      // Should render message without port name
      expect(container).toHaveTextContent("Component is not connected to flow");
      // Port name should not be present (no colon before message)
      const issueMessage = container.querySelector(".issue-message");
      expect(issueMessage?.textContent).not.toContain(":");
    });
  });
});
