import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import WorkSourceState from "./WorkSourceState.svelte";

describe("WorkSourceState", () => {
  it("says it is loading", () => {
    render(WorkSourceState, { props: { state: { kind: "loading" } } });

    expect(screen.getByTestId("work-source-state")).toHaveAttribute("data-state", "loading");
    expect(screen.getByRole("status")).toHaveTextContent(/reading the configured portfolio/i);
  });

  it("says the source is not configured, and that this is not an empty board", () => {
    render(WorkSourceState, { props: { state: { kind: "unconfigured" } } });

    const state = screen.getByTestId("work-source-state");
    expect(state).toHaveAttribute("data-state", "unconfigured");
    expect(screen.getByRole("heading", { name: "Work source not configured" })).toBeInTheDocument();
    expect(state).toHaveTextContent("not an empty board");
    expect(state).toHaveTextContent("WORK_SOURCE");
  });

  it("shows an error code, message and validation issues", () => {
    render(WorkSourceState, {
      props: {
        state: {
          kind: "error",
          code: "PORTFOLIO_INVALID",
          message: "portfolio document is invalid",
          issues: [{ path: "programs[0].id", message: "is required" }],
        },
      },
    });

    expect(screen.getByTestId("work-source-state")).toHaveAttribute("data-code", "PORTFOLIO_INVALID");
    expect(screen.getByRole("alert")).toHaveTextContent("portfolio document is invalid");
    expect(screen.getByTestId("work-source-issues")).toHaveTextContent("programs[0].id is required");
  });

  it("offers a retry that works from the keyboard", async () => {
    const user = userEvent.setup();
    const onretry = vi.fn();
    render(WorkSourceState, {
      props: { state: { kind: "error", code: "NETWORK_ERROR", message: "Failed to fetch" }, onretry },
    });

    await user.tab();
    expect(screen.getByRole("button", { name: "Try again" })).toHaveFocus();
    await user.keyboard("{Enter}");

    expect(onretry).toHaveBeenCalledOnce();
  });
});
