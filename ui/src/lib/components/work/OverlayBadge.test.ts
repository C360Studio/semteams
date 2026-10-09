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

  it("shows the reason as visible text next to the badge when asked, and not twice for assistive technology", () => {
    render(OverlayBadge, {
      props: {
        name: "verification",
        label: "Verification",
        overlay: { state: "unknown", reason: "no verification fact for research runs" },
        noneLabel: "not recorded",
        showReason: true,
      },
    });

    expect(screen.getByTestId("overlay-verification-reason")).toBeVisible();
    expect(screen.getByTestId("overlay-verification-reason")).toHaveTextContent(
      "no verification fact for research runs",
    );
    // The visible line replaces the visually hidden copy; the badge keeps its title.
    expect(screen.queryByText("(no verification fact for research runs)")).not.toBeInTheDocument();
    expect(screen.getByTestId("overlay-verification")).toHaveAttribute(
      "title",
      "no verification fact for research runs",
    );
  });

  it("shows no reason line when the overlay carries none", () => {
    render(OverlayBadge, {
      props: {
        name: "execution-stage",
        label: "Stage",
        overlay: { state: "known", value: "completed" },
        noneLabel: "not recorded",
        showReason: true,
      },
    });

    expect(screen.queryByTestId("overlay-execution-stage-reason")).not.toBeInTheDocument();
  });

  it("keeps the reason hidden from view by default so cards stay compact", () => {
    render(OverlayBadge, {
      props: {
        name: "verification",
        label: "Verification",
        overlay: { state: "unknown", reason: "no verification fact for research runs" },
        noneLabel: "no linked run",
      },
    });

    expect(screen.queryByTestId("overlay-verification-reason")).not.toBeInTheDocument();
  });
});
