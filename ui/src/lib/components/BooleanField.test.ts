import { describe, it, expect } from "vitest";
import { render } from "@testing-library/svelte";
import { tick } from "svelte";
import type { PropertySchema } from "$lib/types/schema";
import BooleanField from "./BooleanField.svelte";

describe("BooleanField", () => {
  // T031: Component test BooleanField - checkbox rendering
  it("should render checkbox for boolean field", () => {
    const schema: PropertySchema = {
      type: "bool",
      description: "Enable component",
    };

    const { container } = render(BooleanField, {
      props: {
        name: "enabled",
        schema,
        value: true,
      },
    });

    const input = container.querySelector('input[type="checkbox"]');
    expect(input).toBeInTheDocument();
    expect(input).toHaveAttribute("id", "enabled");
    expect(input).toBeChecked();
  });

  it("should render unchecked checkbox when value is false", () => {
    const schema: PropertySchema = {
      type: "bool",
      description: "Enable feature",
    };

    const { container } = render(BooleanField, {
      props: {
        name: "enabled",
        schema,
        value: false,
      },
    });

    const input = container.querySelector('input[type="checkbox"]');
    expect(input).toBeInTheDocument();
    expect(input).not.toBeChecked();
  });

  it("should render unchecked when no value provided", () => {
    const schema: PropertySchema = {
      type: "bool",
      description: "Enable feature",
    };

    const { container } = render(BooleanField, {
      props: {
        name: "enabled",
        schema,
      },
    });

    const input = container.querySelector('input[type="checkbox"]');
    expect(input).not.toBeChecked();
  });

  it("should render label and description", () => {
    const schema: PropertySchema = {
      type: "bool",
      description: "Enable debug mode",
    };

    const { container } = render(BooleanField, {
      props: {
        name: "debug",
        schema,
      },
    });

    const label = container.querySelector('label[for="debug"]');
    expect(label).toBeInTheDocument();
    expect(label).toHaveTextContent("debug");

    const description = container.querySelector(".description");
    expect(description).toBeInTheDocument();
    expect(description).toHaveTextContent("Enable debug mode");
  });

  it("should display error message when provided", () => {
    const schema: PropertySchema = {
      type: "bool",
      description: "Test field",
    };

    const { container } = render(BooleanField, {
      props: {
        name: "test",
        schema,
        error: "Invalid value",
      },
    });

    const errorEl = container.querySelector(".error");
    expect(errorEl).toBeInTheDocument();
    expect(errorEl).toHaveTextContent("Invalid value");
  });

  it("should handle checkbox reactivity", async () => {
    const schema: PropertySchema = {
      type: "bool",
      description: "Toggle feature",
    };

    const { container, rerender } = render(BooleanField, {
      props: {
        name: "toggle",
        schema,
        value: false,
      },
    });

    let input = container.querySelector('input[type="checkbox"]');
    expect(input).not.toBeChecked();

    // Update prop using Svelte 5 rerender API
    rerender({ value: true });
    await tick();

    input = container.querySelector('input[type="checkbox"]');
    expect(input).toBeChecked();
  });
});
