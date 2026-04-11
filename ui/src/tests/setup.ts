import "@testing-library/jest-dom/vitest";
import { vi, beforeEach } from "vitest";

// graphology-layout-forceatlas2/worker uses Web Workers + URL.createObjectURL
// which jsdom doesn't support. Mock the worker module globally.
vi.mock("graphology-layout-forceatlas2/worker", () => {
  return {
    default: vi.fn().mockImplementation(() => ({
      start: vi.fn(),
      stop: vi.fn(),
      kill: vi.fn(),
    })),
  };
});

// Sigma.js requires WebGL2RenderingContext which jsdom doesn't provide.
// Mock the sigma module so components that import it can load in tests.
// Real WebGL rendering is tested via Playwright E2E.
vi.mock("sigma", () => {
  const mockCamera = {
    animatedZoom: vi.fn(),
    animatedUnzoom: vi.fn(),
    animatedReset: vi.fn(),
  };
  const MockSigma = vi.fn().mockImplementation(() => ({
    on: vi.fn(),
    off: vi.fn(),
    refresh: vi.fn(),
    kill: vi.fn(),
    getCamera: vi.fn().mockReturnValue(mockCamera),
    getGraph: vi.fn(),
  }));
  return { default: MockSigma };
});

// d3-zoom calls SVGSVGElement.width.baseVal.value and height.baseVal.value
// inside its defaultExtent() function when a zoom gesture fires. jsdom does
// not implement SVGAnimatedLength on SVG elements, so we stub those properties
// to return zero-value objects.  This prevents the unhandled TypeError that
// d3-zoom throws via the d3-timer flush path during DataView tests.
function makeSVGAnimatedLength(value = 0) {
  return { baseVal: { value, valueInSpecifiedUnits: value, unitType: 1 } };
}

if (typeof SVGSVGElement !== "undefined") {
  Object.defineProperty(SVGSVGElement.prototype, "width", {
    get() {
      return makeSVGAnimatedLength(
        this.getAttribute("width") ? Number(this.getAttribute("width")) : 0,
      );
    },
    configurable: true,
  });
  Object.defineProperty(SVGSVGElement.prototype, "height", {
    get() {
      return makeSVGAnimatedLength(
        this.getAttribute("height") ? Number(this.getAttribute("height")) : 0,
      );
    },
    configurable: true,
  });
}

// ---------------------------------------------------------------------------
// Global beforeEach: reset all mocks between tests to prevent mock bleed.
//
// mockClear() (used in most test files' beforeEach) clears call history but
// does NOT clear the mockImplementationOnce queue. This causes "mock bleed"
// when a test sets up more mockImplementationOnce calls than it consumes —
// the excess bleeds into subsequent tests.
//
// vi.resetAllMocks() clears both call history AND the once-queue. After
// resetting, we re-apply the module-level mock implementations for sigma
// and graphology-layout-forceatlas2/worker, which would otherwise lose
// their implementations and return undefined when constructed.
// ---------------------------------------------------------------------------
beforeEach(async () => {
  vi.resetAllMocks();

  // Re-apply FA2 worker mock implementation after reset
  const fa2Module = await import("graphology-layout-forceatlas2/worker");
  vi.mocked(fa2Module.default).mockImplementation((() => ({
    start: vi.fn(),
    stop: vi.fn(),
    kill: vi.fn(),
    isRunning: vi.fn().mockReturnValue(false),
  })) as any);

  // Re-apply Sigma mock implementation after reset
  const sigmaModule = await import("sigma");
  const mockCamera = {
    animatedZoom: vi.fn(),
    animatedUnzoom: vi.fn(),
    animatedReset: vi.fn(),
  };
  vi.mocked(sigmaModule.default).mockImplementation((() => ({
    on: vi.fn(),
    off: vi.fn(),
    refresh: vi.fn(),
    kill: vi.fn(),
    getCamera: vi.fn().mockReturnValue(mockCamera),
    getGraph: vi.fn(),
  })) as any);
});
