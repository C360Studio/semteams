import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/svelte";
import HealthStatusCard from "./HealthStatusCard.svelte";

// ---------------------------------------------------------------------------
// HealthAttachment fixture (inline to avoid importing not-yet-existing type)
// ---------------------------------------------------------------------------

interface HealthAttachment {
  kind: "health";
  componentName: string;
  status: "healthy" | "degraded" | "unhealthy" | "unknown";
  message?: string;
  metrics?: Record<string, number>;
  lastCheck?: string;
}

function makeAttachment(
  overrides: Partial<HealthAttachment> = {},
): HealthAttachment {
  return {
    kind: "health",
    componentName: "http-input",
    status: "healthy",
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// Rendering — root element and testid
// ---------------------------------------------------------------------------

describe("HealthStatusCard — root element", () => {
  it("renders with data-testid='health-status-card'", () => {
    render(HealthStatusCard, { props: { attachment: makeAttachment() } });

    expect(screen.getByTestId("health-status-card")).toBeInTheDocument();
  });

  it("renders without throwing for minimal attachment", () => {
    expect(() =>
      render(HealthStatusCard, { props: { attachment: makeAttachment() } }),
    ).not.toThrow();
  });
});

// ---------------------------------------------------------------------------
// Rendering — component name
// ---------------------------------------------------------------------------

describe("HealthStatusCard — shows component name", () => {
  it("displays the componentName", () => {
    render(HealthStatusCard, {
      props: { attachment: makeAttachment({ componentName: "kafka-output" }) },
    });

    expect(screen.getByTestId("health-status-card")).toHaveTextContent(
      /kafka-output/i,
    );
  });

  it("displays different component names correctly", () => {
    render(HealthStatusCard, {
      props: {
        attachment: makeAttachment({ componentName: "nats-processor" }),
      },
    });

    expect(screen.getByTestId("health-status-card")).toHaveTextContent(
      /nats-processor/i,
    );
  });
});

// ---------------------------------------------------------------------------
// Rendering — status text
// ---------------------------------------------------------------------------

describe("HealthStatusCard — shows status text", () => {
  it("displays 'healthy' status", () => {
    render(HealthStatusCard, {
      props: { attachment: makeAttachment({ status: "healthy" }) },
    });

    expect(screen.getByTestId("health-status-card")).toHaveTextContent(
      /healthy/i,
    );
  });

  it("displays 'degraded' status", () => {
    render(HealthStatusCard, {
      props: { attachment: makeAttachment({ status: "degraded" }) },
    });

    expect(screen.getByTestId("health-status-card")).toHaveTextContent(
      /degraded/i,
    );
  });

  it("displays 'unhealthy' status", () => {
    render(HealthStatusCard, {
      props: { attachment: makeAttachment({ status: "unhealthy" }) },
    });

    expect(screen.getByTestId("health-status-card")).toHaveTextContent(
      /unhealthy/i,
    );
  });

  it("displays 'unknown' status", () => {
    render(HealthStatusCard, {
      props: { attachment: makeAttachment({ status: "unknown" }) },
    });

    expect(screen.getByTestId("health-status-card")).toHaveTextContent(
      /unknown/i,
    );
  });
});

// ---------------------------------------------------------------------------
// Rendering — status indicator styling
// ---------------------------------------------------------------------------

describe("HealthStatusCard — status indicator has appropriate class or attribute", () => {
  it("healthy status indicator has a class or data-status indicating health", () => {
    render(HealthStatusCard, {
      props: { attachment: makeAttachment({ status: "healthy" }) },
    });

    const card = screen.getByTestId("health-status-card");
    const indicator = card.querySelector(
      "[data-status='healthy'], .status-healthy, .healthy, [class*='healthy']",
    );
    expect(indicator).not.toBeNull();
  });

  it("degraded status indicator has a class or data-status indicating degraded", () => {
    render(HealthStatusCard, {
      props: { attachment: makeAttachment({ status: "degraded" }) },
    });

    const card = screen.getByTestId("health-status-card");
    const indicator = card.querySelector(
      "[data-status='degraded'], .status-degraded, .degraded, [class*='degraded']",
    );
    expect(indicator).not.toBeNull();
  });

  it("unhealthy status indicator has a class or data-status indicating unhealthy", () => {
    render(HealthStatusCard, {
      props: { attachment: makeAttachment({ status: "unhealthy" }) },
    });

    const card = screen.getByTestId("health-status-card");
    const indicator = card.querySelector(
      "[data-status='unhealthy'], .status-unhealthy, .unhealthy, [class*='unhealthy']",
    );
    expect(indicator).not.toBeNull();
  });

  it("unknown status indicator has a class or data-status indicating unknown", () => {
    render(HealthStatusCard, {
      props: { attachment: makeAttachment({ status: "unknown" }) },
    });

    const card = screen.getByTestId("health-status-card");
    const indicator = card.querySelector(
      "[data-status='unknown'], .status-unknown, .unknown, [class*='unknown']",
    );
    expect(indicator).not.toBeNull();
  });
});

// ---------------------------------------------------------------------------
// Rendering — message (optional)
// ---------------------------------------------------------------------------

describe("HealthStatusCard — shows message when present", () => {
  it("displays the message text when provided", () => {
    render(HealthStatusCard, {
      props: {
        attachment: makeAttachment({
          status: "degraded",
          message: "Reconnecting to upstream broker",
        }),
      },
    });

    expect(screen.getByTestId("health-status-card")).toHaveTextContent(
      /Reconnecting to upstream broker/i,
    );
  });

  it("displays a different message correctly", () => {
    render(HealthStatusCard, {
      props: {
        attachment: makeAttachment({
          status: "unhealthy",
          message: "TCP connection refused on port 9092",
        }),
      },
    });

    expect(screen.getByTestId("health-status-card")).toHaveTextContent(
      /TCP connection refused/i,
    );
  });

  it("renders without error when message is absent", () => {
    render(HealthStatusCard, {
      props: { attachment: makeAttachment({ message: undefined }) },
    });

    expect(screen.getByTestId("health-status-card")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Rendering — metrics (optional)
// ---------------------------------------------------------------------------

describe("HealthStatusCard — shows metrics as key-value pairs", () => {
  it("renders metric keys when metrics are provided", () => {
    render(HealthStatusCard, {
      props: {
        attachment: makeAttachment({
          metrics: {
            messages_per_second: 150.0,
            error_rate: 0.02,
          },
        }),
      },
    });

    const card = screen.getByTestId("health-status-card");
    // At least one metric key should appear
    const hasMetricKey =
      card.textContent?.includes("messages_per_second") ||
      card.textContent?.includes("messages per second") ||
      card.textContent?.includes("error_rate") ||
      card.textContent?.includes("error rate");
    expect(hasMetricKey).toBe(true);
  });

  it("renders metric values when metrics are provided", () => {
    render(HealthStatusCard, {
      props: {
        attachment: makeAttachment({
          metrics: { throughput: 99.5 },
        }),
      },
    });

    expect(screen.getByTestId("health-status-card")).toHaveTextContent(/99/);
  });

  it("renders multiple metrics", () => {
    render(HealthStatusCard, {
      props: {
        attachment: makeAttachment({
          metrics: {
            messages_per_second: 42,
            queue_depth: 7,
            latency_ms: 13,
          },
        }),
      },
    });

    const card = screen.getByTestId("health-status-card");
    expect(card).toHaveTextContent(/42/);
    expect(card).toHaveTextContent(/7/);
    expect(card).toHaveTextContent(/13/);
  });

  it("renders without error when metrics are absent", () => {
    render(HealthStatusCard, {
      props: { attachment: makeAttachment({ metrics: undefined }) },
    });

    expect(screen.getByTestId("health-status-card")).toBeInTheDocument();
  });

  it("renders without error when metrics is empty object", () => {
    render(HealthStatusCard, {
      props: { attachment: makeAttachment({ metrics: {} }) },
    });

    expect(screen.getByTestId("health-status-card")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Rendering — lastCheck (optional)
// ---------------------------------------------------------------------------

describe("HealthStatusCard — shows lastCheck timestamp when present", () => {
  it("displays lastCheck text when provided", () => {
    render(HealthStatusCard, {
      props: {
        attachment: makeAttachment({
          lastCheck: "2026-03-09T14:30:00Z",
        }),
      },
    });

    const card = screen.getByTestId("health-status-card");
    // The timestamp or a humanized version should appear
    const hasTimestamp =
      card.textContent?.includes("2026") ||
      card.textContent?.includes("14:30") ||
      card.textContent?.includes("ago") ||
      card.textContent?.includes("last check") ||
      card.textContent?.includes("Last check") ||
      card.textContent?.includes("checked");
    expect(hasTimestamp).toBe(true);
  });

  it("renders without error when lastCheck is absent", () => {
    render(HealthStatusCard, {
      props: { attachment: makeAttachment({ lastCheck: undefined }) },
    });

    expect(screen.getByTestId("health-status-card")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Table-driven: all status values render correctly
// ---------------------------------------------------------------------------

describe("HealthStatusCard — table-driven status rendering", () => {
  it.each([
    { status: "healthy" as const, pattern: /healthy/i },
    { status: "degraded" as const, pattern: /degraded/i },
    { status: "unhealthy" as const, pattern: /unhealthy/i },
    { status: "unknown" as const, pattern: /unknown/i },
  ])("status='$status' is visible in the card", ({ status, pattern }) => {
    render(HealthStatusCard, {
      props: {
        attachment: makeAttachment({ componentName: "test-component", status }),
      },
    });

    expect(screen.getByTestId("health-status-card")).toHaveTextContent(pattern);
  });
});

// ---------------------------------------------------------------------------
// Table-driven: component name × status combinations
// ---------------------------------------------------------------------------

describe("HealthStatusCard — component name and status both visible", () => {
  it.each([
    { componentName: "http-input", status: "healthy" as const },
    { componentName: "kafka-output", status: "degraded" as const },
    { componentName: "nats-processor", status: "unhealthy" as const },
    { componentName: "redis-cache", status: "unknown" as const },
  ])(
    "shows componentName='$componentName' and status='$status'",
    ({ componentName, status }) => {
      render(HealthStatusCard, {
        props: { attachment: makeAttachment({ componentName, status }) },
      });

      const card = screen.getByTestId("health-status-card");
      expect(card).toHaveTextContent(new RegExp(componentName, "i"));
      expect(card).toHaveTextContent(new RegExp(status, "i"));
    },
  );
});
