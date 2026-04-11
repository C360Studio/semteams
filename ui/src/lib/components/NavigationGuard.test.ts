import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render } from "@testing-library/svelte";
import NavigationGuard from "./NavigationGuard.svelte";
import type { Navigation } from "@sveltejs/kit";
import { beforeNavigate, goto } from "$app/navigation";

// ========================================================================
// Mock Setup
// ========================================================================

// Capture the beforeNavigate callback
let navigationCallback: ((navigation: Navigation) => void) | null = null;
let mockCancel: ReturnType<typeof vi.fn>;

vi.mock("$app/navigation", () => ({
  beforeNavigate: vi.fn((callback) => {
    navigationCallback = callback;
  }),
  goto: vi.fn(),
}));

vi.mock("$app/environment", () => ({
  browser: true,
}));

describe("NavigationGuard", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    navigationCallback = null;
    mockCancel = vi.fn();
    // Clear window.onbeforeunload
    window.onbeforeunload = null;
  });

  afterEach(() => {
    // Cleanup
    window.onbeforeunload = null;
  });

  // ========================================================================
  // Prop Acceptance Tests
  // ========================================================================

  describe("Prop Acceptance", () => {
    it("should accept saveState prop with all status values", () => {
      const statuses = ["clean", "dirty", "draft", "saving", "error"] as const;

      statuses.forEach((status) => {
        expect(() => {
          render(NavigationGuard, {
            props: {
              saveState: { status, lastSaved: null, error: null },
            },
          });
        }).not.toThrow();
      });
    });

    it("should accept optional showDialog prop", () => {
      const { component } = render(NavigationGuard, {
        props: {
          saveState: { status: "clean", lastSaved: null, error: null },
          showDialog: true,
        },
      });

      expect(component).toBeTruthy();
    });

    it("should accept optional callback props", () => {
      const onShowDialog = vi.fn();
      const onNavigationAllowed = vi.fn();

      const { component } = render(NavigationGuard, {
        props: {
          saveState: { status: "clean", lastSaved: null, error: null },
          onShowDialog,
          onNavigationAllowed,
        },
      });

      expect(component).toBeTruthy();
    });
  });

  // ========================================================================
  // beforeNavigate Hook Tests
  // ========================================================================

  describe("beforeNavigate Hook", () => {
    it("should register beforeNavigate callback on mount", () => {
      render(NavigationGuard, {
        props: {
          saveState: { status: "clean", lastSaved: null, error: null },
        },
      });

      expect(beforeNavigate).toHaveBeenCalled();
      expect(navigationCallback).not.toBeNull();
    });

    it("should cancel navigation when status is dirty", () => {
      render(NavigationGuard, {
        props: {
          saveState: { status: "dirty", lastSaved: null, error: null },
        },
      });

      // Simulate navigation attempt
      const mockNavigation = {
        to: { url: { pathname: "/other-page" } },
        cancel: mockCancel,
      } as unknown as Navigation;

      navigationCallback?.(mockNavigation);

      expect(mockCancel).toHaveBeenCalled();
    });

    it("should not cancel navigation when status is clean", () => {
      render(NavigationGuard, {
        props: {
          saveState: { status: "clean", lastSaved: null, error: null },
        },
      });

      const mockNavigation = {
        to: { url: { pathname: "/other-page" } },
        cancel: mockCancel,
      } as unknown as Navigation;

      navigationCallback?.(mockNavigation);

      expect(mockCancel).not.toHaveBeenCalled();
    });

    it("should not cancel navigation when status is draft", () => {
      render(NavigationGuard, {
        props: {
          saveState: { status: "draft", lastSaved: null, error: null },
        },
      });

      const mockNavigation = {
        to: { url: { pathname: "/other-page" } },
        cancel: mockCancel,
      } as unknown as Navigation;

      navigationCallback?.(mockNavigation);

      expect(mockCancel).not.toHaveBeenCalled();
    });

    it("should not cancel navigation when status is saving", () => {
      render(NavigationGuard, {
        props: {
          saveState: { status: "saving", lastSaved: null, error: null },
        },
      });

      const mockNavigation = {
        to: { url: { pathname: "/other-page" } },
        cancel: mockCancel,
      } as unknown as Navigation;

      navigationCallback?.(mockNavigation);

      expect(mockCancel).not.toHaveBeenCalled();
    });

    it("should not cancel navigation when to is null (browser back/forward)", () => {
      render(NavigationGuard, {
        props: {
          saveState: { status: "dirty", lastSaved: null, error: null },
        },
      });

      const mockNavigation = {
        to: null,
        cancel: mockCancel,
      } as unknown as Navigation;

      navigationCallback?.(mockNavigation);

      expect(mockCancel).not.toHaveBeenCalled();
    });

    it("should allow navigation when isNavigating flag is set", () => {
      const { component } = render(NavigationGuard, {
        props: {
          saveState: { status: "dirty", lastSaved: null, error: null },
        },
      });

      // First navigation - should cancel
      const mockNavigation1 = {
        to: { url: { pathname: "/page1" } },
        cancel: mockCancel,
      } as unknown as Navigation;

      navigationCallback?.(mockNavigation1);
      expect(mockCancel).toHaveBeenCalledTimes(1);

      // Allow navigation
      component.allowNavigation();

      // Second navigation - should NOT cancel (isNavigating flag is set)
      mockCancel.mockClear();
      const mockNavigation2 = {
        to: { url: { pathname: "/page1" } },
        cancel: mockCancel,
      } as unknown as Navigation;

      navigationCallback?.(mockNavigation2);
      expect(mockCancel).not.toHaveBeenCalled();
    });
  });

  // ========================================================================
  // showDialog Prop Tests
  // ========================================================================

  describe("showDialog Bindable Prop", () => {
    it("should update showDialog to true when navigation is blocked", () => {
      let showDialog = false;
      const onShowDialog = vi.fn((value: boolean) => {
        showDialog = value;
      });

      render(NavigationGuard, {
        props: {
          saveState: { status: "dirty", lastSaved: null, error: null },
          showDialog,
          onShowDialog,
        },
      });

      // Simulate navigation
      const mockNavigation = {
        to: { url: { pathname: "/other-page" } },
        cancel: mockCancel,
      } as unknown as Navigation;

      navigationCallback?.(mockNavigation);

      expect(onShowDialog).toHaveBeenCalledWith(true);
    });

    it("should not invoke onShowDialog when navigation is not blocked", () => {
      const onShowDialog = vi.fn();

      render(NavigationGuard, {
        props: {
          saveState: { status: "clean", lastSaved: null, error: null },
          onShowDialog,
        },
      });

      const mockNavigation = {
        to: { url: { pathname: "/other-page" } },
        cancel: mockCancel,
      } as unknown as Navigation;

      navigationCallback?.(mockNavigation);

      expect(onShowDialog).not.toHaveBeenCalled();
    });
  });

  // ========================================================================
  // Exported Function Tests
  // ========================================================================

  describe("allowNavigation()", () => {
    it("should navigate to pending destination when called", () => {
      const { component } = render(NavigationGuard, {
        props: {
          saveState: { status: "dirty", lastSaved: null, error: null },
        },
      });

      // Trigger navigation to store pending destination
      const mockNavigation = {
        to: { url: { pathname: "/target-page" } },
        cancel: mockCancel,
      } as unknown as Navigation;

      navigationCallback?.(mockNavigation);

      // Allow navigation
      component.allowNavigation();

      expect(goto).toHaveBeenCalledWith("/target-page");
    });

    it("should set showDialog to false when called", () => {
      const onShowDialog = vi.fn();

      const { component } = render(NavigationGuard, {
        props: {
          saveState: { status: "dirty", lastSaved: null, error: null },
          onShowDialog,
        },
      });

      // Trigger navigation
      const mockNavigation = {
        to: { url: { pathname: "/target-page" } },
        cancel: mockCancel,
      } as unknown as Navigation;

      navigationCallback?.(mockNavigation);

      // onShowDialog should have been called with true
      expect(onShowDialog).toHaveBeenCalledWith(true);

      // Allow navigation
      onShowDialog.mockClear();
      component.allowNavigation();

      expect(onShowDialog).toHaveBeenCalledWith(false);
    });

    it("should invoke onNavigationAllowed callback when called", () => {
      const onNavigationAllowed = vi.fn();

      const { component } = render(NavigationGuard, {
        props: {
          saveState: { status: "dirty", lastSaved: null, error: null },
          onNavigationAllowed,
        },
      });

      // Trigger navigation
      const mockNavigation = {
        to: { url: { pathname: "/target-page" } },
        cancel: mockCancel,
      } as unknown as Navigation;

      navigationCallback?.(mockNavigation);

      // Allow navigation
      component.allowNavigation();

      expect(onNavigationAllowed).toHaveBeenCalled();
    });

    it("should do nothing when called without pending navigation", () => {
      const { component } = render(NavigationGuard, {
        props: {
          saveState: { status: "dirty", lastSaved: null, error: null },
        },
      });

      // Call without triggering navigation first
      component.allowNavigation();

      expect(goto).not.toHaveBeenCalled();
    });

    it("should clear pending navigation after allowing navigation", () => {
      const { component } = render(NavigationGuard, {
        props: {
          saveState: { status: "dirty", lastSaved: null, error: null },
        },
      });

      // Trigger navigation
      const mockNavigation = {
        to: { url: { pathname: "/target-page" } },
        cancel: mockCancel,
      } as unknown as Navigation;

      navigationCallback?.(mockNavigation);

      // Allow navigation
      component.allowNavigation();
      expect(goto).toHaveBeenCalledTimes(1);

      // Call again - should not navigate
      vi.mocked(goto).mockClear();
      component.allowNavigation();
      expect(goto).not.toHaveBeenCalled();
    });
  });

  describe("cancelNavigation()", () => {
    it("should clear pending navigation when called", () => {
      const { component } = render(NavigationGuard, {
        props: {
          saveState: { status: "dirty", lastSaved: null, error: null },
        },
      });

      // Trigger navigation
      const mockNavigation = {
        to: { url: { pathname: "/target-page" } },
        cancel: mockCancel,
      } as unknown as Navigation;

      navigationCallback?.(mockNavigation);

      // Cancel navigation
      component.cancelNavigation();

      // Trying to allow navigation now should do nothing
      component.allowNavigation();
      expect(goto).not.toHaveBeenCalled();
    });

    it("should set showDialog to false when called", () => {
      const onShowDialog = vi.fn();

      const { component } = render(NavigationGuard, {
        props: {
          saveState: { status: "dirty", lastSaved: null, error: null },
          onShowDialog,
        },
      });

      // Trigger navigation
      const mockNavigation = {
        to: { url: { pathname: "/target-page" } },
        cancel: mockCancel,
      } as unknown as Navigation;

      navigationCallback?.(mockNavigation);
      expect(onShowDialog).toHaveBeenCalledWith(true);

      // Cancel navigation
      onShowDialog.mockClear();
      component.cancelNavigation();

      expect(onShowDialog).toHaveBeenCalledWith(false);
    });

    it("should not invoke onNavigationAllowed callback when called", () => {
      const onNavigationAllowed = vi.fn();

      const { component } = render(NavigationGuard, {
        props: {
          saveState: { status: "dirty", lastSaved: null, error: null },
          onNavigationAllowed,
        },
      });

      // Trigger navigation
      const mockNavigation = {
        to: { url: { pathname: "/target-page" } },
        cancel: mockCancel,
      } as unknown as Navigation;

      navigationCallback?.(mockNavigation);

      // Cancel navigation
      component.cancelNavigation();

      expect(onNavigationAllowed).not.toHaveBeenCalled();
    });
  });

  // ========================================================================
  // beforeunload Handler Tests
  // ========================================================================

  describe("beforeunload Handler", () => {
    it("should set beforeunload handler when status is dirty", async () => {
      const { rerender } = render(NavigationGuard, {
        props: {
          saveState: { status: "clean", lastSaved: null, error: null },
        },
      });

      expect(window.onbeforeunload).toBeNull();

      await rerender({
        saveState: { status: "dirty", lastSaved: null, error: null },
      });

      expect(window.onbeforeunload).toBeTruthy();
    });

    it("should clear beforeunload handler when status becomes clean", async () => {
      const { rerender } = render(NavigationGuard, {
        props: {
          saveState: { status: "dirty", lastSaved: null, error: null },
        },
      });

      expect(window.onbeforeunload).toBeTruthy();

      await rerender({
        saveState: { status: "clean", lastSaved: null, error: null },
      });

      expect(window.onbeforeunload).toBeNull();
    });

    it("should not set beforeunload handler for draft status", () => {
      render(NavigationGuard, {
        props: {
          saveState: { status: "draft", lastSaved: null, error: null },
        },
      });

      expect(window.onbeforeunload).toBeNull();
    });

    it("should not set beforeunload handler for saving status", () => {
      render(NavigationGuard, {
        props: {
          saveState: { status: "saving", lastSaved: null, error: null },
        },
      });

      expect(window.onbeforeunload).toBeNull();
    });

    it("should not set beforeunload handler for error status", () => {
      render(NavigationGuard, {
        props: {
          saveState: {
            status: "error",
            lastSaved: null,
            error: "Error message",
          },
        },
      });

      expect(window.onbeforeunload).toBeNull();
    });

    it("should clear beforeunload handler on unmount", () => {
      const { unmount } = render(NavigationGuard, {
        props: {
          saveState: { status: "dirty", lastSaved: null, error: null },
        },
      });

      expect(window.onbeforeunload).toBeTruthy();

      unmount();

      expect(window.onbeforeunload).toBeNull();
    });

    it("should prevent default and return empty string on beforeunload event", () => {
      render(NavigationGuard, {
        props: {
          saveState: { status: "dirty", lastSaved: null, error: null },
        },
      });

      expect(window.onbeforeunload).toBeTruthy();

      // Simulate beforeunload event
      const event = {
        preventDefault: vi.fn(),
        returnValue: "",
      } as unknown as BeforeUnloadEvent;

      const result = window.onbeforeunload?.(event);

      expect(event.preventDefault).toHaveBeenCalled();
      expect(event.returnValue).toBe("");
      expect(result).toBe("");
    });
  });

  // ========================================================================
  // State Reactivity Tests
  // ========================================================================

  describe("State Reactivity", () => {
    it("should react to status prop changes from clean to dirty", async () => {
      const { rerender } = render(NavigationGuard, {
        props: {
          saveState: { status: "clean", lastSaved: null, error: null },
        },
      });

      expect(window.onbeforeunload).toBeNull();

      await rerender({
        saveState: { status: "dirty", lastSaved: null, error: null },
      });

      expect(window.onbeforeunload).toBeTruthy();

      // Navigation should now be blocked
      const mockNavigation = {
        to: { url: { pathname: "/other-page" } },
        cancel: mockCancel,
      } as unknown as Navigation;

      navigationCallback?.(mockNavigation);
      expect(mockCancel).toHaveBeenCalled();
    });

    it("should stop guarding when dirty becomes clean", async () => {
      const { rerender } = render(NavigationGuard, {
        props: {
          saveState: { status: "dirty", lastSaved: null, error: null },
        },
      });

      await rerender({
        saveState: { status: "clean", lastSaved: null, error: null },
      });

      expect(window.onbeforeunload).toBeNull();

      // Navigation should not be blocked
      const mockNavigation = {
        to: { url: { pathname: "/other-page" } },
        cancel: mockCancel,
      } as unknown as Navigation;

      navigationCallback?.(mockNavigation);
      expect(mockCancel).not.toHaveBeenCalled();
    });

    it("should handle rapid status changes", async () => {
      const { rerender } = render(NavigationGuard, {
        props: {
          saveState: { status: "clean", lastSaved: null, error: null },
        },
      });

      await rerender({
        saveState: { status: "dirty", lastSaved: null, error: null },
      });
      expect(window.onbeforeunload).toBeTruthy();

      await rerender({
        saveState: { status: "clean", lastSaved: null, error: null },
      });
      expect(window.onbeforeunload).toBeNull();

      await rerender({
        saveState: { status: "dirty", lastSaved: null, error: null },
      });
      expect(window.onbeforeunload).toBeTruthy();

      // Final state should be guarding
      const mockNavigation = {
        to: { url: { pathname: "/other-page" } },
        cancel: mockCancel,
      } as unknown as Navigation;

      navigationCallback?.(mockNavigation);
      expect(mockCancel).toHaveBeenCalled();
    });
  });

  // ========================================================================
  // Edge Cases
  // ========================================================================

  describe("Edge Cases", () => {
    it("should handle multiple navigation attempts while dirty", () => {
      render(NavigationGuard, {
        props: {
          saveState: { status: "dirty", lastSaved: null, error: null },
        },
      });

      // First navigation
      const mockNavigation1 = {
        to: { url: { pathname: "/page1" } },
        cancel: mockCancel,
      } as unknown as Navigation;

      navigationCallback?.(mockNavigation1);
      expect(mockCancel).toHaveBeenCalledTimes(1);

      // Second navigation (without resolving first)
      mockCancel.mockClear();
      const mockNavigation2 = {
        to: { url: { pathname: "/page2" } },
        cancel: mockCancel,
      } as unknown as Navigation;

      navigationCallback?.(mockNavigation2);
      expect(mockCancel).toHaveBeenCalledTimes(1);
    });

    it("should handle component unmount during navigation", () => {
      const { unmount } = render(NavigationGuard, {
        props: {
          saveState: { status: "dirty", lastSaved: null, error: null },
        },
      });

      // Trigger navigation
      const mockNavigation = {
        to: { url: { pathname: "/other-page" } },
        cancel: mockCancel,
      } as unknown as Navigation;

      navigationCallback?.(mockNavigation);

      // Unmount should clean up hooks without errors
      expect(() => unmount()).not.toThrow();
      expect(window.onbeforeunload).toBeNull();
    });

    it("should handle navigation to same pathname", () => {
      render(NavigationGuard, {
        props: {
          saveState: { status: "dirty", lastSaved: null, error: null },
        },
      });

      const mockNavigation = {
        to: { url: { pathname: "/current-page" } },
        cancel: mockCancel,
      } as unknown as Navigation;

      // Should still block navigation to same page
      navigationCallback?.(mockNavigation);
      expect(mockCancel).toHaveBeenCalled();
    });

    it("should handle allowNavigation with no callbacks defined", () => {
      const { component } = render(NavigationGuard, {
        props: {
          saveState: { status: "dirty", lastSaved: null, error: null },
          // No callbacks provided
        },
      });

      // Trigger navigation
      const mockNavigation = {
        to: { url: { pathname: "/other-page" } },
        cancel: mockCancel,
      } as unknown as Navigation;

      navigationCallback?.(mockNavigation);

      // Should not throw when calling allowNavigation without callbacks
      expect(() => component.allowNavigation()).not.toThrow();
      expect(goto).toHaveBeenCalled();
    });

    it("should handle cancelNavigation with no callbacks defined", () => {
      const { component } = render(NavigationGuard, {
        props: {
          saveState: { status: "dirty", lastSaved: null, error: null },
          // No callbacks provided
        },
      });

      // Trigger navigation
      const mockNavigation = {
        to: { url: { pathname: "/other-page" } },
        cancel: mockCancel,
      } as unknown as Navigation;

      navigationCallback?.(mockNavigation);

      // Should not throw when calling cancelNavigation without callbacks
      expect(() => component.cancelNavigation()).not.toThrow();
    });
  });

  // ========================================================================
  // Integration Notes
  // ========================================================================

  /**
   * NOTE: Full navigation blocking behavior is validated by E2E tests:
   *
   * 1. Actual browser navigation events (back button, link clicks)
   * 2. Integration with parent components showing confirmation dialogs
   * 3. Save/Discard/Cancel workflows
   * 4. Cross-browser beforeunload dialog behavior
   *
   * These unit tests verify the NavigationGuard component's internal logic,
   * state management, and callback behavior in isolation.
   */
});
