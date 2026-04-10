import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/svelte";
import { tick } from "svelte";
import ConfigPanel from "./ConfigPanel.svelte";
import type { ComponentInstance } from "$lib/types/flow";

// Mock fetch globally
const mockFetch = vi.fn<typeof fetch>();
globalThis.fetch = mockFetch;

const makeComponent = (
  overrides: Partial<ComponentInstance> = {},
): ComponentInstance => ({
  id: "node-1",
  component: "udp-input",
  type: "input",
  name: "UDP Input 1",
  position: { x: 100, y: 100 },
  config: { port: 14550, bind_address: "0.0.0.0" },
  health: { status: "healthy", lastUpdated: new Date().toISOString() },
  ...overrides,
});

beforeEach(() => {
  vi.clearAllMocks();
  // Default: 404 (no schema — falls back to JSON editor)
  mockFetch.mockResolvedValue({ ok: false, status: 404 } as Response);
});

afterEach(() => {
  mockFetch.mockReset();
});

// ============================================================================
// P2-013: {#key component?.id} remount — error state isolation between sessions
// ============================================================================

describe("ConfigPanel - Error State Isolation on Component Switch (P2-013)", () => {
  it("clears JSON parse error when switching to a different component", async () => {
    const compA = makeComponent({ id: "node-1", config: { port: 14550 } });
    const compB = makeComponent({
      id: "node-2",
      component: "websocket-output",
      name: "WebSocket Output",
      config: { port: 8080 },
    });

    const { rerender } = render(ConfigPanel, { props: { component: compA } });

    // Wait for JSON editor to appear
    await waitFor(() => {
      expect(
        screen.queryByRole("textbox", { name: /configuration/i }),
      ).toBeInTheDocument();
    });

    // Introduce a parse error in component A's session
    const textarea = screen.getByRole("textbox", { name: /configuration/i });
    await fireEvent.input(textarea, { target: { value: "not{valid" } });

    // Verify error is visible
    await waitFor(() => {
      expect(screen.queryByText(/unexpected token/i)).toBeInTheDocument();
    });

    // Switch to component B
    await rerender({ component: compB });
    await tick();

    // Wait for new component to render
    await waitFor(() => {
      expect(screen.queryByText("WebSocket Output")).toBeInTheDocument();
    });

    // The parse error from component A's session must not be visible in B's session
    expect(screen.queryByText(/unexpected token/i)).not.toBeInTheDocument();
  });

  it("clears schema error when switching to a different component", async () => {
    const compA = makeComponent({ id: "node-1" });
    const compB = makeComponent({
      id: "node-2",
      component: "websocket-output",
      name: "WebSocket Output",
      config: { port: 8080 },
    });

    // First fetch fails with a server error to trigger schemaError state
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 500,
      statusText: "Internal Server Error",
    } as Response);
    // Second fetch (for compB) returns 404 — JSON fallback, no error
    mockFetch.mockResolvedValueOnce({ ok: false, status: 404 } as Response);

    const { rerender } = render(ConfigPanel, { props: { component: compA } });

    // Wait for schema error to appear
    await waitFor(() => {
      expect(screen.queryByRole("alert")).toBeInTheDocument();
    });

    // Switch to component B
    await rerender({ component: compB });
    await tick();

    // Wait for new component name to appear
    await waitFor(() => {
      expect(screen.queryByText("WebSocket Output")).toBeInTheDocument();
    });

    // Schema error from component A must not bleed into component B's session
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("loads component B's config fresh when switching from A to B", async () => {
    const compA = makeComponent({ id: "node-1", config: { port: 14550 } });
    const compB = makeComponent({
      id: "node-2",
      component: "websocket-output",
      name: "WebSocket Output",
      config: { port: 9999 },
    });

    const { rerender } = render(ConfigPanel, { props: { component: compA } });

    await waitFor(() => {
      expect(
        screen.queryByRole("textbox", { name: /configuration/i }),
      ).toBeInTheDocument();
    });

    const textareaA = screen.getByRole("textbox", {
      name: /configuration/i,
    }) as HTMLTextAreaElement;
    expect(JSON.parse(textareaA.value).port).toBe(14550);

    await rerender({ component: compB });
    await tick();

    await waitFor(() => {
      const textarea = screen.queryByRole("textbox", {
        name: /configuration/i,
      }) as HTMLTextAreaElement | null;
      if (textarea) {
        expect(JSON.parse(textarea.value).port).toBe(9999);
      } else {
        // textarea may momentarily be absent during remount — keep waiting
        throw new Error("textarea not yet rendered");
      }
    });
  });
});

// ============================================================================
// Null → new component transition (three-step: A → null → B)
// ============================================================================

describe("ConfigPanel - Null-Intermediate Component Switch (P2-013)", () => {
  it("renders component B correctly after A → null → B transition", async () => {
    const compA = makeComponent({ id: "node-1", name: "UDP Input 1" });
    const compB = makeComponent({
      id: "node-2",
      component: "websocket-output",
      name: "WebSocket Output",
      config: { port: 8080 },
    });

    const { rerender, container } = render(ConfigPanel, {
      props: { component: compA },
    });

    expect(screen.getByText("UDP Input 1")).toBeInTheDocument();

    // Deselect
    await rerender({ component: null });
    await tick();

    expect(container.querySelector(".config-panel")).not.toBeInTheDocument();

    // Select a different component
    await rerender({ component: compB });
    await tick();

    await waitFor(() => {
      expect(screen.queryByText("WebSocket Output")).toBeInTheDocument();
    });

    // Wait for JSON editor with compB's config
    await waitFor(() => {
      const textarea = screen.queryByRole("textbox", {
        name: /configuration/i,
      }) as HTMLTextAreaElement | null;
      if (!textarea) throw new Error("textarea not yet rendered");
      expect(JSON.parse(textarea.value).port).toBe(8080);
    });
  });

  it("does not show component A content after A → null → B transition", async () => {
    const compA = makeComponent({ id: "node-1", name: "UDP Input 1" });
    const compB = makeComponent({
      id: "node-2",
      component: "tcp-output",
      name: "TCP Output 1",
      config: { host: "localhost" },
    });

    const { rerender } = render(ConfigPanel, { props: { component: compA } });
    expect(screen.getByText("UDP Input 1")).toBeInTheDocument();

    await rerender({ component: null });
    await tick();

    await rerender({ component: compB });
    await tick();

    await waitFor(() => {
      expect(screen.queryByText("TCP Output 1")).toBeInTheDocument();
    });

    expect(screen.queryByText("UDP Input 1")).not.toBeInTheDocument();
  });
});

// ============================================================================
// P2-014: Dirty state written synchronously in field change handlers (not $effect)
// ============================================================================

describe("ConfigPanel - Dirty State Handler Contract (P2-014)", () => {
  it("preserves dirty edits from component A after switching to B and back (no timing hacks)", async () => {
    const compA = makeComponent({ id: "node-a", config: { port: 14550 } });
    const compB = makeComponent({
      id: "node-b",
      component: "websocket-output",
      name: "WebSocket Output",
      config: { port: 3000 },
    });

    const { rerender } = render(ConfigPanel, { props: { component: compA } });

    await waitFor(() => {
      expect(
        screen.queryByRole("textbox", { name: /configuration/i }),
      ).toBeInTheDocument();
    });

    const dirtyConfig = { port: 7777, custom: "preserved" };
    const textarea = screen.getByRole("textbox", { name: /configuration/i });
    await fireEvent.input(textarea, {
      target: { value: JSON.stringify(dirtyConfig, null, 2) },
    });

    // Switch to B — dirty state should be saved as part of the switch (not via a delayed $effect)
    await rerender({ component: compB });
    await tick();

    await waitFor(() => {
      expect(screen.queryByText("WebSocket Output")).toBeInTheDocument();
    });

    // Switch back to A
    await rerender({ component: compA });
    await tick();

    await waitFor(() => {
      const ta = screen.queryByRole("textbox", {
        name: /configuration/i,
      }) as HTMLTextAreaElement | null;
      if (!ta) throw new Error("textarea not yet rendered");
      const parsed = JSON.parse(ta.value);
      expect(parsed.port).toBe(7777);
      expect(parsed.custom).toBe("preserved");
    });
  });

  it("does NOT restore dirty state for component A after a successful save clears it", async () => {
    const onSave = vi.fn();
    const onClose = vi.fn();
    const compA = makeComponent({ id: "node-a", config: { port: 14550 } });
    const compB = makeComponent({
      id: "node-b",
      component: "websocket-output",
      name: "WebSocket Output",
      config: { port: 3000 },
    });

    const { rerender } = render(ConfigPanel, {
      props: { component: compA, onSave, onClose },
    });

    await waitFor(() => {
      expect(
        screen.queryByRole("textbox", { name: /configuration/i }),
      ).toBeInTheDocument();
    });

    // Dirty edit
    const textarea = screen.getByRole("textbox", { name: /configuration/i });
    await fireEvent.input(textarea, {
      target: { value: JSON.stringify({ port: 9999 }) },
    });

    // Save — clears dirty state
    await fireEvent.click(screen.getByRole("button", { name: /apply/i }));
    expect(onSave).toHaveBeenCalledWith("node-a", { port: 9999 });

    // Simulate onClose unmounting/hiding panel, then re-opening compA
    await rerender({ component: compB });
    await tick();

    await rerender({ component: compA });
    await tick();

    await waitFor(() => {
      const ta = screen.queryByRole("textbox", {
        name: /configuration/i,
      }) as HTMLTextAreaElement | null;
      if (!ta) throw new Error("textarea not yet rendered");
      // After a successful save, dirty cache is cleared.
      // Reloading compA should show component.config, not the old dirty value.
      expect(JSON.parse(ta.value).port).toBe(14550);
    });
  });

  it("does NOT restore dirty state for component A after Cancel clears it", async () => {
    const onClose = vi.fn();
    const compA = makeComponent({ id: "node-a", config: { port: 14550 } });
    const compB = makeComponent({
      id: "node-b",
      component: "websocket-output",
      name: "WebSocket Output",
      config: { port: 3000 },
    });

    const { rerender } = render(ConfigPanel, {
      props: { component: compA, onClose },
    });

    await waitFor(() => {
      expect(
        screen.queryByRole("textbox", { name: /configuration/i }),
      ).toBeInTheDocument();
    });

    // Dirty edit
    const textarea = screen.getByRole("textbox", { name: /configuration/i });
    await fireEvent.input(textarea, {
      target: { value: JSON.stringify({ port: 8888 }) },
    });

    // Cancel — should clear dirty state and reset to component.config
    await waitFor(() => {
      expect(
        screen.queryByRole("button", { name: /cancel/i }),
      ).toBeInTheDocument();
    });
    await fireEvent.click(screen.getByRole("button", { name: /cancel/i }));

    // Switch to B then back to A
    await rerender({ component: compB });
    await tick();

    await rerender({ component: compA });
    await tick();

    await waitFor(() => {
      const ta = screen.queryByRole("textbox", {
        name: /configuration/i,
      }) as HTMLTextAreaElement | null;
      if (!ta) throw new Error("textarea not yet rendered");
      // After Cancel, dirty cache is cleared.
      // Reloading compA should show component.config (port: 14550), not the dirty edit.
      expect(JSON.parse(ta.value).port).toBe(14550);
    });
  });
});
