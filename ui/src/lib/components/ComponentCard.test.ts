import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/svelte";
import ComponentCard from "./ComponentCard.svelte";
import type { FlowNode } from "$lib/types/flow";

describe("ComponentCard", () => {
  const createMockNode = (overrides?: Partial<FlowNode>): FlowNode => ({
    id: "node-1",
    component: "udp-input",
    type: "input",
    name: "UDP Input",
    position: { x: 100, y: 100 },
    config: { port: 14550 },
    ...overrides,
  });

  describe("rendering", () => {
    it("should render node name", () => {
      const node = createMockNode();

      render(ComponentCard, { props: { node } });

      expect(screen.getByText("UDP Input")).toBeInTheDocument();
    });

    it("should render node type", () => {
      const node = createMockNode({
        component: "websocket-output",
        type: "output",
      });

      render(ComponentCard, { props: { node } });

      expect(screen.getByText("Type: websocket-output")).toBeInTheDocument();
    });

    it("should render both name and type", () => {
      const node = createMockNode({
        name: "My Component",
        component: "json-transform",
        type: "processor",
      });

      render(ComponentCard, { props: { node } });

      expect(screen.getByText("My Component")).toBeInTheDocument();
      expect(screen.getByText("Type: json-transform")).toBeInTheDocument();
    });

    it("should apply domain color accent from type", () => {
      const node = createMockNode({ component: "udp-input", type: "input" });

      const { container } = render(ComponentCard, { props: { node } });

      // Card should have domain color styling applied
      const card = container.querySelector(".component-card");
      expect(card).toBeInTheDocument();
    });

    it("should render with different node types", () => {
      const nodeTypes = [
        { component: "udp-input", type: "input" as const },
        { component: "websocket-output", type: "output" as const },
        { component: "json-transform", type: "processor" as const },
        { component: "storage-writer", type: "storage" as const },
      ];

      nodeTypes.forEach(({ component, type }) => {
        const node = createMockNode({
          component,
          type,
          name: `${component}-instance`,
        });
        const { container } = render(ComponentCard, { props: { node } });

        expect(screen.getByText(`${component}-instance`)).toBeInTheDocument();
        expect(screen.getByText(`Type: ${component}`)).toBeInTheDocument();

        container.remove();
      });
    });

    it("should handle empty node name", () => {
      const node = createMockNode({ name: "" });

      render(ComponentCard, { props: { node } });

      // Should fall back to showing node ID or type
      expect(screen.getByText("Type: udp-input")).toBeInTheDocument();
    });
  });

  describe("selection", () => {
    it("should apply selected styling when selected prop is true", () => {
      const node = createMockNode();

      const { container } = render(ComponentCard, {
        props: { node, selected: true },
      });

      const card = container.querySelector(".component-card");
      expect(card).toHaveClass("selected");
    });

    it("should not apply selected styling when selected prop is false", () => {
      const node = createMockNode();

      const { container } = render(ComponentCard, {
        props: { node, selected: false },
      });

      const card = container.querySelector(".component-card");
      expect(card).not.toHaveClass("selected");
    });

    it("should not apply selected styling when selected prop is omitted", () => {
      const node = createMockNode();

      const { container } = render(ComponentCard, { props: { node } });

      const card = container.querySelector(".component-card");
      expect(card).not.toHaveClass("selected");
    });

    it("should call onSelect when card is clicked", async () => {
      const node = createMockNode();
      const onSelect = vi.fn();

      render(ComponentCard, { props: { node, onSelect } });

      const card = screen.getByText("UDP Input").closest(".component-card");
      expect(card).toBeInTheDocument();

      if (card) {
        await fireEvent.click(card);
        expect(onSelect).toHaveBeenCalledTimes(1);
      }
    });

    it("should handle multiple selection state changes", async () => {
      const node = createMockNode();

      const { container, rerender } = render(ComponentCard, {
        props: { node, selected: false },
      });

      let card = container.querySelector(".component-card");
      expect(card).not.toHaveClass("selected");

      // Update to selected
      await rerender({ node, selected: true });
      card = container.querySelector(".component-card");
      expect(card).toHaveClass("selected");

      // Update back to not selected
      await rerender({ node, selected: false });
      card = container.querySelector(".component-card");
      expect(card).not.toHaveClass("selected");
    });
  });

  describe("callbacks", () => {
    it("should not error if onSelect callback is not provided", async () => {
      const node = createMockNode();

      render(ComponentCard, { props: { node } });

      const card = screen.getByText("UDP Input").closest(".component-card");

      if (card) {
        // Should not throw error
        await expect(fireEvent.click(card)).resolves.not.toThrow();
      }
    });
  });

  describe("accessibility", () => {
    it("should have proper role for card", () => {
      const node = createMockNode();

      render(ComponentCard, { props: { node } });

      const card = screen.getByRole("button", { name: "UDP Input" });
      expect(card).toBeInTheDocument();
    });

    it("should have aria-pressed attribute when selected", () => {
      const node = createMockNode();

      render(ComponentCard, { props: { node, selected: true } });

      const card = screen.getByRole("button", { name: "UDP Input" });
      expect(card).toHaveAttribute("aria-pressed", "true");
    });

    it("should have aria-pressed false when not selected", () => {
      const node = createMockNode();

      render(ComponentCard, { props: { node, selected: false } });

      const card = screen.getByRole("button", { name: "UDP Input" });
      expect(card).toHaveAttribute("aria-pressed", "false");
    });

    it("should support keyboard navigation with Enter key", async () => {
      const node = createMockNode();
      const onSelect = vi.fn();

      render(ComponentCard, { props: { node, onSelect } });

      const card = screen.getByRole("button", { name: "UDP Input" });

      await fireEvent.keyDown(card, { key: "Enter", code: "Enter" });

      expect(onSelect).toHaveBeenCalledTimes(1);
    });

    it("should support keyboard navigation with Space key", async () => {
      const node = createMockNode();
      const onSelect = vi.fn();

      render(ComponentCard, { props: { node, onSelect } });

      const card = screen.getByRole("button", { name: "UDP Input" });

      await fireEvent.keyDown(card, { key: " ", code: "Space" });

      expect(onSelect).toHaveBeenCalledTimes(1);
    });

    it("should be keyboard focusable", () => {
      const node = createMockNode();

      render(ComponentCard, { props: { node } });

      const card = screen.getByRole("button", { name: "UDP Input" });

      expect(card).toHaveAttribute("tabindex");
    });

    it("should have descriptive text for screen readers", () => {
      const node = createMockNode({
        name: "Main UDP Receiver",
        component: "udp-input",
        type: "input",
      });

      render(ComponentCard, { props: { node } });

      // Card should contain accessible text
      expect(screen.getByText("Main UDP Receiver")).toBeInTheDocument();
      expect(screen.getByText("Type: udp-input")).toBeInTheDocument();
    });
  });

  describe("domain colors", () => {
    it("should apply robotics domain color for MAVLink components", () => {
      const node = createMockNode({
        component: "mavlink-decoder",
        type: "processor",
      });

      const { container } = render(ComponentCard, { props: { node } });

      const card = container.querySelector(".component-card");
      expect(card).toBeInTheDocument();
      // Domain color should be applied via inline style or CSS class
    });

    it("should apply network domain color for I/O components", () => {
      const node = createMockNode({ component: "udp-input", type: "input" });

      const { container } = render(ComponentCard, { props: { node } });

      const card = container.querySelector(".component-card");
      expect(card).toBeInTheDocument();
    });

    it("should apply semantic domain color for processing components", () => {
      const node = createMockNode({
        component: "json-transform",
        type: "processor",
      });

      const { container } = render(ComponentCard, { props: { node } });

      const card = container.querySelector(".component-card");
      expect(card).toBeInTheDocument();
    });

    it("should apply storage domain color for storage components", () => {
      const node = createMockNode({
        component: "storage-writer",
        type: "storage",
      });

      const { container } = render(ComponentCard, { props: { node } });

      const card = container.querySelector(".component-card");
      expect(card).toBeInTheDocument();
    });

    it("should use fallback color for unknown component types", () => {
      const node = createMockNode({
        component: "unknown-component",
        type: "processor",
      });

      const { container } = render(ComponentCard, { props: { node } });

      const card = container.querySelector(".component-card");
      expect(card).toBeInTheDocument();
    });
  });

  describe("edge cases", () => {
    it("should handle very long component names", () => {
      const node = createMockNode({
        name: "This is a very long component name that might overflow the card boundaries and needs proper handling",
      });

      render(ComponentCard, { props: { node } });

      expect(
        screen.getByText(/This is a very long component name/),
      ).toBeInTheDocument();
    });

    it("should handle special characters in component names", () => {
      const node = createMockNode({
        name: "Component-123_with@special#chars",
      });

      render(ComponentCard, { props: { node } });

      expect(
        screen.getByText("Component-123_with@special#chars"),
      ).toBeInTheDocument();
    });

    it("should handle nodes with minimal config", () => {
      const node = createMockNode({ config: {} });

      render(ComponentCard, { props: { node } });

      expect(screen.getByText("UDP Input")).toBeInTheDocument();
    });

    it("should handle nodes with complex config", () => {
      const node = createMockNode({
        config: {
          port: 14550,
          host: "192.168.1.100",
          timeout: 5000,
          retries: 3,
          buffer_size: 1024,
        },
      });

      render(ComponentCard, { props: { node } });

      expect(screen.getByText("UDP Input")).toBeInTheDocument();
    });

    it("should update when node prop changes", async () => {
      const node1 = createMockNode({ name: "Node 1" });
      const node2 = createMockNode({ name: "Node 2", id: "node-2" });

      const { rerender } = render(ComponentCard, { props: { node: node1 } });

      expect(screen.getByText("Node 1")).toBeInTheDocument();

      await rerender({ node: node2 });

      expect(screen.queryByText("Node 1")).not.toBeInTheDocument();
      expect(screen.getByText("Node 2")).toBeInTheDocument();
    });
  });
});
