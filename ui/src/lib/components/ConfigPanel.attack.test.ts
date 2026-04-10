import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import ConfigPanel from "./ConfigPanel.svelte";
import type { ComponentInstance } from "$lib/types/flow";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeComponent(
  id: string,
  componentType = "udp-input",
): ComponentInstance {
  return {
    id,
    component: componentType,
    type: "input",
    name: `${componentType}-${id}`,
    config: { host: "localhost", port: 1234 },
    position: { x: 0, y: 0 },
    health: { status: "not_running" as const, lastUpdated: "" },
  };
}

// Silence fetch errors in tests
beforeEach(() => {
  vi.resetAllMocks();
});

// ---------------------------------------------------------------------------
// Null / undefined component prop
// ---------------------------------------------------------------------------

describe("ConfigPanel — null component", () => {
  it("renders nothing when component is null", () => {
    const { container } = render(ConfigPanel, {
      props: { component: null },
    });
    // No config panel should be mounted
    expect(container.querySelector(".config-panel")).not.toBeInTheDocument();
  });

  it("does not throw when component starts as null", () => {
    expect(() =>
      render(ConfigPanel, { props: { component: null } }),
    ).not.toThrow();
  });
});

// ---------------------------------------------------------------------------
// Schema fetch — loading state and error fallback
// ---------------------------------------------------------------------------

describe("ConfigPanel — schema loading", () => {
  it("shows loading state while schema is fetching", async () => {
    // Slow fetch that never resolves during this test
    const neverResolve = new Promise<Response>(() => {});
    vi.stubGlobal("fetch", vi.fn().mockReturnValue(neverResolve));

    render(ConfigPanel, { props: { component: makeComponent("c1") } });

    await waitFor(() => {
      expect(screen.getByText(/Loading schema/i)).toBeInTheDocument();
    });

    vi.unstubAllGlobals();
  });

  it("falls back to JSON editor when schema fetch returns 404", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(new Response(null, { status: 404 })),
    );

    render(ConfigPanel, { props: { component: makeComponent("c1") } });

    await waitFor(() => {
      expect(screen.getByText(/Schema not available/i)).toBeInTheDocument();
    });

    // JSON editor should be present
    expect(document.querySelector("textarea#json-config")).toBeInTheDocument();

    vi.unstubAllGlobals();
  });

  it("shows error message and JSON fallback when schema fetch fails", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response("Server Error", {
          status: 500,
          statusText: "Internal Server Error",
        }),
      ),
    );

    render(ConfigPanel, { props: { component: makeComponent("c1") } });

    await waitFor(() => {
      expect(screen.getByText(/Failed to load schema/i)).toBeInTheDocument();
    });

    // JSON editor fallback should be rendered
    expect(document.querySelector("textarea#json-config")).toBeInTheDocument();

    vi.unstubAllGlobals();
  });
});

// ---------------------------------------------------------------------------
// Error state isolation across component switches
// ---------------------------------------------------------------------------

describe("ConfigPanel — error state isolation", () => {
  it("clears JSON parse error when switching to a different component", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(new Response(null, { status: 404 })),
    );

    const comp1 = makeComponent("comp-1");
    const comp2 = makeComponent("comp-2");

    // Svelte 5: use rerender() instead of the removed $set() API
    const { rerender } = render(ConfigPanel, { props: { component: comp1 } });

    // Wait for JSON editor to appear
    await waitFor(() => {
      expect(
        document.querySelector("textarea#json-config"),
      ).toBeInTheDocument();
    });

    // Fire an input event with invalid JSON directly (avoids user-event brace escaping)
    const textarea = document.querySelector(
      "textarea#json-config",
    ) as HTMLTextAreaElement;
    fireEvent.input(textarea, { target: { value: "{ invalid json" } });

    // Parse error should appear
    await waitFor(() => {
      expect(document.getElementById("json-error")).toBeInTheDocument();
    });

    // Switch to a different component — error should clear
    await rerender({ component: comp2 });

    await waitFor(() => {
      expect(document.getElementById("json-error")).not.toBeInTheDocument();
    });

    vi.unstubAllGlobals();
  });
});

// ---------------------------------------------------------------------------
// onSave callback
// ---------------------------------------------------------------------------

describe("ConfigPanel — onSave callback", () => {
  it("calls onSave with component id and config when JSON Apply is clicked", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(new Response(null, { status: 404 })),
    );

    const onSave = vi.fn();
    const onClose = vi.fn();
    const comp = makeComponent("save-me");

    render(ConfigPanel, {
      props: { component: comp, onSave, onClose },
    });

    await waitFor(() => {
      expect(
        document.querySelector("textarea#json-config"),
      ).toBeInTheDocument();
    });

    // Click Apply
    const applyButton = screen.getByText("Apply");
    await userEvent.click(applyButton);

    expect(onSave).toHaveBeenCalledWith(
      comp.id,
      expect.objectContaining({ host: "localhost" }),
    );
    expect(onClose).toHaveBeenCalled();

    vi.unstubAllGlobals();
  });

  it("does not call onSave when JSON is invalid", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(new Response(null, { status: 404 })),
    );

    const onSave = vi.fn();
    const comp = makeComponent("invalid-json-test");

    render(ConfigPanel, {
      props: { component: comp, onSave },
    });

    await waitFor(() => {
      expect(
        document.querySelector("textarea#json-config"),
      ).toBeInTheDocument();
    });

    const textarea = document.querySelector(
      "textarea#json-config",
    ) as HTMLTextAreaElement;
    fireEvent.input(textarea, { target: { value: "not valid json" } });

    const applyButton = screen.getByText("Apply");
    await userEvent.click(applyButton);

    expect(onSave).not.toHaveBeenCalled();

    vi.unstubAllGlobals();
  });
});

// ---------------------------------------------------------------------------
// Cancel clears dirty state
// ---------------------------------------------------------------------------

describe("ConfigPanel — cancel resets config", () => {
  it("calls onClose when Cancel is clicked", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(new Response(null, { status: 404 })),
    );

    const onClose = vi.fn();
    const comp = makeComponent("cancel-test");

    render(ConfigPanel, {
      props: { component: comp, onClose },
    });

    await waitFor(() => {
      expect(
        document.querySelector("textarea#json-config"),
      ).toBeInTheDocument();
    });

    await userEvent.click(screen.getByText("Cancel"));

    expect(onClose).toHaveBeenCalled();

    vi.unstubAllGlobals();
  });
});
