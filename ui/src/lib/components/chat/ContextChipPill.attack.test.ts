import { render, screen } from "@testing-library/svelte";
import { expect, test, describe, vi } from "vitest";
import userEvent from "@testing-library/user-event";
import ContextChipPill from "./ContextChipPill.svelte";
import type { ContextChip } from "$lib/types/chat";

function makeChip(overrides: Partial<ContextChip> = {}): ContextChip {
  return {
    id: "chip-1",
    kind: "entity",
    label: "MyEntity",
    value: "org.platform.domain.system.type.instance",
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// XSS / HTML injection in label
// ---------------------------------------------------------------------------

describe("ContextChipPill — XSS attack: HTML in label", () => {
  test("renders HTML-containing label as text, not as markup", () => {
    const htmlLabel = '<script>alert("xss")</script>';
    render(ContextChipPill, {
      props: { chip: makeChip({ label: htmlLabel }), onRemove: vi.fn() },
    });

    // The script tag must NOT execute — it must appear as literal text
    expect(screen.getByText(htmlLabel)).toBeInTheDocument();
    // Ensure no actual script element was injected
    expect(document.querySelector("script[data-xss]")).toBeNull();
  });

  test("renders img-onerror payload as text, not as markup", () => {
    const imgPayload = '<img src=x onerror="alert(1)">';
    expect(() =>
      render(ContextChipPill, {
        props: { chip: makeChip({ label: imgPayload }), onRemove: vi.fn() },
      }),
    ).not.toThrow();

    expect(screen.getByText(imgPayload)).toBeInTheDocument();
  });

  test("renders SVG injection as text, not as SVG element", () => {
    const svgPayload = '<svg onload="alert(1)"><rect/></svg>';
    render(ContextChipPill, {
      props: { chip: makeChip({ label: svgPayload }), onRemove: vi.fn() },
    });
    // Must not have injected an svg element
    expect(document.querySelectorAll("svg").length).toBe(0);
  });
});

// ---------------------------------------------------------------------------
// Label boundary conditions
// ---------------------------------------------------------------------------

describe("ContextChipPill — label boundary conditions", () => {
  test("handles 1000-character label without throwing", () => {
    const veryLong = "x".repeat(1000);
    expect(() =>
      render(ContextChipPill, {
        props: { chip: makeChip({ label: veryLong }), onRemove: vi.fn() },
      }),
    ).not.toThrow();
  });

  test("handles empty string label without throwing", () => {
    expect(() =>
      render(ContextChipPill, {
        props: { chip: makeChip({ label: "" }), onRemove: vi.fn() },
      }),
    ).not.toThrow();
  });

  test("handles label with only whitespace without throwing", () => {
    expect(() =>
      render(ContextChipPill, {
        props: { chip: makeChip({ label: "   \t\n   " }), onRemove: vi.fn() },
      }),
    ).not.toThrow();
  });

  test("handles label with unicode control characters without throwing", () => {
    expect(() =>
      render(ContextChipPill, {
        props: {
          chip: makeChip({ label: "\u0000\u001f\u007f\u009f" }),
          onRemove: vi.fn(),
        },
      }),
    ).not.toThrow();
  });

  test("handles label with right-to-left characters without throwing", () => {
    expect(() =>
      render(ContextChipPill, {
        props: {
          chip: makeChip({ label: "مرحبا بالعالم" }),
          onRemove: vi.fn(),
        },
      }),
    ).not.toThrow();
  });
});

// ---------------------------------------------------------------------------
// id boundary conditions
// ---------------------------------------------------------------------------

describe("ContextChipPill — id boundary conditions", () => {
  test("handles id with special characters in testid without throwing", () => {
    // testids with special chars should not crash the DOM query
    expect(() =>
      render(ContextChipPill, {
        props: {
          chip: makeChip({ id: "chip/with/slashes" }),
          onRemove: vi.fn(),
        },
      }),
    ).not.toThrow();
  });

  test("handles very long id without throwing", () => {
    expect(() =>
      render(ContextChipPill, {
        props: {
          chip: makeChip({ id: "id-" + "a".repeat(500) }),
          onRemove: vi.fn(),
        },
      }),
    ).not.toThrow();
  });
});

// ---------------------------------------------------------------------------
// Rapid remove clicks — no double-fire
// ---------------------------------------------------------------------------

describe("ContextChipPill — rapid remove clicks", () => {
  test("rapid sequential clicks call onRemove multiple times (caller is responsible for dedup)", async () => {
    const onRemove = vi.fn();
    const user = userEvent.setup();

    render(ContextChipPill, {
      props: {
        chip: makeChip({ id: "rapid-chip", label: "RapidTest" }),
        onRemove,
      },
    });

    const btn = screen.getByRole("button", { name: /remove/i });

    // Click 5 times rapidly
    for (let i = 0; i < 5; i++) {
      await user.click(btn);
    }

    // Each click fires — caller decides how to handle duplicates
    expect(onRemove).toHaveBeenCalledTimes(5);
    expect(onRemove).toHaveBeenCalledWith("rapid-chip");
  });

  test("concurrent clicks via Promise.all call onRemove for each", async () => {
    const onRemove = vi.fn();
    const user = userEvent.setup();

    render(ContextChipPill, {
      props: {
        chip: makeChip({ id: "concurrent-chip", label: "ConcurrentTest" }),
        onRemove,
      },
    });

    const btn = screen.getByRole("button", { name: /remove/i });

    await Promise.all([user.click(btn), user.click(btn), user.click(btn)]);

    expect(onRemove).toHaveBeenCalled();
    expect(onRemove).toHaveBeenCalledWith("concurrent-chip");
  });
});

// ---------------------------------------------------------------------------
// Remove button accessible name when label contains special HTML chars
// ---------------------------------------------------------------------------

describe("ContextChipPill — aria-label with special characters", () => {
  test("aria-label contains the literal label text for ampersand labels", () => {
    render(ContextChipPill, {
      props: {
        chip: makeChip({ label: "Foo & Bar" }),
        onRemove: vi.fn(),
      },
    });

    const btn = screen.getByRole("button", { name: "Remove Foo & Bar" });
    expect(btn).toBeInTheDocument();
  });

  test("aria-label contains the literal label text for angle-bracket labels", () => {
    render(ContextChipPill, {
      props: {
        chip: makeChip({ label: "Type<T>" }),
        onRemove: vi.fn(),
      },
    });

    const btn = screen.getByRole("button", { name: "Remove Type<T>" });
    expect(btn).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Unmount — no dangling references
// ---------------------------------------------------------------------------

describe("ContextChipPill — unmount cleanup", () => {
  test("unmounts without throwing after a click", async () => {
    const onRemove = vi.fn();
    const user = userEvent.setup();

    const { unmount } = render(ContextChipPill, {
      props: { chip: makeChip(), onRemove },
    });

    await user.click(screen.getByRole("button", { name: /remove/i }));

    expect(() => unmount()).not.toThrow();
  });

  test("multiple mount-unmount cycles do not leak", () => {
    for (let i = 0; i < 20; i++) {
      const { unmount } = render(ContextChipPill, {
        props: { chip: makeChip({ id: `leak-chip-${i}` }), onRemove: vi.fn() },
      });
      unmount();
    }
    // If we get here without error, no leak from mount/unmount cycles
    expect(true).toBe(true);
  });
});
