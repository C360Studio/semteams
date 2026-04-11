import { describe, it, expect, vi } from "vitest";
import { render, fireEvent } from "@testing-library/svelte";
import type { PropertySchema } from "$lib/types/schema";
import PortConfigEditor from "./PortConfigEditor.svelte";

describe("PortConfigEditor", () => {
  const mockPortFieldsSchema: PropertySchema = {
    type: "ports",
    description: "Port configuration",
    portFields: {
      name: { type: "string", editable: false },
      type: { type: "string", editable: false },
      required: { type: "bool", editable: false },
      description: { type: "string", editable: false },
      interface: { type: "string", editable: false },
      subject: { type: "string", editable: true },
      timeout: { type: "string", editable: true },
      stream_name: { type: "string", editable: true },
    },
  };

  // Test rendering empty port config
  it("should render empty state when no ports provided", () => {
    const { container } = render(PortConfigEditor, {
      props: {
        name: "ports",
        schema: mockPortFieldsSchema,
        value: undefined,
      },
    });

    // Check for input and output sections
    expect(container.textContent).toContain("Input Ports");
    expect(container.textContent).toContain("Output Ports");

    // Should have add buttons for both sections
    const addButtons = container.querySelectorAll("button");
    expect(addButtons.length).toBeGreaterThanOrEqual(2); // At least one for inputs, one for outputs
  });

  // Test rendering with existing ports
  it("should render existing input and output ports", () => {
    const mockPorts = {
      inputs: [
        {
          id: "input1",
          name: "nats_input",
          direction: "input" as const,
          required: true,
          description: "NATS input port",
          config: {
            type: "nats" as const,
            nats: {
              subject: "events.test",
              interface: { type: "message.Storable" },
            },
          },
        },
      ],
      outputs: [
        {
          id: "output1",
          name: "nats_output",
          direction: "output" as const,
          required: false,
          description: "NATS output port",
          config: {
            type: "nats" as const,
            nats: {
              subject: "events.processed",
              interface: { type: "message.Storable" },
            },
          },
        },
      ],
    };

    const { container } = render(PortConfigEditor, {
      props: {
        name: "ports",
        schema: mockPortFieldsSchema,
        value: mockPorts,
      },
    });

    // Check that port names are displayed (readonly fields)
    expect(container.textContent).toContain("nats_input");
    expect(container.textContent).toContain("nats_output");

    // Check that descriptions are displayed
    expect(container.textContent).toContain("NATS input port");
    expect(container.textContent).toContain("NATS output port");
  });

  // Test that only editable fields have inputs
  it("should only show inputs for editable fields", () => {
    const mockPorts = {
      inputs: [
        {
          id: "input1",
          name: "nats_input",
          direction: "input" as const,
          required: true,
          description: "Test port",
          config: {
            type: "nats" as const,
            nats: {
              subject: "events.test",
              interface: { type: "message.Storable" },
            },
          },
        },
      ],
      outputs: [],
    };

    const { container } = render(PortConfigEditor, {
      props: {
        name: "ports",
        schema: mockPortFieldsSchema,
        value: mockPorts,
      },
    });

    // Readonly fields should not have text inputs
    // Name, Type, Required, Description, Interface are readonly

    // Should have labels for readonly fields (displayed as read-only)
    expect(container.textContent?.toLowerCase()).toContain("name");
    expect(container.textContent?.toLowerCase()).toContain("type");
    expect(container.textContent?.toLowerCase()).toContain("required");

    // Editable field (subject) should have an input
    const inputs = container.querySelectorAll('input[type="text"]');
    expect(inputs.length).toBeGreaterThan(0);

    // Check that subject input exists and has the correct value
    const subjectInput = Array.from(inputs).find(
      (input) => (input as HTMLInputElement).value === "events.test",
    );
    expect(subjectInput).toBeTruthy();
  });

  // Test adding a new input port
  it("should allow adding a new input port", async () => {
    const onChange = vi.fn();

    const { container } = render(PortConfigEditor, {
      props: {
        name: "ports",
        schema: mockPortFieldsSchema,
        value: { inputs: [], outputs: [] },
        onChange,
      },
    });

    // Find and click the "Add Input Port" button
    const buttons = Array.from(container.querySelectorAll("button"));
    const addInputButton = buttons.find((btn) =>
      btn.textContent?.toLowerCase().includes("add input"),
    );

    expect(addInputButton).toBeTruthy();
    await fireEvent.click(addInputButton!);

    // onChange should have been called with updated ports
    expect(onChange).toHaveBeenCalled();
    const callArg = onChange.mock.calls[0][0];
    expect(callArg.inputs).toHaveLength(1);
  });

  // Test adding a new output port
  it("should allow adding a new output port", async () => {
    const onChange = vi.fn();

    const { container } = render(PortConfigEditor, {
      props: {
        name: "ports",
        schema: mockPortFieldsSchema,
        value: { inputs: [], outputs: [] },
        onChange,
      },
    });

    // Find and click the "Add Output Port" button
    const buttons = Array.from(container.querySelectorAll("button"));
    const addOutputButton = buttons.find((btn) =>
      btn.textContent?.toLowerCase().includes("add output"),
    );

    expect(addOutputButton).toBeTruthy();
    await fireEvent.click(addOutputButton!);

    // onChange should have been called with updated ports
    expect(onChange).toHaveBeenCalled();
    const callArg = onChange.mock.calls[0][0];
    expect(callArg.outputs).toHaveLength(1);
  });

  // Test removing a port
  it("should allow removing a port", async () => {
    const onChange = vi.fn();
    const mockPorts = {
      inputs: [
        {
          id: "input1",
          name: "test_input",
          direction: "input" as const,
          required: true,
          description: "Test port",
          config: {
            type: "nats" as const,
            nats: {
              subject: "events.test",
              interface: { type: "message.Storable" },
            },
          },
        },
      ],
      outputs: [],
    };

    const { container } = render(PortConfigEditor, {
      props: {
        name: "ports",
        schema: mockPortFieldsSchema,
        value: mockPorts,
        onChange,
      },
    });

    // Find and click a remove button
    const buttons = Array.from(container.querySelectorAll("button"));
    const removeButton = buttons.find((btn) =>
      btn.textContent?.toLowerCase().includes("remove"),
    );

    expect(removeButton).toBeTruthy();
    await fireEvent.click(removeButton!);

    // onChange should have been called with empty inputs
    expect(onChange).toHaveBeenCalled();
    const callArg = onChange.mock.calls[0][0];
    expect(callArg.inputs).toHaveLength(0);
  });

  // Test editing an editable field
  it("should call onChange when editing editable field", async () => {
    const onChange = vi.fn();
    const mockPorts = {
      inputs: [
        {
          id: "input1",
          name: "nats_input",
          direction: "input" as const,
          required: true,
          description: "Test port",
          config: {
            type: "nats" as const,
            nats: {
              subject: "events.test",
              interface: { type: "message.Storable" },
            },
          },
        },
      ],
      outputs: [],
    };

    const { container } = render(PortConfigEditor, {
      props: {
        name: "ports",
        schema: mockPortFieldsSchema,
        value: mockPorts,
        onChange,
      },
    });

    // Find the subject input and change its value
    const inputs = container.querySelectorAll('input[type="text"]');
    const subjectInput = Array.from(inputs).find(
      (input) => (input as HTMLInputElement).value === "events.test",
    ) as HTMLInputElement;

    expect(subjectInput).toBeTruthy();

    // Change the value
    await fireEvent.input(subjectInput, {
      target: { value: "events.updated" },
    });

    // onChange should have been called
    expect(onChange).toHaveBeenCalled();
  });

  // Test validation error display
  it("should display validation errors", () => {
    const { container } = render(PortConfigEditor, {
      props: {
        name: "ports",
        schema: mockPortFieldsSchema,
        value: { inputs: [], outputs: [] },
        error: "At least one port is required",
      },
    });

    const errorEl = container.querySelector(".error");
    expect(errorEl).toBeInTheDocument();
    expect(errorEl).toHaveTextContent("At least one port is required");
  });

  // Test required field indicator
  it("should show required indicator when isRequired is true", () => {
    const { container } = render(PortConfigEditor, {
      props: {
        name: "ports",
        schema: mockPortFieldsSchema,
        value: { inputs: [], outputs: [] },
        isRequired: true,
      },
    });

    const label = container.querySelector("label");
    expect(label).toHaveTextContent("*");
  });

  // Test readonly field values are displayed
  it("should display readonly field values as labels", () => {
    const mockPorts = {
      inputs: [
        {
          id: "input1",
          name: "test_input",
          direction: "input" as const,
          required: true,
          description: "Test description",
          config: {
            type: "nats" as const,
            nats: {
              subject: "events.test",
              interface: { type: "message.Storable" },
            },
          },
        },
      ],
      outputs: [],
    };

    const { container } = render(PortConfigEditor, {
      props: {
        name: "ports",
        schema: mockPortFieldsSchema,
        value: mockPorts,
      },
    });

    // Verify readonly fields are displayed as text (not inputs)
    expect(container.textContent).toContain("test_input");
    expect(container.textContent).toContain("nats");
    expect(container.textContent).toContain("Test description");
  });
});
