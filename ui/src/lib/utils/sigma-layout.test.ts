import { describe, it, expect, vi, beforeEach } from "vitest";
import { LayoutController } from "./sigma-layout";

// Mock the FA2 worker — no Web Workers in vitest
vi.mock("graphology-layout-forceatlas2/worker", () => {
  return {
    default: vi.fn().mockImplementation(() => ({
      start: vi.fn(),
      stop: vi.fn(),
      kill: vi.fn(),
    })),
  };
});

// graphology is real (no WebGL needed for graph data structure)
import Graph from "graphology";

describe("LayoutController", () => {
  let controller: LayoutController;

  beforeEach(() => {
    controller = new LayoutController();
    vi.clearAllMocks();
  });

  it("should not be running initially", () => {
    expect(controller.isRunning).toBe(false);
  });

  it("should start layout on non-empty graph", () => {
    const graph = new Graph();
    graph.addNode("a", { x: 0, y: 0 });

    controller.start(graph);

    expect(controller.isRunning).toBe(true);
  });

  it("should not start layout on empty graph", () => {
    const graph = new Graph();

    controller.start(graph);

    expect(controller.isRunning).toBe(false);
  });

  it("should stop layout", () => {
    const graph = new Graph();
    graph.addNode("a", { x: 0, y: 0 });

    controller.start(graph);
    expect(controller.isRunning).toBe(true);

    controller.stop();
    expect(controller.isRunning).toBe(false);
  });

  it("should handle multiple start/stop cycles", () => {
    const graph = new Graph();
    graph.addNode("a", { x: 0, y: 0 });

    controller.start(graph);
    controller.stop();
    controller.start(graph);
    controller.stop();

    expect(controller.isRunning).toBe(false);
  });

  it("should stop previous layout when starting a new one", () => {
    const graph = new Graph();
    graph.addNode("a", { x: 0, y: 0 });

    controller.start(graph);
    controller.start(graph);

    expect(controller.isRunning).toBe(true);
    controller.stop();
    expect(controller.isRunning).toBe(false);
  });
});
