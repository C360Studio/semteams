import { describe, it, expect } from "vitest";
import { render } from "@testing-library/svelte";
import type { PropertySchema } from "$lib/types/schema";
import SchemaField from "./SchemaField.svelte";

describe("SchemaField", () => {
  // T028: Component test SchemaField - router to type-specific components
  it("should render StringField for string type", () => {
    const schema: PropertySchema = {
      type: "string",
      description: "String field",
    };

    const { container } = render(SchemaField, {
      props: {
        name: "test_string",
        schema,
        value: "test value",
      },
    });

    // Should render text input (StringField)
    const input = container.querySelector('input[type="text"]');
    expect(input).toBeInTheDocument();
    expect(input).toHaveValue("test value");
  });

  it("should render NumberField for int type", () => {
    const schema: PropertySchema = {
      type: "int",
      description: "Integer field",
      minimum: 1,
      maximum: 100,
    };

    const { container } = render(SchemaField, {
      props: {
        name: "test_int",
        schema,
        value: 42,
      },
    });

    // Should render number input (NumberField)
    const input = container.querySelector('input[type="number"]');
    expect(input).toBeInTheDocument();
    expect(input).toHaveValue(42);
    expect(input).toHaveAttribute("min", "1");
    expect(input).toHaveAttribute("max", "100");
  });

  it("should render NumberField for float type", () => {
    const schema: PropertySchema = {
      type: "float",
      description: "Float field",
      minimum: 0.1,
      maximum: 10.0,
    };

    const { container } = render(SchemaField, {
      props: {
        name: "test_float",
        schema,
        value: 5.5,
      },
    });

    // Should render number input (NumberField)
    const input = container.querySelector('input[type="number"]');
    expect(input).toBeInTheDocument();
    expect(input).toHaveValue(5.5);
  });

  it("should render BooleanField for bool type", () => {
    const schema: PropertySchema = {
      type: "bool",
      description: "Boolean field",
    };

    const { container } = render(SchemaField, {
      props: {
        name: "test_bool",
        schema,
        value: true,
      },
    });

    // Should render checkbox (BooleanField)
    const input = container.querySelector('input[type="checkbox"]');
    expect(input).toBeInTheDocument();
    expect(input).toBeChecked();
  });

  it("should render EnumField for enum type", () => {
    const schema: PropertySchema = {
      type: "enum",
      description: "Enum field",
      enum: ["option1", "option2", "option3"],
    };

    const { container } = render(SchemaField, {
      props: {
        name: "test_enum",
        schema,
        value: "option2",
      },
    });

    // Should render select dropdown (EnumField)
    const select = container.querySelector("select");
    expect(select).toBeInTheDocument();
    expect(select).toHaveValue("option2");

    const options = container.querySelectorAll("option");
    expect(options).toHaveLength(3);
  });

  // T042: Component test nested object/array fallback
  it("should show JSON editor fallback for object type", () => {
    const schema: PropertySchema = {
      type: "object",
      description: "Complex object field",
    };

    const { container } = render(SchemaField, {
      props: {
        name: "test_object",
        schema,
      },
    });

    // Should render fallback message
    const fallback = container.querySelector(".complex-field-fallback");
    expect(fallback).toBeInTheDocument();
    expect(fallback).toHaveTextContent("complex configuration");
    expect(fallback).toHaveTextContent("JSON editor");
  });

  it("should show JSON editor fallback for array type", () => {
    const schema: PropertySchema = {
      type: "array",
      description: "Array field",
    };

    const { container } = render(SchemaField, {
      props: {
        name: "test_array",
        schema,
      },
    });

    // Should render fallback message
    const fallback = container.querySelector(".complex-field-fallback");
    expect(fallback).toBeInTheDocument();
    expect(fallback).toHaveTextContent("complex configuration");
    expect(fallback).toHaveTextContent("JSON editor");
  });

  // T038: Component test required field visual marking
  it("should pass isRequired prop to child components", () => {
    const schema: PropertySchema = {
      type: "string",
      description: "Required field",
    };

    const { container } = render(SchemaField, {
      props: {
        name: "required_field",
        schema,
        isRequired: true,
      },
    });

    // Should render required marker
    const label = container.querySelector("label");
    expect(label).toHaveTextContent("*");

    const input = container.querySelector("input");
    expect(input).toHaveAttribute("aria-required", "true");
  });

  it("should pass error prop to child components", () => {
    const schema: PropertySchema = {
      type: "string",
      description: "Field with error",
    };

    const { container } = render(SchemaField, {
      props: {
        name: "error_field",
        schema,
        error: "This field has an error",
      },
    });

    const errorEl = container.querySelector(".error");
    expect(errorEl).toBeInTheDocument();
    expect(errorEl).toHaveTextContent("This field has an error");
  });

  it("should render PortConfigEditor for ports type", () => {
    const schema: PropertySchema = {
      type: "ports",
      description: "Port configuration",
      portFields: {
        name: { type: "string", editable: false },
        subject: { type: "string", editable: true },
      },
    };

    const { container } = render(SchemaField, {
      props: {
        name: "ports",
        schema,
        value: { inputs: [], outputs: [] },
      },
    });

    // Should render port config editor sections
    expect(container.textContent).toContain("Input Ports");
    expect(container.textContent).toContain("Output Ports");

    // Should have add buttons
    const buttons = container.querySelectorAll("button");
    expect(buttons.length).toBeGreaterThanOrEqual(2);
  });
});
