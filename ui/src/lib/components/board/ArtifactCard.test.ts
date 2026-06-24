import { afterEach, describe, it, expect, vi } from "vitest";
import { fireEvent, render, screen, within } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import ArtifactCard from "./ArtifactCard.svelte";

const originalClipboardDescriptor = Object.getOwnPropertyDescriptor(
  navigator,
  "clipboard",
);
const originalCreateObjectURLDescriptor = Object.getOwnPropertyDescriptor(
  URL,
  "createObjectURL",
);
const originalRevokeObjectURLDescriptor = Object.getOwnPropertyDescriptor(
  URL,
  "revokeObjectURL",
);

function restoreProperty(
  target: object,
  key: string,
  descriptor: PropertyDescriptor | undefined,
) {
  if (descriptor) {
    Object.defineProperty(target, key, descriptor);
    return;
  }
  Reflect.deleteProperty(target, key);
}

function emitChangeArgs(): Record<string, unknown> {
  return {
    slug: "add-mfa",
    proposal: {
      intent: "Let account owners enable a second login factor.",
      scope_in: ["TOTP enrollment", "TOTP verification"],
      scope_out: ["Hardware security keys"],
      approach: "Add an OpenSpec-first MFA slice with one backend task.",
    },
    design: {
      technical_approach:
        "Use existing auth routes and add one verified TOTP path.",
      decisions: [
        {
          name: "Store TOTP secrets encrypted",
          body: "Keep the secret out of plaintext application tables.",
        },
      ],
      data_flow:
        "Enrollment writes a secret; verification checks the current code.",
      file_changes: [{ path: "cmd/auth/mfa.go", kind: "new" }],
    },
    deltas: [
      {
        capability: "auth",
        added: [
          {
            name: "TOTP Login",
            statement:
              "The system SHALL verify a valid TOTP code before login completes.",
            scenarios: [
              {
                name: "Valid TOTP",
                steps: [
                  { kw: "GIVEN", text: "a user has MFA enabled" },
                  { kw: "WHEN", text: "they submit a valid TOTP code" },
                  { kw: "THEN", text: "login completes" },
                ],
              },
            ],
          },
        ],
        modified: [],
        removed: [],
      },
    ],
    tasks: [
      {
        section: "1. MFA",
        number: "1.1",
        text: "Implement verified TOTP login.",
        done: true,
        goal: "TOTP login path rejects invalid codes and accepts valid codes.",
        target_files: ["cmd/auth/**"],
        test_command: "go test ./cmd/auth",
        assumptions: [],
        non_goals: ["Hardware security keys"],
        requirement_ref: "auth/TOTP Login",
      },
    ],
    acceptance_command: "go test ./...",
  };
}

function readBlobText(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.addEventListener("load", () => resolve(String(reader.result ?? "")));
    reader.addEventListener("error", () => reject(reader.error));
    reader.readAsText(blob);
  });
}

afterEach(() => {
  vi.restoreAllMocks();
  restoreProperty(navigator, "clipboard", originalClipboardDescriptor);
  restoreProperty(URL, "createObjectURL", originalCreateObjectURLDescriptor);
  restoreProperty(URL, "revokeObjectURL", originalRevokeObjectURLDescriptor);
});

describe("ArtifactCard", () => {
  it("renders the tool name in the header", () => {
    render(ArtifactCard, { props: { toolName: "emit_plan", args: {} } });
    expect(screen.getByTestId("artifact-tool-name")).toHaveTextContent(
      "emit_plan",
    );
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
    expect(screen.getByTestId("artifact-revision")).toHaveTextContent(
      "rev alpha",
    );
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

  it("renders a multi-paragraph string field as separate markdown paragraphs", () => {
    render(ArtifactCard, {
      props: {
        toolName: "emit_plan",
        args: { goal: "line one\n\nline three" },
      },
    });
    const field = screen.getByTestId("artifact-field");
    expect(field).toHaveAttribute("data-field-key", "goal");
    // A blank line is a paragraph break: two <p class="md-p"> elements.
    const paras = field.querySelectorAll("p.md-p");
    expect(paras.length).toBe(2);
    expect(paras[0].textContent).toBe("line one");
    expect(paras[1].textContent).toBe("line three");
  });

  it("renders markdown inside a string field (bold + inline code)", () => {
    render(ArtifactCard, {
      props: {
        toolName: "emit_research_artifact",
        args: { summary: "Cut **wallclock** with `go test -p`." },
      },
    });
    const field = screen.getByTestId("artifact-field");
    expect(field.querySelector("strong")?.textContent).toBe("wallclock");
    expect(field.querySelector("code")?.textContent).toBe("go test -p");
  });

  it("escapes HTML in a string field (no raw markup injected)", () => {
    render(ArtifactCard, {
      props: {
        toolName: "emit_plan",
        args: { goal: "<img src=x onerror=alert(1)>" },
      },
    });
    const field = screen.getByTestId("artifact-field");
    expect(field.querySelector("img")).toBeNull();
    expect(field.textContent).toContain("<img src=x onerror=alert(1)>");
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
          actors: [{ name: "MQTT broker", role: "TCP pub/sub" }],
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

  it("renders emit_change as a reviewable OpenSpec artifact", () => {
    render(ArtifactCard, {
      props: { toolName: "emit_change", args: emitChangeArgs() },
    });

    expect(screen.getByTestId("openspec-review-panel")).toBeInTheDocument();
    expect(screen.getByTestId("openspec-title")).toHaveTextContent("add-mfa");
    expect(screen.getByTestId("openspec-acceptance")).toHaveTextContent(
      "go test ./...",
    );
    expect(screen.queryByTestId("artifact-field")).not.toBeInTheDocument();

    const preview = screen.getByTestId("openspec-preview");
    expect(preview.textContent).toContain("# OpenSpec change: add-mfa");
    expect(preview.textContent).toContain(
      "<!-- openspec/changes/add-mfa/proposal.md -->",
    );
    expect(preview.textContent).toContain(
      "<!-- openspec/changes/add-mfa/specs/auth/spec.md -->",
    );
    expect(preview.textContent).toContain(
      "The system SHALL verify a valid TOTP code before login completes.",
    );
    expect(preview.textContent).toContain(
      "- [x] 1.1 Implement verified TOTP login.",
    );
  });

  it("lets a human edit the OpenSpec handoff document before export", async () => {
    const user = userEvent.setup();
    render(ArtifactCard, {
      props: { toolName: "emit_change", args: emitChangeArgs() },
    });

    await user.click(screen.getByTestId("openspec-edit"));
    const editor = screen.getByTestId("openspec-editor") as HTMLTextAreaElement;
    expect(editor.value).toContain("# OpenSpec change: add-mfa");

    await fireEvent.input(editor, {
      target: { value: "# Edited OpenSpec\n\nManual reviewer note.\n" },
    });
    await user.click(screen.getByTestId("openspec-save-edit"));

    expect(screen.getByTestId("openspec-export-state")).toHaveTextContent(
      "Draft updated",
    );
    expect(screen.getByTestId("openspec-preview").textContent).toContain(
      "Manual reviewer note.",
    );
  });

  it("records approve, request-revision, and reject review decisions", async () => {
    const user = userEvent.setup();
    render(ArtifactCard, {
      props: { toolName: "emit_change", args: emitChangeArgs() },
    });

    await user.click(screen.getByTestId("openspec-approve"));
    expect(screen.getByTestId("openspec-review-state")).toHaveTextContent(
      "Approved",
    );
    expect(screen.getByTestId("openspec-review-state")).toHaveAttribute(
      "data-state",
      "approved",
    );

    await user.click(screen.getByTestId("openspec-revision"));
    expect(screen.getByTestId("openspec-review-state")).toHaveTextContent(
      "Revision requested",
    );
    expect(screen.getByTestId("openspec-review-state")).toHaveAttribute(
      "data-state",
      "revision",
    );

    await user.click(screen.getByTestId("openspec-reject"));
    expect(screen.getByTestId("openspec-review-state")).toHaveTextContent(
      "Rejected",
    );
    expect(screen.getByTestId("openspec-review-state")).toHaveAttribute(
      "data-state",
      "rejected",
    );
  });

  it("exports a markdown handoff document via clipboard", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    const user = userEvent.setup();
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });

    render(ArtifactCard, {
      props: { toolName: "emit_change", args: emitChangeArgs() },
    });

    await user.click(screen.getByTestId("openspec-copy"));

    expect(writeText).toHaveBeenCalledTimes(1);
    const copied = writeText.mock.calls[0][0] as string;
    expect(copied).toContain("<!-- openspec/changes/add-mfa/proposal.md -->");
    expect(copied).toContain("# Delta for Auth");
    expect(
      await screen.findByTestId("openspec-export-state"),
    ).toHaveTextContent("Copied");
  });

  it("downloads both the rendered document and folder manifest", async () => {
    const downloadedBlobs: Blob[] = [];
    const createObjectURL = vi.fn((blob: Blob) => {
      downloadedBlobs.push(blob);
      return "blob:openspec";
    });
    const revokeObjectURL = vi.fn();
    const anchorClick = vi
      .spyOn(HTMLAnchorElement.prototype, "click")
      .mockImplementation(() => {});
    Object.defineProperty(URL, "createObjectURL", {
      configurable: true,
      value: createObjectURL,
    });
    Object.defineProperty(URL, "revokeObjectURL", {
      configurable: true,
      value: revokeObjectURL,
    });

    const user = userEvent.setup();
    render(ArtifactCard, {
      props: { toolName: "emit_change", args: emitChangeArgs() },
    });

    await user.click(screen.getByTestId("openspec-download"));
    expect(anchorClick).toHaveBeenCalledTimes(1);
    expect(createObjectURL).toHaveBeenCalledTimes(1);
    const documentBlob = downloadedBlobs[0];
    expect(await readBlobText(documentBlob)).toContain(
      "# OpenSpec change: add-mfa",
    );
    expect(screen.getByTestId("openspec-export-state")).toHaveTextContent(
      "Document downloaded",
    );

    await user.click(screen.getByTestId("openspec-download-folder"));
    expect(anchorClick).toHaveBeenCalledTimes(2);
    const manifestBlob = downloadedBlobs[1];
    const manifest = JSON.parse(await readBlobText(manifestBlob)) as {
      kind: string;
      slug: string;
      files: Array<{ path: string; content: string }>;
    };
    expect(manifest.kind).toBe("openspec.change.folder");
    expect(manifest.slug).toBe("add-mfa");
    expect(manifest.files).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          path: "openspec/changes/add-mfa/tasks.md",
        }),
        expect.objectContaining({
          path: "openspec/changes/add-mfa/specs/auth/spec.md",
        }),
      ]),
    );
    expect(screen.getByTestId("openspec-export-state")).toHaveTextContent(
      "Folder manifest downloaded",
    );
  });
});
