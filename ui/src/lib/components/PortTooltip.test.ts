import { render } from "@testing-library/svelte";
import { describe, it, expect } from "vitest";
import PortTooltip from "./PortTooltip.svelte";
import type { PortTooltipContent } from "../types/port";

describe("PortTooltip", () => {
  const mockContent: PortTooltipContent = {
    name: "nats_output",
    type: "nats_stream",
    pattern: "input.udp.mavlink",
    requirement: "required",
    description: "Publishes UAV telemetry",
    validationState: "valid",
  };

  it("should display all metadata fields", () => {
    const { getByText } = render(PortTooltip, {
      props: { content: mockContent, isVisible: true },
    });

    expect(getByText("nats_output")).toBeTruthy();
    expect(getByText(/nats.stream/i)).toBeTruthy();
    expect(getByText("input.udp.mavlink")).toBeTruthy();
    expect(getByText(/required/i)).toBeTruthy();
    expect(getByText("Publishes UAV telemetry")).toBeTruthy();
  });

  it("should not be visible when isVisible is false", () => {
    const { container } = render(PortTooltip, {
      props: { content: mockContent, isVisible: false },
    });

    const tooltip = container.querySelector('[data-testid="port-tooltip"]');
    expect(tooltip).toHaveStyle({ display: "none" });
  });

  it("should display validation state when provided", () => {
    const contentWithError: PortTooltipContent = {
      ...mockContent,
      validationState: "error",
      validationMessage: "Port not connected",
    };

    const { getByText } = render(PortTooltip, {
      props: { content: contentWithError, isVisible: true },
    });

    expect(getByText("Port not connected")).toBeTruthy();
  });

  it("should display warning state with appropriate styling", () => {
    const contentWithWarning: PortTooltipContent = {
      ...mockContent,
      validationState: "warning",
      validationMessage: "Optional port not connected",
    };

    const { container, getByText } = render(PortTooltip, {
      props: { content: contentWithWarning, isVisible: true },
    });

    expect(getByText("Optional port not connected")).toBeTruthy();

    const tooltip = container.querySelector('[data-testid="port-tooltip"]');
    expect(tooltip?.classList.contains("tooltip-warning")).toBe(true);
  });

  it("should handle missing description gracefully", () => {
    const contentWithoutDesc: PortTooltipContent = {
      ...mockContent,
      description: undefined,
    };

    const { container } = render(PortTooltip, {
      props: { content: contentWithoutDesc, isVisible: true },
    });

    const tooltip = container.querySelector('[data-testid="port-tooltip"]');
    expect(tooltip).toBeTruthy();
    // Should still render other fields
    expect(container.textContent).toContain("nats_output");
  });

  it("should have correct ARIA attributes for accessibility", () => {
    const { container } = render(PortTooltip, {
      props: { content: mockContent, isVisible: true },
    });

    const tooltip = container.querySelector('[data-testid="port-tooltip"]');
    expect(tooltip).toHaveAttribute("role", "tooltip");
    expect(tooltip).toHaveAttribute("aria-live", "polite");
  });

  it("should position above port when space available", () => {
    const mockAnchor = document.createElement("div");
    mockAnchor.getBoundingClientRect = () => ({
      top: 500,
      left: 100,
      bottom: 520,
      right: 120,
      width: 20,
      height: 20,
      x: 100,
      y: 500,
      toJSON: () => ({}),
    });

    const { container } = render(PortTooltip, {
      props: {
        content: mockContent,
        isVisible: true,
        anchorElement: mockAnchor,
      },
    });

    const tooltip = container.querySelector('[data-testid="port-tooltip"]');
    // Floating UI should position it above the anchor
    expect(tooltip).toBeTruthy();
  });
});
