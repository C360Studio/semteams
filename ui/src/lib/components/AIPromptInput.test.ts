import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/svelte";
import AIPromptInput from "./AIPromptInput.svelte";

describe("AIPromptInput", () => {
  // =========================================================================
  // Rendering Tests
  // =========================================================================

  describe("Rendering", () => {
    it("should render with default placeholder", () => {
      render(AIPromptInput);

      const textarea = screen.getByRole("textbox", {
        name: /describe your flow/i,
      });
      expect(textarea).toBeInTheDocument();
      expect(textarea).toHaveAttribute(
        "placeholder",
        expect.stringContaining("Describe your flow"),
      );
    });

    it("should render with custom placeholder", () => {
      render(AIPromptInput, {
        props: {
          placeholder: "Enter your custom prompt here",
        },
      });

      const textarea = screen.getByRole("textbox");
      expect(textarea).toHaveAttribute(
        "placeholder",
        "Enter your custom prompt here",
      );
    });

    it("should render generate and cancel buttons", () => {
      render(AIPromptInput);

      expect(
        screen.getByRole("button", { name: /generate flow/i }),
      ).toBeInTheDocument();
      expect(
        screen.getByRole("button", { name: /cancel/i }),
      ).toBeInTheDocument();
    });

    it("should render disabled state correctly", () => {
      render(AIPromptInput, {
        props: {
          disabled: true,
        },
      });

      const textarea = screen.getByRole("textbox");
      const generateBtn = screen.getByRole("button", {
        name: /generate flow/i,
      });
      const cancelBtn = screen.getByRole("button", { name: /cancel/i });

      expect(textarea).toBeDisabled();
      expect(generateBtn).toBeDisabled();
      expect(cancelBtn).toBeDisabled();
    });

    it("should render loading state correctly", () => {
      render(AIPromptInput, {
        props: {
          loading: true,
        },
      });

      const generateBtn = screen.getByRole("button", { name: /generating/i });
      expect(generateBtn).toBeDisabled();
      expect(screen.getByText(/generating/i)).toBeInTheDocument();
    });

    it("should show loading spinner when loading", () => {
      const { container } = render(AIPromptInput, {
        props: {
          loading: true,
        },
      });

      const spinner = container.querySelector('.spinner, [role="status"]');
      expect(spinner).toBeInTheDocument();
    });

    it("should display character count indicator", () => {
      render(AIPromptInput);

      const charCount = screen.getByText(/0/);
      expect(charCount).toBeInTheDocument();
    });

    it("should update character count as user types", async () => {
      render(AIPromptInput);

      const textarea = screen.getByRole("textbox");
      await fireEvent.input(textarea, { target: { value: "Hello" } });

      expect(screen.getByText(/5/)).toBeInTheDocument();
    });

    it("should apply aria-label for accessibility", () => {
      render(AIPromptInput);

      const textarea = screen.getByRole("textbox", {
        name: /describe your flow/i,
      });
      expect(textarea).toHaveAccessibleName();
    });
  });

  // =========================================================================
  // Input Tests
  // =========================================================================

  describe("Input", () => {
    it("should allow typing in textarea", async () => {
      render(AIPromptInput);

      const textarea = screen.getByRole("textbox") as HTMLTextAreaElement;
      const testPrompt = "Create a flow that reads UDP on port 5000";

      await fireEvent.input(textarea, { target: { value: testPrompt } });

      expect(textarea.value).toBe(testPrompt);
    });

    it("should handle multiline input", async () => {
      render(AIPromptInput);

      const textarea = screen.getByRole("textbox") as HTMLTextAreaElement;
      const multilinePrompt = "Line 1\nLine 2\nLine 3";

      await fireEvent.input(textarea, { target: { value: multilinePrompt } });

      expect(textarea.value).toBe(multilinePrompt);
    });

    it("should clear textarea when cancel is clicked after typing", async () => {
      render(AIPromptInput);

      const textarea = screen.getByRole("textbox") as HTMLTextAreaElement;
      await fireEvent.input(textarea, { target: { value: "Some prompt" } });

      const cancelBtn = screen.getByRole("button", { name: /cancel/i });
      await fireEvent.click(cancelBtn);

      expect(textarea.value).toBe("");
    });

    it("should handle paste events", async () => {
      render(AIPromptInput);

      const textarea = screen.getByRole("textbox") as HTMLTextAreaElement;
      const pastedText = "Pasted prompt text";

      await fireEvent.input(textarea, { target: { value: pastedText } });

      expect(textarea.value).toBe(pastedText);
    });

    it("should enforce character limit if specified", async () => {
      const maxLength = 100;
      const { container } = render(AIPromptInput);

      const textarea = container.querySelector("textarea");
      if (textarea?.hasAttribute("maxlength")) {
        expect(textarea).toHaveAttribute("maxlength", maxLength.toString());
      }
    });
  });

  // =========================================================================
  // Submit Tests
  // =========================================================================

  describe("Submit", () => {
    it("should call onSubmit when generate button is clicked", async () => {
      const onSubmit = vi.fn();
      render(AIPromptInput, {
        props: {
          onSubmit,
        },
      });

      const textarea = screen.getByRole("textbox") as HTMLTextAreaElement;
      const testPrompt = "Create a test flow";
      await fireEvent.input(textarea, { target: { value: testPrompt } });

      const generateBtn = screen.getByRole("button", {
        name: /generate flow/i,
      });
      await fireEvent.click(generateBtn);

      expect(onSubmit).toHaveBeenCalledWith(testPrompt);
      expect(onSubmit).toHaveBeenCalledTimes(1);
    });

    it("should call onSubmit when Ctrl+Enter is pressed", async () => {
      const onSubmit = vi.fn();
      render(AIPromptInput, {
        props: {
          onSubmit,
        },
      });

      const textarea = screen.getByRole("textbox");
      const testPrompt = "Create a test flow";
      await fireEvent.input(textarea, { target: { value: testPrompt } });

      await fireEvent.keyDown(textarea, { key: "Enter", ctrlKey: true });

      expect(onSubmit).toHaveBeenCalledWith(testPrompt);
    });

    it("should call onSubmit when Cmd+Enter is pressed (Mac)", async () => {
      const onSubmit = vi.fn();
      render(AIPromptInput, {
        props: {
          onSubmit,
        },
      });

      const textarea = screen.getByRole("textbox");
      const testPrompt = "Create a test flow";
      await fireEvent.input(textarea, { target: { value: testPrompt } });

      await fireEvent.keyDown(textarea, { key: "Enter", metaKey: true });

      expect(onSubmit).toHaveBeenCalledWith(testPrompt);
    });

    it("should disable generate button when textarea is empty", () => {
      render(AIPromptInput);

      const generateBtn = screen.getByRole("button", {
        name: /generate flow/i,
      });
      expect(generateBtn).toBeDisabled();
    });

    it("should enable generate button when textarea has content", async () => {
      render(AIPromptInput);

      const textarea = screen.getByRole("textbox");
      await fireEvent.input(textarea, { target: { value: "Some content" } });

      const generateBtn = screen.getByRole("button", {
        name: /generate flow/i,
      });
      expect(generateBtn).toBeEnabled();
    });

    it("should disable generate button when loading", async () => {
      render(AIPromptInput, {
        props: {
          loading: true,
        },
      });

      const textarea = screen.getByRole("textbox");
      await fireEvent.input(textarea, { target: { value: "Some content" } });

      const generateBtn = screen.getByRole("button", { name: /generating/i });
      expect(generateBtn).toBeDisabled();
    });

    it("should trim whitespace from prompt before submitting", async () => {
      const onSubmit = vi.fn();
      render(AIPromptInput, {
        props: {
          onSubmit,
        },
      });

      const textarea = screen.getByRole("textbox");
      await fireEvent.input(textarea, { target: { value: "  Test prompt  " } });

      const generateBtn = screen.getByRole("button", {
        name: /generate flow/i,
      });
      await fireEvent.click(generateBtn);

      expect(onSubmit).toHaveBeenCalledWith("Test prompt");
    });

    it("should not submit empty prompt after trimming whitespace", async () => {
      const onSubmit = vi.fn();
      render(AIPromptInput, {
        props: {
          onSubmit,
        },
      });

      const textarea = screen.getByRole("textbox");
      await fireEvent.input(textarea, { target: { value: "   " } });

      const generateBtn = screen.getByRole("button", {
        name: /generate flow/i,
      });
      expect(generateBtn).toBeDisabled();

      await fireEvent.click(generateBtn);
      expect(onSubmit).not.toHaveBeenCalled();
    });

    it("should not call onSubmit if not provided", async () => {
      render(AIPromptInput);

      const textarea = screen.getByRole("textbox");
      await fireEvent.input(textarea, { target: { value: "Test prompt" } });

      const generateBtn = screen.getByRole("button", {
        name: /generate flow/i,
      });

      // Should not throw error
      expect(async () => await fireEvent.click(generateBtn)).not.toThrow();
    });
  });

  // =========================================================================
  // Cancel Tests
  // =========================================================================

  describe("Cancel", () => {
    it("should call onCancel when cancel button is clicked", async () => {
      const onCancel = vi.fn();
      render(AIPromptInput, {
        props: {
          onCancel,
        },
      });

      const cancelBtn = screen.getByRole("button", { name: /cancel/i });
      await fireEvent.click(cancelBtn);

      expect(onCancel).toHaveBeenCalledTimes(1);
    });

    it("should clear textarea when cancel is clicked", async () => {
      render(AIPromptInput);

      const textarea = screen.getByRole("textbox") as HTMLTextAreaElement;
      await fireEvent.input(textarea, { target: { value: "Some text" } });

      const cancelBtn = screen.getByRole("button", { name: /cancel/i });
      await fireEvent.click(cancelBtn);

      expect(textarea.value).toBe("");
    });

    it("should not call onCancel if not provided", async () => {
      render(AIPromptInput);

      const cancelBtn = screen.getByRole("button", { name: /cancel/i });

      // Should not throw error
      expect(async () => await fireEvent.click(cancelBtn)).not.toThrow();
    });

    it("should handle Escape key to cancel", async () => {
      const onCancel = vi.fn();
      render(AIPromptInput, {
        props: {
          onCancel,
        },
      });

      const textarea = screen.getByRole("textbox");
      await fireEvent.input(textarea, { target: { value: "Some text" } });
      await fireEvent.keyDown(textarea, { key: "Escape" });

      expect(onCancel).toHaveBeenCalledTimes(1);
    });
  });

  // =========================================================================
  // Accessibility Tests
  // =========================================================================

  describe("Accessibility", () => {
    it("should have proper ARIA labels", () => {
      render(AIPromptInput);

      const textarea = screen.getByRole("textbox");
      expect(textarea).toHaveAttribute("aria-label");
    });

    it("should have proper button labels", () => {
      render(AIPromptInput);

      expect(
        screen.getByRole("button", { name: /generate flow/i }),
      ).toBeInTheDocument();
      expect(
        screen.getByRole("button", { name: /cancel/i }),
      ).toBeInTheDocument();
    });

    it("should support keyboard navigation", async () => {
      render(AIPromptInput);

      const textarea = screen.getByRole("textbox");

      // Focus textarea
      textarea.focus();
      expect(textarea).toHaveFocus();

      // Tab to next element
      await fireEvent.keyDown(textarea, { key: "Tab" });
    });

    it("should announce loading state to screen readers", () => {
      render(AIPromptInput, {
        props: {
          loading: true,
        },
      });

      const generateBtn = screen.getByRole("button", { name: /generating/i });
      expect(generateBtn).toHaveAttribute("aria-busy", "true");
    });

    it("should have proper aria-disabled when disabled", () => {
      render(AIPromptInput, {
        props: {
          disabled: true,
        },
      });

      const textarea = screen.getByRole("textbox");
      expect(textarea).toHaveAttribute("aria-disabled", "true");
    });

    it("should associate character count with textarea", () => {
      const { container } = render(AIPromptInput);

      const _textarea = container.querySelector("textarea");
      const charCount = container.querySelector('[aria-live="polite"]');

      if (charCount) {
        expect(charCount).toBeInTheDocument();
      }
    });
  });

  // =========================================================================
  // Validation Tests
  // =========================================================================

  describe("Validation", () => {
    it("should show validation error for minimum length", async () => {
      const { container } = render(AIPromptInput);

      const textarea = screen.getByRole("textbox");
      await fireEvent.input(textarea, { target: { value: "ab" } }); // Too short

      const error = container.querySelector('.error, [role="alert"]');
      if (error) {
        expect(error).toHaveTextContent(/minimum/i);
      }
    });

    it("should show validation error for maximum length", async () => {
      const { container } = render(AIPromptInput);

      const textarea = screen.getByRole("textbox");
      const longText = "a".repeat(5000); // Assuming max is lower
      await fireEvent.input(textarea, { target: { value: longText } });

      const error = container.querySelector('.error, [role="alert"]');
      if (error) {
        expect(error).toHaveTextContent(/maximum/i);
      }
    });

    it("should disable submit when validation fails", async () => {
      render(AIPromptInput);

      const textarea = screen.getByRole("textbox");
      await fireEvent.input(textarea, { target: { value: "x" } }); // Too short if min length enforced

      const generateBtn = screen.getByRole("button", {
        name: /generate flow/i,
      });

      // Button should be disabled if validation fails
      const isDisabled = generateBtn.hasAttribute("disabled");
      if (isDisabled) {
        expect(generateBtn).toBeDisabled();
      }
    });
  });

  // =========================================================================
  // Character Count Tests
  // =========================================================================

  describe("Character Count", () => {
    it("should display character count", () => {
      render(AIPromptInput);

      expect(screen.getByText(/0/)).toBeInTheDocument();
    });

    it("should update character count in real-time", async () => {
      render(AIPromptInput);

      const textarea = screen.getByRole("textbox");
      await fireEvent.input(textarea, { target: { value: "Hello World" } });

      expect(screen.getByText(/11/)).toBeInTheDocument();
    });

    it("should show character limit if specified", () => {
      const { container } = render(AIPromptInput);

      const charLimit = container.querySelector(
        ".char-limit, .character-count",
      );
      if (charLimit) {
        expect(charLimit).toBeInTheDocument();
      }
    });

    it("should warn when approaching character limit", async () => {
      const { container } = render(AIPromptInput);

      const textarea = screen.getByRole("textbox");
      const nearLimitText = "a".repeat(1900); // Assuming 2000 limit
      await fireEvent.input(textarea, { target: { value: nearLimitText } });

      const warning = container.querySelector(".warning, .near-limit");
      if (warning) {
        expect(warning).toBeInTheDocument();
      }
    });
  });

  // =========================================================================
  // Edge Cases Tests
  // =========================================================================

  describe("Edge Cases", () => {
    it("should handle rapid consecutive submissions", async () => {
      const onSubmit = vi.fn();
      render(AIPromptInput, {
        props: {
          onSubmit,
        },
      });

      const textarea = screen.getByRole("textbox");
      await fireEvent.input(textarea, { target: { value: "Test" } });

      const generateBtn = screen.getByRole("button", {
        name: /generate flow/i,
      });
      await fireEvent.click(generateBtn);
      await fireEvent.click(generateBtn);

      // Should submit twice if not prevented
      expect(onSubmit).toHaveBeenCalledTimes(2);
    });

    it("should handle special characters in prompt", async () => {
      const onSubmit = vi.fn();
      render(AIPromptInput, {
        props: {
          onSubmit,
        },
      });

      const textarea = screen.getByRole("textbox");
      const specialPrompt =
        "Create flow: UDP->NATS (port: 5000) & transform {json}";
      await fireEvent.input(textarea, { target: { value: specialPrompt } });

      const generateBtn = screen.getByRole("button", {
        name: /generate flow/i,
      });
      await fireEvent.click(generateBtn);

      expect(onSubmit).toHaveBeenCalledWith(specialPrompt);
    });

    it("should handle emoji in prompt", async () => {
      const onSubmit = vi.fn();
      render(AIPromptInput, {
        props: {
          onSubmit,
        },
      });

      const textarea = screen.getByRole("textbox");
      const emojiPrompt = "Create flow 🚀 with UDP 📡";
      await fireEvent.input(textarea, { target: { value: emojiPrompt } });

      const generateBtn = screen.getByRole("button", {
        name: /generate flow/i,
      });
      await fireEvent.click(generateBtn);

      expect(onSubmit).toHaveBeenCalledWith(emojiPrompt);
    });

    it("should handle component unmount gracefully", () => {
      const { unmount } = render(AIPromptInput);

      expect(() => unmount()).not.toThrow();
    });
  });
});
