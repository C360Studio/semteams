import { describe, it, expect, vi } from "vitest";
import { render, fireEvent } from "@testing-library/svelte";
import { tick } from "svelte";
import EditComponentModal from "./EditComponentModal.svelte";
import type { FlowNode } from "$lib/types/flow";
import type { ComponentType } from "$lib/types/component";

describe("EditComponentModal", () => {
  const mockComponentType: ComponentType = {
    id: "udp-input",
    name: "UDP Input",
    type: "input",
    protocol: "udp",
    category: "input",
    description: "Receives data over UDP protocol",
    version: "1.0.0",
    schema: {
      type: "object",
      properties: {
        port: {
          type: "number",
          description: "UDP port to listen on",
          default: 5000,
          minimum: 1024,
          maximum: 65535,
        },
        bufferSize: {
          type: "number",
          description: "Buffer size in bytes",
          default: 1024,
        },
      },
      required: ["port"],
    },
  };

  const mockNode: FlowNode = {
    id: "node_123_abc",
    component: "udp-input",
    type: "input",
    name: "udp-input-1",
    position: { x: 100, y: 100 },
    config: {
      port: 5000,
      bufferSize: 1024,
    },
  };

  // Test 1: DOM Output - Rendering States
  describe("DOM Output - Rendering States", () => {
    it("should not render when isOpen is false", () => {
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: false,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete: vi.fn(),
          onClose: vi.fn(),
        },
      });

      expect(
        container.querySelector('[role="dialog"]'),
      ).not.toBeInTheDocument();
    });

    it("should not render when node is null", () => {
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: null,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete: vi.fn(),
          onClose: vi.fn(),
        },
      });

      expect(
        container.querySelector('[role="dialog"]'),
      ).not.toBeInTheDocument();
    });

    it("should render dialog when isOpen is true and node is provided", () => {
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete: vi.fn(),
          onClose: vi.fn(),
        },
      });

      const dialog = container.querySelector('[role="dialog"]');
      expect(dialog).toBeInTheDocument();
    });

    it("should render correct dialog title with node name", () => {
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete: vi.fn(),
          onClose: vi.fn(),
        },
      });

      const title = container.querySelector("#dialog-title");
      expect(title).toBeInTheDocument();
      expect(title).toHaveTextContent("Edit: udp-input-1");
    });

    it("should render name input field pre-populated", () => {
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete: vi.fn(),
          onClose: vi.fn(),
        },
      });

      const nameInput = container.querySelector(
        'input[name="name"]',
      ) as HTMLInputElement;
      expect(nameInput).toBeInTheDocument();
      expect(nameInput.value).toBe("udp-input-1");
    });

    it("should render type as read-only text", () => {
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete: vi.fn(),
          onClose: vi.fn(),
        },
      });

      const typeText = container.querySelector(".component-type-readonly");
      expect(typeText).toBeInTheDocument();
      expect(typeText).toHaveTextContent(/Type:.*udp-input/i);
    });

    it("should render config fields pre-populated with current values", () => {
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete: vi.fn(),
          onClose: vi.fn(),
        },
      });

      const portInput = container.querySelector(
        'input[name="config.port"]',
      ) as HTMLInputElement;
      const bufferInput = container.querySelector(
        'input[name="config.bufferSize"]',
      ) as HTMLInputElement;

      expect(portInput).toBeInTheDocument();
      expect(portInput.value).toBe("5000");
      expect(bufferInput).toBeInTheDocument();
      expect(bufferInput.value).toBe("1024");
    });

    it("should render Delete, Cancel, and Save buttons", () => {
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete: vi.fn(),
          onClose: vi.fn(),
        },
      });

      const buttons = container.querySelectorAll("button");
      const deleteButton = Array.from(buttons).find((btn) =>
        btn.textContent?.includes("Delete"),
      );
      const cancelButton = Array.from(buttons).find((btn) =>
        btn.textContent?.includes("Cancel"),
      );
      const saveButton = Array.from(buttons).find((btn) =>
        btn.textContent?.includes("Save"),
      );

      expect(deleteButton).toBeInTheDocument();
      expect(cancelButton).toBeInTheDocument();
      expect(saveButton).toBeInTheDocument();
    });

    it("should render close button (X)", () => {
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete: vi.fn(),
          onClose: vi.fn(),
        },
      });

      const closeButton = container.querySelector(".close-button");
      expect(closeButton).toBeInTheDocument();
      expect(closeButton).toHaveAttribute("aria-label", "Close dialog");
    });
  });

  // Test 2: Form Fields
  describe("Form Fields", () => {
    it("should allow editing the name field", async () => {
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete: vi.fn(),
          onClose: vi.fn(),
        },
      });

      const nameInput = container.querySelector(
        'input[name="name"]',
      ) as HTMLInputElement;

      // Edit name
      await fireEvent.input(nameInput, { target: { value: "my-custom-name" } });
      await tick();

      expect(nameInput.value).toBe("my-custom-name");
    });

    it("should not allow editing the type field", () => {
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete: vi.fn(),
          onClose: vi.fn(),
        },
      });

      // Type should be displayed as text, not an input
      const typeInput = container.querySelector('input[name="type"]');
      expect(typeInput).not.toBeInTheDocument();

      // Should show as read-only text
      const typeText = container.querySelector(".component-type-readonly");
      expect(typeText).toBeInTheDocument();
    });

    it("should allow editing config fields", async () => {
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete: vi.fn(),
          onClose: vi.fn(),
        },
      });

      const portInput = container.querySelector(
        'input[name="config.port"]',
      ) as HTMLInputElement;

      // Edit port
      await fireEvent.input(portInput, { target: { value: "8000" } });
      await tick();

      expect(portInput.value).toBe("8000");
    });

    it("should show validation errors for invalid config values", async () => {
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete: vi.fn(),
          onClose: vi.fn(),
        },
      });

      const portInput = container.querySelector(
        'input[name="config.port"]',
      ) as HTMLInputElement;

      // Enter invalid value (below minimum)
      await fireEvent.input(portInput, { target: { value: "500" } });
      await tick();

      const errorMessage = container.querySelector(".validation-error");
      expect(errorMessage).toBeInTheDocument();
      expect(errorMessage).toHaveTextContent(/minimum.*1024/i);
    });
  });

  // Test 3: Form Validation
  describe("Form Validation", () => {
    it("should disable Save button when no changes made", () => {
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete: vi.fn(),
          onClose: vi.fn(),
        },
      });

      const buttons = container.querySelectorAll("button");
      const saveButton = Array.from(buttons).find((btn) =>
        btn.textContent?.includes("Save"),
      ) as HTMLButtonElement;

      expect(saveButton).toBeDisabled();
    });

    it("should enable Save button when changes made", async () => {
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete: vi.fn(),
          onClose: vi.fn(),
        },
      });

      const nameInput = container.querySelector(
        'input[name="name"]',
      ) as HTMLInputElement;

      // Make a change
      await fireEvent.input(nameInput, { target: { value: "new-name" } });
      await tick();

      const buttons = container.querySelectorAll("button");
      const saveButton = Array.from(buttons).find((btn) =>
        btn.textContent?.includes("Save"),
      ) as HTMLButtonElement;

      expect(saveButton).not.toBeDisabled();
    });

    it("should disable Save button when name is empty", async () => {
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete: vi.fn(),
          onClose: vi.fn(),
        },
      });

      const nameInput = container.querySelector(
        'input[name="name"]',
      ) as HTMLInputElement;

      // Clear name
      await fireEvent.input(nameInput, { target: { value: "" } });
      await tick();

      const buttons = container.querySelectorAll("button");
      const saveButton = Array.from(buttons).find((btn) =>
        btn.textContent?.includes("Save"),
      ) as HTMLButtonElement;

      expect(saveButton).toBeDisabled();
    });

    it("should disable Save button when required config field is empty", async () => {
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete: vi.fn(),
          onClose: vi.fn(),
        },
      });

      const portInput = container.querySelector(
        'input[name="config.port"]',
      ) as HTMLInputElement;

      // Clear required field
      await fireEvent.input(portInput, { target: { value: "" } });
      await tick();

      const buttons = container.querySelectorAll("button");
      const saveButton = Array.from(buttons).find((btn) =>
        btn.textContent?.includes("Save"),
      ) as HTMLButtonElement;

      expect(saveButton).toBeDisabled();
    });

    it("should disable Save button when validation errors exist", async () => {
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete: vi.fn(),
          onClose: vi.fn(),
        },
      });

      const portInput = container.querySelector(
        'input[name="config.port"]',
      ) as HTMLInputElement;

      // Enter invalid value
      await fireEvent.input(portInput, { target: { value: "70000" } });
      await tick();

      const buttons = container.querySelectorAll("button");
      const saveButton = Array.from(buttons).find((btn) =>
        btn.textContent?.includes("Save"),
      ) as HTMLButtonElement;

      expect(saveButton).toBeDisabled();
    });
  });

  // Test 4: User Actions - Save
  describe("User Actions - Save", () => {
    it("should call onSave with updated data when Save clicked", async () => {
      const onSave = vi.fn();
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onSave,
          onDelete: vi.fn(),
          onClose: vi.fn(),
        },
      });

      const nameInput = container.querySelector(
        'input[name="name"]',
      ) as HTMLInputElement;
      const portInput = container.querySelector(
        'input[name="config.port"]',
      ) as HTMLInputElement;

      // Make changes
      await fireEvent.input(nameInput, { target: { value: "updated-name" } });
      await fireEvent.input(portInput, { target: { value: "8000" } });
      await tick();

      // Click Save
      const buttons = container.querySelectorAll("button");
      const saveButton = Array.from(buttons).find((btn) =>
        btn.textContent?.includes("Save"),
      ) as HTMLButtonElement;
      await fireEvent.click(saveButton);

      // Verify onSave called with correct data
      expect(onSave).toHaveBeenCalledTimes(1);
      expect(onSave).toHaveBeenCalledWith("node_123_abc", "updated-name", {
        port: 8000,
        bufferSize: 1024,
      });
    });

    it("should call onSave with only changed fields", async () => {
      const onSave = vi.fn();
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onSave,
          onDelete: vi.fn(),
          onClose: vi.fn(),
        },
      });

      const nameInput = container.querySelector(
        'input[name="name"]',
      ) as HTMLInputElement;

      // Only change name
      await fireEvent.input(nameInput, { target: { value: "new-name" } });
      await tick();

      // Click Save
      const buttons = container.querySelectorAll("button");
      const saveButton = Array.from(buttons).find((btn) =>
        btn.textContent?.includes("Save"),
      ) as HTMLButtonElement;
      await fireEvent.click(saveButton);

      // Verify onSave called with new name but original config
      expect(onSave).toHaveBeenCalledWith("node_123_abc", "new-name", {
        port: 5000,
        bufferSize: 1024,
      });
    });
  });

  // Test 5: User Actions - Delete
  describe("User Actions - Delete", () => {
    it("should show confirmation dialog when Delete clicked", async () => {
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete: vi.fn(),
          onClose: vi.fn(),
        },
      });

      const buttons = container.querySelectorAll("button");
      const deleteButton = Array.from(buttons).find((btn) =>
        btn.textContent?.includes("Delete"),
      ) as HTMLButtonElement;

      await fireEvent.click(deleteButton);
      await tick();

      // Should show confirmation dialog
      const confirmDialog = container.querySelector(".confirm-dialog");
      expect(confirmDialog).toBeInTheDocument();
      expect(confirmDialog).toHaveTextContent(/are you sure/i);
    });

    it("should call onDelete when delete confirmed", async () => {
      const onDelete = vi.fn();
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete,
          onClose: vi.fn(),
        },
      });

      const buttons = container.querySelectorAll("button");
      const deleteButton = Array.from(buttons).find((btn) =>
        btn.textContent?.includes("Delete"),
      ) as HTMLButtonElement;

      // Click Delete
      await fireEvent.click(deleteButton);
      await tick();

      // Confirm deletion
      const confirmButton = container.querySelector(
        ".confirm-delete-button",
      ) as HTMLButtonElement;
      await fireEvent.click(confirmButton);

      // Verify onDelete called with node ID
      expect(onDelete).toHaveBeenCalledTimes(1);
      expect(onDelete).toHaveBeenCalledWith("node_123_abc");
    });

    it("should NOT call onDelete when delete cancelled", async () => {
      const onDelete = vi.fn();
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete,
          onClose: vi.fn(),
        },
      });

      const buttons = container.querySelectorAll("button");
      const deleteButton = Array.from(buttons).find((btn) =>
        btn.textContent?.includes("Delete"),
      ) as HTMLButtonElement;

      // Click Delete
      await fireEvent.click(deleteButton);
      await tick();

      // Cancel deletion
      const cancelButton = container.querySelector(
        ".cancel-delete-button",
      ) as HTMLButtonElement;
      await fireEvent.click(cancelButton);

      // Verify onDelete NOT called
      expect(onDelete).not.toHaveBeenCalled();

      // Confirmation dialog should close
      const confirmDialog = container.querySelector(".confirm-dialog");
      expect(confirmDialog).not.toBeInTheDocument();
    });

    it("should close confirmation dialog when ESC pressed", async () => {
      const onDelete = vi.fn();
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete,
          onClose: vi.fn(),
        },
      });

      const buttons = container.querySelectorAll("button");
      const deleteButton = Array.from(buttons).find((btn) =>
        btn.textContent?.includes("Delete"),
      ) as HTMLButtonElement;

      // Open confirmation dialog
      await fireEvent.click(deleteButton);
      await tick();

      // Press ESC
      const escEvent = new KeyboardEvent("keydown", { key: "Escape" });
      window.dispatchEvent(escEvent);
      await tick();

      // Confirmation dialog should close
      const confirmDialog = container.querySelector(".confirm-dialog");
      expect(confirmDialog).not.toBeInTheDocument();

      // onDelete should NOT be called
      expect(onDelete).not.toHaveBeenCalled();
    });
  });

  // Test 6: User Actions - Cancel/Close
  describe("User Actions - Cancel/Close", () => {
    it("should call onClose when Cancel button clicked", async () => {
      const onClose = vi.fn();
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete: vi.fn(),
          onClose,
        },
      });

      const buttons = container.querySelectorAll("button");
      const cancelButton = Array.from(buttons).find((btn) =>
        btn.textContent?.includes("Cancel"),
      ) as HTMLButtonElement;

      await fireEvent.click(cancelButton);

      expect(onClose).toHaveBeenCalledTimes(1);
    });

    it("should discard changes when Cancel clicked", async () => {
      const onClose = vi.fn();
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete: vi.fn(),
          onClose,
        },
      });

      const nameInput = container.querySelector(
        'input[name="name"]',
      ) as HTMLInputElement;

      // Make changes
      await fireEvent.input(nameInput, { target: { value: "changed-name" } });
      await tick();

      // Cancel
      const buttons = container.querySelectorAll("button");
      const cancelButton = Array.from(buttons).find((btn) =>
        btn.textContent?.includes("Cancel"),
      ) as HTMLButtonElement;
      await fireEvent.click(cancelButton);

      expect(onClose).toHaveBeenCalled();
      // Changes should be discarded (component closed)
    });

    it("should call onClose when close button (X) clicked", async () => {
      const onClose = vi.fn();
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete: vi.fn(),
          onClose,
        },
      });

      const closeButton = container.querySelector(
        ".close-button",
      ) as HTMLButtonElement;

      await fireEvent.click(closeButton);

      expect(onClose).toHaveBeenCalledTimes(1);
    });

    it("should call onClose when clicking outside dialog", async () => {
      const onClose = vi.fn();
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete: vi.fn(),
          onClose,
        },
      });

      const overlay = container.querySelector(".dialog-overlay") as HTMLElement;

      // Click on overlay background
      await fireEvent.click(overlay);

      expect(onClose).toHaveBeenCalledTimes(1);
    });

    it("should NOT close when clicking inside dialog content", async () => {
      const onClose = vi.fn();
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete: vi.fn(),
          onClose,
        },
      });

      const dialogContent = container.querySelector(
        ".dialog-content",
      ) as HTMLElement;

      // Click inside dialog content
      await fireEvent.click(dialogContent);

      expect(onClose).not.toHaveBeenCalled();
    });
  });

  // Test 7: Keyboard Navigation
  describe("Keyboard Navigation", () => {
    it("should close dialog when ESC key pressed", async () => {
      const onClose = vi.fn();
      render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete: vi.fn(),
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
      render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete: vi.fn(),
          onClose,
        },
      });

      // Simulate other key presses
      const enterEvent = new KeyboardEvent("keydown", { key: "Enter" });
      window.dispatchEvent(enterEvent);

      await tick();

      expect(onClose).not.toHaveBeenCalled();
    });

    it("should NOT trigger ESC handler when dialog is closed", async () => {
      const onClose = vi.fn();
      render(EditComponentModal, {
        props: {
          isOpen: false,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete: vi.fn(),
          onClose,
        },
      });

      // Simulate ESC key press
      const escEvent = new KeyboardEvent("keydown", { key: "Escape" });
      window.dispatchEvent(escEvent);

      await tick();

      expect(onClose).not.toHaveBeenCalled();
    });
  });

  // Test 8: Accessibility
  describe("Accessibility", () => {
    it("should have proper ARIA attributes", () => {
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete: vi.fn(),
          onClose: vi.fn(),
        },
      });

      const dialog = container.querySelector('[role="dialog"]');
      expect(dialog).toHaveAttribute("aria-modal", "true");
      expect(dialog).toHaveAttribute("aria-labelledby", "dialog-title");

      const title = container.querySelector("#dialog-title");
      expect(title).toBeInTheDocument();
    });

    it("should have aria-label on close button", () => {
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete: vi.fn(),
          onClose: vi.fn(),
        },
      });

      const closeButton = container.querySelector(".close-button");
      expect(closeButton).toHaveAttribute("aria-label", "Close dialog");
    });

    it("should have proper label associations for form fields", () => {
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete: vi.fn(),
          onClose: vi.fn(),
        },
      });

      // Name field
      const nameLabel = container.querySelector('label[for="name"]');
      const nameInput = container.querySelector("#name");
      expect(nameLabel).toBeInTheDocument();
      expect(nameInput).toBeInTheDocument();
    });

    it("should mark required fields with aria-required", () => {
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete: vi.fn(),
          onClose: vi.fn(),
        },
      });

      const nameInput = container.querySelector('input[name="name"]');
      expect(nameInput).toHaveAttribute("aria-required", "true");
    });

    it("should have aria-invalid on fields with validation errors", async () => {
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete: vi.fn(),
          onClose: vi.fn(),
        },
      });

      const portInput = container.querySelector(
        'input[name="config.port"]',
      ) as HTMLInputElement;

      // Enter invalid value
      await fireEvent.input(portInput, { target: { value: "500" } });
      await tick();

      expect(portInput).toHaveAttribute("aria-invalid", "true");
    });

    it("should have proper role for delete confirmation dialog", async () => {
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete: vi.fn(),
          onClose: vi.fn(),
        },
      });

      const buttons = container.querySelectorAll("button");
      const deleteButton = Array.from(buttons).find((btn) =>
        btn.textContent?.includes("Delete"),
      ) as HTMLButtonElement;

      // Open confirmation dialog
      await fireEvent.click(deleteButton);
      await tick();

      const confirmDialog = container.querySelector(".confirm-dialog");
      expect(confirmDialog).toHaveAttribute("role", "alertdialog");
    });
  });

  // Test 9: Reactivity
  describe("Reactivity", () => {
    it("should update when isOpen changes", async () => {
      const { container, rerender } = render(EditComponentModal, {
        props: {
          isOpen: false,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete: vi.fn(),
          onClose: vi.fn(),
        },
      });

      // Initially not visible
      expect(
        container.querySelector('[role="dialog"]'),
      ).not.toBeInTheDocument();

      // Open modal
      await rerender({ isOpen: true });
      await tick();

      // Now visible
      expect(container.querySelector('[role="dialog"]')).toBeInTheDocument();
    });

    it("should update when node changes", async () => {
      const { container, rerender } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete: vi.fn(),
          onClose: vi.fn(),
        },
      });

      const nameInput = container.querySelector(
        'input[name="name"]',
      ) as HTMLInputElement;

      // Initially shows first node
      expect(nameInput.value).toBe("udp-input-1");

      // Update node
      const newNode: FlowNode = {
        ...mockNode,
        id: "node_456_def",
        name: "udp-input-2",
        config: { port: 8000, bufferSize: 2048 },
      };

      await rerender({ node: newNode });
      await tick();

      // Should show new node data
      expect(nameInput.value).toBe("udp-input-2");

      const portInput = container.querySelector(
        'input[name="config.port"]',
      ) as HTMLInputElement;
      expect(portInput.value).toBe("8000");
    });

    it("should reset dirty state when node changes", async () => {
      const { container, rerender } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete: vi.fn(),
          onClose: vi.fn(),
        },
      });

      const nameInput = container.querySelector(
        'input[name="name"]',
      ) as HTMLInputElement;

      // Make changes
      await fireEvent.input(nameInput, { target: { value: "changed-name" } });
      await tick();

      // Save button should be enabled
      const buttons = container.querySelectorAll("button");
      let saveButton = Array.from(buttons).find((btn) =>
        btn.textContent?.includes("Save"),
      ) as HTMLButtonElement;
      expect(saveButton).not.toBeDisabled();

      // Update to different node
      const newNode: FlowNode = {
        ...mockNode,
        id: "node_999_xyz",
        name: "different-node",
      };

      await rerender({ node: newNode });
      await tick();

      // Save button should be disabled (no changes to new node)
      const updatedButtons = container.querySelectorAll("button");
      saveButton = Array.from(updatedButtons).find((btn) =>
        btn.textContent?.includes("Save"),
      ) as HTMLButtonElement;
      expect(saveButton).toBeDisabled();
    });
  });

  // Test 10: Edge Cases
  describe("Edge Cases", () => {
    it("should handle node without config", () => {
      const nodeWithoutConfig: FlowNode = {
        id: "node_simple",
        component: "simple-component",
        type: "processor",
        name: "simple-1",
        position: { x: 0, y: 0 },
        config: {},
      };

      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: nodeWithoutConfig,
          onSave: vi.fn(),
          onDelete: vi.fn(),
          onClose: vi.fn(),
        },
      });

      // Should render without errors
      const dialog = container.querySelector('[role="dialog"]');
      expect(dialog).toBeInTheDocument();

      // Should not show config section
      const configSection = container.querySelector(".config-section");
      expect(configSection).not.toBeInTheDocument();
    });

    it("should handle missing componentType", () => {
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          onSave: vi.fn(),
          onDelete: vi.fn(),
          onClose: vi.fn(),
        },
      });

      // Should render without errors
      const dialog = container.querySelector('[role="dialog"]');
      expect(dialog).toBeInTheDocument();

      // Config fields may not be rendered without schema
      // But basic fields (name) should still work
      const nameInput = container.querySelector('input[name="name"]');
      expect(nameInput).toBeInTheDocument();
    });

    it("should handle missing onSave callback", async () => {
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onDelete: vi.fn(),
          onClose: vi.fn(),
        },
      });

      const nameInput = container.querySelector(
        'input[name="name"]',
      ) as HTMLInputElement;

      // Make changes
      await fireEvent.input(nameInput, { target: { value: "new-name" } });
      await tick();

      // Click Save
      const buttons = container.querySelectorAll("button");
      const saveButton = Array.from(buttons).find((btn) =>
        btn.textContent?.includes("Save"),
      ) as HTMLButtonElement;

      // Should not throw error
      expect(() => fireEvent.click(saveButton)).not.toThrow();
    });

    it("should handle missing onDelete callback", async () => {
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onClose: vi.fn(),
        },
      });

      const buttons = container.querySelectorAll("button");
      const deleteButton = Array.from(buttons).find((btn) =>
        btn.textContent?.includes("Delete"),
      ) as HTMLButtonElement;

      // Click Delete
      await fireEvent.click(deleteButton);
      await tick();

      // Confirm deletion
      const confirmButton = container.querySelector(
        ".confirm-delete-button",
      ) as HTMLButtonElement;

      // Should not throw error
      expect(() => fireEvent.click(confirmButton)).not.toThrow();
    });

    it("should handle missing onClose callback", async () => {
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: mockNode,
          componentType: mockComponentType,
          onSave: vi.fn(),
          onDelete: vi.fn(),
        },
      });

      const buttons = container.querySelectorAll("button");
      const cancelButton = Array.from(buttons).find((btn) =>
        btn.textContent?.includes("Cancel"),
      ) as HTMLButtonElement;

      // Should not throw error
      expect(() => fireEvent.click(cancelButton)).not.toThrow();
    });

    it("should preserve other config fields when editing", async () => {
      const nodeWithManyFields: FlowNode = {
        id: "node_complex",
        component: "udp-input",
        type: "input",
        name: "complex-1",
        position: { x: 0, y: 0 },
        config: {
          port: 5000,
          bufferSize: 1024,
          extraField: "preserved",
          nested: {
            value: 123,
          },
        },
      };

      const onSave = vi.fn();
      const { container } = render(EditComponentModal, {
        props: {
          isOpen: true,
          node: nodeWithManyFields,
          componentType: mockComponentType,
          onSave,
          onDelete: vi.fn(),
          onClose: vi.fn(),
        },
      });

      const portInput = container.querySelector(
        'input[name="config.port"]',
      ) as HTMLInputElement;

      // Change only port
      await fireEvent.input(portInput, { target: { value: "8000" } });
      await tick();

      // Save
      const buttons = container.querySelectorAll("button");
      const saveButton = Array.from(buttons).find((btn) =>
        btn.textContent?.includes("Save"),
      ) as HTMLButtonElement;
      await fireEvent.click(saveButton);

      // Should preserve all fields
      expect(onSave).toHaveBeenCalledWith(
        "node_complex",
        "complex-1",
        expect.objectContaining({
          port: 8000,
          bufferSize: 1024,
          extraField: "preserved",
          nested: { value: 123 },
        }),
      );
    });
  });
});
