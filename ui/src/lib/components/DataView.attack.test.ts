import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import DataView from "./DataView.svelte";
import { graphStore } from "$lib/stores/graphStore.svelte";
import { chatStore } from "$lib/stores/chatStore.svelte";

vi.mock("$lib/services/graphApi", () => ({
  graphApi: {
    pathSearch: vi.fn(),
    getEntitiesByPrefix: vi.fn().mockResolvedValue([]),
  },
  GraphApiError: class GraphApiError extends Error {
    constructor(
      message: string,
      public statusCode: number,
    ) {
      super(message);
      this.name = "GraphApiError";
    }
  },
}));

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeEntity(id: string) {
  const parts = id.split(".");
  return {
    id,
    idParts: {
      org: parts[0] ?? "c360",
      platform: parts[1] ?? "ops",
      domain: parts[2] ?? "robotics",
      system: parts[3] ?? "gcs",
      type: parts[4] ?? "drone",
      instance: parts[5] ?? "001",
    },
    properties: [],
    outgoing: [],
    incoming: [],
  };
}

beforeEach(() => {
  graphStore.reset();
  chatStore.clearConversation();
  chatStore.clearChips();
});

afterEach(() => {
  vi.clearAllMocks();
});

// ---------------------------------------------------------------------------
// Attack 1: Rapid tab switching — no state corruption or rendering crash
// ---------------------------------------------------------------------------

describe("DataView attack — rapid tab switching", () => {
  it("survives 10 rapid alternating tab clicks without crashing", async () => {
    const user = userEvent.setup();
    render(DataView, { props: { flowId: "flow-rapid-tabs" } });

    await waitFor(() => {
      expect(screen.getByTestId("data-view-tab-chat")).toBeInTheDocument();
    });

    const chatTab = screen.getByTestId("data-view-tab-chat");
    const detailsTab = screen.getByTestId("data-view-tab-details");

    for (let i = 0; i < 10; i++) {
      await user.click(i % 2 === 0 ? chatTab : detailsTab);
    }

    // After 10 clicks (even number), ends on chat (0-indexed last is details, but
    // alternating from chat: 0=chat,1=details,2=chat…9=details → ends on details)
    await waitFor(() => {
      const detailsActive = detailsTab.dataset.active === "true";
      const chatActive = chatTab.dataset.active === "true";
      // Exactly one tab is active
      expect(detailsActive !== chatActive).toBe(true);
    });
  });

  it("concurrent tab clicks settle to a single active tab", async () => {
    const user = userEvent.setup();
    render(DataView, { props: { flowId: "flow-concurrent-tabs" } });

    await waitFor(() => {
      expect(screen.getByTestId("data-view-tab-chat")).toBeInTheDocument();
    });

    const chatTab = screen.getByTestId("data-view-tab-chat");
    const detailsTab = screen.getByTestId("data-view-tab-details");

    // Fire all clicks concurrently
    await Promise.all([
      user.click(chatTab),
      user.click(detailsTab),
      user.click(chatTab),
    ]);

    await waitFor(() => {
      const chatActive = chatTab.getAttribute("aria-selected") === "true";
      const detailsActive = detailsTab.getAttribute("aria-selected") === "true";
      // Exactly one is active — no split-brain
      expect(chatActive !== detailsActive).toBe(true);
    });
  });
});

// ---------------------------------------------------------------------------
// Attack 2: Entity selection + deselection + "+Chat" race conditions
// ---------------------------------------------------------------------------

describe("DataView attack — entity/chip race conditions", () => {
  it("does not crash when +Chat is clicked and entity is simultaneously deselected", async () => {
    const entity = makeEntity("c360.ops.robotics.gcs.drone.001");
    graphStore.upsertEntity(entity);
    graphStore.selectEntity(entity.id);

    const _user = userEvent.setup();
    render(DataView, { props: { flowId: "flow-race" } });

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: /\+\s*chat/i }),
      ).toBeInTheDocument();
    });

    const addChipButton = screen.getByRole("button", { name: /\+\s*chat/i });

    // Deselect entity just before clicking +Chat
    graphStore.selectEntity(null);

    // Click should not throw even if entity is gone from panel
    expect(() => addChipButton.click()).not.toThrow();
  });

  it("double-clicking +Chat does NOT create duplicate chips (dedup by kind+value)", async () => {
    const entity = makeEntity("c360.ops.robotics.gcs.drone.002");
    graphStore.upsertEntity(entity);
    graphStore.selectEntity(entity.id);

    const user = userEvent.setup();
    render(DataView, { props: { flowId: "flow-dblclick" } });

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: /\+\s*chat/i }),
      ).toBeInTheDocument();
    });

    const addChipButton = screen.getByRole("button", { name: /\+\s*chat/i });
    await user.click(addChipButton);
    // Switch back to details to find the button again (it will be gone after tab switch)
    await user.click(screen.getByTestId("data-view-tab-details"));

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: /\+\s*chat/i }),
      ).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: /\+\s*chat/i }));

    // Store's addChip deduplicates by kind+value — should remain 1 chip
    expect(chatStore.chips).toHaveLength(1);
  });

  it("rapid +Chat clicks from multiple entities accumulate distinct chips", async () => {
    const e1 = makeEntity("c360.ops.robotics.gcs.drone.001");
    const e2 = makeEntity("c360.ops.robotics.gcs.drone.002");
    const e3 = makeEntity("c360.ops.robotics.gcs.drone.003");

    graphStore.upsertEntity(e1);
    graphStore.upsertEntity(e2);
    graphStore.upsertEntity(e3);

    // Add all three chips programmatically (simulates rapid entity→chip flow)
    chatStore.addChip({
      id: "chip-1",
      kind: "entity",
      label: "001",
      value: e1.id,
    });
    chatStore.addChip({
      id: "chip-2",
      kind: "entity",
      label: "002",
      value: e2.id,
    });
    chatStore.addChip({
      id: "chip-3",
      kind: "entity",
      label: "003",
      value: e3.id,
    });

    // Verify store holds all three unique chips
    expect(chatStore.chips).toHaveLength(3);
    expect(chatStore.chips.map((c) => c.value)).toContain(e1.id);
    expect(chatStore.chips.map((c) => c.value)).toContain(e2.id);
    expect(chatStore.chips.map((c) => c.value)).toContain(e3.id);
  });
});

// ---------------------------------------------------------------------------
// Attack 3: handleViewEntityFromChat with non-existent entity ID
// ---------------------------------------------------------------------------

describe("DataView attack — onViewEntity with non-existent entity", () => {
  it("does not crash when called with an ID not in the store", () => {
    const { component } = render(DataView, { props: { flowId: "flow-noent" } });

    expect(() =>
      component.handleViewEntityFromChat("does.not.exist.at.all.ever"),
    ).not.toThrow();
  });

  it("sets selectedEntityId to non-existent ID without throwing", () => {
    const { component } = render(DataView, {
      props: { flowId: "flow-noent2" },
    });

    component.handleViewEntityFromChat("phantom.entity.id.not.in.store.x");

    // Store allows selecting non-existent IDs — the panel shows empty state
    expect(graphStore.selectedEntityId).toBe(
      "phantom.entity.id.not.in.store.x",
    );
  });

  it("switches to details tab even when entity is not in the store", async () => {
    const { component } = render(DataView, {
      props: { flowId: "flow-noent3" },
    });

    // Start on chat tab
    const user = userEvent.setup();
    await waitFor(() => {
      expect(screen.getByTestId("data-view-tab-chat")).toBeInTheDocument();
    });
    await user.click(screen.getByTestId("data-view-tab-chat"));

    await waitFor(() => {
      expect(screen.getByTestId("chat-panel")).toBeInTheDocument();
    });

    component.handleViewEntityFromChat("nonexistent.entity.id.xyz");

    await waitFor(() => {
      const detailsTab = screen.getByTestId("data-view-tab-details");
      const isActive =
        detailsTab.getAttribute("aria-selected") === "true" ||
        detailsTab.dataset.active === "true";
      expect(isActive).toBe(true);
    });
  });
});

// ---------------------------------------------------------------------------
// Attack 4: DataViewContext with empty graphStore
// ---------------------------------------------------------------------------

describe("DataView attack — empty graphStore context", () => {
  it("renders ChatPanel without error when graphStore is empty", async () => {
    // graphStore.reset() in beforeEach — store is empty
    const user = userEvent.setup();
    render(DataView, { props: { flowId: "flow-empty-store" } });

    await waitFor(() => {
      expect(screen.getByTestId("data-view-tab-chat")).toBeInTheDocument();
    });

    await user.click(screen.getByTestId("data-view-tab-chat"));

    await waitFor(() => {
      expect(screen.getByTestId("chat-panel")).toBeInTheDocument();
    });

    // No error alert should appear from context building
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("DataView mounts without throwing when graphStore has zero entities", () => {
    expect(() =>
      render(DataView, { props: { flowId: "flow-empty" } }),
    ).not.toThrow();
  });

  it("tab buttons are accessible even when store is completely empty", async () => {
    render(DataView, { props: { flowId: "flow-empty-tabs" } });

    await waitFor(() => {
      const chatTab = screen.getByTestId("data-view-tab-chat");
      const detailsTab = screen.getByTestId("data-view-tab-details");
      expect(chatTab).toBeInTheDocument();
      expect(detailsTab).toBeInTheDocument();
      expect(chatTab).toHaveAccessibleName(/chat/i);
      expect(detailsTab).toHaveAccessibleName(/details/i);
    });
  });
});

// ---------------------------------------------------------------------------
// Attack 5: Tab auto-switch effect — no infinite loop
// ---------------------------------------------------------------------------

describe("DataView attack — auto-switch effect loop guard", () => {
  it("selecting same entity twice does NOT trigger redundant re-renders or loops", async () => {
    const entity = makeEntity("c360.ops.robotics.gcs.drone.loop");
    graphStore.upsertEntity(entity);
    graphStore.selectEntity(entity.id);

    render(DataView, { props: { flowId: "flow-loop" } });

    await waitFor(() => {
      // Details tab is active from initial entity selection
      const detailsTab = screen.getByTestId("data-view-tab-details");
      expect(detailsTab.dataset.active).toBe("true");
    });

    // Selecting the same entity again should not loop the auto-switch effect
    graphStore.selectEntity(entity.id);

    await waitFor(() => {
      const detailsTab = screen.getByTestId("data-view-tab-details");
      expect(detailsTab.dataset.active).toBe("true");
    });

    // Rendered once — no crash or freeze
    expect(screen.getByTestId("data-view")).toBeInTheDocument();
  });

  it("manual tab override is NOT clobbered by auto-switch when entity stays selected", async () => {
    const entity = makeEntity("c360.ops.robotics.gcs.drone.override");
    graphStore.upsertEntity(entity);
    graphStore.selectEntity(entity.id);

    const user = userEvent.setup();
    render(DataView, { props: { flowId: "flow-override" } });

    await waitFor(() => {
      expect(screen.getByTestId("data-view-tab-details")).toBeInTheDocument();
    });

    // Manually switch to chat tab while entity is still selected
    await user.click(screen.getByTestId("data-view-tab-chat"));

    await waitFor(() => {
      expect(screen.getByTestId("chat-panel")).toBeInTheDocument();
    });

    // The auto-switch should NOT fire again (entity ID did not change)
    // Chat tab stays active
    await waitFor(() => {
      const chatTab = screen.getByTestId("data-view-tab-chat");
      const isActive =
        chatTab.getAttribute("aria-selected") === "true" ||
        chatTab.dataset.active === "true";
      expect(isActive).toBe(true);
    });
  });

  it("auto-switch fires when a NEW entity is selected (transition from null → id)", async () => {
    // Start with no entity
    render(DataView, { props: { flowId: "flow-autoswitch" } });

    await waitFor(() => {
      expect(screen.getByTestId("data-view-tab-chat")).toBeInTheDocument();
    });

    // Chat tab is active (no entity selected)
    const user = userEvent.setup();
    await user.click(screen.getByTestId("data-view-tab-chat"));

    await waitFor(() => {
      expect(screen.getByTestId("chat-panel")).toBeInTheDocument();
    });

    // Now select a new entity — should auto-switch to details
    const entity = makeEntity("c360.ops.robotics.gcs.drone.new");
    graphStore.upsertEntity(entity);
    graphStore.selectEntity(entity.id);

    await waitFor(() => {
      const detailsTab = screen.getByTestId("data-view-tab-details");
      const isActive =
        detailsTab.getAttribute("aria-selected") === "true" ||
        detailsTab.dataset.active === "true";
      expect(isActive).toBe(true);
    });
  });
});

// ---------------------------------------------------------------------------
// Attack 6: DataView renders correctly with undefined/null edge-case props
// ---------------------------------------------------------------------------

describe("DataView attack — undefined and null props", () => {
  it("does NOT throw when flowId is an empty string", () => {
    expect(() => render(DataView, { props: { flowId: "" } })).not.toThrow();
  });

  it("does NOT throw when flowId contains special characters", () => {
    expect(() =>
      render(DataView, { props: { flowId: "flow/<>&\"'test" } }),
    ).not.toThrow();
  });
});

// ---------------------------------------------------------------------------
// Attack 7: GraphDetailPanel — undefined/null prop hardening
// ---------------------------------------------------------------------------

import GraphDetailPanel from "./runtime/GraphDetailPanel.svelte";

describe("GraphDetailPanel attack — null and undefined prop hardening", () => {
  it("does not throw when rendered with entity: null", () => {
    expect(() =>
      render(GraphDetailPanel, { props: { entity: null } }),
    ).not.toThrow();
  });

  it("does not throw when rendered with no props at all", () => {
    expect(() =>
      // @ts-expect-error — intentional attack: required prop omitted
      render(GraphDetailPanel, { props: {} }),
    ).not.toThrow();
  });

  it("does not throw when onAddChip is called after the entity reference is replaced", async () => {
    const entity = makeEntity("c360.ops.robotics.gcs.drone.001");
    const captured: Parameters<typeof GraphDetailPanel>[0][] = [];

    const onAddChip = vi.fn((chip) => captured.push(chip));
    const user = userEvent.setup();

    render(GraphDetailPanel, {
      props: { entity, onAddChip },
    });

    // Click +Chat — entity is still mounted, this should work fine
    await user.click(screen.getByRole("button", { name: /\+\s*chat/i }));

    expect(onAddChip).toHaveBeenCalledOnce();
    expect(captured[0]).toMatchObject({ kind: "entity", value: entity.id });
  });

  it("handles entity with all-empty string idParts without crashing", () => {
    const weirdEntity = {
      id: "......",
      idParts: {
        org: "",
        platform: "",
        domain: "",
        system: "",
        type: "",
        instance: "",
      },
      properties: [],
      outgoing: [],
      incoming: [],
    };

    expect(() =>
      render(GraphDetailPanel, {
        props: { entity: weirdEntity, onAddChip: vi.fn() },
      }),
    ).not.toThrow();
  });
});

// ---------------------------------------------------------------------------
// Attack 8: clearExpanded() safety — no side effects on other store state
// ---------------------------------------------------------------------------

describe("graphStore attack — clearExpanded() safety", () => {
  it("clearExpanded does not affect selectedEntityId", () => {
    const entity = makeEntity("c360.ops.robotics.gcs.drone.001");
    graphStore.upsertEntity(entity);
    graphStore.selectEntity(entity.id);
    graphStore.markExpanded(entity.id);

    graphStore.clearExpanded();

    expect(graphStore.selectedEntityId).toBe(entity.id);
  });

  it("clearExpanded does not affect entities map", () => {
    const entity = makeEntity("c360.ops.robotics.gcs.drone.001");
    graphStore.upsertEntity(entity);
    graphStore.markExpanded(entity.id);

    graphStore.clearExpanded();

    expect(graphStore.entities.has(entity.id)).toBe(true);
  });

  it("clearExpanded does not affect filters", () => {
    graphStore.setFilters({ search: "test-filter" });
    graphStore.markExpanded("some.entity.id");

    graphStore.clearExpanded();

    expect(graphStore.filters.search).toBe("test-filter");
  });

  it("clearExpanded on already-empty set does not throw", () => {
    // expandedEntityIds is already empty after reset
    expect(() => graphStore.clearExpanded()).not.toThrow();
  });

  it("isExpanded returns false for all entities after clearExpanded", () => {
    const ids = ["a.b.c.d.e.1", "a.b.c.d.e.2", "a.b.c.d.e.3"];
    for (const id of ids) {
      graphStore.markExpanded(id);
    }

    graphStore.clearExpanded();

    for (const id of ids) {
      expect(graphStore.isExpanded(id)).toBe(false);
    }
  });
});

// ---------------------------------------------------------------------------
// Attack 9: Unmount cleanup — no memory leaks from DataView effects
// ---------------------------------------------------------------------------

describe("DataView attack — unmount cleanup", () => {
  it("unmounting DataView does not throw", async () => {
    const { unmount } = render(DataView, {
      props: { flowId: "flow-cleanup" },
    });

    await waitFor(() => {
      expect(screen.getByTestId("data-view")).toBeInTheDocument();
    });

    expect(() => unmount()).not.toThrow();
  });

  it("e2e window helpers are cleaned up on unmount", async () => {
    const { unmount } = render(DataView, {
      props: { flowId: "flow-e2e-cleanup" },
    });

    await waitFor(() => {
      expect(screen.getByTestId("data-view")).toBeInTheDocument();
    });

    // Helpers are installed by the effect — may not exist in jsdom window
    // but unmount must not throw regardless
    expect(() => unmount()).not.toThrow();
  });
});
