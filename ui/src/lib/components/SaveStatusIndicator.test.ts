import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/svelte";
import SaveStatusIndicator from "./SaveStatusIndicator.svelte";
import type { SaveState } from "$lib/types/ui-state";

describe("SaveStatusIndicator (Prop-Based Architecture)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  // ========================================================================
  // Clean State Tests
  // ========================================================================

  describe("Clean State", () => {
    it('should display "Saved" when status is clean', () => {
      const saveState: SaveState = {
        status: "clean",
        lastSaved: new Date(),
        error: null,
      };

      render(SaveStatusIndicator, { props: { saveState } });

      expect(screen.getByText(/saved/i)).toBeInTheDocument();
    });

    it("should display formatted time for lastSaved timestamp", () => {
      const now = new Date();
      const saveState: SaveState = {
        status: "clean",
        lastSaved: now,
        error: null,
      };

      render(SaveStatusIndicator, { props: { saveState } });

      // Should show "at HH:MM:SS"
      expect(screen.getByText(/at \d+:\d+:\d+/i)).toBeInTheDocument();
    });

    it("should have clean CSS class when status is clean", () => {
      const saveState: SaveState = {
        status: "clean",
        lastSaved: new Date(),
        error: null,
      };

      const { container } = render(SaveStatusIndicator, {
        props: { saveState },
      });

      // Spec 015: Component shows data-status attribute, not status-clean class
      const indicator = container.querySelector('[data-status="clean"]');
      expect(indicator).toBeInTheDocument();
    });

    it("should disable save button when status is clean", () => {
      const saveState: SaveState = {
        status: "clean",
        lastSaved: new Date(),
        error: null,
      };

      const onSave = vi.fn();
      render(SaveStatusIndicator, { props: { saveState, onSave } });

      const saveButton = screen.getByRole("button", { name: /save flow/i });
      expect(saveButton).toBeDisabled();
    });
  });

  // ========================================================================
  // Dirty State Tests
  // ========================================================================

  describe("Dirty State", () => {
    it('should display "Unsaved changes" when status is dirty', () => {
      const saveState: SaveState = {
        status: "dirty",
        lastSaved: new Date(Date.now() - 5 * 60 * 1000),
        error: null,
      };

      render(SaveStatusIndicator, { props: { saveState } });

      expect(screen.getByText(/unsaved changes/i)).toBeInTheDocument();
    });

    it("should not display timestamp when status is dirty", () => {
      const saveState: SaveState = {
        status: "dirty",
        lastSaved: new Date(),
        error: null,
      };

      render(SaveStatusIndicator, { props: { saveState } });

      // Timestamp only shown when status is 'clean' or 'draft'
      expect(screen.queryByText(/at \d+:\d+:\d+/i)).not.toBeInTheDocument();
    });

    it("should have dirty CSS class when status is dirty", () => {
      const saveState: SaveState = {
        status: "dirty",
        lastSaved: null,
        error: null,
      };

      const { container } = render(SaveStatusIndicator, {
        props: { saveState },
      });

      const indicator = container.querySelector(".status-dirty");
      expect(indicator).toBeInTheDocument();
    });

    it("should enable save button when status is dirty", () => {
      const saveState: SaveState = {
        status: "dirty",
        lastSaved: null,
        error: null,
      };

      const onSave = vi.fn();
      render(SaveStatusIndicator, { props: { saveState, onSave } });

      const saveButton = screen.getByRole("button", { name: /save flow/i });
      expect(saveButton).not.toBeDisabled();
    });
  });

  // ========================================================================
  // Saving State Tests
  // ========================================================================

  describe("Saving State", () => {
    it('should display "Saving..." when status is saving', () => {
      const saveState: SaveState = {
        status: "saving",
        lastSaved: null,
        error: null,
      };

      render(SaveStatusIndicator, { props: { saveState } });

      expect(screen.getByText(/saving/i)).toBeInTheDocument();
    });

    it("should have saving CSS class", () => {
      const saveState: SaveState = {
        status: "saving",
        lastSaved: null,
        error: null,
      };

      const { container } = render(SaveStatusIndicator, {
        props: { saveState },
      });

      const indicator = container.querySelector(".status-saving");
      expect(indicator).toBeInTheDocument();
    });

    it("should disable save button when status is saving", () => {
      const saveState: SaveState = {
        status: "saving",
        lastSaved: null,
        error: null,
      };

      const onSave = vi.fn();
      render(SaveStatusIndicator, { props: { saveState, onSave } });

      const saveButton = screen.getByRole("button", { name: /save flow/i });
      expect(saveButton).toBeDisabled();
    });
  });

  // ========================================================================
  // Error State Tests
  // ========================================================================

  describe("Error State", () => {
    it("should display error message when status is error", () => {
      const saveState: SaveState = {
        status: "error",
        lastSaved: null,
        error: "Network connection failed",
      };

      render(SaveStatusIndicator, { props: { saveState } });

      expect(screen.getByText(/save failed/i)).toBeInTheDocument();
      expect(screen.getByText("Network connection failed")).toBeInTheDocument();
    });

    it("should have error CSS class when status is error", () => {
      const saveState: SaveState = {
        status: "error",
        lastSaved: null,
        error: "Test error",
      };

      const { container } = render(SaveStatusIndicator, {
        props: { saveState },
      });

      const indicator = container.querySelector(".status-error");
      expect(indicator).toBeInTheDocument();
    });

    it("should not display error message when error is null", () => {
      const saveState: SaveState = {
        status: "error",
        lastSaved: null,
        error: null,
      };

      render(SaveStatusIndicator, { props: { saveState } });

      expect(screen.getByText(/save failed/i)).toBeInTheDocument();
      // No error message element should be present when error is null
      expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    });

    it("should enable save button when status is error", () => {
      const saveState: SaveState = {
        status: "error",
        lastSaved: null,
        error: "Test error",
      };

      const onSave = vi.fn();
      render(SaveStatusIndicator, { props: { saveState, onSave } });

      const saveButton = screen.getByRole("button", { name: /save flow/i });
      expect(saveButton).not.toBeDisabled();
    });
  });

  // ========================================================================
  // Accessibility Tests
  // ========================================================================

  describe("Accessibility", () => {
    it("should have proper aria-label for status icon", () => {
      const saveState: SaveState = {
        status: "clean",
        lastSaved: new Date(),
        error: null,
      };

      // Spec 015: Pass validationResult to test validation status display
      const validationResult = {
        validation_status: "valid" as const,
        errors: [],
        warnings: [],
        nodes: [],
        discovered_connections: [],
      };

      render(SaveStatusIndicator, { props: { saveState, validationResult } });

      // Validation status button shows "Valid" when validationResult is passed
      const validationButton = screen.getByRole("button", {
        name: /view validation details/i,
      });
      expect(validationButton).toBeInTheDocument();
      expect(validationButton).toHaveTextContent(/valid/i);
    });

    it('should have role="alert" for error messages', () => {
      const saveState: SaveState = {
        status: "error",
        lastSaved: null,
        error: "Test error",
      };

      render(SaveStatusIndicator, { props: { saveState } });

      const errorElement = screen.getByRole("alert");
      expect(errorElement).toBeInTheDocument();
      expect(errorElement).toHaveTextContent("Test error");
    });

    it("should have proper button labels", () => {
      const saveState: SaveState = {
        status: "dirty",
        lastSaved: null,
        error: null,
      };

      const onSave = vi.fn();
      render(SaveStatusIndicator, { props: { saveState, onSave } });

      const saveButton = screen.getByRole("button", { name: /save flow/i });
      expect(saveButton).toBeInTheDocument();
    });
  });

  // ========================================================================
  // Edge Cases
  // ========================================================================

  describe("Edge Cases", () => {
    it("should handle null lastSaved in clean state", () => {
      const saveState: SaveState = {
        status: "clean",
        lastSaved: null,
        error: null,
      };

      const { container } = render(SaveStatusIndicator, {
        props: { saveState },
      });

      // Should still render without crashing
      // Spec 015: Clean state with no validationResult shows only save status (no text)
      const indicator = container.querySelector('[data-status="clean"]');
      expect(indicator).toBeInTheDocument();
      // No timestamp should be displayed (lastSaved is null)
      expect(screen.queryByText(/at \d+:\d+:\d+/i)).not.toBeInTheDocument();
    });

    it("should handle null lastSaved in dirty state", () => {
      const saveState: SaveState = {
        status: "dirty",
        lastSaved: null,
        error: null,
      };

      render(SaveStatusIndicator, { props: { saveState } });

      // Should display "Unsaved changes" without timestamp
      expect(screen.getByText(/unsaved changes/i)).toBeInTheDocument();
    });

    it("should format timestamps correctly", () => {
      const now = new Date();
      const saveState: SaveState = {
        status: "clean",
        lastSaved: now,
        error: null,
      };

      render(SaveStatusIndicator, { props: { saveState } });

      // Should show time in format like "at 3:45:23 PM"
      expect(screen.getByText(/at \d+:\d+:\d+/i)).toBeInTheDocument();
    });

    it("should not render save button when onSave is not provided", () => {
      const saveState: SaveState = {
        status: "dirty",
        lastSaved: null,
        error: null,
      };

      render(SaveStatusIndicator, { props: { saveState } });

      expect(
        screen.queryByRole("button", { name: /save flow/i }),
      ).not.toBeInTheDocument();
    });

    it("should call onSave when save button is clicked", async () => {
      const saveState: SaveState = {
        status: "dirty",
        lastSaved: null,
        error: null,
      };

      const onSave = vi.fn();
      render(SaveStatusIndicator, { props: { saveState, onSave } });

      const saveButton = screen.getByRole("button", { name: /save flow/i });
      await saveButton.click();

      expect(onSave).toHaveBeenCalledTimes(1);
    });
  });
});
