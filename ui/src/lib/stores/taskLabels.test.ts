import { describe, it, expect, beforeEach } from "vitest";
import { taskLabels } from "./taskLabels.svelte";

beforeEach(() => {
  localStorage.clear();
  taskLabels.reset();
});

describe("taskLabels — title overrides", () => {
  it("returns null when no override is set", () => {
    expect(taskLabels.getTitle("loop_a")).toBeNull();
  });

  it("setTitle stores the override; getTitle returns it", () => {
    taskLabels.setTitle("loop_a", "My custom title");
    expect(taskLabels.getTitle("loop_a")).toBe("My custom title");
  });

  it("setTitle trims whitespace", () => {
    taskLabels.setTitle("loop_a", "  spaced out  ");
    expect(taskLabels.getTitle("loop_a")).toBe("spaced out");
  });

  it("setTitle with empty string clears the override", () => {
    taskLabels.setTitle("loop_a", "Original");
    taskLabels.setTitle("loop_a", "");
    expect(taskLabels.getTitle("loop_a")).toBeNull();
  });

  it("setTitle with whitespace-only clears the override", () => {
    taskLabels.setTitle("loop_a", "Original");
    taskLabels.setTitle("loop_a", "   ");
    expect(taskLabels.getTitle("loop_a")).toBeNull();
  });

  it("clearTitle removes the override", () => {
    taskLabels.setTitle("loop_a", "Original");
    taskLabels.clearTitle("loop_a");
    expect(taskLabels.getTitle("loop_a")).toBeNull();
  });

  it("titles for different loops are independent", () => {
    taskLabels.setTitle("loop_a", "A");
    taskLabels.setTitle("loop_b", "B");
    expect(taskLabels.getTitle("loop_a")).toBe("A");
    expect(taskLabels.getTitle("loop_b")).toBe("B");
  });
});

describe("taskLabels — aliases", () => {
  it("returns empty array by default", () => {
    expect(taskLabels.getAliases("loop_a")).toEqual([]);
  });

  it("addAlias appends and persists", () => {
    expect(taskLabels.addAlias("loop_a", "mqtt")).toBe(true);
    expect(taskLabels.getAliases("loop_a")).toEqual(["mqtt"]);
  });

  it("addAlias normalises (lowercase, strips @/#)", () => {
    taskLabels.addAlias("loop_a", "@MQTT");
    taskLabels.addAlias("loop_a", "#NATS");
    expect(taskLabels.getAliases("loop_a")).toEqual(["mqtt", "nats"]);
  });

  it("addAlias rejects duplicates across different loops", () => {
    taskLabels.addAlias("loop_a", "mqtt");
    expect(taskLabels.addAlias("loop_b", "mqtt")).toBe(false);
    expect(taskLabels.getAliases("loop_b")).toEqual([]);
  });

  it("addAlias is idempotent for already-attached aliases", () => {
    taskLabels.addAlias("loop_a", "mqtt");
    expect(taskLabels.addAlias("loop_a", "mqtt")).toBe(true);
    expect(taskLabels.getAliases("loop_a")).toEqual(["mqtt"]);
  });

  it("addAlias rejects empty input", () => {
    expect(taskLabels.addAlias("loop_a", "")).toBe(false);
    expect(taskLabels.addAlias("loop_a", "   ")).toBe(false);
    expect(taskLabels.addAlias("loop_a", "@@@")).toBe(false);
    expect(taskLabels.getAliases("loop_a")).toEqual([]);
  });

  it("removeAlias removes the alias", () => {
    taskLabels.addAlias("loop_a", "mqtt");
    taskLabels.addAlias("loop_a", "nats");
    taskLabels.removeAlias("loop_a", "mqtt");
    expect(taskLabels.getAliases("loop_a")).toEqual(["nats"]);
  });

  it("removeAlias is no-op for non-existent aliases", () => {
    taskLabels.removeAlias("loop_a", "nope");
    expect(taskLabels.getAliases("loop_a")).toEqual([]);
  });

  it("findLoopByAlias reverse-resolves", () => {
    taskLabels.addAlias("loop_a", "mqtt");
    taskLabels.addAlias("loop_b", "rust");
    expect(taskLabels.findLoopByAlias("mqtt")).toBe("loop_a");
    expect(taskLabels.findLoopByAlias("RUST")).toBe("loop_b");
    expect(taskLabels.findLoopByAlias("@mqtt")).toBe("loop_a");
    expect(taskLabels.findLoopByAlias("nope")).toBeNull();
  });
});

describe("taskLabels — persistence", () => {
  it("persists titles + aliases to localStorage", () => {
    taskLabels.setTitle("loop_a", "Custom");
    taskLabels.addAlias("loop_a", "mqtt");

    const raw = localStorage.getItem("semteams:task-labels:v1");
    expect(raw).not.toBeNull();
    const parsed = JSON.parse(raw!);
    expect(parsed.titles).toEqual({ loop_a: "Custom" });
    expect(parsed.aliases).toEqual({ loop_a: ["mqtt"] });
  });
});
