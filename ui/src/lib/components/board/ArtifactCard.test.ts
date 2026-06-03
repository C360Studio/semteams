import { describe, it, expect } from "vitest";
import { render, screen, within } from "@testing-library/svelte";
import ArtifactCard from "./ArtifactCard.svelte";

describe("ArtifactCard", () => {
  it("renders the tool name in the header", () => {
    render(ArtifactCard, { props: { toolName: "emit_plan", args: {} } });
    expect(screen.getByTestId("artifact-tool-name")).toHaveTextContent("emit_plan");
  });

  it("lifts `title` into a heading", () => {
    render(ArtifactCard, {
      props: {
        toolName: "emit_research_artifact",
        args: { title: "MQTT vs NATS for IoT edge", revision: 1 },
      },
    });
    expect(screen.getByTestId("artifact-title")).toHaveTextContent(
      "MQTT vs NATS for IoT edge",
    );
  });

  it("lifts `revision` (number) into a rev badge", () => {
    render(ArtifactCard, {
      props: { toolName: "emit_plan", args: { revision: 2 } },
    });
    expect(screen.getByTestId("artifact-revision")).toHaveTextContent("rev 2");
  });

  it("lifts `revision` (string) into a rev badge", () => {
    render(ArtifactCard, {
      props: { toolName: "emit_plan", args: { revision: "alpha" } },
    });
    expect(screen.getByTestId("artifact-revision")).toHaveTextContent("rev alpha");
  });

  it("renders a fallback when args is undefined", () => {
    render(ArtifactCard, {
      props: { toolName: "emit_plan", args: undefined },
    });
    expect(screen.getByText("No additional fields.")).toBeInTheDocument();
  });

  it("renders a fallback when args has no fields beyond title/revision", () => {
    render(ArtifactCard, {
      props: { toolName: "emit_plan", args: { title: "x", revision: 1 } },
    });
    expect(screen.getByText("No additional fields.")).toBeInTheDocument();
  });

  it("renders a string field with white-space preserved", () => {
    render(ArtifactCard, {
      props: {
        toolName: "emit_plan",
        args: { goal: "line one\n\nline three" },
      },
    });
    const field = screen.getByTestId("artifact-field");
    expect(field).toHaveAttribute("data-field-key", "goal");
    expect(within(field).getByText(/line one/).textContent).toContain("line three");
  });

  it("renders an array of strings as a bullet list", () => {
    render(ArtifactCard, {
      props: {
        toolName: "emit_plan",
        args: { scope_in: ["MQTT brokers", "NATS servers", "ARM hardware"] },
      },
    });
    const field = screen.getByTestId("artifact-field");
    expect(field).toHaveAttribute("data-field-key", "scope_in");
    expect(within(field).getByText("MQTT brokers")).toBeInTheDocument();
    expect(within(field).getByText("NATS servers")).toBeInTheDocument();
    expect(within(field).getByText("ARM hardware")).toBeInTheDocument();
    // Bullet list rendering, not a comma-joined string.
    const items = within(field).getAllByRole("listitem");
    expect(items).toHaveLength(3);
  });

  it("renders an array of objects as a per-item definition list", () => {
    render(ArtifactCard, {
      props: {
        toolName: "emit_research_artifact",
        args: {
          actors: [
            { name: "MQTT broker", role: "TCP pub/sub" },
            { name: "NATS server", role: "subject pub/sub + request-reply" },
          ],
        },
      },
    });
    const field = screen.getByTestId("artifact-field");
    expect(field).toHaveAttribute("data-field-key", "actors");
    expect(within(field).getByText("MQTT broker")).toBeInTheDocument();
    expect(within(field).getByText("TCP pub/sub")).toBeInTheDocument();
    expect(within(field).getByText("NATS server")).toBeInTheDocument();
    // Each object renders as its own list item.
    const items = within(field).getAllByRole("listitem");
    expect(items).toHaveLength(2);
  });

  it("labels keys human-friendly (snake_case → Title case)", () => {
    render(ArtifactCard, {
      props: {
        toolName: "emit_research_artifact",
        args: { integration_points: [] },
      },
    });
    expect(screen.getByText("Integration points")).toBeInTheDocument();
  });

  it("renders an empty array as `(empty)`", () => {
    render(ArtifactCard, {
      props: {
        toolName: "emit_research_artifact",
        args: { substrate_mutations: [] },
      },
    });
    expect(screen.getByText("(empty)")).toBeInTheDocument();
  });

  it("renders nested plain objects as a definition list", () => {
    render(ArtifactCard, {
      props: {
        toolName: "emit_autoresearch_baseline",
        args: { metrics: { value: 1.023, pass: true } },
      },
    });
    const field = screen.getByTestId("artifact-field");
    expect(field).toHaveAttribute("data-field-key", "metrics");
    expect(within(field).getByText("Value")).toBeInTheDocument();
    expect(within(field).getByText("1.023")).toBeInTheDocument();
    expect(within(field).getByText("Pass")).toBeInTheDocument();
    expect(within(field).getByText("true")).toBeInTheDocument();
  });

  it("renders a number / boolean scalar field", () => {
    render(ArtifactCard, {
      props: {
        toolName: "emit_autoresearch_baseline",
        args: { baseline_value: 1.023, baseline_pass: true },
      },
    });
    expect(screen.getByText("1.023")).toBeInTheDocument();
    expect(screen.getByText("true")).toBeInTheDocument();
  });

  it("handles a mixed full-shape research-artifact payload", () => {
    // Mirror of the research-mvp.yaml synthesize fixture — proves the
    // renderer survives the real wire shape end-to-end.
    render(ArtifactCard, {
      props: {
        toolName: "emit_research_artifact",
        args: {
          revision: 1,
          title: "MQTT vs NATS for IoT edge",
          actors: [
            { name: "MQTT broker", role: "TCP pub/sub" },
          ],
          integration_points: [
            { from: "MQTT broker", to: "ARM device", direction: "read" },
          ],
          tasks: ["Choose MQTT for sub-ms latency"],
          addressed_gaps: ["Question scope answered"],
          open_gaps: ["not applicable"],
          substrate_mutations: [],
        },
      },
    });
    // Title + revision in header.
    expect(screen.getByTestId("artifact-title")).toHaveTextContent(
      "MQTT vs NATS for IoT edge",
    );
    expect(screen.getByTestId("artifact-revision")).toHaveTextContent("rev 1");
    // Each typed field rendered as its own section.
    const fields = screen.getAllByTestId("artifact-field");
    const fieldKeys = fields.map((f) => f.getAttribute("data-field-key"));
    expect(fieldKeys).toEqual([
      "actors",
      "integration_points",
      "tasks",
      "addressed_gaps",
      "open_gaps",
      "substrate_mutations",
    ]);
  });
});
