import { describe, test, expect } from "vitest";
import { classifyVerdict, verdictLabel } from "./verdict";

describe("classifyVerdict", () => {
  test("terminal-positive outcomes → approve", () => {
    for (const a of [
      "approved",
      "plan_approved",
      "respond_direct",
      "accepted",
    ]) {
      expect(classifyVerdict(a)).toBe("approve");
    }
  });

  test("negative outcomes → reject", () => {
    for (const a of [
      "rejected",
      "rejected_retry",
      "plan_rejected",
      "plan_rejected_retry",
      "failed",
      "blocked",
      "aborted",
      "insufficient",
    ]) {
      expect(classifyVerdict(a)).toBe("reject");
    }
  });

  test("hand-back outcomes → clarify", () => {
    for (const a of ["needs_clarification", "escalate"]) {
      expect(classifyVerdict(a)).toBe("clarify");
    }
  });

  test("pack routing tokens → route", () => {
    for (const a of [
      "research",
      "gather",
      "synthesize",
      "autoresearch",
      "propose",
      "measure",
      "measured",
      "emit",
      "planned",
      "dev_via_test",
      "dev_via_test_finalize",
    ]) {
      expect(classifyVerdict(a)).toBe("route");
    }
  });

  test("case-insensitive and trimmed", () => {
    expect(classifyVerdict("  APPROVED ")).toBe("approve");
    expect(classifyVerdict("Rejected")).toBe("reject");
  });

  test("undefined / empty → route", () => {
    expect(classifyVerdict(undefined)).toBe("route");
    expect(classifyVerdict("")).toBe("route");
  });
});

describe("verdictLabel", () => {
  test("snake_case → Title case", () => {
    expect(verdictLabel("respond_direct")).toBe("Respond direct");
    expect(verdictLabel("dev_via_test")).toBe("Dev via test");
    expect(verdictLabel("approved")).toBe("Approved");
  });

  test("undefined / empty → decided", () => {
    expect(verdictLabel(undefined)).toBe("decided");
    expect(verdictLabel("  ")).toBe("decided");
  });
});
