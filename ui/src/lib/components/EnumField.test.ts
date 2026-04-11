import { describe, it, expect } from "vitest";
import { render } from "@testing-library/svelte";
import type { PropertySchema } from "$lib/types/schema";
import EnumField from "./EnumField.svelte";

describe("EnumField", () => {
  // T032: Component test EnumField - dropdown rendering
  it("should render dropdown with enum options", () => {
    const schema: PropertySchema = {
      type: "enum",
      description: "Log level",
      enum: ["debug", "info", "warn", "error"],
    };

    const { container } = render(EnumField, {
      props: {
        name: "log_level",
        schema,
        value: "info",
      },
    });

    const select = container.querySelector("select");
    expect(select).toBeInTheDocument();
    expect(select).toHaveAttribute("id", "log_level");
    expect(select).toHaveValue("info");

    // Check all options are present
    const options = container.querySelectorAll("option");
    expect(options).toHaveLength(4);
    expect(options[0]).toHaveValue("debug");
    expect(options[0]).toHaveTextContent("debug");
    expect(options[1]).toHaveValue("info");
    expect(options[2]).toHaveValue("warn");
    expect(options[3]).toHaveValue("error");
  });

  it("should render empty dropdown when enum array is empty", () => {
    const schema: PropertySchema = {
      type: "enum",
      description: "Empty enum",
      enum: [],
    };

    const { container } = render(EnumField, {
      props: {
        name: "empty",
        schema,
      },
    });

    const select = container.querySelector("select");
    expect(select).toBeInTheDocument();

    const options = container.querySelectorAll("option");
    expect(options).toHaveLength(0);
  });

  it("should render without value (no selection)", () => {
    const schema: PropertySchema = {
      type: "enum",
      description: "Priority",
      enum: ["low", "medium", "high"],
    };

    const { container } = render(EnumField, {
      props: {
        name: "priority",
        schema,
      },
    });

    const select = container.querySelector("select");
    expect(select).toBeInTheDocument();
    expect(select).toHaveValue(""); // No selection
  });

  it("should render label and description", () => {
    const schema: PropertySchema = {
      type: "enum",
      description: "Select log level",
      enum: ["debug", "info"],
    };

    const { container } = render(EnumField, {
      props: {
        name: "log_level",
        schema,
      },
    });

    const label = container.querySelector('label[for="log_level"]');
    expect(label).toBeInTheDocument();
    expect(label).toHaveTextContent("log_level");

    const description = container.querySelector(".description");
    expect(description).toBeInTheDocument();
    expect(description).toHaveTextContent("Select log level");
  });

  it("should display error message", () => {
    const schema: PropertySchema = {
      type: "enum",
      description: "Priority level",
      enum: ["low", "high"],
    };

    const { container } = render(EnumField, {
      props: {
        name: "priority",
        schema,
        error: "Invalid selection",
      },
    });

    const errorEl = container.querySelector(".error");
    expect(errorEl).toBeInTheDocument();
    expect(errorEl).toHaveTextContent("Invalid selection");
  });

  it("should mark as required when isRequired is true", () => {
    const schema: PropertySchema = {
      type: "enum",
      description: "Required enum",
      enum: ["opt1", "opt2"],
    };

    const { container } = render(EnumField, {
      props: {
        name: "required_enum",
        schema,
        isRequired: true,
      },
    });

    const select = container.querySelector("select");
    expect(select).toHaveAttribute("aria-required", "true");

    const label = container.querySelector("label");
    expect(label).toHaveTextContent("*"); // Required marker
  });

  it("should handle single enum option without required (shows empty option)", () => {
    const schema: PropertySchema = {
      type: "enum",
      description: "Single option",
      enum: ["only-option"],
    };

    const { container } = render(EnumField, {
      props: {
        name: "single",
        schema,
      },
    });

    // Not required, so should have empty option + the value
    const options = container.querySelectorAll("option");
    expect(options).toHaveLength(2);
    expect(options[0]).toHaveValue(""); // Empty option
    expect(options[1]).toHaveValue("only-option");
  });

  it("should handle single enum option when required (no empty option)", () => {
    const schema: PropertySchema = {
      type: "enum",
      description: "Single required option",
      enum: ["only-option"],
    };

    const { container } = render(EnumField, {
      props: {
        name: "single_required",
        schema,
        isRequired: true,
      },
    });

    // Required field, so no empty option
    const options = container.querySelectorAll("option");
    expect(options).toHaveLength(1);
    expect(options[0]).toHaveValue("only-option");
  });
});
