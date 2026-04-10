import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import NlqSearchBar from "./NlqSearchBar.svelte";
import type { SearchMode } from "$lib/types/graph";

// ---------------------------------------------------------------------------
// Toggle visibility
// ---------------------------------------------------------------------------

describe("NlqSearchBar merge/replace toggle", () => {
  describe("toggle visibility", () => {
    it("should NOT render search mode toggle when searchMode prop is not provided", () => {
      render(NlqSearchBar, {
        props: {
          onSearch: vi.fn(),
          onClear: vi.fn(),
        },
      });

      expect(
        screen.queryByTestId("search-mode-toggle"),
      ).not.toBeInTheDocument();
    });

    it("should render search mode toggle with data-testid when searchMode is provided", () => {
      render(NlqSearchBar, {
        props: {
          onSearch: vi.fn(),
          onClear: vi.fn(),
          searchMode: "replace" as SearchMode,
          onSearchModeChange: vi.fn(),
        },
      });

      expect(screen.getByTestId("search-mode-toggle")).toBeInTheDocument();
    });

    it("should NOT render search mode toggle when searchMode is undefined", () => {
      render(NlqSearchBar, {
        props: {
          onSearch: vi.fn(),
          onClear: vi.fn(),
          searchMode: undefined,
          onSearchModeChange: vi.fn(),
        },
      });

      expect(
        screen.queryByTestId("search-mode-toggle"),
      ).not.toBeInTheDocument();
    });
  });

  // -------------------------------------------------------------------------
  // Label display
  // -------------------------------------------------------------------------

  describe("toggle label display", () => {
    it("should show 'Replace' label when searchMode is 'replace'", () => {
      render(NlqSearchBar, {
        props: {
          onSearch: vi.fn(),
          onClear: vi.fn(),
          searchMode: "replace" as SearchMode,
          onSearchModeChange: vi.fn(),
        },
      });

      // The toggle area (label or surrounding element) should contain "Replace"
      const toggle = screen.getByTestId("search-mode-toggle");
      expect(toggle).toHaveTextContent(/replace/i);
    });

    it("should show 'Merge' label when searchMode is 'merge'", () => {
      render(NlqSearchBar, {
        props: {
          onSearch: vi.fn(),
          onClear: vi.fn(),
          searchMode: "merge" as SearchMode,
          onSearchModeChange: vi.fn(),
        },
      });

      const toggle = screen.getByTestId("search-mode-toggle");
      expect(toggle).toHaveTextContent(/merge/i);
    });

    it("should update displayed label when searchMode prop changes from replace to merge", async () => {
      const { rerender } = render(NlqSearchBar, {
        props: {
          onSearch: vi.fn(),
          onClear: vi.fn(),
          searchMode: "replace" as SearchMode,
          onSearchModeChange: vi.fn(),
        },
      });

      expect(screen.getByTestId("search-mode-toggle")).toHaveTextContent(
        /replace/i,
      );

      await rerender({
        onSearch: vi.fn(),
        onClear: vi.fn(),
        searchMode: "merge" as SearchMode,
        onSearchModeChange: vi.fn(),
      });

      expect(screen.getByTestId("search-mode-toggle")).toHaveTextContent(
        /merge/i,
      );
    });
  });

  // -------------------------------------------------------------------------
  // Callback behavior
  // -------------------------------------------------------------------------

  describe("onSearchModeChange callback", () => {
    it("should call onSearchModeChange with 'merge' when toggled from replace mode", async () => {
      const onSearchModeChange = vi.fn();
      const user = userEvent.setup();

      render(NlqSearchBar, {
        props: {
          onSearch: vi.fn(),
          onClear: vi.fn(),
          searchMode: "replace" as SearchMode,
          onSearchModeChange,
        },
      });

      const toggle = screen.getByTestId("search-mode-toggle");
      await user.click(toggle);

      expect(onSearchModeChange).toHaveBeenCalledOnce();
      expect(onSearchModeChange).toHaveBeenCalledWith("merge");
    });

    it("should call onSearchModeChange with 'replace' when toggled from merge mode", async () => {
      const onSearchModeChange = vi.fn();
      const user = userEvent.setup();

      render(NlqSearchBar, {
        props: {
          onSearch: vi.fn(),
          onClear: vi.fn(),
          searchMode: "merge" as SearchMode,
          onSearchModeChange,
        },
      });

      const toggle = screen.getByTestId("search-mode-toggle");
      await user.click(toggle);

      expect(onSearchModeChange).toHaveBeenCalledOnce();
      expect(onSearchModeChange).toHaveBeenCalledWith("replace");
    });

    it("should NOT call onSearchModeChange when search is submitted", async () => {
      const onSearchModeChange = vi.fn();
      const onSearch = vi.fn();
      const user = userEvent.setup();

      render(NlqSearchBar, {
        props: {
          onSearch,
          onClear: vi.fn(),
          searchMode: "merge" as SearchMode,
          onSearchModeChange,
        },
      });

      const input = screen.getByRole("textbox");
      await user.type(input, "find drones{Enter}");

      expect(onSearch).toHaveBeenCalledOnce();
      expect(onSearchModeChange).not.toHaveBeenCalled();
    });
  });

  // -------------------------------------------------------------------------
  // Search functionality unaffected by mode
  // -------------------------------------------------------------------------

  describe("search functionality in each mode", () => {
    it("should call onSearch when Enter is pressed in merge mode", async () => {
      const onSearch = vi.fn();
      const user = userEvent.setup();

      render(NlqSearchBar, {
        props: {
          onSearch,
          onClear: vi.fn(),
          searchMode: "merge" as SearchMode,
          onSearchModeChange: vi.fn(),
        },
      });

      const input = screen.getByRole("textbox");
      await user.type(input, "find west coast fleet{Enter}");

      expect(onSearch).toHaveBeenCalledOnce();
      expect(onSearch).toHaveBeenCalledWith("find west coast fleet");
    });

    it("should call onSearch when search button is clicked in merge mode", async () => {
      const onSearch = vi.fn();
      const user = userEvent.setup();

      render(NlqSearchBar, {
        props: {
          onSearch,
          onClear: vi.fn(),
          searchMode: "merge" as SearchMode,
          onSearchModeChange: vi.fn(),
        },
      });

      await user.type(screen.getByRole("textbox"), "fleet alpha drones");
      await user.click(screen.getByRole("button", { name: /search/i }));

      expect(onSearch).toHaveBeenCalledOnce();
      expect(onSearch).toHaveBeenCalledWith("fleet alpha drones");
    });

    it("should call onSearch when Enter is pressed in replace mode", async () => {
      const onSearch = vi.fn();
      const user = userEvent.setup();

      render(NlqSearchBar, {
        props: {
          onSearch,
          onClear: vi.fn(),
          searchMode: "replace" as SearchMode,
          onSearchModeChange: vi.fn(),
        },
      });

      const input = screen.getByRole("textbox");
      await user.type(input, "show all sensors{Enter}");

      expect(onSearch).toHaveBeenCalledOnce();
      expect(onSearch).toHaveBeenCalledWith("show all sensors");
    });

    it("should NOT call onSearch when input is empty in merge mode", async () => {
      const onSearch = vi.fn();
      const user = userEvent.setup();

      render(NlqSearchBar, {
        props: {
          onSearch,
          onClear: vi.fn(),
          searchMode: "merge" as SearchMode,
          onSearchModeChange: vi.fn(),
        },
      });

      const input = screen.getByRole("textbox");
      await user.type(input, "{Enter}");

      expect(onSearch).not.toHaveBeenCalled();
    });
  });

  // -------------------------------------------------------------------------
  // Mode persistence — toggle state does not change on search submit
  // -------------------------------------------------------------------------

  describe("mode persistence across searches", () => {
    it("should not change displayed mode label when search is submitted in merge mode", async () => {
      const onSearch = vi.fn();
      const user = userEvent.setup();

      render(NlqSearchBar, {
        props: {
          onSearch,
          onClear: vi.fn(),
          searchMode: "merge" as SearchMode,
          onSearchModeChange: vi.fn(),
        },
      });

      // Verify merge label before search
      expect(screen.getByTestId("search-mode-toggle")).toHaveTextContent(
        /merge/i,
      );

      await user.type(screen.getByRole("textbox"), "query one{Enter}");

      // Label remains merge after submission
      expect(screen.getByTestId("search-mode-toggle")).toHaveTextContent(
        /merge/i,
      );
    });

    it("should not change displayed mode label when search is submitted in replace mode", async () => {
      const onSearch = vi.fn();
      const user = userEvent.setup();

      render(NlqSearchBar, {
        props: {
          onSearch,
          onClear: vi.fn(),
          searchMode: "replace" as SearchMode,
          onSearchModeChange: vi.fn(),
        },
      });

      expect(screen.getByTestId("search-mode-toggle")).toHaveTextContent(
        /replace/i,
      );

      await user.type(screen.getByRole("textbox"), "another query{Enter}");

      expect(screen.getByTestId("search-mode-toggle")).toHaveTextContent(
        /replace/i,
      );
    });
  });

  // -------------------------------------------------------------------------
  // Disabled state during loading
  // -------------------------------------------------------------------------

  describe("toggle disabled state", () => {
    it("should disable the search mode toggle during loading", () => {
      render(NlqSearchBar, {
        props: {
          onSearch: vi.fn(),
          onClear: vi.fn(),
          loading: true,
          searchMode: "replace" as SearchMode,
          onSearchModeChange: vi.fn(),
        },
      });

      const toggle = screen.getByTestId("search-mode-toggle");
      // The toggle element (checkbox, button, or input) should be disabled
      expect(toggle).toBeDisabled();
    });

    it("should NOT disable the search mode toggle when not loading", () => {
      render(NlqSearchBar, {
        props: {
          onSearch: vi.fn(),
          onClear: vi.fn(),
          loading: false,
          searchMode: "replace" as SearchMode,
          onSearchModeChange: vi.fn(),
        },
      });

      const toggle = screen.getByTestId("search-mode-toggle");
      expect(toggle).not.toBeDisabled();
    });

    it("should NOT call onSearchModeChange when toggle is clicked during loading", async () => {
      const onSearchModeChange = vi.fn();
      const user = userEvent.setup();

      render(NlqSearchBar, {
        props: {
          onSearch: vi.fn(),
          onClear: vi.fn(),
          loading: true,
          searchMode: "replace" as SearchMode,
          onSearchModeChange,
        },
      });

      const toggle = screen.getByTestId("search-mode-toggle");
      // Clicking a disabled element should not fire the handler
      await user.click(toggle);

      expect(onSearchModeChange).not.toHaveBeenCalled();
    });
  });

  // -------------------------------------------------------------------------
  // Table-driven: mode prop combinations
  // -------------------------------------------------------------------------

  describe("table-driven: toggle state across prop combinations", () => {
    const modeCases: Array<{
      description: string;
      searchMode: SearchMode;
      loading: boolean;
      expectedLabel: RegExp;
      expectDisabled: boolean;
    }> = [
      {
        description: "replace mode, not loading",
        searchMode: "replace",
        loading: false,
        expectedLabel: /replace/i,
        expectDisabled: false,
      },
      {
        description: "merge mode, not loading",
        searchMode: "merge",
        loading: false,
        expectedLabel: /merge/i,
        expectDisabled: false,
      },
      {
        description: "replace mode, loading",
        searchMode: "replace",
        loading: true,
        expectedLabel: /replace/i,
        expectDisabled: true,
      },
      {
        description: "merge mode, loading",
        searchMode: "merge",
        loading: true,
        expectedLabel: /merge/i,
        expectDisabled: true,
      },
    ];

    it.each(modeCases)(
      "$description",
      ({ searchMode, loading, expectedLabel, expectDisabled }) => {
        render(NlqSearchBar, {
          props: {
            onSearch: vi.fn(),
            onClear: vi.fn(),
            searchMode,
            loading,
            onSearchModeChange: vi.fn(),
          },
        });

        const toggle = screen.getByTestId("search-mode-toggle");
        expect(toggle).toHaveTextContent(expectedLabel);

        if (expectDisabled) {
          expect(toggle).toBeDisabled();
        } else {
          expect(toggle).not.toBeDisabled();
        }
      },
    );
  });

  // -------------------------------------------------------------------------
  // Co-existence with existing props (inSearchMode + toggle)
  // -------------------------------------------------------------------------

  describe("co-existence with inSearchMode", () => {
    it("should show both Back to browse button and toggle when inSearchMode is true and searchMode is provided", () => {
      render(NlqSearchBar, {
        props: {
          onSearch: vi.fn(),
          onClear: vi.fn(),
          inSearchMode: true,
          searchMode: "merge" as SearchMode,
          onSearchModeChange: vi.fn(),
        },
      });

      expect(
        screen.getByRole("button", { name: /back to browse/i }),
      ).toBeInTheDocument();
      expect(screen.getByTestId("search-mode-toggle")).toBeInTheDocument();
    });

    it("should show toggle but NOT Back to browse when inSearchMode is false and searchMode is provided", () => {
      render(NlqSearchBar, {
        props: {
          onSearch: vi.fn(),
          onClear: vi.fn(),
          inSearchMode: false,
          searchMode: "replace" as SearchMode,
          onSearchModeChange: vi.fn(),
        },
      });

      expect(
        screen.queryByRole("button", { name: /back to browse/i }),
      ).not.toBeInTheDocument();
      expect(screen.getByTestId("search-mode-toggle")).toBeInTheDocument();
    });
  });
});
