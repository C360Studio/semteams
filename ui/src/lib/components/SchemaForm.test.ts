import { describe, it, expect, vi } from "vitest";
import { render, fireEvent } from "@testing-library/svelte";
import { tick } from "svelte";
import type { ConfigSchema } from "$lib/types/schema";
import SchemaForm from "./SchemaForm.svelte";

describe("SchemaForm", () => {
  // T024: Component test SchemaForm - renders basic fields section
  it("should render basic fields in Basic Configuration section", () => {
    const schema: ConfigSchema = {
      properties: {
        port: {
          type: "int",
          description: "UDP port",
          category: "basic",
          minimum: 1,
          maximum: 65535,
        },
        bind_address: {
          type: "string",
          description: "IP address",
          category: "basic",
        },
        buffer_size: {
          type: "int",
          description: "Buffer size",
          category: "advanced",
        },
      },
      required: ["port"],
    };

    const { container } = render(SchemaForm, {
      props: { schema },
    });

    // Check for Basic Configuration section
    const basicSection = container.querySelector(".basic-config");
    expect(basicSection).toBeInTheDocument();

    const basicHeading = container.querySelector(".basic-config h3");
    expect(basicHeading).toHaveTextContent("Basic Configuration");

    // Basic fields should be in this section
    const portInput = basicSection?.querySelector("input#port");
    expect(portInput).toBeInTheDocument();

    const bindAddressInput = basicSection?.querySelector("input#bind_address");
    expect(bindAddressInput).toBeInTheDocument();
  });

  // T025: Component test SchemaForm - renders advanced fields collapsible
  it("should render advanced fields in collapsible Advanced Configuration section", () => {
    const schema: ConfigSchema = {
      properties: {
        port: {
          type: "int",
          description: "UDP port",
          category: "basic",
        },
        buffer_size: {
          type: "int",
          description: "Buffer size",
          category: "advanced",
        },
        timeout: {
          type: "float",
          description: "Timeout",
          // No category defaults to advanced
        },
      },
      required: [],
    };

    const { container } = render(SchemaForm, {
      props: { schema },
    });

    // Check for Advanced Configuration section (collapsible details)
    const advancedSection = container.querySelector("details.advanced-config");
    expect(advancedSection).toBeInTheDocument();

    const summary = advancedSection?.querySelector("summary");
    expect(summary).toHaveTextContent("Advanced Configuration");

    // Advanced fields should be in this section
    const bufferInput = advancedSection?.querySelector("input#buffer_size");
    expect(bufferInput).toBeInTheDocument();

    const timeoutInput = advancedSection?.querySelector("input#timeout");
    expect(timeoutInput).toBeInTheDocument();
  });

  // T026: Component test SchemaForm - pre-fills defaults
  it("should pre-fill fields with schema defaults", async () => {
    const schema: ConfigSchema = {
      properties: {
        port: {
          type: "int",
          description: "UDP port",
          default: 14550,
          category: "basic",
        },
        bind_address: {
          type: "string",
          description: "IP address",
          default: "0.0.0.0",
          category: "basic",
        },
        enabled: {
          type: "bool",
          description: "Enabled",
          default: true,
          category: "basic",
        },
      },
      required: [],
    };

    const { container } = render(SchemaForm, {
      props: { schema },
    });

    // Wait for $effect to apply defaults
    await tick();

    const portInput = container.querySelector("input#port") as HTMLInputElement;
    expect(portInput?.value).toBe("14550");

    const bindInput = container.querySelector(
      "input#bind_address",
    ) as HTMLInputElement;
    expect(bindInput?.value).toBe("0.0.0.0");

    const enabledInput = container.querySelector(
      "input#enabled",
    ) as HTMLInputElement;
    expect(enabledInput?.checked).toBe(true);
  });

  // T027: Component test SchemaForm - validates callback props pattern
  it("should call onSave callback when form submitted", async () => {
    const onSave = vi.fn();
    const schema: ConfigSchema = {
      properties: {
        port: {
          type: "int",
          description: "Port",
          category: "basic",
        },
      },
      required: [],
    };

    const { container } = render(SchemaForm, {
      props: {
        schema,
        onSave,
      },
    });

    const form = container.querySelector("form");
    expect(form).toBeInTheDocument();

    // Fill in a value
    const portInput = container.querySelector("input#port") as HTMLInputElement;
    portInput.value = "8080";
    await fireEvent.input(portInput);

    // Submit form
    await fireEvent.submit(form!);

    // Verify callback called with config
    expect(onSave).toHaveBeenCalledTimes(1);
    expect(onSave).toHaveBeenCalledWith(
      expect.objectContaining({
        port: expect.any(Number),
      }),
    );
  });

  // T036: Component test validation error display inline
  it("should display validation error inline after save attempt", async () => {
    const onSave = vi.fn();
    const schema: ConfigSchema = {
      properties: {
        port: {
          type: "int",
          description: "Port",
          minimum: 1,
          maximum: 65535,
          category: "basic",
        },
      },
      required: ["port"],
    };

    const { container } = render(SchemaForm, {
      props: {
        schema,
        onSave,
      },
    });

    // Submit without filling required field
    const form = container.querySelector("form");
    await fireEvent.submit(form!);

    // Wait for validation
    await tick();

    // Error should appear below port field
    const errorEl = container.querySelector(".error");
    expect(errorEl).toBeInTheDocument();
    expect(errorEl).toHaveTextContent(/required/i);

    // onSave should NOT be called when invalid
    expect(onSave).not.toHaveBeenCalled();
  });

  // T037: Component test debounced real-time validation
  it("should show validation feedback after debounce", async () => {
    const schema: ConfigSchema = {
      properties: {
        port: {
          type: "int",
          description: "Port",
          minimum: 1,
          maximum: 65535,
          category: "basic",
        },
      },
      required: [],
    };

    const { container } = render(SchemaForm, {
      props: { schema },
    });

    const portInput = container.querySelector("input#port") as HTMLInputElement;

    // Type invalid value
    portInput.value = "99999";
    await fireEvent.input(portInput);

    // Error should NOT appear immediately
    let errorEl = container.querySelector(".error");
    expect(errorEl).not.toBeInTheDocument();

    // Wait for debounce (300ms)
    await new Promise((resolve) => setTimeout(resolve, 350));
    await tick();

    // Now error should appear
    errorEl = container.querySelector(".error");
    expect(errorEl).toBeInTheDocument();
    expect(errorEl).toHaveTextContent(/65535/);
  });

  // T039: Component test prevent submission with validation errors
  it("should prevent submission if validation fails", async () => {
    const onSave = vi.fn();
    const schema: ConfigSchema = {
      properties: {
        port: {
          type: "int",
          description: "Port",
          category: "basic",
        },
      },
      required: ["port"],
    };

    const { container } = render(SchemaForm, {
      props: {
        schema,
        onSave,
      },
    });

    // Try to submit with empty required field
    const form = container.querySelector("form");
    await fireEvent.submit(form!);
    await tick();

    // onSave should NOT be called
    expect(onSave).not.toHaveBeenCalled();

    // Error message shown
    const errorEl = container.querySelector(".error");
    expect(errorEl).toBeInTheDocument();
  });

  it("should call onCancel callback when cancel clicked", async () => {
    const onCancel = vi.fn();
    const schema: ConfigSchema = {
      properties: {
        port: {
          type: "int",
          description: "Port",
          category: "basic",
        },
      },
      required: [],
    };

    const { container } = render(SchemaForm, {
      props: {
        schema,
        onCancel,
      },
    });

    const cancelButton = container.querySelector('button[type="button"]');
    expect(cancelButton).toBeInTheDocument();
    expect(cancelButton).toHaveTextContent(/cancel/i);

    await fireEvent.click(cancelButton!);

    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  it("should handle existing config values", async () => {
    const config = {
      port: 8080,
      bind_address: "127.0.0.1",
    };

    const schema: ConfigSchema = {
      properties: {
        port: {
          type: "int",
          description: "Port",
          default: 14550,
          category: "basic",
        },
        bind_address: {
          type: "string",
          description: "IP",
          default: "0.0.0.0",
          category: "basic",
        },
      },
      required: [],
    };

    const { container } = render(SchemaForm, {
      props: {
        schema,
        config,
      },
    });

    await tick();

    // Should use existing config, not defaults
    const portInput = container.querySelector("input#port") as HTMLInputElement;
    expect(portInput?.value).toBe("8080");

    const bindInput = container.querySelector(
      "input#bind_address",
    ) as HTMLInputElement;
    expect(bindInput?.value).toBe("127.0.0.1");
  });

  it("should alphabetically sort fields within each category", () => {
    const schema: ConfigSchema = {
      properties: {
        zebra: {
          type: "string",
          description: "Last alphabetically",
          category: "basic",
        },
        alpha: {
          type: "string",
          description: "First alphabetically",
          category: "basic",
        },
        beta: {
          type: "string",
          description: "Second alphabetically",
          category: "basic",
        },
      },
      required: [],
    };

    const { container } = render(SchemaForm, {
      props: { schema },
    });

    const basicSection = container.querySelector(".basic-config");
    const inputs = basicSection?.querySelectorAll("input");

    // Should be sorted: alpha, beta, zebra
    expect(inputs?.[0]).toHaveAttribute("id", "alpha");
    expect(inputs?.[1]).toHaveAttribute("id", "beta");
    expect(inputs?.[2]).toHaveAttribute("id", "zebra");
  });

  // T006: Test for config prop reactivity (Bug #2)
  it("should sync internal state when config prop changes", async () => {
    const schema: ConfigSchema = {
      properties: {
        port: {
          type: "int",
          description: "Port",
          category: "basic",
        },
      },
      required: [],
    };

    const config1 = { port: 14550 };
    const config2 = { port: 8080 };

    // Step 1: Initial render with config1
    const { container, rerender } = render(SchemaForm, {
      props: { schema, config: config1 },
    });

    await tick();

    // Step 2: Verify initial state
    const portInput = container.querySelector("input#port") as HTMLInputElement;
    expect(portInput?.value).toBe("14550");

    // Step 3: Update config prop (Svelte 5 pattern)
    await rerender({ schema, config: config2 });
    await tick();

    // Step 4: Verify component updated (THIS WILL FAIL - Bug #2)
    // Current: $state(config) initializes once and never updates
    // Expected: Should reactively sync with config prop changes
    expect(portInput?.value).toBe("8080");
  });
});
