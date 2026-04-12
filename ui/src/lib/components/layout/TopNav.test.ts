import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/svelte";
import TopNav from "./TopNav.svelte";

// Mock $app/stores for page URL
vi.mock("$app/stores", async () => {
  const { readable } = await import("svelte/store");
  return {
    page: readable({ url: new URL("http://localhost/") }),
  };
});

describe("TopNav", () => {
  it("renders three tabs: Board, Graph, Flows", () => {
    render(TopNav);

    expect(screen.getByTestId("tab-board")).toBeInTheDocument();
    expect(screen.getByTestId("tab-graph")).toBeInTheDocument();
    expect(screen.getByTestId("tab-flows")).toBeInTheDocument();
  });

  it("renders the app name", () => {
    render(TopNav);

    expect(screen.getByText("semteams")).toBeInTheDocument();
  });

  it("Board tab links to /", () => {
    render(TopNav);

    expect(screen.getByTestId("tab-board")).toHaveAttribute("href", "/");
  });

  it("Graph tab links to /graph", () => {
    render(TopNav);

    expect(screen.getByTestId("tab-graph")).toHaveAttribute("href", "/graph");
  });

  it("Flows tab links to /flows", () => {
    render(TopNav);

    expect(screen.getByTestId("tab-flows")).toHaveAttribute("href", "/flows");
  });

  it("has a tab-bar navigation element", () => {
    render(TopNav);

    expect(screen.getByTestId("tab-bar")).toBeInTheDocument();
  });
});
