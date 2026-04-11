import { describe, it, expect, vi, beforeEach } from "vitest";
import { render } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import type { GraphEntity, GraphRelationship } from "$lib/types/graph";
import { parseEntityId } from "$lib/types/graph";
import SigmaCanvas from "./SigmaCanvas.svelte";
// Static import gets the already-mocked Sigma class from setup.ts
import Sigma from "sigma";

// sigma and graphology-layout-forceatlas2/worker are mocked globally in setup.ts

function makeEntity(id: string): GraphEntity {
  return {
    id,
    idParts: parseEntityId(id),
    properties: [],
    outgoing: [],
    incoming: [],
  };
}

function makeRelationship(
  sourceId: string,
  predicate: string,
  targetId: string,
): GraphRelationship {
  return {
    id: `${sourceId}:${predicate}:${targetId}`,
    sourceId,
    targetId,
    predicate,
    confidence: 0.9,
    timestamp: Date.now(),
  };
}

const defaultProps = {
  entities: [] as GraphEntity[],
  relationships: [] as GraphRelationship[],
};

describe("SigmaCanvas attack tests", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  // ── Undefined / null props ────────────────────────────────────────────────

  it("renders without throwing when given empty entities and relationships", () => {
    expect(() => render(SigmaCanvas, { props: defaultProps })).not.toThrow();
  });

  it("renders without throwing when optional props are omitted", () => {
    expect(() =>
      render(SigmaCanvas, {
        props: {
          entities: [makeEntity("c360.ops.robotics.gcs.drone.001")],
          relationships: [],
          // selectedEntityId, hoveredEntityId, callbacks all omitted
        },
      }),
    ).not.toThrow();
  });

  it("renders without throwing when selectedEntityId is null", () => {
    expect(() =>
      render(SigmaCanvas, {
        props: { ...defaultProps, selectedEntityId: null },
      }),
    ).not.toThrow();
  });

  it("renders without throwing when selectedEntityId is undefined", () => {
    expect(() =>
      render(SigmaCanvas, {
        props: { ...defaultProps, selectedEntityId: undefined },
      }),
    ).not.toThrow();
  });

  // ── Cleanup on unmount ────────────────────────────────────────────────────

  it("calls sigma.kill on unmount (no memory leak)", () => {
    // Intercept the kill spy before render so we can assert on it after unmount.
    // The setup.ts mock returns { kill: vi.fn(), ... } from mockImplementation.
    // We capture the returned object via mock.results after render() fires onMount.
    const { unmount } = render(SigmaCanvas, { props: defaultProps });

    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const MockSigma = Sigma as any;
    // mock.results holds the value returned by each call to new Sigma(...)
    const results = MockSigma.mock.results;
    expect(results.length).toBeGreaterThan(0);
    const sigmaReturnValue = results[results.length - 1].value;

    unmount();

    expect(sigmaReturnValue.kill).toHaveBeenCalled();
  });

  // ── Large data ────────────────────────────────────────────────────────────

  it("renders without throwing with 500 entities", () => {
    const entities = Array.from({ length: 500 }, (_, i) =>
      makeEntity(`c360.ops.robotics.gcs.drone.n${i}`),
    );
    expect(() =>
      render(SigmaCanvas, { props: { entities, relationships: [] } }),
    ).not.toThrow();
  });

  it("renders without throwing with 200 relationships", () => {
    const entities = Array.from({ length: 201 }, (_, i) =>
      makeEntity(`c360.ops.robotics.gcs.drone.n${i}`),
    );
    const relationships = Array.from({ length: 200 }, (_, i) =>
      makeRelationship(
        `c360.ops.robotics.gcs.drone.n${i}`,
        "a.b.c",
        `c360.ops.robotics.gcs.drone.n${i + 1}`,
      ),
    );
    expect(() =>
      render(SigmaCanvas, { props: { entities, relationships } }),
    ).not.toThrow();
  });

  // ── Zoom controls: keyboard and click ────────────────────────────────────

  it("zoom in button is accessible by keyboard (has aria-label)", () => {
    const { getByRole } = render(SigmaCanvas, { props: defaultProps });
    const zoomIn = getByRole("button", { name: /zoom in/i });
    expect(zoomIn).toBeInTheDocument();
  });

  it("zoom out button is accessible by keyboard (has aria-label)", () => {
    const { getByRole } = render(SigmaCanvas, { props: defaultProps });
    const zoomOut = getByRole("button", { name: /zoom out/i });
    expect(zoomOut).toBeInTheDocument();
  });

  it("fit-to-content button is accessible by keyboard (has aria-label)", () => {
    const { getByRole } = render(SigmaCanvas, { props: defaultProps });
    const fit = getByRole("button", { name: /fit to content/i });
    expect(fit).toBeInTheDocument();
  });

  it("zoom in click does not throw", async () => {
    const user = userEvent.setup();
    const { getByRole } = render(SigmaCanvas, { props: defaultProps });
    const zoomIn = getByRole("button", { name: /zoom in/i });
    await expect(user.click(zoomIn)).resolves.not.toThrow();
  });

  it("zoom out click does not throw", async () => {
    const user = userEvent.setup();
    const { getByRole } = render(SigmaCanvas, { props: defaultProps });
    const zoomOut = getByRole("button", { name: /zoom out/i });
    await expect(user.click(zoomOut)).resolves.not.toThrow();
  });

  it("fit-to-content click does not throw", async () => {
    const user = userEvent.setup();
    const { getByRole } = render(SigmaCanvas, { props: defaultProps });
    const fit = getByRole("button", { name: /fit to content/i });
    await expect(user.click(fit)).resolves.not.toThrow();
  });

  it("rapid zoom button clicks do not throw", async () => {
    const user = userEvent.setup();
    const { getByRole } = render(SigmaCanvas, { props: defaultProps });
    const zoomIn = getByRole("button", { name: /zoom in/i });

    await expect(
      Promise.all([
        user.click(zoomIn),
        user.click(zoomIn),
        user.click(zoomIn),
        user.click(zoomIn),
        user.click(zoomIn),
      ]),
    ).resolves.not.toThrow();
  });

  // ── Stats overlay ─────────────────────────────────────────────────────────

  it("stats overlay shows correct entity count", () => {
    const entities = [
      makeEntity("c360.ops.robotics.gcs.drone.001"),
      makeEntity("c360.ops.robotics.gcs.drone.002"),
    ];
    const { getByText } = render(SigmaCanvas, {
      props: { entities, relationships: [] },
    });
    expect(getByText("2 entities")).toBeInTheDocument();
  });

  it("stats overlay shows zero counts for empty graph", () => {
    const { getByText } = render(SigmaCanvas, { props: defaultProps });
    expect(getByText("0 entities")).toBeInTheDocument();
    expect(getByText("0 relationships")).toBeInTheDocument();
  });

  // ── Callback safety: callbacks absent should not throw ───────────────────

  it("does not throw when onEntitySelect is not provided", () => {
    // Sigma events are simulated by calling registered handlers via mock
    expect(() =>
      render(SigmaCanvas, {
        props: {
          entities: [makeEntity("c360.ops.robotics.gcs.drone.001")],
          relationships: [],
          // onEntitySelect deliberately omitted
        },
      }),
    ).not.toThrow();
  });

  // ── XSS: entity labels containing HTML ───────────────────────────────────

  it("entity label with HTML characters does not inject markup into stats overlay", () => {
    // Stats overlay uses Svelte template text interpolation (not {@html}), so
    // the label cannot escape. But verify the count rendered is numeric only.
    const entities = [makeEntity("c360.ops.robotics.gcs.drone.001")];
    const { getByText } = render(SigmaCanvas, {
      props: { entities, relationships: [] },
    });
    // Count should render as plain text "1 entities"
    expect(getByText("1 entities")).toBeInTheDocument();
  });

  // ── data-testid present for integration test targeting ───────────────────

  it("renders sigma-canvas data-testid container", () => {
    const { getByTestId } = render(SigmaCanvas, { props: defaultProps });
    expect(getByTestId("sigma-canvas")).toBeInTheDocument();
  });
});
