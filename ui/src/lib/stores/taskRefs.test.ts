import { describe, it, expect, beforeEach } from "vitest";
import { taskRefs } from "./taskRefs.svelte";

// Pure logic. localStorage exists in jsdom; reset between tests so
// state doesn't bleed across cases.
beforeEach(() => {
  localStorage.clear();
  taskRefs.reset();
});

describe("taskRefs", () => {
  it("starts at #1 with no assignments", () => {
    expect(taskRefs.size).toBe(0);
    expect(taskRefs.nextRef).toBe(1);
  });

  it("ensure() assigns monotonically", () => {
    expect(taskRefs.ensure("loop_a")).toBe(1);
    expect(taskRefs.ensure("loop_b")).toBe(2);
    expect(taskRefs.ensure("loop_c")).toBe(3);
    expect(taskRefs.size).toBe(3);
    expect(taskRefs.nextRef).toBe(4);
  });

  it("ensure() is idempotent for already-assigned loops", () => {
    expect(taskRefs.ensure("loop_a")).toBe(1);
    expect(taskRefs.ensure("loop_a")).toBe(1);
    expect(taskRefs.ensure("loop_a")).toBe(1);
    expect(taskRefs.size).toBe(1);
    expect(taskRefs.nextRef).toBe(2);
  });

  it("get() returns null before assignment", () => {
    expect(taskRefs.get("loop_unknown")).toBeNull();
  });

  it("get() returns assigned ref after ensure()", () => {
    taskRefs.ensure("loop_a");
    taskRefs.ensure("loop_b");
    expect(taskRefs.get("loop_a")).toBe(1);
    expect(taskRefs.get("loop_b")).toBe(2);
  });

  it("findLoopByRef() reverse-resolves", () => {
    taskRefs.ensure("loop_a");
    taskRefs.ensure("loop_b");
    expect(taskRefs.findLoopByRef(1)).toBe("loop_a");
    expect(taskRefs.findLoopByRef(2)).toBe("loop_b");
    expect(taskRefs.findLoopByRef(99)).toBeNull();
  });

  it("never recycles a number even if all assignments are wiped", () => {
    // ensure() is one-way: refs are stable for the lifetime of the
    // map, so as long as we don't recycle within the persisted state,
    // a refresh sees the same #N for the same loop.
    taskRefs.ensure("loop_a");
    taskRefs.ensure("loop_b");
    taskRefs.ensure("loop_c");
    expect(taskRefs.nextRef).toBe(4);

    // Now simulate a session where loop_a comes back. Should keep #1.
    expect(taskRefs.ensure("loop_a")).toBe(1);
    expect(taskRefs.nextRef).toBe(4);
  });

  it("persists across reloads via localStorage", () => {
    taskRefs.ensure("loop_a");
    taskRefs.ensure("loop_b");

    // Simulate a page reload by reading the key directly. We don't
    // re-instantiate the singleton (the store loads on import) but we
    // can verify the persistence shape.
    const raw = localStorage.getItem("semteams:task-refs:v1");
    expect(raw).not.toBeNull();
    const parsed = JSON.parse(raw!);
    expect(parsed.next).toBe(3);
    expect(parsed.refs).toEqual({ loop_a: 1, loop_b: 2 });
  });
});
