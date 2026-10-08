import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/svelte";
import OverlayBadge from "./OverlayBadge.svelte";

describe("OverlayBadge", () => {
  it("renders a known value as text", () => {
    render(OverlayBadge, {
      props: {
        name: "execution-stage",
        label: "Stage",
        overlay: { state: "known", value: "completed" },
        noneLabel: "no linked run",
      },
    });

    const badge = screen.getByTestId("overlay-execution-stage");
    expect(badge).toHaveAttribute("data-state", "known");
    expect(badge).toHaveTextContent("Stage");
    expect(badge).toHaveTextContent("completed");
  });

  it("formats a known value with the caller's formatter, including false", () => {
    render(OverlayBadge, {
      props: {
        name: "needs-you",
        label: "Needs you",
        overlay: { state: "known", value: false },
        noneLabel: "no linked run",
        format: (value) => (value ? "yes" : "no"),
      },
    });

    expect(screen.getByTestId("overlay-needs-you")).toHaveTextContent("no");
  });

  it("renders none as the explicit none label, not as unknown", () => {
    render(OverlayBadge, {
      props: {
        name: "execution-stage",
        label: "Stage",
        overlay: { state: "none" },
        noneLabel: "no linked run",
      },
    });

    const badge = screen.getByTestId("overlay-execution-stage");
    expect(badge).toHaveAttribute("data-state", "none");
    expect(badge).toHaveTextContent("no linked run");
    expect(badge).not.toHaveTextContent("unknown");
  });

  it("renders unknown as text with the reason in the title and for assistive technology", () => {
    render(OverlayBadge, {
      props: {
        name: "verification",
        label: "Verification",
        overlay: { state: "unknown", reason: "no verification fact for research runs" },
        noneLabel: "no linked run",
      },
    });

    const badge = screen.getByTestId("overlay-verification");
    expect(badge).toHaveAttribute("data-state", "unknown");
    expect(badge).toHaveAttribute("title", "no verification fact for research runs");
    expect(badge).toHaveTextContent("unknown");
    expect(screen.getByText("(no verification fact for research runs)")).toBeInTheDocument();
  });

  it("keeps the reason a known overlay carries", () => {
    render(OverlayBadge, {
      props: {
        name: "needs-you",
        label: "Needs you",
        overlay: { state: "known", value: true, reason: "coordinator loop is awaiting approval" },
        noneLabel: "no linked run",
        format: (value) => (value ? "yes" : "no"),
      },
    });

    const badge = screen.getByTestId("overlay-needs-you");
    expect(badge).toHaveAttribute("title", "coordinator loop is awaiting approval");
    expect(badge).toHaveAttribute("data-value", "true");
  });
});
