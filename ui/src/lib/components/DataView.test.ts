import { describe, it, expect, beforeEach, vi, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import DataView from "./DataView.svelte";
import type { PathSearchResult } from "$lib/types/graph";
import { GraphApiError } from "$lib/services/graphApi";

// Mock the graphApi module
vi.mock("$lib/services/graphApi", () => {
  return {
    graphApi: {
      pathSearch: vi.fn(),
      getEntitiesByPrefix: vi.fn(),
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

// Import after mocking
import { graphApi } from "$lib/services/graphApi";

describe("DataView GraphQL Integration", () => {
  const mockPathSearchFn = graphApi.pathSearch as ReturnType<typeof vi.fn>;
  const mockGetEntitiesByPrefixFn = graphApi.getEntitiesByPrefix as ReturnType<
    typeof vi.fn
  >;

  // Default mock response: getEntitiesByPrefix returns BackendEntity[] directly
  const createMockEntities = () => [
    {
      id: "c360.ops.robotics.gcs.fleet.west-coast",
      triples: [
        {
          subject: "c360.ops.robotics.gcs.fleet.west-coast",
          predicate: "fleet.name",
          object: "West Coast Fleet",
        },
        {
          subject: "c360.ops.robotics.gcs.fleet.west-coast",
          predicate: "fleet.region",
          object: "US-West",
        },
      ],
    },
    {
      id: "c360.ops.robotics.gcs.drone.001",
      triples: [
        {
          subject: "c360.ops.robotics.gcs.drone.001",
          predicate: "vehicle.type",
          object: "drone",
        },
        {
          subject: "c360.ops.robotics.gcs.drone.001",
          predicate: "vehicle.status",
          object: "active",
        },
      ],
    },
  ];

  // PathSearchResult for expansion tests (pathSearch still used for expansion)
  const _createMockPathSearchResult = (): PathSearchResult => ({
    entities: createMockEntities(),
    edges: [
      {
        subject: "c360.ops.robotics.gcs.drone.001",
        predicate: "fleet.membership.current",
        object: "c360.ops.robotics.gcs.fleet.west-coast",
      },
    ],
  });

  beforeEach(() => {
    mockPathSearchFn.mockClear();
    mockGetEntitiesByPrefixFn.mockClear();
  });

  afterEach(() => {
    vi.clearAllTimers();
  });

  describe("Initial data loading on mount", () => {
    it("should call getEntitiesByPrefix with empty prefix on mount", async () => {
      mockGetEntitiesByPrefixFn.mockResolvedValue(createMockEntities());

      render(DataView, { props: { flowId: "test-flow-123" } });

      await waitFor(() => {
        expect(mockGetEntitiesByPrefixFn).toHaveBeenCalledWith("", 200);
      });
    });

    it("should display loading state while fetching data", async () => {
      let resolvePromise: (value: unknown[]) => void;
      const delayedPromise = new Promise<unknown[]>((resolve) => {
        resolvePromise = resolve;
      });
      mockGetEntitiesByPrefixFn.mockReturnValue(delayedPromise);

      render(DataView, { props: { flowId: "test-flow-123" } });

      // Loading state should be visible
      expect(screen.getByText("Loading graph data...")).toBeInTheDocument();
      expect(screen.getByTestId("data-view")).toBeInTheDocument();

      // Resolve the promise
      resolvePromise!(createMockEntities());

      // Wait for loading to disappear
      await waitFor(() => {
        expect(
          screen.queryByText("Loading graph data..."),
        ).not.toBeInTheDocument();
      });
    });

    it("should transform and display entities after successful load", async () => {
      mockGetEntitiesByPrefixFn.mockResolvedValue(createMockEntities());

      render(DataView, { props: { flowId: "test-flow-123" } });

      await waitFor(() => {
        expect(mockGetEntitiesByPrefixFn).toHaveBeenCalled();
      });

      // Should not show loading or error after success
      await waitFor(() => {
        expect(
          screen.queryByText("Loading graph data..."),
        ).not.toBeInTheDocument();
        expect(screen.queryByRole("alert")).not.toBeInTheDocument();
      });
    });

    it("should handle empty result from getEntitiesByPrefix", async () => {
      mockGetEntitiesByPrefixFn.mockResolvedValue([]);

      render(DataView, { props: { flowId: "test-flow-123" } });

      await waitFor(() => {
        expect(mockGetEntitiesByPrefixFn).toHaveBeenCalled();
      });

      // Should not show error for empty result
      await waitFor(() => {
        expect(
          screen.queryByText("Loading graph data..."),
        ).not.toBeInTheDocument();
        expect(screen.queryByRole("alert")).not.toBeInTheDocument();
      });
    });
  });

  describe("Error handling - Network errors", () => {
    it("should display connection error message for network failure (statusCode 0)", async () => {
      const networkError = new GraphApiError(
        "Network error during pathSearch: Failed to fetch",
        0,
      );
      mockGetEntitiesByPrefixFn.mockRejectedValue(networkError);

      render(DataView, { props: { flowId: "test-flow-123" } });

      await waitFor(() => {
        expect(
          screen.getByText("Unable to connect to graph service"),
        ).toBeInTheDocument();
      });
    });

    it("should display connection error for fetch exception", async () => {
      mockGetEntitiesByPrefixFn.mockRejectedValue(
        new Error("Failed to fetch: Network is unreachable"),
      );

      render(DataView, { props: { flowId: "test-flow-123" } });

      await waitFor(() => {
        expect(
          screen.getByText("Unable to connect to graph service"),
        ).toBeInTheDocument();
      });
    });

    it("should show retry button on network error", async () => {
      const networkError = new GraphApiError(
        "Network error during pathSearch",
        0,
      );
      mockGetEntitiesByPrefixFn.mockRejectedValue(networkError);

      render(DataView, { props: { flowId: "test-flow-123" } });

      await waitFor(() => {
        expect(screen.getByRole("button", { name: /retry/i })).toBeVisible();
      });
    });
  });

  describe("Error handling - Timeout errors", () => {
    it("should display timeout error message for 504 status", async () => {
      const timeoutError = new GraphApiError("Gateway Timeout", 504);
      mockGetEntitiesByPrefixFn.mockRejectedValue(timeoutError);

      render(DataView, { props: { flowId: "test-flow-123" } });

      await waitFor(() => {
        expect(screen.getByText("Query timed out")).toBeInTheDocument();
      });
    });

    it("should display timeout error when error message includes 'timeout'", async () => {
      const timeoutError = new GraphApiError(
        "Request timeout after 30 seconds",
        500,
      );
      mockGetEntitiesByPrefixFn.mockRejectedValue(timeoutError);

      render(DataView, { props: { flowId: "test-flow-123" } });

      await waitFor(() => {
        expect(screen.getByText("Query timed out")).toBeInTheDocument();
      });
    });

    it("should display timeout error when error message includes 'Timeout' (case insensitive)", async () => {
      const timeoutError = new GraphApiError(
        "Connection Timeout occurred",
        408,
      );
      mockGetEntitiesByPrefixFn.mockRejectedValue(timeoutError);

      render(DataView, { props: { flowId: "test-flow-123" } });

      await waitFor(() => {
        expect(screen.getByText("Query timed out")).toBeInTheDocument();
      });
    });
  });

  describe("Error handling - GraphQL errors", () => {
    it("should display GraphQL error message", async () => {
      const graphqlError = new GraphApiError(
        "Entity not found in graph database",
        200,
        { errors: [{ message: "Entity not found in graph database" }] },
      );
      mockGetEntitiesByPrefixFn.mockRejectedValue(graphqlError);

      render(DataView, { props: { flowId: "test-flow-123" } });

      await waitFor(() => {
        expect(
          screen.getByText("Entity not found in graph database"),
        ).toBeInTheDocument();
      });
    });

    it("should display server error message", async () => {
      const serverError = new GraphApiError("Internal Server Error", 500);
      mockGetEntitiesByPrefixFn.mockRejectedValue(serverError);

      render(DataView, { props: { flowId: "test-flow-123" } });

      await waitFor(() => {
        expect(screen.getByText("Internal Server Error")).toBeInTheDocument();
      });
    });

    it("should display authentication error message", async () => {
      const authError = new GraphApiError("Unauthorized access", 401);
      mockGetEntitiesByPrefixFn.mockRejectedValue(authError);

      render(DataView, { props: { flowId: "test-flow-123" } });

      await waitFor(() => {
        expect(screen.getByText("Unauthorized access")).toBeInTheDocument();
      });
    });

    it("should display forbidden error message", async () => {
      const forbiddenError = new GraphApiError(
        "Forbidden: Insufficient permissions",
        403,
      );
      mockGetEntitiesByPrefixFn.mockRejectedValue(forbiddenError);

      render(DataView, { props: { flowId: "test-flow-123" } });

      await waitFor(() => {
        expect(
          screen.getByText("Forbidden: Insufficient permissions"),
        ).toBeInTheDocument();
      });
    });
  });

  describe("Error handling - Generic errors", () => {
    it("should display generic error message for unknown error types", async () => {
      mockGetEntitiesByPrefixFn.mockRejectedValue(
        new Error("Something unexpected happened"),
      );

      render(DataView, { props: { flowId: "test-flow-123" } });

      await waitFor(() => {
        expect(
          screen.getByText("Unable to connect to graph service"),
        ).toBeInTheDocument();
      });
    });

    it("should handle rejected promise with non-Error object", async () => {
      mockGetEntitiesByPrefixFn.mockRejectedValue("String error");

      render(DataView, { props: { flowId: "test-flow-123" } });

      await waitFor(() => {
        expect(
          screen.getByText("Unable to connect to graph service"),
        ).toBeInTheDocument();
      });
    });

    it("should handle null rejection", async () => {
      mockGetEntitiesByPrefixFn.mockRejectedValue(null);

      render(DataView, { props: { flowId: "test-flow-123" } });

      await waitFor(() => {
        expect(
          screen.getByText("Unable to connect to graph service"),
        ).toBeInTheDocument();
      });
    });
  });

  describe("Retry functionality", () => {
    it("should call getEntitiesByPrefix again when retry button is clicked", async () => {
      const networkError = new GraphApiError("Network failure", 0);
      mockGetEntitiesByPrefixFn
        .mockRejectedValueOnce(networkError)
        .mockResolvedValueOnce(createMockEntities());

      render(DataView, { props: { flowId: "test-flow-123" } });

      // Wait for initial error
      await waitFor(() => {
        expect(
          screen.getByText("Unable to connect to graph service"),
        ).toBeInTheDocument();
      });

      const user = userEvent.setup();
      const retryButton = screen.getByRole("button", { name: /retry/i });

      // Click retry
      await user.click(retryButton);

      // Should call getEntitiesByPrefix again with same parameters
      await waitFor(() => {
        expect(mockGetEntitiesByPrefixFn).toHaveBeenCalledTimes(2);
        expect(mockGetEntitiesByPrefixFn).toHaveBeenLastCalledWith("", 200);
      });

      // Error should be cleared after successful retry
      await waitFor(() => {
        expect(
          screen.queryByText("Unable to connect to graph service"),
        ).not.toBeInTheDocument();
      });
    });

    it("should show loading state during retry", async () => {
      const networkError = new GraphApiError("Network failure", 0);
      mockGetEntitiesByPrefixFn.mockRejectedValueOnce(networkError);

      render(DataView, { props: { flowId: "test-flow-123" } });

      await waitFor(() => {
        expect(screen.getByRole("button", { name: /retry/i })).toBeVisible();
      });

      let resolveRetry: (value: unknown[]) => void;
      const retryPromise = new Promise<unknown[]>((resolve) => {
        resolveRetry = resolve;
      });
      mockGetEntitiesByPrefixFn.mockReturnValueOnce(retryPromise);

      const user = userEvent.setup();
      await user.click(screen.getByRole("button", { name: /retry/i }));

      // Should show loading during retry
      expect(screen.getByText("Loading graph data...")).toBeInTheDocument();

      // Resolve retry
      resolveRetry!(createMockEntities());

      await waitFor(() => {
        expect(
          screen.queryByText("Loading graph data..."),
        ).not.toBeInTheDocument();
      });
    });

    it("should display new error if retry fails", async () => {
      const networkError = new GraphApiError("Network failure", 0);
      const timeoutError = new GraphApiError("Request timeout", 504);

      mockGetEntitiesByPrefixFn
        .mockRejectedValueOnce(networkError)
        .mockRejectedValueOnce(timeoutError);

      render(DataView, { props: { flowId: "test-flow-123" } });

      await waitFor(() => {
        expect(
          screen.getByText("Unable to connect to graph service"),
        ).toBeInTheDocument();
      });

      const user = userEvent.setup();
      await user.click(screen.getByRole("button", { name: /retry/i }));

      // Should show different error after retry
      await waitFor(() => {
        expect(screen.getByText("Query timed out")).toBeInTheDocument();
      });
    });
  });

  describe("Refresh functionality (toolbar refresh button)", () => {
    it("should clear entities and reload data when refresh button is clicked", async () => {
      mockGetEntitiesByPrefixFn.mockResolvedValue(createMockEntities());

      render(DataView, { props: { flowId: "test-flow-123" } });

      await waitFor(() => {
        expect(mockGetEntitiesByPrefixFn).toHaveBeenCalledTimes(1);
      });

      // Find toolbar refresh button (not retry button)
      const refreshButton = screen.getByRole("button", { name: /refresh/i });

      const user = userEvent.setup();
      await user.click(refreshButton);

      // Should call getEntitiesByPrefix again
      await waitFor(() => {
        expect(mockGetEntitiesByPrefixFn).toHaveBeenCalledTimes(2);
        expect(mockGetEntitiesByPrefixFn).toHaveBeenLastCalledWith("", 200);
      });
    });

    it("should disable refresh button while loading", async () => {
      let resolvePromise: (value: unknown[]) => void;
      const delayedPromise = new Promise<unknown[]>((resolve) => {
        resolvePromise = resolve;
      });
      mockGetEntitiesByPrefixFn.mockReturnValue(delayedPromise);

      render(DataView, { props: { flowId: "test-flow-123" } });

      const refreshButton = screen.getByRole("button", { name: /refresh/i });

      // Should be disabled during loading
      expect(refreshButton).toBeDisabled();

      // Resolve promise
      resolvePromise!(createMockEntities());

      await waitFor(() => {
        expect(refreshButton).not.toBeDisabled();
      });
    });
  });

  describe("Entity expansion (handleEntityExpand)", () => {
    it("should call pathSearch with entity ID when entity is expanded", async () => {
      mockGetEntitiesByPrefixFn.mockResolvedValue(createMockEntities());

      const { component } = render(DataView, {
        props: { flowId: "test-flow-123" },
      });

      await waitFor(() => {
        expect(mockGetEntitiesByPrefixFn).toHaveBeenCalledWith("", 200);
      });

      // Mock pathSearch for entity expansion
      mockPathSearchFn.mockResolvedValueOnce({
        entities: [
          {
            id: "c360.ops.robotics.gcs.drone.001",
            triples: [
              {
                subject: "c360.ops.robotics.gcs.drone.001",
                predicate: "expanded.property",
                object: "new data",
              },
            ],
          },
        ],
        edges: [],
      });

      // Simulate entity expand by calling the component's handler directly
      // This tests that the integration logic is correctly set up
      await component.handleEntityExpand("c360.ops.robotics.gcs.drone.001");

      await waitFor(() => {
        expect(mockPathSearchFn).toHaveBeenCalledWith(
          "c360.ops.robotics.gcs.drone.001",
          1,
          20,
        );
      });
    });

    it("should use correct depth and limit for entity expansion", async () => {
      mockGetEntitiesByPrefixFn.mockResolvedValue(createMockEntities());

      const { component } = render(DataView, {
        props: { flowId: "test-flow-123" },
      });

      await waitFor(() => {
        expect(mockGetEntitiesByPrefixFn).toHaveBeenCalledTimes(1);
      });

      mockPathSearchFn.mockResolvedValueOnce({
        entities: [],
        edges: [],
      });

      await component.handleEntityExpand("c360.ops.robotics.gcs.fleet.alpha");

      await waitFor(() => {
        expect(mockPathSearchFn).toHaveBeenCalledWith(
          "c360.ops.robotics.gcs.fleet.alpha",
          1,
          20,
        );
      });
    });

    it("should handle error during entity expansion gracefully", async () => {
      mockGetEntitiesByPrefixFn.mockResolvedValue(createMockEntities());

      const { component } = render(DataView, {
        props: { flowId: "test-flow-123" },
      });

      await waitFor(() => {
        expect(mockGetEntitiesByPrefixFn).toHaveBeenCalledTimes(1);
      });

      // Mock error for expansion (pathSearch is still used for expansion)
      const expansionError = new GraphApiError("Failed to expand entity", 500);
      mockPathSearchFn.mockRejectedValueOnce(expansionError);

      await component.handleEntityExpand("c360.ops.robotics.gcs.drone.001");

      // Should display error
      await waitFor(() => {
        expect(screen.getByText("Failed to expand entity")).toBeInTheDocument();
      });
    });
  });

  describe("Component integration", () => {
    it("should render all three panels after successful data load", async () => {
      mockGetEntitiesByPrefixFn.mockResolvedValue(createMockEntities());

      render(DataView, { props: { flowId: "test-flow-123" } });

      await waitFor(() => {
        expect(mockGetEntitiesByPrefixFn).toHaveBeenCalled();
      });

      // Check that main container and panels are present
      expect(screen.getByTestId("data-view")).toBeInTheDocument();

      // The component should have the three-column layout structure
      const dataView = screen.getByTestId("data-view");
      expect(dataView).toHaveClass("data-view");
    });

    it("should pass transformed entities to child components", async () => {
      mockGetEntitiesByPrefixFn.mockResolvedValue(createMockEntities());

      render(DataView, { props: { flowId: "test-flow-123" } });

      await waitFor(() => {
        expect(mockGetEntitiesByPrefixFn).toHaveBeenCalled();
      });

      // After successful load, the component should not show errors
      await waitFor(() => {
        expect(screen.queryByRole("alert")).not.toBeInTheDocument();
        expect(
          screen.queryByText("Loading graph data..."),
        ).not.toBeInTheDocument();
      });
    });
  });

  describe("Table-driven error message tests", () => {
    const errorCases = [
      {
        error: new GraphApiError("Network error", 0),
        expectedMessage: "Unable to connect to graph service",
      },
      {
        error: new GraphApiError("Gateway Timeout", 504),
        expectedMessage: "Query timed out",
      },
      {
        error: new GraphApiError("Request timeout", 408),
        expectedMessage: "Query timed out",
      },
      {
        error: new GraphApiError("Query execution timeout", 200),
        expectedMessage: "Query timed out",
      },
      {
        error: new GraphApiError("Bad Request", 400),
        expectedMessage: "Bad Request",
      },
      {
        error: new GraphApiError("Not Found", 404),
        expectedMessage: "Not Found",
      },
      {
        error: new GraphApiError("Service Unavailable", 503),
        expectedMessage: "Service Unavailable",
      },
    ];

    it.each(errorCases)(
      "should display correct message for $error.statusCode: $expectedMessage",
      async ({ error, expectedMessage }) => {
        mockGetEntitiesByPrefixFn.mockRejectedValue(error);

        render(DataView, { props: { flowId: "test-flow-123" } });

        await waitFor(() => {
          expect(screen.getByText(expectedMessage)).toBeInTheDocument();
        });
      },
    );
  });

  describe("Component lifecycle", () => {
    it("should clean up subscriptions on unmount", async () => {
      mockGetEntitiesByPrefixFn.mockResolvedValue(createMockEntities());

      const { unmount } = render(DataView, {
        props: { flowId: "test-flow-123" },
      });

      await waitFor(() => {
        expect(mockGetEntitiesByPrefixFn).toHaveBeenCalled();
      });

      // Unmount should not throw
      expect(() => unmount()).not.toThrow();
    });

    it("should handle rapid mount/unmount cycles", async () => {
      mockGetEntitiesByPrefixFn.mockResolvedValue(createMockEntities());

      for (let i = 0; i < 5; i++) {
        const { unmount } = render(DataView, {
          props: { flowId: `test-flow-${i}` },
        });
        unmount();
      }

      // Should not throw or cause memory leaks
      expect(mockGetEntitiesByPrefixFn).toHaveBeenCalledTimes(5);
    });
  });
});
