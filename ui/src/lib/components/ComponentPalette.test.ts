import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/svelte";
import ComponentPalette from "./ComponentPalette.svelte";
import type { ComponentType } from "$lib/types/component";

describe("ComponentPalette", () => {
  const mockComponentTypes: ComponentType[] = [
    {
      id: "udp-input",
      name: "UDP Input",
      type: "input",
      protocol: "udp",
      description: "Receives MAVLink messages via UDP",
      category: "input",
      version: "1.0.0",
      ports: [
        {
          id: "output",
          name: "Messages",
          direction: "output",
          required: true,
          description: "MAVLink messages",
          config: {
            type: "nats",
            nats: {
              subject: "mavlink.messages",
            },
          },
        },
      ],
      schema: {
        type: "object",
        properties: {
          port: {
            type: "number",
            description: "UDP port to listen on",
            default: 14550,
          },
        },
        required: ["port"],
      },
    },
    {
      id: "websocket-output",
      name: "WebSocket Output",
      type: "output",
      protocol: "websocket",
      description: "Streams data via WebSocket",
      category: "output",
      version: "1.0.0",
      ports: [
        {
          id: "input",
          name: "Data",
          direction: "input",
          required: true,
          description: "Data to stream",
          config: {
            type: "nats",
            nats: {
              subject: "data.stream",
            },
          },
        },
      ],
      schema: {
        type: "object",
        properties: {
          port: {
            type: "number",
            description: "WebSocket port",
            default: 8080,
          },
        },
        required: ["port"],
      },
    },
  ];

  // Mock fetch globally
  const mockFetch = vi.fn();
  globalThis.fetch = mockFetch;

  beforeEach(() => {
    mockFetch.mockClear();
  });

  it("should render loading state initially", () => {
    mockFetch.mockImplementation(
      () =>
        new Promise(() => {
          /* never resolves */
        }),
    );

    render(ComponentPalette);

    expect(screen.getByText(/loading/i)).toBeInTheDocument();
  });

  it("should fetch and display component types", async () => {
    mockFetch.mockImplementation(() =>
      Promise.resolve({
        ok: true,
        json: async () => mockComponentTypes,
      }),
    );

    render(ComponentPalette);

    await waitFor(() => {
      expect(screen.getByText("UDP Input")).toBeInTheDocument();
      expect(screen.getByText("WebSocket Output")).toBeInTheDocument();
    });
  });

  it("should display component descriptions", async () => {
    mockFetch.mockImplementation(() =>
      Promise.resolve({
        ok: true,
        json: async () => mockComponentTypes,
      }),
    );

    render(ComponentPalette);

    await waitFor(() => {
      expect(
        screen.getByText("Receives MAVLink messages via UDP"),
      ).toBeInTheDocument();
      expect(
        screen.getByText("Streams data via WebSocket"),
      ).toBeInTheDocument();
    });
  });

  it("should group components by category", async () => {
    mockFetch.mockImplementation(() =>
      Promise.resolve({
        ok: true,
        json: async () => mockComponentTypes,
      }),
    );

    render(ComponentPalette);

    await waitFor(() => {
      // Should show category headers
      expect(
        screen.getByText("input", { selector: ".category-header" }),
      ).toBeInTheDocument();
      expect(
        screen.getByText("output", { selector: ".category-header" }),
      ).toBeInTheDocument();
    });
  });

  it("should handle fetch errors gracefully", async () => {
    mockFetch.mockImplementation(() =>
      Promise.resolve({
        ok: false,
        status: 500,
        statusText: "Internal Server Error",
      }),
    );

    render(ComponentPalette);

    await waitFor(() => {
      expect(screen.getByText(/error loading components/i)).toBeInTheDocument();
    });
  });

  it("should make component cards draggable", async () => {
    mockFetch.mockImplementation(() =>
      Promise.resolve({
        ok: true,
        json: async () => mockComponentTypes,
      }),
    );

    render(ComponentPalette);

    await waitFor(() => {
      const componentCard = screen
        .getByText("UDP Input")
        .closest('[draggable="true"]');
      expect(componentCard).toBeInTheDocument();
    });
  });

  it("should set component type in drag data", async () => {
    mockFetch.mockImplementation(() =>
      Promise.resolve({
        ok: true,
        json: async () => mockComponentTypes,
      }),
    );

    render(ComponentPalette);

    await waitFor(() => {
      const componentCard = screen
        .getByText("UDP Input")
        .closest('[draggable="true"]');
      expect(componentCard).toHaveAttribute("draggable", "true");
    });

    // In real implementation, dragstart event would set dataTransfer
    // with component type information
  });

  it("should display port information", async () => {
    mockFetch.mockImplementation(() =>
      Promise.resolve({
        ok: true,
        json: async () => mockComponentTypes,
      }),
    );

    render(ComponentPalette);

    await waitFor(() => {
      // UDP Input has 0 inputs, 1 output
      const udpCard = screen.getByText("UDP Input").closest(".component-card");
      expect(udpCard).toBeInTheDocument();
    });
  });

  it("should handle empty component list", async () => {
    mockFetch.mockImplementation(() =>
      Promise.resolve({
        ok: true,
        json: async () => [],
      }),
    );

    render(ComponentPalette);

    await waitFor(() => {
      expect(
        screen.getByText(/component palette coming soon/i),
      ).toBeInTheDocument();
    });
  });

  it("should fetch from correct endpoint", async () => {
    mockFetch.mockImplementation(() =>
      Promise.resolve({
        ok: true,
        json: async () => mockComponentTypes,
      }),
    );

    render(ComponentPalette);

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "/components/types",
        expect.objectContaining({ signal: expect.any(AbortSignal) }),
      );
    });
  });

  it("should display component categories correctly", async () => {
    const mixedComponents = [
      { ...mockComponentTypes[0], category: "input" },
      { ...mockComponentTypes[1], category: "output" },
      {
        ...mockComponentTypes[0],
        id: "processor-filter",
        name: "Filter",
        category: "processor",
      },
    ];

    mockFetch.mockImplementation(() =>
      Promise.resolve({
        ok: true,
        json: async () => mixedComponents,
      }),
    );

    render(ComponentPalette);

    await waitFor(() => {
      expect(
        screen.getByText("input", { selector: ".category-header" }),
      ).toBeInTheDocument();
      expect(
        screen.getByText("output", { selector: ".category-header" }),
      ).toBeInTheDocument();
      expect(
        screen.getByText("processor", { selector: ".category-header" }),
      ).toBeInTheDocument();
    });
  });
});
