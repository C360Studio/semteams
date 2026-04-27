import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/svelte";
import TopNav from "./TopNav.svelte";

describe("TopNav", () => {
  it("renders the app name as a home link", () => {
    render(TopNav);
    const brand = screen.getByTestId("brand-home");
    expect(brand).toBeInTheDocument();
    expect(brand).toHaveAttribute("href", "/");
    expect(brand).toHaveTextContent("semteams");
  });

  it("does NOT render legacy Board/Graph/Flows tabs", () => {
    // Per ui-redesign.md: SemTeams is delegate-and-watch, not flow-builder.
    // Top-level tab navigation is gone; brand-only header.
    render(TopNav);
    expect(screen.queryByTestId("tab-board")).not.toBeInTheDocument();
    expect(screen.queryByTestId("tab-graph")).not.toBeInTheDocument();
    expect(screen.queryByTestId("tab-flows")).not.toBeInTheDocument();
    expect(screen.queryByTestId("tab-bar")).not.toBeInTheDocument();
  });
});
