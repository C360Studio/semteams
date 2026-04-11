import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/svelte";
import ComponentList from "./ComponentList.svelte";
import type { FlowNode } from "$lib/types/flow";

describe("ComponentList", () => {
  const createMockNode = (
    id: string,
    name: string,
    component: string,
    type: string = "processor",
  ): FlowNode => ({
    id,
    component,
    type,
    name,
    position: { x: 100, y: 100 },
    config: {},
  });

  const mockNodes: FlowNode[] = [
    createMockNode("node-1", "UDP Input 1", "udp-input", "input"),
    createMockNode("node-2", "JSON Transform", "json-transform", "processor"),
    createMockNode("node-3", "WebSocket Output", "websocket-output", "output"),
    createMockNode("node-4", "UDP Input 2", "udp-input", "input"),
  ];

  describe("rendering", () => {
    it("should display empty state when no nodes", () => {
      render(ComponentList, { props: { nodes: [] } });

      expect(screen.getByText(/no components/i)).toBeInTheDocument();
    });

    it("should display empty state message", () => {
      render(ComponentList, { props: { nodes: [] } });

      expect(
        screen.getByText(/add a component to get started/i),
      ).toBeInTheDocument();
    });

    it("should render single node", () => {
      const nodes = [
        createMockNode("node-1", "UDP Input", "udp-input", "input"),
      ];

      render(ComponentList, { props: { nodes } });

      expect(screen.getByText("UDP Input")).toBeInTheDocument();
      expect(screen.getByText("Type: udp-input")).toBeInTheDocument();
    });

    it("should render multiple nodes", () => {
      render(ComponentList, { props: { nodes: mockNodes } });

      expect(screen.getByText("UDP Input 1")).toBeInTheDocument();
      expect(screen.getByText("JSON Transform")).toBeInTheDocument();
      expect(screen.getByText("WebSocket Output")).toBeInTheDocument();
      expect(screen.getByText("UDP Input 2")).toBeInTheDocument();
    });

    it("should display component count", () => {
      render(ComponentList, { props: { nodes: mockNodes } });

      // Header should show count
      expect(
        screen.getByRole("heading", { name: /components \(4\)/i }),
      ).toBeInTheDocument();
    });

    it("should display header with Add button", () => {
      render(ComponentList, { props: { nodes: mockNodes } });

      expect(screen.getByRole("button", { name: /add/i })).toBeInTheDocument();
    });

    it("should render search input", () => {
      render(ComponentList, { props: { nodes: mockNodes } });

      const searchInput = screen.getByPlaceholderText(/search/i);
      expect(searchInput).toBeInTheDocument();
    });

    it("should render all node types correctly", () => {
      const diverseNodes = [
        createMockNode("n1", "UDP In", "udp-input", "input"),
        createMockNode("n2", "WS Out", "websocket-output", "output"),
        createMockNode("n3", "Transform", "json-transform", "processor"),
        createMockNode("n4", "Storage", "storage-writer", "storage"),
      ];

      render(ComponentList, { props: { nodes: diverseNodes } });

      expect(screen.getByText("Type: udp-input")).toBeInTheDocument();
      expect(screen.getByText("Type: websocket-output")).toBeInTheDocument();
      expect(screen.getByText("Type: json-transform")).toBeInTheDocument();
      expect(screen.getByText("Type: storage-writer")).toBeInTheDocument();
    });
  });

  describe("search and filter", () => {
    it("should filter nodes by name", async () => {
      render(ComponentList, { props: { nodes: mockNodes } });

      const searchInput = screen.getByPlaceholderText(/search/i);
      await fireEvent.input(searchInput, { target: { value: "UDP" } });

      // Should show UDP nodes
      expect(screen.getByText("UDP Input 1")).toBeInTheDocument();
      expect(screen.getByText("UDP Input 2")).toBeInTheDocument();

      // Should not show other nodes
      expect(screen.queryByText("JSON Transform")).not.toBeInTheDocument();
      expect(screen.queryByText("WebSocket Output")).not.toBeInTheDocument();
    });

    it("should filter nodes by type", async () => {
      render(ComponentList, { props: { nodes: mockNodes } });

      const searchInput = screen.getByPlaceholderText(/search/i);
      await fireEvent.input(searchInput, { target: { value: "json" } });

      // Should show JSON transform
      expect(screen.getByText("JSON Transform")).toBeInTheDocument();

      // Should not show other nodes
      expect(screen.queryByText("UDP Input 1")).not.toBeInTheDocument();
      expect(screen.queryByText("WebSocket Output")).not.toBeInTheDocument();
    });

    it("should be case insensitive when filtering", async () => {
      render(ComponentList, { props: { nodes: mockNodes } });

      const searchInput = screen.getByPlaceholderText(/search/i);
      await fireEvent.input(searchInput, { target: { value: "udp" } });

      expect(screen.getByText("UDP Input 1")).toBeInTheDocument();
      expect(screen.getByText("UDP Input 2")).toBeInTheDocument();
    });

    it("should show all nodes when search is cleared", async () => {
      render(ComponentList, { props: { nodes: mockNodes } });

      const searchInput = screen.getByPlaceholderText(/search/i);

      // Filter
      await fireEvent.input(searchInput, { target: { value: "UDP" } });
      expect(screen.queryByText("JSON Transform")).not.toBeInTheDocument();

      // Clear filter
      await fireEvent.input(searchInput, { target: { value: "" } });

      // All nodes should be visible again
      expect(screen.getByText("UDP Input 1")).toBeInTheDocument();
      expect(screen.getByText("JSON Transform")).toBeInTheDocument();
      expect(screen.getByText("WebSocket Output")).toBeInTheDocument();
    });

    it("should show no results message when filter matches nothing", async () => {
      render(ComponentList, { props: { nodes: mockNodes } });

      const searchInput = screen.getByPlaceholderText(/search/i);
      await fireEvent.input(searchInput, { target: { value: "nonexistent" } });

      expect(screen.getByText(/no components found/i)).toBeInTheDocument();
    });

    it("should filter by partial matches", async () => {
      render(ComponentList, { props: { nodes: mockNodes } });

      const searchInput = screen.getByPlaceholderText(/search/i);
      await fireEvent.input(searchInput, { target: { value: "Input" } });

      expect(screen.getByText("UDP Input 1")).toBeInTheDocument();
      expect(screen.getByText("UDP Input 2")).toBeInTheDocument();
    });

    it("should update filter results in real-time", async () => {
      render(ComponentList, { props: { nodes: mockNodes } });

      const searchInput = screen.getByPlaceholderText(/search/i);

      // Type 'U'
      await fireEvent.input(searchInput, { target: { value: "U" } });
      expect(screen.getByText("UDP Input 1")).toBeInTheDocument();

      // Type 'UD'
      await fireEvent.input(searchInput, { target: { value: "UD" } });
      expect(screen.getByText("UDP Input 1")).toBeInTheDocument();

      // Type 'UDP'
      await fireEvent.input(searchInput, { target: { value: "UDP" } });
      expect(screen.getByText("UDP Input 1")).toBeInTheDocument();
    });
  });

  describe("selection", () => {
    it("should highlight selected node", () => {
      render(ComponentList, {
        props: { nodes: mockNodes, selectedNodeId: "node-1" },
      });

      // The selected node card should have selected styling
      const cards = screen.getAllByRole("button");
      const selectedCard = cards.find((card) =>
        card.textContent?.includes("UDP Input 1"),
      );

      expect(selectedCard).toBeInTheDocument();
    });

    it("should call onSelectNode when node card is clicked", async () => {
      const onSelectNode = vi.fn();

      render(ComponentList, {
        props: { nodes: mockNodes, onSelectNode },
      });

      const nodeCard = screen.getByText("UDP Input 1").closest("button");

      if (nodeCard) {
        await fireEvent.click(nodeCard);
        expect(onSelectNode).toHaveBeenCalledWith("node-1");
      }
    });

    it("should support keyboard selection with Enter", async () => {
      const onSelectNode = vi.fn();

      render(ComponentList, {
        props: { nodes: mockNodes, onSelectNode },
      });

      const nodeCard = screen.getByText("UDP Input 1").closest("button");

      if (nodeCard) {
        await fireEvent.keyDown(nodeCard, { key: "Enter", code: "Enter" });
        expect(onSelectNode).toHaveBeenCalledWith("node-1");
      }
    });

    it("should support keyboard selection with Space", async () => {
      const onSelectNode = vi.fn();

      render(ComponentList, {
        props: { nodes: mockNodes, onSelectNode },
      });

      const nodeCard = screen.getByText("UDP Input 1").closest("button");

      if (nodeCard) {
        await fireEvent.keyDown(nodeCard, { key: " ", code: "Space" });
        expect(onSelectNode).toHaveBeenCalledWith("node-1");
      }
    });

    it("should update selection when selectedNodeId changes", async () => {
      const { rerender } = render(ComponentList, {
        props: { nodes: mockNodes, selectedNodeId: "node-1" },
      });

      // Update selection
      await rerender({ nodes: mockNodes, selectedNodeId: "node-2" });

      // Component should re-render with new selection
      expect(screen.getByText("JSON Transform")).toBeInTheDocument();
    });

    it("should handle no selection", () => {
      render(ComponentList, {
        props: { nodes: mockNodes, selectedNodeId: null },
      });

      // Should render without errors
      expect(screen.getByText("UDP Input 1")).toBeInTheDocument();
    });

    it("should handle undefined selectedNodeId", () => {
      render(ComponentList, {
        props: { nodes: mockNodes },
      });

      // Should render without errors
      expect(screen.getByText("UDP Input 1")).toBeInTheDocument();
    });
  });

  describe("actions", () => {
    it("should call onAddComponent when Add button is clicked", async () => {
      const onAddComponent = vi.fn();

      render(ComponentList, {
        props: { nodes: mockNodes, onAddComponent },
      });

      const addButton = screen.getByRole("button", { name: /add/i });
      await fireEvent.click(addButton);

      expect(onAddComponent).toHaveBeenCalledTimes(1);
    });

    it("should show Add button in empty state", () => {
      render(ComponentList, { props: { nodes: [] } });

      expect(screen.getByRole("button", { name: /add/i })).toBeInTheDocument();
    });

    it("should show Add button when nodes exist", () => {
      render(ComponentList, { props: { nodes: mockNodes } });

      expect(screen.getByRole("button", { name: /add/i })).toBeInTheDocument();
    });

    it("should not error if callbacks are not provided", async () => {
      render(ComponentList, { props: { nodes: mockNodes } });

      const addButton = screen.getByRole("button", { name: /add/i });

      // Should not throw error
      await expect(fireEvent.click(addButton)).resolves.not.toThrow();
    });
  });

  describe("accessibility", () => {
    it("should have proper ARIA labels for search", () => {
      render(ComponentList, { props: { nodes: mockNodes } });

      const searchInput = screen.getByLabelText(/search components/i);
      expect(searchInput).toBeInTheDocument();
    });

    it("should have proper ARIA label for Add button", () => {
      render(ComponentList, { props: { nodes: mockNodes } });

      const addButton = screen.getByRole("button", { name: /add component/i });
      expect(addButton).toBeInTheDocument();
    });

    it("should have semantic list structure", () => {
      render(ComponentList, { props: { nodes: mockNodes } });

      // Component list should be in a list container
      const list = screen.getByRole("list");
      expect(list).toBeInTheDocument();
    });

    it("should have list items for each component", () => {
      render(ComponentList, { props: { nodes: mockNodes } });

      const listItems = screen.getAllByRole("listitem");
      expect(listItems).toHaveLength(mockNodes.length);
    });

    it("should support keyboard navigation through components", async () => {
      render(ComponentList, { props: { nodes: mockNodes } });

      const cards = screen.getAllByRole("button");
      const componentCards = cards.filter(
        (card) =>
          card.getAttribute("aria-label") &&
          !card.textContent?.toLowerCase().includes("add"),
      );

      // All component cards should be keyboard accessible with tabindex
      componentCards.forEach((card) => {
        expect(card).toHaveAttribute("tabindex", "0");
      });
    });

    it("should have proper focus management", async () => {
      render(ComponentList, { props: { nodes: mockNodes } });

      const searchInput = screen.getByPlaceholderText(/search/i);

      // Search should be focusable
      searchInput.focus();
      expect(document.activeElement).toBe(searchInput);
    });

    it("should announce filtered results count to screen readers", async () => {
      render(ComponentList, { props: { nodes: mockNodes } });

      const searchInput = screen.getByPlaceholderText(/search/i);
      await fireEvent.input(searchInput, { target: { value: "UDP" } });

      // Should have aria-live region for result count
      expect(
        screen.getByRole("heading", { name: /components \(2\)/i }),
      ).toBeInTheDocument();
    });

    it("should indicate selected state to screen readers", () => {
      render(ComponentList, {
        props: { nodes: mockNodes, selectedNodeId: "node-1" },
      });

      const selectedCard = screen.getByRole("button", { name: "UDP Input 1" });
      expect(selectedCard).toHaveAttribute("aria-pressed", "true");
    });
  });

  describe("edge cases", () => {
    it("should handle empty node array", () => {
      render(ComponentList, { props: { nodes: [] } });

      expect(screen.getByText(/no components/i)).toBeInTheDocument();
    });

    it("should handle single node", () => {
      const nodes = [
        createMockNode("node-1", "Single Node", "udp-input", "input"),
      ];

      render(ComponentList, { props: { nodes } });

      expect(screen.getByText("Single Node")).toBeInTheDocument();
    });

    it("should handle many nodes", () => {
      const manyNodes = Array.from({ length: 50 }, (_, i) =>
        createMockNode(`node-${i}`, `Component ${i}`, "udp-input", "input"),
      );

      render(ComponentList, { props: { nodes: manyNodes } });

      // Should render without performance issues
      expect(screen.getByText("Component 0")).toBeInTheDocument();
      expect(screen.getByText("Component 49")).toBeInTheDocument();
    });

    it("should handle nodes with duplicate names", () => {
      const duplicateNodes = [
        createMockNode("node-1", "UDP Input", "udp-input", "input"),
        createMockNode("node-2", "UDP Input", "udp-input", "input"),
        createMockNode("node-3", "UDP Input", "udp-input", "input"),
      ];

      render(ComponentList, { props: { nodes: duplicateNodes } });

      const cards = screen.getAllByText("UDP Input");
      expect(cards).toHaveLength(3);
    });

    it("should update when nodes prop changes", async () => {
      const { rerender } = render(ComponentList, {
        props: { nodes: [mockNodes[0]] },
      });

      expect(screen.getByText("UDP Input 1")).toBeInTheDocument();
      expect(screen.queryByText("JSON Transform")).not.toBeInTheDocument();

      await rerender({ nodes: mockNodes });

      expect(screen.getByText("UDP Input 1")).toBeInTheDocument();
      expect(screen.getByText("JSON Transform")).toBeInTheDocument();
    });

    it("should handle special characters in search", async () => {
      const specialNodes = [
        createMockNode("n1", "Component-123", "udp-input", "input"),
        createMockNode("n2", "Component@456", "websocket-output", "output"),
        createMockNode("n3", "Component#789", "json-transform", "processor"),
      ];

      render(ComponentList, { props: { nodes: specialNodes } });

      const searchInput = screen.getByPlaceholderText(/search/i);
      await fireEvent.input(searchInput, { target: { value: "@456" } });

      expect(screen.getByText("Component@456")).toBeInTheDocument();
      expect(screen.queryByText("Component-123")).not.toBeInTheDocument();
    });

    it("should handle whitespace in search", async () => {
      render(ComponentList, { props: { nodes: mockNodes } });

      const searchInput = screen.getByPlaceholderText(/search/i);
      await fireEvent.input(searchInput, { target: { value: "  UDP  " } });

      // Should trim whitespace and filter correctly
      expect(screen.getByText("UDP Input 1")).toBeInTheDocument();
    });

    it("should maintain search state when nodes update", async () => {
      const { rerender } = render(ComponentList, {
        props: { nodes: [mockNodes[0]] },
      });

      const searchInput = screen.getByPlaceholderText(/search/i);
      await fireEvent.input(searchInput, { target: { value: "JSON" } });

      // No results yet
      expect(screen.getByText(/no components found/i)).toBeInTheDocument();

      // Add more nodes
      await rerender({ nodes: mockNodes });

      // Filter should still be active
      expect(screen.getByText("JSON Transform")).toBeInTheDocument();
      expect(screen.queryByText("UDP Input 1")).not.toBeInTheDocument();
    });

    it("should handle rapid filter changes", async () => {
      render(ComponentList, { props: { nodes: mockNodes } });

      const searchInput = screen.getByPlaceholderText(/search/i);

      // Rapid typing
      await fireEvent.input(searchInput, { target: { value: "U" } });
      await fireEvent.input(searchInput, { target: { value: "UD" } });
      await fireEvent.input(searchInput, { target: { value: "UDP" } });
      await fireEvent.input(searchInput, { target: { value: "UDP " } });
      await fireEvent.input(searchInput, { target: { value: "UDP I" } });

      // Should show correct filtered results
      expect(screen.getByText("UDP Input 1")).toBeInTheDocument();
      expect(screen.getByText("UDP Input 2")).toBeInTheDocument();
    });

    it("should handle selecting non-existent node", () => {
      render(ComponentList, {
        props: { nodes: mockNodes, selectedNodeId: "non-existent-id" },
      });

      // Should render without errors
      expect(screen.getByText("UDP Input 1")).toBeInTheDocument();
    });
  });

  describe("layout and styling", () => {
    it("should apply proper container classes", () => {
      const { container } = render(ComponentList, {
        props: { nodes: mockNodes },
      });

      const componentList = container.querySelector(".component-list");
      expect(componentList).toBeInTheDocument();
    });

    it("should render header section", () => {
      render(ComponentList, { props: { nodes: mockNodes } });

      // Header with title and add button
      expect(
        screen.getByRole("heading", { name: /components \(4\)/i }),
      ).toBeInTheDocument();
      expect(screen.getByRole("button", { name: /add/i })).toBeInTheDocument();
    });

    it("should render search section", () => {
      render(ComponentList, { props: { nodes: mockNodes } });

      const searchInput = screen.getByPlaceholderText(/search/i);
      expect(searchInput).toBeInTheDocument();
    });

    it("should render component cards in list", () => {
      render(ComponentList, { props: { nodes: mockNodes } });

      const list = screen.getByRole("list");
      expect(list).toBeInTheDocument();
    });

    it("should apply scrollable container for many items", () => {
      const manyNodes = Array.from({ length: 20 }, (_, i) =>
        createMockNode(`node-${i}`, `Component ${i}`, "udp-input", "input"),
      );

      const { container } = render(ComponentList, {
        props: { nodes: manyNodes },
      });

      const listContainer = container.querySelector(".component-list-items");
      expect(listContainer).toBeInTheDocument();
    });
  });
});
