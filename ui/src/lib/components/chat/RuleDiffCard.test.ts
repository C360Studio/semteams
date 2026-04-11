import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/svelte";
import RuleDiffCard from "./RuleDiffCard.svelte";

// ---------------------------------------------------------------------------
// RuleDiffAttachment fixture (inline to match ToolCallCard test pattern)
// ---------------------------------------------------------------------------

interface RuleDiffAttachment {
  kind: "rule-diff";
  ruleId: string;
  ruleName: string;
  operation: "create" | "update" | "delete";
  before?: Record<string, unknown>;
  after?: Record<string, unknown>;
}

function makeAttachment(
  overrides: Partial<RuleDiffAttachment> = {},
): RuleDiffAttachment {
  return {
    kind: "rule-diff",
    ruleId: "rule-123",
    ruleName: "email-routing",
    operation: "update",
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// Rendering — root element and testid
// ---------------------------------------------------------------------------

describe("RuleDiffCard — root element", () => {
  it("renders with data-testid='rule-diff-card'", () => {
    render(RuleDiffCard, { props: { attachment: makeAttachment() } });

    expect(screen.getByTestId("rule-diff-card")).toBeInTheDocument();
  });

  it("renders without throwing for minimal attachment", () => {
    expect(() =>
      render(RuleDiffCard, { props: { attachment: makeAttachment() } }),
    ).not.toThrow();
  });
});

// ---------------------------------------------------------------------------
// Rendering — rule name
// ---------------------------------------------------------------------------

describe("RuleDiffCard — displays rule name", () => {
  it("displays the ruleName", () => {
    render(RuleDiffCard, {
      props: { attachment: makeAttachment({ ruleName: "spam-filter" }) },
    });

    expect(screen.getByTestId("rule-diff-card")).toHaveTextContent(
      /spam-filter/,
    );
  });

  it("displays different rule names correctly", () => {
    render(RuleDiffCard, {
      props: {
        attachment: makeAttachment({ ruleName: "dedup-check" }),
      },
    });

    expect(screen.getByTestId("rule-diff-card")).toHaveTextContent(
      /dedup-check/,
    );
  });
});

// ---------------------------------------------------------------------------
// Rendering — rule ID
// ---------------------------------------------------------------------------

describe("RuleDiffCard — displays rule ID", () => {
  it("displays the ruleId", () => {
    render(RuleDiffCard, {
      props: { attachment: makeAttachment({ ruleId: "rule-abc-456" }) },
    });

    expect(screen.getByTestId("rule-diff-card")).toHaveTextContent(
      /rule-abc-456/,
    );
  });
});

// ---------------------------------------------------------------------------
// Rendering — operation badge text and data-operation attribute
// ---------------------------------------------------------------------------

describe("RuleDiffCard — operation badge", () => {
  it("displays operation text", () => {
    render(RuleDiffCard, {
      props: { attachment: makeAttachment({ operation: "create" }) },
    });

    expect(screen.getByTestId("rule-diff-card")).toHaveTextContent(/create/i);
  });

  it("has data-operation attribute matching the operation", () => {
    render(RuleDiffCard, {
      props: { attachment: makeAttachment({ operation: "delete" }) },
    });

    const card = screen.getByTestId("rule-diff-card");
    const badge = card.querySelector("[data-operation='delete']");
    expect(badge).not.toBeNull();
  });
});

// ---------------------------------------------------------------------------
// Rendering — update operation (Before + After collapsible sections)
// ---------------------------------------------------------------------------

describe("RuleDiffCard — update operation", () => {
  it("shows Before and After details when operation is update", () => {
    render(RuleDiffCard, {
      props: {
        attachment: makeAttachment({
          operation: "update",
          before: { threshold: 0.5, enabled: true },
          after: { threshold: 0.8, enabled: true },
        }),
      },
    });

    const card = screen.getByTestId("rule-diff-card");
    const beforeDetails = card.querySelector("details.diff-before");
    const afterDetails = card.querySelector("details.diff-after");
    expect(beforeDetails).not.toBeNull();
    expect(afterDetails).not.toBeNull();
    expect(card).toHaveTextContent(/Before/);
    expect(card).toHaveTextContent(/After/);
    // Verify before/after data is rendered as JSON
    expect(card).toHaveTextContent(/0\.5/);
    expect(card).toHaveTextContent(/0\.8/);
  });

  it("does NOT show diff sections when before/after are undefined for update", () => {
    render(RuleDiffCard, {
      props: {
        attachment: makeAttachment({
          operation: "update",
          before: undefined,
          after: undefined,
        }),
      },
    });

    const card = screen.getByTestId("rule-diff-card");
    const details = card.querySelectorAll("details");
    expect(details.length).toBe(0);
  });
});

// ---------------------------------------------------------------------------
// Rendering — create operation (Rule Definition section with after data)
// ---------------------------------------------------------------------------

describe("RuleDiffCard — create operation", () => {
  it("shows Rule Definition details when operation is create", () => {
    render(RuleDiffCard, {
      props: {
        attachment: makeAttachment({
          operation: "create",
          after: { action: "forward", target: "queue-1" },
        }),
      },
    });

    const card = screen.getByTestId("rule-diff-card");
    const afterDetails = card.querySelector("details.diff-after");
    expect(afterDetails).not.toBeNull();
    expect(card).toHaveTextContent(/Rule Definition/);
    expect(card).toHaveTextContent(/forward/);
    expect(card).toHaveTextContent(/queue-1/);
  });

  it("does NOT show Before section for create operation", () => {
    render(RuleDiffCard, {
      props: {
        attachment: makeAttachment({
          operation: "create",
          after: { action: "forward" },
        }),
      },
    });

    const card = screen.getByTestId("rule-diff-card");
    const beforeDetails = card.querySelector("details.diff-before");
    expect(beforeDetails).toBeNull();
  });

  it("does NOT show Rule Definition when after is undefined for create", () => {
    render(RuleDiffCard, {
      props: {
        attachment: makeAttachment({
          operation: "create",
          after: undefined,
        }),
      },
    });

    const card = screen.getByTestId("rule-diff-card");
    const details = card.querySelectorAll("details");
    expect(details.length).toBe(0);
  });
});

// ---------------------------------------------------------------------------
// Rendering — delete operation (Deleted Rule section with before data)
// ---------------------------------------------------------------------------

describe("RuleDiffCard — delete operation", () => {
  it("shows Deleted Rule details when operation is delete", () => {
    render(RuleDiffCard, {
      props: {
        attachment: makeAttachment({
          operation: "delete",
          before: { action: "reject", reason: "spam" },
        }),
      },
    });

    const card = screen.getByTestId("rule-diff-card");
    const beforeDetails = card.querySelector("details.diff-before");
    expect(beforeDetails).not.toBeNull();
    expect(card).toHaveTextContent(/Deleted Rule/);
    expect(card).toHaveTextContent(/reject/);
    expect(card).toHaveTextContent(/spam/);
  });

  it("does NOT show After section for delete operation", () => {
    render(RuleDiffCard, {
      props: {
        attachment: makeAttachment({
          operation: "delete",
          before: { action: "reject" },
        }),
      },
    });

    const card = screen.getByTestId("rule-diff-card");
    const afterDetails = card.querySelector("details.diff-after");
    expect(afterDetails).toBeNull();
  });

  it("does NOT show Deleted Rule when before is undefined for delete", () => {
    render(RuleDiffCard, {
      props: {
        attachment: makeAttachment({
          operation: "delete",
          before: undefined,
        }),
      },
    });

    const card = screen.getByTestId("rule-diff-card");
    const details = card.querySelectorAll("details");
    expect(details.length).toBe(0);
  });
});

// ---------------------------------------------------------------------------
// Table-driven: all 3 operations with CSS class validation
// ---------------------------------------------------------------------------

describe("RuleDiffCard — table-driven operation rendering", () => {
  it.each([
    { operation: "create" as const },
    { operation: "update" as const },
    { operation: "delete" as const },
  ])(
    "operation='$operation' badge has CSS class and data-operation attribute matching the operation",
    ({ operation }) => {
      render(RuleDiffCard, {
        props: {
          attachment: makeAttachment({ operation }),
        },
      });

      const card = screen.getByTestId("rule-diff-card");
      // Check data-operation attribute
      const badge = card.querySelector(`[data-operation='${operation}']`);
      expect(badge).not.toBeNull();
      // Check CSS class
      const byClass = card.querySelector(
        `.${operation}, [class*='${operation}']`,
      );
      expect(byClass).not.toBeNull();
    },
  );
});

// ---------------------------------------------------------------------------
// Table-driven: rule name x operation combinations
// ---------------------------------------------------------------------------

describe("RuleDiffCard — rule name and operation both visible", () => {
  it.each([
    { ruleName: "email-routing", operation: "create" as const },
    { ruleName: "spam-filter", operation: "update" as const },
    { ruleName: "dedup-check", operation: "delete" as const },
  ])(
    "shows ruleName='$ruleName' and operation='$operation'",
    ({ ruleName, operation }) => {
      render(RuleDiffCard, {
        props: { attachment: makeAttachment({ ruleName, operation }) },
      });

      const card = screen.getByTestId("rule-diff-card");
      expect(card).toHaveTextContent(new RegExp(ruleName));
      expect(card).toHaveTextContent(new RegExp(operation, "i"));
    },
  );
});
