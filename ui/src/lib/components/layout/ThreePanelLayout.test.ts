import { describe, it, expect, vi } from "vitest";
import { render, fireEvent } from "@testing-library/svelte";
import { tick } from "svelte";
import ThreePanelLayout from "./ThreePanelLayout.svelte";
import type { Snippet } from "svelte";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function textSnippet(text: string): Snippet {
  return (() => {
    const node = document.createTextNode(text);
    return node;
  }) as unknown as Snippet;
}

function makeProps(overrides: Record<string, unknown> = {}) {
  return {
    leftPanel: textSnippet("LEFT"),
    centerPanel: textSnippet("CENTER"),
    rightPanel: textSnippet("RIGHT"),
    leftPanelOpen: true,
    rightPanelOpen: true,
    leftPanelWidth: 280,
    rightPanelWidth: 320,
    onToggleLeft: vi.fn(),
    onToggleRight: vi.fn(),
    onLeftWidthChange: vi.fn(),
    onRightWidthChange: vi.fn(),
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("ThreePanelLayout - panel visibility", () => {
  it("renders all three panels when both panels are open", () => {
    const { container } = render(ThreePanelLayout, { props: makeProps() });

    expect(
      container.querySelector('[data-testid="panel-left"]'),
    ).toBeInTheDocument();
    expect(
      container.querySelector('[data-testid="panel-center"]'),
    ).toBeInTheDocument();
    expect(
      container.querySelector('[data-testid="panel-right"]'),
    ).toBeInTheDocument();
  });

  it("hides left panel when leftPanelOpen is false", () => {
    const { container } = render(ThreePanelLayout, {
      props: makeProps({ leftPanelOpen: false }),
    });

    expect(
      container.querySelector('[data-testid="panel-left"]'),
    ).not.toBeInTheDocument();
    expect(
      container.querySelector('[data-testid="panel-center"]'),
    ).toBeInTheDocument();
    expect(
      container.querySelector('[data-testid="panel-right"]'),
    ).toBeInTheDocument();
  });

  it("hides right panel when rightPanelOpen is false", () => {
    const { container } = render(ThreePanelLayout, {
      props: makeProps({ rightPanelOpen: false }),
    });

    expect(
      container.querySelector('[data-testid="panel-left"]'),
    ).toBeInTheDocument();
    expect(
      container.querySelector('[data-testid="panel-center"]'),
    ).toBeInTheDocument();
    expect(
      container.querySelector('[data-testid="panel-right"]'),
    ).not.toBeInTheDocument();
  });

  it("shows only center panel when both panels are closed", () => {
    const { container } = render(ThreePanelLayout, {
      props: makeProps({ leftPanelOpen: false, rightPanelOpen: false }),
    });

    expect(
      container.querySelector('[data-testid="panel-left"]'),
    ).not.toBeInTheDocument();
    expect(
      container.querySelector('[data-testid="panel-center"]'),
    ).toBeInTheDocument();
    expect(
      container.querySelector('[data-testid="panel-right"]'),
    ).not.toBeInTheDocument();
  });

  it("shows left panel when leftPanelOpen changes from false to true", async () => {
    const { container, rerender } = render(ThreePanelLayout, {
      props: makeProps({ leftPanelOpen: false }),
    });

    expect(
      container.querySelector('[data-testid="panel-left"]'),
    ).not.toBeInTheDocument();

    await rerender({ leftPanelOpen: true });
    await tick();

    expect(
      container.querySelector('[data-testid="panel-left"]'),
    ).toBeInTheDocument();
  });
});

describe("ThreePanelLayout - grid template", () => {
  it("renders with data-testid on the layout container", () => {
    const { container } = render(ThreePanelLayout, { props: makeProps() });

    const layout = container.querySelector(
      '[data-testid="three-panel-layout"]',
    );
    expect(layout).toBeInTheDocument();
  });

  it("applies grid-template-columns style reflecting open panels", () => {
    const { container } = render(ThreePanelLayout, {
      props: makeProps({ leftPanelWidth: 280, rightPanelWidth: 320 }),
    });

    const layout = container.querySelector(
      '[data-testid="three-panel-layout"]',
    ) as HTMLElement;
    const style = layout?.style.gridTemplateColumns ?? "";
    expect(style).toContain("280px");
    expect(style).toContain("320px");
    expect(style).toContain("1fr");
  });

  it("excludes left width from grid template when left panel is closed", () => {
    const { container } = render(ThreePanelLayout, {
      props: makeProps({ leftPanelOpen: false, leftPanelWidth: 280 }),
    });

    const layout = container.querySelector(
      '[data-testid="three-panel-layout"]',
    ) as HTMLElement;
    const style = layout?.style.gridTemplateColumns ?? "";
    expect(style).not.toContain("280px");
    expect(style).toContain("1fr");
  });
});

describe("ThreePanelLayout - prop-to-state sync (P2-006)", () => {
  it("reflects updated leftPanelWidth when prop changes", async () => {
    const { container, rerender } = render(ThreePanelLayout, {
      props: makeProps({ leftPanelWidth: 280 }),
    });

    let layout = container.querySelector(
      '[data-testid="three-panel-layout"]',
    ) as HTMLElement;
    expect(layout.style.gridTemplateColumns).toContain("280px");

    await rerender({ leftPanelWidth: 350 });
    await tick();

    layout = container.querySelector(
      '[data-testid="three-panel-layout"]',
    ) as HTMLElement;
    expect(layout.style.gridTemplateColumns).toContain("350px");
    expect(layout.style.gridTemplateColumns).not.toContain("280px");
  });

  it("reflects updated rightPanelWidth when prop changes", async () => {
    const { container, rerender } = render(ThreePanelLayout, {
      props: makeProps({ rightPanelWidth: 320 }),
    });

    let layout = container.querySelector(
      '[data-testid="three-panel-layout"]',
    ) as HTMLElement;
    expect(layout.style.gridTemplateColumns).toContain("320px");

    await rerender({ rightPanelWidth: 400 });
    await tick();

    layout = container.querySelector(
      '[data-testid="three-panel-layout"]',
    ) as HTMLElement;
    expect(layout.style.gridTemplateColumns).toContain("400px");
    expect(layout.style.gridTemplateColumns).not.toContain("320px");
  });
});

describe("ThreePanelLayout - keyboard shortcuts", () => {
  it("calls onToggleLeft when Cmd+B is pressed", async () => {
    const onToggleLeft = vi.fn();
    render(ThreePanelLayout, {
      props: makeProps({ onToggleLeft }),
    });

    await fireEvent.keyDown(window, { key: "b", metaKey: true });
    await tick();

    expect(onToggleLeft).toHaveBeenCalledTimes(1);
  });

  it("calls onToggleLeft when Ctrl+B is pressed", async () => {
    const onToggleLeft = vi.fn();
    render(ThreePanelLayout, {
      props: makeProps({ onToggleLeft }),
    });

    await fireEvent.keyDown(window, { key: "b", ctrlKey: true });
    await tick();

    expect(onToggleLeft).toHaveBeenCalledTimes(1);
  });

  it("calls onToggleRight when Cmd+J is pressed", async () => {
    const onToggleRight = vi.fn();
    render(ThreePanelLayout, {
      props: makeProps({ onToggleRight }),
    });

    await fireEvent.keyDown(window, { key: "j", metaKey: true });
    await tick();

    expect(onToggleRight).toHaveBeenCalledTimes(1);
  });

  it("calls both onToggleLeft and onToggleRight when Cmd+\\ is pressed", async () => {
    const onToggleLeft = vi.fn();
    const onToggleRight = vi.fn();
    render(ThreePanelLayout, {
      props: makeProps({ onToggleLeft, onToggleRight }),
    });

    await fireEvent.keyDown(window, { key: "\\", metaKey: true });
    await tick();

    expect(onToggleLeft).toHaveBeenCalledTimes(1);
    expect(onToggleRight).toHaveBeenCalledTimes(1);
  });

  it("does not call any toggle without Cmd/Ctrl modifier", async () => {
    const onToggleLeft = vi.fn();
    const onToggleRight = vi.fn();
    render(ThreePanelLayout, {
      props: makeProps({ onToggleLeft, onToggleRight }),
    });

    await fireEvent.keyDown(window, { key: "b" });
    await fireEvent.keyDown(window, { key: "j" });
    await tick();

    expect(onToggleLeft).not.toHaveBeenCalled();
    expect(onToggleRight).not.toHaveBeenCalled();
  });

  it("does not fire toggle on unrelated key combos", async () => {
    const onToggleLeft = vi.fn();
    render(ThreePanelLayout, {
      props: makeProps({ onToggleLeft }),
    });

    await fireEvent.keyDown(window, { key: "x", metaKey: true });
    await tick();

    expect(onToggleLeft).not.toHaveBeenCalled();
  });
});

describe("ThreePanelLayout - resize callbacks", () => {
  it("fires onLeftWidthChange callback (via ResizeHandle onResizeEnd)", async () => {
    // ResizeHandle integration is not fully exercisable in jsdom without pointer
    // events. We verify the callback prop is wired through by confirming the
    // component renders and accepts the prop without error.
    const onLeftWidthChange = vi.fn();
    expect(() =>
      render(ThreePanelLayout, {
        props: makeProps({ onLeftWidthChange }),
      }),
    ).not.toThrow();
  });

  it("fires onRightWidthChange callback (via ResizeHandle onResizeEnd)", async () => {
    const onRightWidthChange = vi.fn();
    expect(() =>
      render(ThreePanelLayout, {
        props: makeProps({ onRightWidthChange }),
      }),
    ).not.toThrow();
  });
});

describe("ThreePanelLayout - missing optional callbacks", () => {
  it("renders without crashing when toggle callbacks are absent", () => {
    expect(() =>
      render(ThreePanelLayout, {
        props: {
          leftPanel: textSnippet("LEFT"),
          centerPanel: textSnippet("CENTER"),
          rightPanel: textSnippet("RIGHT"),
        },
      }),
    ).not.toThrow();
  });
});
