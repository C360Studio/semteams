import { describe, it, expect } from "vitest";
import { render } from "@testing-library/svelte";
import type { PropertySchema } from "$lib/types/schema";
import NumberField from "./NumberField.svelte";

describe("NumberField", () => {
  // T030: Component test NumberField - number input with min/max
  it("should render number input with min/max from schema", () => {
    const schema: PropertySchema = {
      type: "int",
      description: "UDP port to listen on",
      minimum: 1,
      maximum: 65535,
    };

    const { container } = render(NumberField, {
      props: {
        name: "port",
        schema,
        value: 14550,
      },
    });

    const input = container.querySelector('input[type="number"]');
    expect(input).toBeInTheDocument();
    expect(input).toHaveAttribute("id", "port");
    expect(input).toHaveAttribute("min", "1");
    expect(input).toHaveAttribute("max", "65535");
    expect(input).toHaveValue(14550);
  });

  it("should render without min/max when not specified", () => {
    const schema: PropertySchema = {
      type: "int",
      description: "Count field",
    };

    const { container } = render(NumberField, {
      props: {
        name: "count",
        schema,
      },
    });

    const input = container.querySelector('input[type="number"]');
    expect(input).toBeInTheDocument();
    expect(input).not.toHaveAttribute("min");
    expect(input).not.toHaveAttribute("max");
  });

  it("should render label and description", () => {
    const schema: PropertySchema = {
      type: "int",
      description: "Port number",
    };

    const { container } = render(NumberField, {
      props: {
        name: "port",
        schema,
      },
    });

    const label = container.querySelector('label[for="port"]');
    expect(label).toBeInTheDocument();
    expect(label).toHaveTextContent("port");

    const description = container.querySelector(".description");
    expect(description).toBeInTheDocument();
    expect(description).toHaveTextContent("Port number");
  });

  it("should display error message", () => {
    const schema: PropertySchema = {
      type: "int",
      description: "Port",
    };

    const { container } = render(NumberField, {
      props: {
        name: "port",
        schema,
        error: "Must be between 1 and 65535",
      },
    });

    const errorEl = container.querySelector(".error");
    expect(errorEl).toBeInTheDocument();
    expect(errorEl).toHaveTextContent("Must be between 1 and 65535");
  });

  it("should handle float type", () => {
    const schema: PropertySchema = {
      type: "float",
      description: "Timeout in seconds",
      minimum: 0.1,
      maximum: 60.0,
    };

    const { container } = render(NumberField, {
      props: {
        name: "timeout",
        schema,
        value: 5.5,
      },
    });

    const input = container.querySelector('input[type="number"]');
    expect(input).toBeInTheDocument();
    expect(input).toHaveAttribute("step", "any"); // For floats
    expect(input).toHaveValue(5.5);
  });

  it("should mark as required when isRequired is true", () => {
    const schema: PropertySchema = {
      type: "int",
      description: "Required number",
    };

    const { container } = render(NumberField, {
      props: {
        name: "required_num",
        schema,
        isRequired: true,
      },
    });

    const input = container.querySelector("input");
    expect(input).toHaveAttribute("aria-required", "true");

    const label = container.querySelector("label");
    expect(label).toHaveTextContent("*"); // Required marker
  });
});
