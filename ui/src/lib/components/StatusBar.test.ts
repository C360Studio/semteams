/// <reference lib="es2015" />
import { describe, it, expect, vi, beforeEach } from "vitest";
import "@testing-library/jest-dom/vitest";
import { render, screen, fireEvent } from "@testing-library/svelte";
import StatusBar from "./StatusBar.svelte";
import type { RuntimeStateInfo } from "$lib/types/ui-state";

describe("StatusBar", () => {
  const defaultProps = {
    runtimeState: {
      state: "not_deployed",
      message: null,
      lastTransition: null,
    } as RuntimeStateInfo,
    isFlowValid: true,
    onDeploy: vi.fn(),
    onStart: vi.fn(),
    onStop: vi.fn(),
    onToggleRuntimePanel: vi.fn(),
    showRuntimePanel: false,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe("Runtime State Display", () => {
    it("should display not_deployed state", () => {
      render(StatusBar, { props: defaultProps });

      expect(screen.getByText("not_deployed")).toBeInTheDocument();
    });

    it("should display deployed_stopped state", () => {
      render(StatusBar, {
        props: {
          ...defaultProps,
          runtimeState: {
            state: "deployed_stopped",
            message: null,
            lastTransition: null,
          },
        },
      });

      expect(screen.getByText("deployed_stopped")).toBeInTheDocument();
    });

    it("should display running state", () => {
      render(StatusBar, {
        props: {
          ...defaultProps,
          runtimeState: {
            state: "running",
            message: null,
            lastTransition: null,
          },
        },
      });

      expect(screen.getByText("running")).toBeInTheDocument();
    });

    it("should display error state", () => {
      render(StatusBar, {
        props: {
          ...defaultProps,
          runtimeState: { state: "error", message: null, lastTransition: null },
        },
      });

      expect(screen.getByText("error")).toBeInTheDocument();
    });

    it("should display error message when in error state", () => {
      render(StatusBar, {
        props: {
          ...defaultProps,
          runtimeState: {
            state: "error",
            message: "Component failed to start",
            lastTransition: null,
          },
        },
      });

      expect(screen.getByText("error")).toBeInTheDocument();
      expect(screen.getByText("Component failed to start")).toBeInTheDocument();
    });
  });

  describe("Deployment Buttons", () => {
    it("should show Deploy button when not deployed", () => {
      render(StatusBar, { props: defaultProps });

      expect(
        screen.getByRole("button", { name: /deploy/i }),
      ).toBeInTheDocument();
    });

    it("should call onDeploy when Deploy clicked", async () => {
      const onDeploy = vi.fn();
      render(StatusBar, { props: { ...defaultProps, onDeploy } });

      const deployButton = screen.getByRole("button", { name: /deploy/i });
      await fireEvent.click(deployButton);

      expect(onDeploy).toHaveBeenCalled();
    });

    it("should disable Deploy button when flow is invalid", () => {
      render(StatusBar, { props: { ...defaultProps, isFlowValid: false } });

      const deployButton = screen.getByRole("button", { name: /deploy/i });
      expect(deployButton).toBeDisabled();
    });

    it("should show Start button when deployed but stopped", () => {
      render(StatusBar, {
        props: {
          ...defaultProps,
          runtimeState: {
            state: "deployed_stopped",
            message: null,
            lastTransition: null,
          },
        },
      });

      expect(
        screen.getByRole("button", { name: /start/i }),
      ).toBeInTheDocument();
    });

    it("should call onStart when Start clicked", async () => {
      const onStart = vi.fn();
      render(StatusBar, {
        props: {
          ...defaultProps,
          runtimeState: {
            state: "deployed_stopped",
            message: null,
            lastTransition: null,
          },
          onStart,
        },
      });

      const startButton = screen.getByRole("button", { name: /start/i });
      await fireEvent.click(startButton);

      expect(onStart).toHaveBeenCalled();
    });

    it("should show Stop button when running", () => {
      render(StatusBar, {
        props: {
          ...defaultProps,
          runtimeState: {
            state: "running",
            message: null,
            lastTransition: null,
          },
        },
      });

      expect(screen.getByRole("button", { name: /stop/i })).toBeInTheDocument();
    });

    it("should call onStop when Stop clicked", async () => {
      const onStop = vi.fn();
      render(StatusBar, {
        props: {
          ...defaultProps,
          runtimeState: {
            state: "running",
            message: null,
            lastTransition: null,
          },
          onStop,
        },
      });

      const stopButton = screen.getByRole("button", { name: /stop/i });
      await fireEvent.click(stopButton);

      expect(onStop).toHaveBeenCalled();
    });
  });

  describe("Button State Logic", () => {
    it("should not show Deploy button when already running", () => {
      render(StatusBar, {
        props: {
          ...defaultProps,
          runtimeState: {
            state: "running",
            message: null,
            lastTransition: null,
          },
        },
      });

      expect(
        screen.queryByRole("button", { name: /^deploy$/i }),
      ).not.toBeInTheDocument();
    });

    it("should not show Start button when not deployed", () => {
      render(StatusBar, { props: defaultProps });

      expect(
        screen.queryByRole("button", { name: /^start$/i }),
      ).not.toBeInTheDocument();
    });

    it("should not show Stop button when not running", () => {
      render(StatusBar, { props: defaultProps });

      expect(
        screen.queryByRole("button", { name: /^stop$/i }),
      ).not.toBeInTheDocument();
    });

    it("should not show Stop button when deployed but stopped", () => {
      render(StatusBar, {
        props: {
          ...defaultProps,
          runtimeState: {
            state: "deployed_stopped",
            message: null,
            lastTransition: null,
          },
        },
      });

      expect(
        screen.queryByRole("button", { name: /^stop$/i }),
      ).not.toBeInTheDocument();
    });
  });

  describe("Runtime Status Only (Feature 015)", () => {
    it("does NOT show validation status (Draft)", () => {
      const runtimeState = {
        state: "not_deployed" as const,
        message: null,
        lastTransition: null,
      };

      render(StatusBar, {
        props: { ...defaultProps, runtimeState, isFlowValid: false },
      });

      const bar = screen.getByTestId("status-bar");
      expect(bar).not.toHaveTextContent("Draft");
      expect(bar).not.toHaveTextContent("Valid");
    });

    it("does NOT show validation errors in status bar", () => {
      const runtimeState = {
        state: "not_deployed" as const,
        message: null,
        lastTransition: null,
      };

      render(StatusBar, {
        props: { ...defaultProps, runtimeState, isFlowValid: false },
      });

      const bar = screen.getByTestId("status-bar");
      expect(bar).not.toHaveTextContent(/\d+ error/i);
    });

    it("shows runtime error message when state is error", () => {
      const runtimeState = {
        state: "error" as const,
        message: "Component crashed: udp-input-1",
        lastTransition: null,
      };

      render(StatusBar, {
        props: { ...defaultProps, runtimeState },
      });

      const bar = screen.getByTestId("status-bar");
      expect(bar).toHaveTextContent(/component crashed/i);
    });

    it("shows runtime status with distinct visual styling", () => {
      const runtimeState = {
        state: "running" as const,
        message: null,
        lastTransition: null,
      };

      render(StatusBar, {
        props: { ...defaultProps, runtimeState, onStop: vi.fn() },
      });

      // Use data-state attribute since text "running" appears multiple times (icon + text)
      const runtimeStateElement = screen
        .getByTestId("status-bar")
        .querySelector('[data-state="running"]');
      expect(runtimeStateElement).toBeInTheDocument();
      expect(runtimeStateElement).toHaveClass("running");
      expect(runtimeStateElement).not.toHaveClass("draft");
      expect(runtimeStateElement).not.toHaveClass("valid");
      expect(runtimeStateElement).not.toHaveClass("error");
    });
  });

  describe("Debug Button (Runtime Panel Toggle)", () => {
    it("should not show Debug button when not running", () => {
      render(StatusBar, {
        props: {
          ...defaultProps,
          runtimeState: {
            state: "not_deployed",
            message: null,
            lastTransition: null,
          },
        },
      });

      expect(
        screen.queryByTestId("debug-toggle-button"),
      ).not.toBeInTheDocument();
    });

    it("should not show Debug button when deployed but stopped", () => {
      render(StatusBar, {
        props: {
          ...defaultProps,
          runtimeState: {
            state: "deployed_stopped",
            message: null,
            lastTransition: null,
          },
        },
      });

      expect(
        screen.queryByTestId("debug-toggle-button"),
      ).not.toBeInTheDocument();
    });

    it("should show Debug button when running", () => {
      render(StatusBar, {
        props: {
          ...defaultProps,
          runtimeState: {
            state: "running",
            message: null,
            lastTransition: null,
          },
        },
      });

      expect(screen.getByTestId("debug-toggle-button")).toBeInTheDocument();
    });

    it("should display up arrow when panel is closed", () => {
      render(StatusBar, {
        props: {
          ...defaultProps,
          runtimeState: {
            state: "running",
            message: null,
            lastTransition: null,
          },
          showRuntimePanel: false,
        },
      });

      const button = screen.getByTestId("debug-toggle-button");
      expect(button).toHaveTextContent("▲ Debug");
    });

    it("should display down arrow when panel is open", () => {
      render(StatusBar, {
        props: {
          ...defaultProps,
          runtimeState: {
            state: "running",
            message: null,
            lastTransition: null,
          },
          showRuntimePanel: true,
        },
      });

      const button = screen.getByTestId("debug-toggle-button");
      expect(button).toHaveTextContent("▼ Debug");
    });

    it("should call onToggleRuntimePanel when clicked", async () => {
      const onToggleRuntimePanel = vi.fn();
      render(StatusBar, {
        props: {
          ...defaultProps,
          runtimeState: {
            state: "running",
            message: null,
            lastTransition: null,
          },
          onToggleRuntimePanel,
        },
      });

      const button = screen.getByTestId("debug-toggle-button");
      await fireEvent.click(button);

      expect(onToggleRuntimePanel).toHaveBeenCalledOnce();
    });

    it("should have keyboard shortcut in tooltip", () => {
      render(StatusBar, {
        props: {
          ...defaultProps,
          runtimeState: {
            state: "running",
            message: null,
            lastTransition: null,
          },
        },
      });

      const button = screen.getByTestId("debug-toggle-button");
      expect(button).toHaveAttribute("title", "Toggle runtime panel (Ctrl+`)");
    });

    it("should have proper aria-label", () => {
      render(StatusBar, {
        props: {
          ...defaultProps,
          runtimeState: {
            state: "running",
            message: null,
            lastTransition: null,
          },
        },
      });

      const button = screen.getByTestId("debug-toggle-button");
      expect(button).toHaveAttribute("aria-label", "Toggle runtime panel");
    });
  });
});
