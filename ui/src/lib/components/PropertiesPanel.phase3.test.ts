import { describe, it, expect, vi } from "vitest";
import { render, fireEvent } from "@testing-library/svelte";
import { tick } from "svelte";
import type { ComponentType } from "$lib/types/component";
import type { FlowNode } from "$lib/types/flow";
import PropertiesPanel from "./PropertiesPanel.svelte";

const makeComponentType = (): ComponentType => ({
  id: "test-comp",
  name: "Test Component",
  type: "processor",
  protocol: "nats",
  category: "processor",
  description: "Test",
  version: "1.0.0",
  schema: {
    type: "object",
    properties: {
      host: { type: "string", description: "Host", category: "basic" },
      port: {
        type: "int",
        description: "Port number",
        category: "basic",
        minimum: 1,
        maximum: 65535,
      },
    },
    required: ["host"],
  },
});

const makeNode = (
  id: string,
  name: string,
  config: Record<string, unknown>,
): FlowNode => ({
  id,
  component: "test-comp",
  type: "processor",
  name,
  position: { x: 0, y: 0 },
  config,
});

describe("PropertiesPanel - Phase 3 gap: form state resets when node prop changes", () => {
  it("should reset editedName to the new node name when node changes", async () => {
    const nodeA = makeNode("node-a", "Alpha Node", {
      host: "alpha.example.com",
      port: 1111,
    });
    const nodeB = makeNode("node-b", "Beta Node", {
      host: "beta.example.com",
      port: 2222,
    });
    const componentType = makeComponentType();

    const { container, rerender } = render(PropertiesPanel, {
      props: {
        mode: "edit",
        node: nodeA,
        nodeComponentType: componentType,
      },
    });

    // Verify node A is loaded
    const nameInput = container.querySelector(
      '[data-testid="prop-name-input"]',
    ) as HTMLInputElement;
    expect(nameInput.value).toBe("Alpha Node");

    // Modify the name in the form (makes it dirty)
    await fireEvent.input(nameInput, { target: { value: "Modified Alpha" } });
    await tick();
    expect(nameInput.value).toBe("Modified Alpha");

    // Switch to node B
    await rerender({ node: nodeB });
    await tick();

    // Form must reflect node B, not the unsaved edit
    expect(nameInput.value).toBe("Beta Node");
  });

  it("should reset editedConfig to the new node config when node changes", async () => {
    const nodeA = makeNode("node-a", "Alpha Node", {
      host: "alpha.example.com",
      port: 1111,
    });
    const nodeB = makeNode("node-b", "Beta Node", {
      host: "beta.example.com",
      port: 2222,
    });
    const componentType = makeComponentType();

    const { container, rerender } = render(PropertiesPanel, {
      props: {
        mode: "edit",
        node: nodeA,
        nodeComponentType: componentType,
      },
    });

    // Switch to node B
    await rerender({ node: nodeB });
    await tick();

    // Config fields should reflect node B's values
    const hostInput = container.querySelector("#host") as HTMLInputElement;
    const portInput = container.querySelector("#port") as HTMLInputElement;

    expect(hostInput.value).toBe("beta.example.com");
    expect(portInput.value).toBe("2222");
  });

  it("should clear search query when node changes", async () => {
    const nodeA = makeNode("node-a", "Alpha Node", {
      host: "localhost",
      port: 9000,
    });
    const nodeB = makeNode("node-b", "Beta Node", {
      host: "remotehost",
      port: 9001,
    });
    const componentType = makeComponentType();

    const { container, rerender } = render(PropertiesPanel, {
      props: {
        mode: "edit",
        node: nodeA,
        nodeComponentType: componentType,
      },
    });

    // Type something in the search box
    const searchInput = container.querySelector(
      '[data-testid="field-search"]',
    ) as HTMLInputElement;
    await fireEvent.input(searchInput, { target: { value: "host" } });
    await tick();

    expect(searchInput.value).toBe("host");

    // Switch to node B
    await rerender({ node: nodeB });
    await tick();

    // Search query must be cleared — it should not carry over between nodes
    expect(searchInput.value).toBe("");
  });

  it("should dismiss delete confirmation when node changes", async () => {
    const nodeA = makeNode("node-a", "Alpha Node", {
      host: "localhost",
      port: 9000,
    });
    const nodeB = makeNode("node-b", "Beta Node", {
      host: "remotehost",
      port: 9001,
    });
    const componentType = makeComponentType();

    const { container, rerender } = render(PropertiesPanel, {
      props: {
        mode: "edit",
        node: nodeA,
        nodeComponentType: componentType,
        onDelete: vi.fn(),
      },
    });

    // Open delete confirmation
    const deleteButton = container.querySelector(
      ".btn-danger",
    ) as HTMLButtonElement;
    await fireEvent.click(deleteButton);
    await tick();

    expect(
      container.querySelector('[data-testid="delete-confirm"]'),
    ).toBeInTheDocument();

    // Switch to node B
    await rerender({ node: nodeB });
    await tick();

    // Delete confirmation must be dismissed — it was for node A
    expect(
      container.querySelector('[data-testid="delete-confirm"]'),
    ).not.toBeInTheDocument();
  });
});
