import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/svelte";
import FlowList from "./FlowList.svelte";
import type { Flow } from "$lib/types/flow";

describe("FlowList", () => {
  const createMockFlow = (
    id: string,
    name: string,
    runtimeState: Flow["runtime_state"] = "not_deployed",
  ): Flow => ({
    version: 1,
    id,
    name,
    description: `Description for ${name}`,
    nodes: [],
    connections: [],
    runtime_state: runtimeState,
    created_at: "2025-10-10T12:00:00Z",
    updated_at: "2025-10-10T12:00:00Z",
    created_by: "user-123",
    last_modified: "2025-10-10T12:00:00Z",
  });

  it("should render empty state when no flows", () => {
    render(FlowList, { props: { flows: [] } });

    expect(screen.getByText(/no flows/i)).toBeInTheDocument();
  });

  it("should render flow list from props", () => {
    const flows = [
      createMockFlow("flow-1", "Test Flow 1"),
      createMockFlow("flow-2", "Test Flow 2"),
      createMockFlow("flow-3", "Test Flow 3"),
    ];

    render(FlowList, { props: { flows } });

    expect(screen.getByText("Test Flow 1")).toBeInTheDocument();
    expect(screen.getByText("Test Flow 2")).toBeInTheDocument();
    expect(screen.getByText("Test Flow 3")).toBeInTheDocument();
  });

  it("should display flow descriptions", () => {
    const flows = [createMockFlow("flow-1", "Test Flow")];

    render(FlowList, { props: { flows } });

    expect(screen.getByText("Description for Test Flow")).toBeInTheDocument();
  });

  it("should display runtime state badges", () => {
    const flows = [
      createMockFlow("flow-1", "Not Deployed Flow", "not_deployed"),
      createMockFlow("flow-2", "Running Flow", "running"),
      createMockFlow("flow-3", "Stopped Flow", "deployed_stopped"),
      createMockFlow("flow-4", "Error Flow", "error"),
    ];

    render(FlowList, { props: { flows } });

    expect(screen.getByText("not_deployed")).toBeInTheDocument();
    expect(screen.getByText("running")).toBeInTheDocument();
    expect(screen.getByText("deployed_stopped")).toBeInTheDocument();
    expect(screen.getByText("error")).toBeInTheDocument();
  });

  it("should call onFlowClick when flow is clicked", async () => {
    const flows = [createMockFlow("flow-123", "Clickable Flow")];
    const onFlowClick = vi.fn();

    render(FlowList, { props: { flows, onFlowClick } });

    const flowCard = screen.getByText("Clickable Flow").closest("button");
    expect(flowCard).toBeInTheDocument();

    if (flowCard) {
      await fireEvent.click(flowCard);
      expect(onFlowClick).toHaveBeenCalledWith("flow-123");
    }
  });

  it('should call onCreate when "Create New Flow" button is clicked', async () => {
    const onCreate = vi.fn();

    render(FlowList, { props: { flows: [], onCreate } });

    const createButton = screen.getByRole("button", {
      name: /create new flow/i,
    });
    await fireEvent.click(createButton);

    expect(onCreate).toHaveBeenCalled();
  });

  it("should show Create button even when flows exist", () => {
    const flows = [createMockFlow("flow-1", "Test Flow")];

    render(FlowList, { props: { flows } });

    expect(
      screen.getByRole("button", { name: /create new flow/i }),
    ).toBeInTheDocument();
  });

  it("should display multiple flows with correct data", () => {
    const flows = [
      createMockFlow("flow-1", "Alpha Flow", "running"),
      createMockFlow("flow-2", "Beta Flow", "deployed_stopped"),
      createMockFlow("flow-3", "Gamma Flow", "not_deployed"),
    ];

    render(FlowList, { props: { flows } });

    // Check all flows are rendered
    expect(screen.getByText("Alpha Flow")).toBeInTheDocument();
    expect(screen.getByText("Beta Flow")).toBeInTheDocument();
    expect(screen.getByText("Gamma Flow")).toBeInTheDocument();

    // Check runtime states
    expect(screen.getByText("running")).toBeInTheDocument();
    expect(screen.getByText("deployed_stopped")).toBeInTheDocument();
    expect(screen.getByText("not_deployed")).toBeInTheDocument();
  });

  it("should handle flows without descriptions", () => {
    const flowWithoutDesc: Flow = {
      ...createMockFlow("flow-1", "No Description Flow"),
      description: "",
    };

    render(FlowList, { props: { flows: [flowWithoutDesc] } });

    expect(screen.getByText("No Description Flow")).toBeInTheDocument();
    expect(screen.queryByText("Description for")).not.toBeInTheDocument();
  });
});
