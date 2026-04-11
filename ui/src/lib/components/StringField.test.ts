import { describe, it, expect } from "vitest";
import { render } from "@testing-library/svelte";
import type { PropertySchema } from "$lib/types/schema";
import StringField from "./StringField.svelte";

describe("StringField", () => {
  // T029: Component test StringField - text input rendering
  it("should render text input with label and description", () => {
    const schema: PropertySchema = {
      type: "string",
      description: "IP address to bind to",
    };

    const { container } = render(StringField, {
      props: {
        name: "bind_address",
        schema,
        value: "0.0.0.0",
      },
    });

    // Check for input element
    const input = container.querySelector('input[type="text"]');
    expect(input).toBeInTheDocument();
    expect(input).toHaveAttribute("id", "bind_address");
    expect(input).toHaveValue("0.0.0.0");

    // Check for label
    const label = container.querySelector('label[for="bind_address"]');
    expect(label).toBeInTheDocument();
    expect(label).toHaveTextContent("bind_address");

    // Check for description
    const description = container.querySelector(".description");
    expect(description).toBeInTheDocument();
    expect(description).toHaveTextContent("IP address to bind to");
  });

  it("should render without value (empty)", () => {
    const schema: PropertySchema = {
      type: "string",
      description: "Optional field",
    };

    const { container } = render(StringField, {
      props: {
        name: "optional",
        schema,
      },
    });

    const input = container.querySelector('input[type="text"]');
    expect(input).toBeInTheDocument();
    expect(input).toHaveValue("");
  });

  it("should display error message when provided", () => {
    const schema: PropertySchema = {
      type: "string",
      description: "Test field",
    };

    const { container } = render(StringField, {
      props: {
        name: "test",
        schema,
        error: "This field is required",
      },
    });

    const errorEl = container.querySelector(".error");
    expect(errorEl).toBeInTheDocument();
    expect(errorEl).toHaveTextContent("This field is required");
  });

  it("should not display error span when no error", () => {
    const schema: PropertySchema = {
      type: "string",
      description: "Test field",
    };

    const { container } = render(StringField, {
      props: {
        name: "test",
        schema,
      },
    });

    const errorEl = container.querySelector(".error");
    expect(errorEl).not.toBeInTheDocument();
  });

  it("should apply required attribute when isRequired is true", () => {
    const schema: PropertySchema = {
      type: "string",
      description: "Required field",
    };

    const { container } = render(StringField, {
      props: {
        name: "required_field",
        schema,
        isRequired: true,
      },
    });

    const input = container.querySelector("input");
    expect(input).toHaveAttribute("aria-required", "true");

    const label = container.querySelector("label");
    expect(label).toHaveTextContent("*"); // Required marker
  });

  it("should not show required marker when isRequired is false", () => {
    const schema: PropertySchema = {
      type: "string",
      description: "Optional field",
    };

    const { container } = render(StringField, {
      props: {
        name: "optional_field",
        schema,
        isRequired: false,
      },
    });

    const requiredMarker = container.querySelector(".required");
    expect(requiredMarker).not.toBeInTheDocument();
  });
});
