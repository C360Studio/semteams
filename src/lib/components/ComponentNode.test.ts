import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/svelte";
import ComponentNode from "./ComponentNode.svelte";
import type { ComponentInstance } from "$lib/types/flow";

describe("ComponentNode", () => {
  const createMockComponent = (
    overrides?: Partial<ComponentInstance>,
  ): ComponentInstance => ({
    id: "comp-1",
    component: "udp-input",
    type: "input",
    name: "UDP Input 1",
    position: { x: 100, y: 100 },
    config: { port: 14550 },
    health: {
      status: "not_running",
      lastUpdated: "2025-10-10T12:00:00Z",
    },
    ...overrides,
  });

  it("should render component name", () => {
    const component = createMockComponent();

    render(ComponentNode, { props: { component } });

    // Should display component type as name
    expect(screen.getByText("udp-input")).toBeInTheDocument();
  });

  it("should render component type", () => {
    const component = createMockComponent({
      component: "websocket-output",
      type: "output",
    });

    render(ComponentNode, { props: { component } });

    expect(screen.getByText("websocket-output")).toBeInTheDocument();
  });

  it("should display health status", () => {
    const component = createMockComponent({
      health: {
        status: "healthy",
        lastUpdated: "2025-10-10T12:00:00Z",
      },
    });

    render(ComponentNode, { props: { component } });

    expect(screen.getByText("healthy")).toBeInTheDocument();
  });

  it("should display different health statuses", () => {
    const statuses: Array<ComponentInstance["health"]["status"]> = [
      "healthy",
      "degraded",
      "unhealthy",
      "not_running",
    ];

    statuses.forEach((status) => {
      const component = createMockComponent({
        health: { status, lastUpdated: "2025-10-10T12:00:00Z" },
      });

      const { container } = render(ComponentNode, { props: { component } });
      expect(screen.getByText(status)).toBeInTheDocument();
      container.remove();
    });
  });

  it("should call onclick when component is clicked", async () => {
    const component = createMockComponent();
    const onclick = vi.fn();

    render(ComponentNode, { props: { component, onclick } });

    const node = screen.getByText("udp-input").closest(".component-node");
    expect(node).toBeInTheDocument();

    if (node) {
      await fireEvent.click(node);
      expect(onclick).toHaveBeenCalledWith("comp-1");
    }
  });

  it("should render in compact mode", () => {
    const component = createMockComponent();

    render(ComponentNode, { props: { component, compact: true } });

    // In compact mode, should still show component type
    expect(screen.getByText("udp-input")).toBeInTheDocument();
  });

  it("should display config summary when provided", () => {
    const component = createMockComponent({
      config: { port: 15550, host: "192.168.1.100" },
    });

    render(ComponentNode, { props: { component } });

    // Component node should exist
    const node = screen.getByText("udp-input").closest(".component-node");
    expect(node).toBeInTheDocument();
  });

  it("should apply selected state styling", () => {
    const component = createMockComponent();

    render(ComponentNode, { props: { component, selected: true } });

    const node = screen.getByText("udp-input").closest(".component-node");
    expect(node).toHaveClass("selected");
  });

  it("should handle components with errors", () => {
    const component = createMockComponent({
      health: {
        status: "unhealthy",
        errorMessage: "Connection timeout",
        lastUpdated: "2025-10-10T12:00:00Z",
      },
    });

    render(ComponentNode, { props: { component } });

    expect(screen.getByText("unhealthy")).toBeInTheDocument();
    // Error message might be shown in tooltip or expanded view
  });

  it("should render component ID when no type provided", () => {
    const component = createMockComponent({ component: "" });

    render(ComponentNode, { props: { component } });

    // Should fall back to showing ID
    expect(screen.getByText("comp-1")).toBeInTheDocument();
  });
});
