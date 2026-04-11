import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import EntityDetailCard from "./EntityDetailCard.svelte";

// ---------------------------------------------------------------------------
// Phase 4 expanded type fixtures
// ---------------------------------------------------------------------------

interface EntityDetailAttachmentV4 {
  kind: "entity-detail";
  entity: {
    id: string;
    label: string;
    type: string;
    domain: string;
    properties: Array<{ predicate: string; value: unknown }>;
    relationships: Array<{ predicate: string; targetId: string }>;
  };
}

function makeAttachment(
  overrides: Partial<EntityDetailAttachmentV4["entity"]> = {},
): EntityDetailAttachmentV4 {
  return {
    kind: "entity-detail",
    entity: {
      id: "c360.ops.robotics.gcs.drone.001",
      label: "001",
      type: "drone",
      domain: "robotics",
      properties: [
        { predicate: "status.operational.active", value: true },
        { predicate: "location.gps.lat", value: 32.7157 },
        { predicate: "location.gps.lon", value: -117.1611 },
      ],
      relationships: [
        {
          predicate: "fleet.membership.current",
          targetId: "c360.ops.robotics.gcs.fleet.alpha",
        },
        {
          predicate: "command.link.primary",
          targetId: "c360.ops.robotics.gcs.station.001",
        },
      ],
      ...overrides,
    },
  };
}

function makeMinimalAttachment(): EntityDetailAttachmentV4 {
  return {
    kind: "entity-detail",
    entity: {
      id: "c360.ops.robotics.gcs.drone.bare",
      label: "bare",
      type: "drone",
      domain: "robotics",
      properties: [],
      relationships: [],
    },
  };
}

// ---------------------------------------------------------------------------
// Rendering — root element and testid
// ---------------------------------------------------------------------------

describe("EntityDetailCard — root element", () => {
  it("renders with data-testid='entity-detail-card'", () => {
    render(EntityDetailCard, {
      props: { detail: makeAttachment() },
    });

    expect(screen.getByTestId("entity-detail-card")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Rendering — entity label
// ---------------------------------------------------------------------------

describe("EntityDetailCard — shows entity label", () => {
  it("displays the entity label", () => {
    const attachment = makeAttachment({ label: "alpha" });
    render(EntityDetailCard, { props: { detail: attachment } });

    expect(screen.getByTestId("entity-detail-card")).toHaveTextContent(
      /alpha/i,
    );
  });

  it("displays different labels correctly", () => {
    const attachment = makeAttachment({ label: "omega-station" });
    render(EntityDetailCard, { props: { detail: attachment } });

    expect(screen.getByTestId("entity-detail-card")).toHaveTextContent(
      /omega-station/i,
    );
  });
});

// ---------------------------------------------------------------------------
// Rendering — entity type and domain
// ---------------------------------------------------------------------------

describe("EntityDetailCard — shows entity type and domain", () => {
  it("displays the entity type", () => {
    const attachment = makeAttachment({ type: "sensor" });
    render(EntityDetailCard, { props: { detail: attachment } });

    expect(screen.getByTestId("entity-detail-card")).toHaveTextContent(
      /sensor/i,
    );
  });

  it("displays the entity domain", () => {
    const attachment = makeAttachment({ domain: "maritime" });
    render(EntityDetailCard, { props: { detail: attachment } });

    expect(screen.getByTestId("entity-detail-card")).toHaveTextContent(
      /maritime/i,
    );
  });

  it("shows both type and domain on the same card", () => {
    const attachment = makeAttachment({ type: "vessel", domain: "maritime" });
    render(EntityDetailCard, { props: { detail: attachment } });

    const card = screen.getByTestId("entity-detail-card");
    expect(card).toHaveTextContent(/vessel/i);
    expect(card).toHaveTextContent(/maritime/i);
  });
});

// ---------------------------------------------------------------------------
// Rendering — properties
// ---------------------------------------------------------------------------

describe("EntityDetailCard — renders properties", () => {
  it("renders each property as a key-value pair", () => {
    const attachment = makeAttachment({
      properties: [
        { predicate: "status.operational.active", value: true },
        { predicate: "location.gps.lat", value: 32.7157 },
      ],
    });
    render(EntityDetailCard, { props: { detail: attachment } });

    // Each property predicate should appear somewhere in the card
    expect(screen.getByTestId("entity-detail-card")).toHaveTextContent(
      /status\.operational\.active|status|active/i,
    );
    expect(screen.getByTestId("entity-detail-card")).toHaveTextContent(
      /location\.gps\.lat|lat/i,
    );
  });

  it("renders property values alongside their keys", () => {
    const attachment = makeAttachment({
      properties: [{ predicate: "altitude.meters.current", value: 120 }],
    });
    render(EntityDetailCard, { props: { detail: attachment } });

    expect(screen.getByTestId("entity-detail-card")).toHaveTextContent(/120/);
  });

  it("renders all three properties for a three-property entity", () => {
    const attachment = makeAttachment({
      properties: [
        { predicate: "prop.a.x", value: "alpha" },
        { predicate: "prop.b.y", value: "beta" },
        { predicate: "prop.c.z", value: "gamma" },
      ],
    });
    render(EntityDetailCard, { props: { detail: attachment } });

    const card = screen.getByTestId("entity-detail-card");
    expect(card).toHaveTextContent(/alpha/i);
    expect(card).toHaveTextContent(/beta/i);
    expect(card).toHaveTextContent(/gamma/i);
  });

  it("handles entity with no properties gracefully", () => {
    render(EntityDetailCard, {
      props: { detail: makeMinimalAttachment() },
    });

    expect(screen.getByTestId("entity-detail-card")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Rendering — relationships
// ---------------------------------------------------------------------------

describe("EntityDetailCard — shows relationship info", () => {
  it("shows relationship count when relationships are present", () => {
    const attachment = makeAttachment({
      relationships: [
        {
          predicate: "fleet.membership.current",
          targetId: "c360.ops.robotics.gcs.fleet.alpha",
        },
        {
          predicate: "command.link.primary",
          targetId: "c360.ops.robotics.gcs.station.001",
        },
      ],
    });
    render(EntityDetailCard, { props: { detail: attachment } });

    // Should show count (2) or list them
    expect(screen.getByTestId("entity-detail-card")).toHaveTextContent(
      /2|relationship/i,
    );
  });

  it("renders relationship predicates or target ids", () => {
    const attachment = makeAttachment({
      relationships: [
        {
          predicate: "fleet.membership.current",
          targetId: "c360.ops.robotics.gcs.fleet.alpha",
        },
      ],
    });
    render(EntityDetailCard, { props: { detail: attachment } });

    const card = screen.getByTestId("entity-detail-card");
    // Either the predicate or the targetId should appear
    const hasContent =
      card.textContent?.includes("fleet") ||
      card.textContent?.includes("membership") ||
      card.textContent?.includes("alpha");
    expect(hasContent).toBe(true);
  });

  it("handles entity with no relationships gracefully", () => {
    render(EntityDetailCard, {
      props: { detail: makeMinimalAttachment() },
    });

    expect(screen.getByTestId("entity-detail-card")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Interaction — clicking entity calls onViewEntity
// ---------------------------------------------------------------------------

describe("EntityDetailCard — clicking entity calls onViewEntity", () => {
  it("clicking the entity triggers onViewEntity with entity id", async () => {
    const onViewEntity = vi.fn();
    const user = userEvent.setup();

    render(EntityDetailCard, {
      props: { detail: makeAttachment(), onViewEntity },
    });

    // The card or entity id should be clickable
    const clickTarget = screen.getByTestId("entity-detail-entity-id");
    await user.click(clickTarget);

    expect(onViewEntity).toHaveBeenCalledOnce();
    expect(onViewEntity).toHaveBeenCalledWith(
      "c360.ops.robotics.gcs.drone.001",
    );
  });

  it("onViewEntity is called with correct id for different entities", async () => {
    const onViewEntity = vi.fn();
    const user = userEvent.setup();

    const attachment = makeAttachment({
      id: "c360.sec.border.surveillance.sensor.007",
      label: "007",
      type: "sensor",
      domain: "border",
    });

    render(EntityDetailCard, {
      props: { detail: attachment, onViewEntity },
    });

    await user.click(screen.getByTestId("entity-detail-entity-id"));

    expect(onViewEntity).toHaveBeenCalledWith(
      "c360.sec.border.surveillance.sensor.007",
    );
  });
});

// ---------------------------------------------------------------------------
// Interaction — onViewEntity prop is optional
// ---------------------------------------------------------------------------

describe("EntityDetailCard — onViewEntity is optional", () => {
  it("does not throw when onViewEntity is not provided", async () => {
    const user = userEvent.setup();

    render(EntityDetailCard, { props: { detail: makeAttachment() } });

    await expect(
      user.click(screen.getByTestId("entity-detail-entity-id")),
    ).resolves.not.toThrow();
  });
});

// ---------------------------------------------------------------------------
// Table-driven: type × domain display
// ---------------------------------------------------------------------------

describe("EntityDetailCard — table-driven type × domain", () => {
  it.each([
    { type: "drone", domain: "robotics" },
    { type: "sensor", domain: "border" },
    { type: "vessel", domain: "maritime" },
    { type: "station", domain: "command" },
  ])("shows type=$type and domain=$domain", ({ type, domain }) => {
    render(EntityDetailCard, {
      props: { detail: makeAttachment({ type, domain }) },
    });

    const card = screen.getByTestId("entity-detail-card");
    expect(card).toHaveTextContent(new RegExp(type, "i"));
    expect(card).toHaveTextContent(new RegExp(domain, "i"));
  });
});
