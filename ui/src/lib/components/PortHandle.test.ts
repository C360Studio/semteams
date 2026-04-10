import { render } from "@testing-library/svelte";
import { describe, it, expect } from "vitest";
import PortHandle from "./PortHandle.svelte";
import type { ValidatedPort } from "../types/port";

describe("PortHandle", () => {
  const mockPort: ValidatedPort = {
    name: "nats_output",
    direction: "output",
    required: true,
    pattern: "stream", // Backend pattern type (not connection_id)
    type: "nats_stream",
    connection_id: "input.udp.mavlink",
    description: "Publishes UAV telemetry",
  };

  it("should render port handle with correct color", () => {
    const { container } = render(PortHandle, { props: { port: mockPort } });

    const handle = container.querySelector("[data-port-handle]");
    expect(handle).toBeTruthy();
    // Color is set via CSS variables (var(--port-pattern-stream))
    // CSS variables don't resolve in test environment, so we verify classes instead
    expect(handle?.classList.contains("port-nats_stream")).toBe(true);
  });

  it("should render solid border for required port", () => {
    const { container } = render(PortHandle, { props: { port: mockPort } });

    const handle = container.querySelector("[data-port-handle]");
    expect(handle).toHaveAttribute("data-border-pattern", "solid");
  });

  it("should render dashed border for optional port", () => {
    const optionalPort = { ...mockPort, required: false };
    const { container } = render(PortHandle, { props: { port: optionalPort } });

    const handle = container.querySelector("[data-port-handle]");
    expect(handle).toHaveAttribute("data-border-pattern", "dashed");
  });

  it("should render icon for port type", async () => {
    const { container } = render(PortHandle, { props: { port: mockPort } });

    const icon = container.querySelector("svg[data-icon]");
    expect(icon).toBeTruthy();
    expect(icon).toHaveAttribute("data-icon", "arrow-path-rounded-square");
  });

  it("should have ARIA label for accessibility", () => {
    const { container } = render(PortHandle, { props: { port: mockPort } });

    const handle = container.querySelector("[data-port-handle]");
    expect(handle).toHaveAccessibleName(/nats stream.*output.*required/i);
  });

  it("should render purple color for NATS request port", () => {
    const requestPort: ValidatedPort = {
      ...mockPort,
      pattern: "request", // Backend pattern determines styling
      type: "message.Request",
    };
    const { container } = render(PortHandle, { props: { port: requestPort } });

    const handle = container.querySelector("[data-port-handle]");
    // Color is set via CSS variables (var(--port-pattern-request))
    expect(handle?.classList.contains("port-nats_request")).toBe(true);
  });

  it("should render green color for KV watch port", () => {
    const kvPort: ValidatedPort = {
      ...mockPort,
      pattern: "watch", // Backend pattern determines styling
      type: "kv.Entry",
    };
    const { container } = render(PortHandle, { props: { port: kvPort } });

    const handle = container.querySelector("[data-port-handle]");
    // Color is set via CSS variables (var(--port-pattern-watch))
    expect(handle?.classList.contains("port-kv_watch")).toBe(true);
  });

  it("should render orange color for network port", () => {
    const networkPort: ValidatedPort = {
      ...mockPort,
      pattern: "api", // Backend pattern determines styling
      type: "bytes",
    };
    const { container } = render(PortHandle, { props: { port: networkPort } });

    const handle = container.querySelector("[data-port-handle]");
    // Color is set via CSS variables (var(--port-pattern-api))
    expect(handle?.classList.contains("port-network")).toBe(true);
  });

  it("should render gray color for file port", () => {
    const filePort: ValidatedPort = {
      ...mockPort,
      pattern: "unknown", // Unknown pattern with file:// connection_id
      type: "bytes",
      connection_id: "file:///data/input.csv",
    };
    const { container } = render(PortHandle, { props: { port: filePort } });

    const handle = container.querySelector("[data-port-handle]");
    // Color is set via CSS variables (var(--port-pattern-file))
    expect(handle?.classList.contains("port-file")).toBe(true);
  });

  it("should apply correct CSS classes", () => {
    const { container } = render(PortHandle, { props: { port: mockPort } });

    const handle = container.querySelector("[data-port-handle]");
    expect(handle?.classList.contains("port-handle")).toBe(true);
    expect(handle?.classList.contains("port-nats_stream")).toBe(true);
    expect(handle?.classList.contains("port-solid")).toBe(true);
  });
});
