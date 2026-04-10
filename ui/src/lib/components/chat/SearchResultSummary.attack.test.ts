import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import SearchResultSummary from "./SearchResultSummary.svelte";
import type { SearchResultAttachment } from "$lib/types/chat";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeAttachment(
  overrides: Partial<SearchResultAttachment> = {},
): SearchResultAttachment {
  return {
    kind: "search-result",
    query: "drones",
    results: [
      {
        id: "c360.ops.robotics.gcs.drone.001",
        label: "001",
        type: "drone",
        domain: "robotics",
      },
    ],
    totalCount: 1,
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// XSS in result label fields
// ---------------------------------------------------------------------------

describe("SearchResultSummary.attack — XSS in result fields", () => {
  it("renders XSS payload in label as text, not markup", () => {
    const xssLabel = '<script>alert("xss")</script>';
    const attachment = makeAttachment({
      results: [
        {
          id: "c360.ops.robotics.gcs.drone.001",
          label: xssLabel,
          type: "drone",
          domain: "robotics",
        },
      ],
    });
    const { container } = render(SearchResultSummary, {
      props: { attachment },
    });

    // The script tag must NOT be executed or present as an element
    expect(container.querySelector("script")).toBeNull();
    // The text content should be present (escaped), not nothing
    expect(container.textContent).toContain("alert");
  });

  it("renders XSS payload in query field as text, not markup", () => {
    const xssQuery = "<img src=x onerror=alert(1)>";
    const attachment = makeAttachment({
      query: xssQuery,
      results: [],
      totalCount: 0,
    });
    const { container } = render(SearchResultSummary, {
      props: { attachment },
    });

    expect(container.querySelector("img[onerror]")).toBeNull();
  });

  it("renders XSS payload in type field as text, not markup", () => {
    const attachment = makeAttachment({
      results: [
        {
          id: "c360.ops.robotics.gcs.drone.001",
          label: "drone",
          type: '<iframe src="evil.com">',
          domain: "robotics",
        },
      ],
    });
    const { container } = render(SearchResultSummary, {
      props: { attachment },
    });
    expect(container.querySelector("iframe")).toBeNull();
  });

  it("renders XSS payload in domain field as text, not markup", () => {
    const attachment = makeAttachment({
      results: [
        {
          id: "c360.ops.robotics.gcs.drone.001",
          label: "drone",
          type: "drone",
          domain: '"><script>evil()</script>',
        },
      ],
    });
    const { container } = render(SearchResultSummary, {
      props: { attachment },
    });
    expect(container.querySelector("script")).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// Very many results
// ---------------------------------------------------------------------------

describe("SearchResultSummary.attack — very many results", () => {
  it("renders 1000 results without crashing", () => {
    const results = Array.from({ length: 1_000 }, (_, i) => ({
      id: `c360.ops.robotics.gcs.drone.${String(i).padStart(4, "0")}`,
      label: String(i),
      type: "drone",
      domain: "robotics",
    }));
    const attachment = makeAttachment({ results, totalCount: 1_000 });
    expect(() =>
      render(SearchResultSummary, { props: { attachment } }),
    ).not.toThrow();
  });

  it("renders a very long label string without crashing", () => {
    const longLabel = "drone-".repeat(500);
    const attachment = makeAttachment({
      results: [
        {
          id: "c360.ops.robotics.gcs.drone.001",
          label: longLabel,
          type: "drone",
          domain: "robotics",
        },
      ],
    });
    expect(() =>
      render(SearchResultSummary, { props: { attachment } }),
    ).not.toThrow();
  });
});

// ---------------------------------------------------------------------------
// Missing / undefined optional result fields
// ---------------------------------------------------------------------------

describe("SearchResultSummary.attack — missing fields on result objects", () => {
  it("renders when result is missing domain field", () => {
    const attachment: SearchResultAttachment = {
      kind: "search-result",
      query: "drones",
      results: [
        {
          id: "c360.ops.robotics.gcs.drone.001",
          label: "001",
          type: "drone",
          domain: "",
        },
      ],
      totalCount: 1,
    };
    expect(() =>
      render(SearchResultSummary, { props: { attachment } }),
    ).not.toThrow();
  });

  it("renders when results array items have empty string fields", () => {
    const attachment = makeAttachment({
      results: [
        {
          id: "c360.ops.robotics.gcs.drone.001",
          label: "",
          type: "",
          domain: "",
        },
      ],
    });
    expect(() =>
      render(SearchResultSummary, { props: { attachment } }),
    ).not.toThrow();
  });

  it("handles undefined results field (phase-1 backward compat shape)", () => {
    // Phase 1 attachments may lack results
    const attachment: SearchResultAttachment = {
      kind: "search-result",
      query: "drones",
      entityIds: ["c360.ops.robotics.gcs.drone.001"],
      count: 1,
    };
    expect(() =>
      render(SearchResultSummary, { props: { attachment } }),
    ).not.toThrow();
  });

  it("shows zero-result state when results is an empty array", () => {
    const attachment = makeAttachment({ results: [], totalCount: 0 });
    render(SearchResultSummary, { props: { attachment } });
    expect(screen.getByTestId("search-result-summary")).toHaveTextContent(
      /no results/i,
    );
  });
});

// ---------------------------------------------------------------------------
// Interaction — no onViewEntity callback
// ---------------------------------------------------------------------------

describe("SearchResultSummary.attack — safe when onViewEntity missing", () => {
  it("clicking a result item without onViewEntity does not throw", async () => {
    const user = userEvent.setup();
    const attachment = makeAttachment();
    render(SearchResultSummary, { props: { attachment } });

    const item = screen.getByTestId(
      "search-result-item-c360.ops.robotics.gcs.drone.001",
    );
    await expect(user.click(item)).resolves.not.toThrow();
  });

  it("does not throw when onViewEntity is explicitly set to undefined", async () => {
    const user = userEvent.setup();
    const attachment = makeAttachment();
    render(SearchResultSummary, {
      props: { attachment, onViewEntity: undefined },
    });

    const item = screen.getByTestId(
      "search-result-item-c360.ops.robotics.gcs.drone.001",
    );
    await expect(user.click(item)).resolves.not.toThrow();
  });
});

// ---------------------------------------------------------------------------
// Rapid clicks on the same item
// ---------------------------------------------------------------------------

describe("SearchResultSummary.attack — rapid clicks", () => {
  it("rapid clicks on a result call onViewEntity each time", async () => {
    const user = userEvent.setup();
    const onViewEntity = vi.fn();
    const attachment = makeAttachment();
    render(SearchResultSummary, { props: { attachment, onViewEntity } });

    const item = screen.getByTestId(
      "search-result-item-c360.ops.robotics.gcs.drone.001",
    );
    await user.click(item);
    await user.click(item);
    await user.click(item);

    expect(onViewEntity).toHaveBeenCalledTimes(3);
    expect(onViewEntity).toHaveBeenCalledWith(
      "c360.ops.robotics.gcs.drone.001",
    );
  });
});

// ---------------------------------------------------------------------------
// totalCount vs results length mismatch
// ---------------------------------------------------------------------------

describe("SearchResultSummary.attack — totalCount vs results mismatch", () => {
  it("renders correctly when totalCount > results.length (server-side pagination)", () => {
    const attachment = makeAttachment({ totalCount: 999 });
    render(SearchResultSummary, { props: { attachment } });
    const summary = screen.getByTestId("search-result-summary");
    expect(summary).toHaveTextContent("999");
  });

  it("renders correctly when totalCount is 0 but results has items", () => {
    const attachment = makeAttachment({ totalCount: 0 });
    render(SearchResultSummary, { props: { attachment } });
    // Should render results items and show 0 count — no crash
    expect(screen.getByTestId("search-result-summary")).toBeInTheDocument();
  });
});
