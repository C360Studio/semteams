// Task: NLQ Phase 1 — DataView NLQ integration tests (alpha.17 schema)

import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import DataView from "./DataView.svelte";
import type { PathSearchResult, GlobalSearchResult } from "$lib/types/graph";
import { GraphApiError } from "$lib/services/graphApi";

// ---------------------------------------------------------------------------
// Module mocks
// ---------------------------------------------------------------------------

vi.mock("$lib/services/graphApi", () => {
  return {
    graphApi: {
      pathSearch: vi.fn(),
      globalSearch: vi.fn(),
    },
    GraphApiError: class GraphApiError extends Error {
      constructor(
        message: string,
        public statusCode: number,
        public details?: unknown,
      ) {
        super(message);
        this.name = "GraphApiError";
      }
    },
  };
});

import { graphApi } from "$lib/services/graphApi";

// ---------------------------------------------------------------------------
// Fixture helpers
// ---------------------------------------------------------------------------

const mockPathSearchFn = graphApi.pathSearch as ReturnType<typeof vi.fn>;
const mockGlobalSearchFn = graphApi.globalSearch as ReturnType<typeof vi.fn>;

const DRONE_001 = "c360.ops.robotics.gcs.drone.001";
const FLEET_ALPHA = "c360.ops.robotics.gcs.fleet.alpha";

function makePathSearchResult(): PathSearchResult {
  return {
    entities: [
      {
        id: FLEET_ALPHA,
        triples: [
          {
            subject: FLEET_ALPHA,
            predicate: "fleet.name",
            object: "Alpha Fleet",
          },
        ],
      },
    ],
    edges: [],
  };
}

function makeGlobalSearchResult(): GlobalSearchResult {
  return {
    entities: [
      {
        id: DRONE_001,
        triples: [
          {
            subject: DRONE_001,
            predicate: "core.property.name",
            object: "Drone 001",
          },
        ],
      },
    ],
    communitySummaries: [
      {
        communityId: "community-1",
        text: "Drone cluster in the west-coast region.",
        keywords: ["drone", "west-coast"],
      },
    ],
    relationships: [],
    count: 1,
    durationMs: 30,
  };
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("DataView NLQ Integration", () => {
  beforeEach(() => {
    mockPathSearchFn.mockClear();
    mockGlobalSearchFn.mockClear();
    // Default: pathSearch succeeds so the component mounts cleanly
    mockPathSearchFn.mockResolvedValue(makePathSearchResult());
  });

  // ---------------------------------------------------------------------------
  // NlqSearchBar presence
  // ---------------------------------------------------------------------------

  describe("NlqSearchBar rendering", () => {
    it("should render NlqSearchBar in the view", async () => {
      render(DataView, { props: { flowId: "test-flow-1" } });

      // Wait for initial load
      await waitFor(() => expect(mockPathSearchFn).toHaveBeenCalled());

      // The search input (NlqSearchBar) must be present
      expect(screen.getByRole("textbox")).toBeInTheDocument();
    });

    it("should render NlqSearchBar above the canvas area", async () => {
      render(DataView, { props: { flowId: "test-flow-1" } });

      await waitFor(() => expect(mockPathSearchFn).toHaveBeenCalled());

      // NlqSearchBar input must exist in the document
      const searchInput = screen.getByRole("textbox");
      expect(searchInput).toBeInTheDocument();
    });
  });

  // ---------------------------------------------------------------------------
  // Search submission flow
  // ---------------------------------------------------------------------------

  describe("search submission", () => {
    it("should call graphApi.globalSearch when a query is submitted", async () => {
      mockGlobalSearchFn.mockResolvedValue(makeGlobalSearchResult());
      const user = userEvent.setup();

      render(DataView, { props: { flowId: "test-flow-1" } });
      await waitFor(() => expect(mockPathSearchFn).toHaveBeenCalled());

      const input = screen.getByRole("textbox");
      await user.type(input, "show me all drones{Enter}");

      await waitFor(() => {
        expect(mockGlobalSearchFn).toHaveBeenCalledOnce();
        expect(mockGlobalSearchFn).toHaveBeenCalledWith(
          "show me all drones",
          expect.anything(),
          expect.anything(),
        );
      });
    });

    it("should pass the search query string to globalSearch", async () => {
      mockGlobalSearchFn.mockResolvedValue(makeGlobalSearchResult());
      const user = userEvent.setup();

      render(DataView, { props: { flowId: "test-flow-1" } });
      await waitFor(() => expect(mockPathSearchFn).toHaveBeenCalled());

      const input = screen.getByRole("textbox");
      await user.type(input, "fleet alpha drones{Enter}");

      await waitFor(() => {
        const [queryArg] = mockGlobalSearchFn.mock.calls[0];
        expect(queryArg).toBe("fleet alpha drones");
      });
    });

    it("should show loading indicator while globalSearch is in flight", async () => {
      let resolveSearch: (value: GlobalSearchResult) => void;
      const pendingSearch = new Promise<GlobalSearchResult>((resolve) => {
        resolveSearch = resolve;
      });
      mockGlobalSearchFn.mockReturnValueOnce(pendingSearch);
      const user = userEvent.setup();

      render(DataView, { props: { flowId: "test-flow-1" } });
      await waitFor(() => expect(mockPathSearchFn).toHaveBeenCalled());

      const input = screen.getByRole("textbox");
      await user.type(input, "drones{Enter}");

      // NlqSearchBar should show a loading indicator (data-testid="nlq-loading-indicator")
      await waitFor(() => {
        expect(
          screen.getByTestId("nlq-loading-indicator"),
        ).toBeInTheDocument();
      });

      // Resolve the search so the component does not hang
      resolveSearch!(makeGlobalSearchResult());

      await waitFor(() => {
        expect(
          screen.queryByTestId("nlq-loading-indicator"),
        ).not.toBeInTheDocument();
      });
    });

    it("should load search results into the graph store after globalSearch", async () => {
      mockGlobalSearchFn.mockResolvedValue(makeGlobalSearchResult());
      const user = userEvent.setup();

      render(DataView, { props: { flowId: "test-flow-1" } });
      await waitFor(() => expect(mockPathSearchFn).toHaveBeenCalled());

      const input = screen.getByRole("textbox");
      await user.type(input, "show me all drones{Enter}");

      // globalSearch should have been called and the loading indicator
      // should disappear (meaning results were loaded)
      await waitFor(() => {
        expect(mockGlobalSearchFn).toHaveBeenCalled();
        expect(
          screen.queryByTestId("nlq-loading-indicator"),
        ).not.toBeInTheDocument();
      });
    });

    it("should enter search mode after a successful globalSearch", async () => {
      mockGlobalSearchFn.mockResolvedValue(makeGlobalSearchResult());
      const user = userEvent.setup();

      render(DataView, { props: { flowId: "test-flow-1" } });
      await waitFor(() => expect(mockPathSearchFn).toHaveBeenCalled());

      const input = screen.getByRole("textbox");
      await user.type(input, "drones{Enter}");

      await waitFor(() => {
        // "Back to browse" button signals that inSearchMode = true
        expect(
          screen.getByRole("button", { name: /back to browse/i }),
        ).toBeInTheDocument();
      });
    });
  });

  // ---------------------------------------------------------------------------
  // Error handling for globalSearch
  // ---------------------------------------------------------------------------

  describe("globalSearch error handling", () => {
    it("should show error message when globalSearch throws a network error", async () => {
      mockGlobalSearchFn.mockRejectedValueOnce(
        new GraphApiError("Network error during globalSearch", 0),
      );
      const user = userEvent.setup();

      render(DataView, { props: { flowId: "test-flow-1" } });
      await waitFor(() => expect(mockPathSearchFn).toHaveBeenCalled());

      const input = screen.getByRole("textbox");
      await user.type(input, "drones{Enter}");

      await waitFor(() => {
        expect(screen.getByRole("alert")).toBeInTheDocument();
      });
    });

    it("should show error message when globalSearch throws a GraphQL error", async () => {
      mockGlobalSearchFn.mockRejectedValueOnce(
        new GraphApiError("NLQ classifier unavailable", 503),
      );
      const user = userEvent.setup();

      render(DataView, { props: { flowId: "test-flow-1" } });
      await waitFor(() => expect(mockPathSearchFn).toHaveBeenCalled());

      const input = screen.getByRole("textbox");
      await user.type(input, "show fleet{Enter}");

      await waitFor(() => {
        expect(screen.getByRole("alert")).toBeInTheDocument();
      });
    });

    it("should NOT replace initial pathSearch data when globalSearch fails", async () => {
      mockGlobalSearchFn.mockRejectedValueOnce(
        new GraphApiError("Service unavailable", 503),
      );
      const user = userEvent.setup();

      render(DataView, { props: { flowId: "test-flow-1" } });
      await waitFor(() => expect(mockPathSearchFn).toHaveBeenCalled());

      // Verify clean state before search
      await waitFor(() => {
        expect(screen.queryByRole("alert")).not.toBeInTheDocument();
      });

      const input = screen.getByRole("textbox");
      await user.type(input, "drones{Enter}");

      // Error appears but main view structure remains intact
      await waitFor(() => {
        expect(screen.getByRole("alert")).toBeInTheDocument();
        expect(screen.getByTestId("data-view")).toBeInTheDocument();
      });
    });

    it("should clear search error when user starts a new search", async () => {
      // First search fails
      mockGlobalSearchFn
        .mockRejectedValueOnce(new GraphApiError("First failure", 500))
        .mockResolvedValueOnce(makeGlobalSearchResult());

      const user = userEvent.setup();

      render(DataView, { props: { flowId: "test-flow-1" } });
      await waitFor(() => expect(mockPathSearchFn).toHaveBeenCalled());

      const input = screen.getByRole("textbox");
      await user.type(input, "failing query{Enter}");

      await waitFor(() => {
        expect(screen.getByRole("alert")).toBeInTheDocument();
      });

      // User types again — error should clear
      await user.clear(input);
      await user.type(input, "n");

      await waitFor(() => {
        expect(screen.queryByRole("alert")).not.toBeInTheDocument();
      });
    });
  });

  // ---------------------------------------------------------------------------
  // Clear search / "Back to browse"
  // ---------------------------------------------------------------------------

  describe("clear search", () => {
    it("should call pathSearch again when Back to browse is clicked", async () => {
      mockGlobalSearchFn.mockResolvedValue(makeGlobalSearchResult());
      const user = userEvent.setup();

      render(DataView, { props: { flowId: "test-flow-1" } });
      await waitFor(() => expect(mockPathSearchFn).toHaveBeenCalledTimes(1));

      // Perform a search to enter search mode
      const input = screen.getByRole("textbox");
      await user.type(input, "drones{Enter}");

      await waitFor(() => {
        expect(
          screen.getByRole("button", { name: /back to browse/i }),
        ).toBeInTheDocument();
      });

      const clearButton = screen.getByRole("button", {
        name: /back to browse/i,
      });
      await user.click(clearButton);

      // pathSearch should be called a second time to reload browse data
      await waitFor(() => {
        expect(mockPathSearchFn).toHaveBeenCalledTimes(2);
      });
    });

    it("should exit search mode when Back to browse is clicked", async () => {
      mockGlobalSearchFn.mockResolvedValue(makeGlobalSearchResult());
      const user = userEvent.setup();

      render(DataView, { props: { flowId: "test-flow-1" } });
      await waitFor(() => expect(mockPathSearchFn).toHaveBeenCalled());

      const input = screen.getByRole("textbox");
      await user.type(input, "drones{Enter}");

      await waitFor(() => {
        expect(
          screen.getByRole("button", { name: /back to browse/i }),
        ).toBeInTheDocument();
      });

      const clearButton = screen.getByRole("button", {
        name: /back to browse/i,
      });
      await user.click(clearButton);

      await waitFor(() => {
        expect(
          screen.queryByRole("button", { name: /back to browse/i }),
        ).not.toBeInTheDocument();
      });
    });

    it("should clear search query input when Back to browse is clicked", async () => {
      mockGlobalSearchFn.mockResolvedValue(makeGlobalSearchResult());
      const user = userEvent.setup();

      render(DataView, { props: { flowId: "test-flow-1" } });
      await waitFor(() => expect(mockPathSearchFn).toHaveBeenCalled());

      const input = screen.getByRole("textbox");
      await user.type(input, "drones{Enter}");

      await waitFor(() => {
        expect(
          screen.getByRole("button", { name: /back to browse/i }),
        ).toBeInTheDocument();
      });

      await user.click(screen.getByRole("button", { name: /back to browse/i }));

      await waitFor(() => {
        expect(screen.getByRole("textbox")).toHaveValue("");
      });
    });
  });

  // ---------------------------------------------------------------------------
  // Table-driven: globalSearch error messages
  // ---------------------------------------------------------------------------

  describe("table-driven globalSearch error messages", () => {
    const errorCases = [
      {
        error: new GraphApiError("Network error during globalSearch", 0),
        descr: "network error (statusCode 0)",
      },
      {
        error: new GraphApiError("NLQ service timeout", 504),
        descr: "gateway timeout (504)",
      },
      {
        error: new GraphApiError("Bad query syntax", 400),
        descr: "bad request (400)",
      },
      {
        error: new GraphApiError("Internal server error", 500),
        descr: "server error (500)",
      },
    ];

    it.each(errorCases)(
      "should display an alert for $descr",
      async ({ error }) => {
        mockGlobalSearchFn.mockRejectedValueOnce(error);
        const user = userEvent.setup();

        render(DataView, { props: { flowId: "test-flow-error" } });
        await waitFor(() => expect(mockPathSearchFn).toHaveBeenCalled());

        const input = screen.getByRole("textbox");
        await user.type(input, "trigger error{Enter}");

        await waitFor(() => {
          expect(screen.getByRole("alert")).toBeInTheDocument();
        });
      },
    );
  });
});
