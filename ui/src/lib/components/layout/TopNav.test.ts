import { beforeEach, describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/svelte";
import TopNav from "./TopNav.svelte";

// The nav marks the current page, so it reads the route id from `$app/state`.
const pageMock = vi.hoisted(() => ({ route: { id: "/" as string | null } }));
vi.mock("$app/state", () => ({ page: pageMock }));

beforeEach(() => {
  pageMock.route.id = "/";
});

describe("TopNav", () => {
  it("renders the app name as a home link", () => {
    render(TopNav);
    const brand = screen.getByTestId("brand-home");
    expect(brand).toBeInTheDocument();
    expect(brand).toHaveAttribute("href", "/");
    expect(brand).toHaveTextContent("semteams");
  });

  it("links to the work lens without displacing the brand link", () => {
    render(TopNav);
    const work = screen.getByTestId("nav-work");
    expect(work).toHaveAttribute("href", "/work");
    expect(work).toHaveTextContent("Work");
    expect(screen.getByRole("navigation", { name: "Primary" })).toContainElement(work);
    expect(screen.getByTestId("brand-home")).toHaveAttribute("href", "/");
  });

  it("marks the work link as the current page only on the work route", () => {
    pageMock.route.id = "/work";
    const onWork = render(TopNav);
    expect(screen.getByTestId("nav-work")).toHaveAttribute("aria-current", "page");
    expect(screen.getByTestId("nav-work")).toHaveAccessibleName("Work");
    onWork.unmount();

    pageMock.route.id = "/";
    render(TopNav);
    expect(screen.getByTestId("nav-work")).not.toHaveAttribute("aria-current");
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
