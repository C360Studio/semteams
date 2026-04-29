import { afterEach, beforeEach, describe, expect, it } from "vitest";

import {
  USER_IDENTITY_DEFAULT,
  USER_IDENTITY_STORAGE_KEY,
  userIdentity,
} from "./userIdentity.svelte";

describe("userIdentity store", () => {
  beforeEach(() => {
    // Each test starts from a clean localStorage and the default value.
    localStorage.removeItem(USER_IDENTITY_STORAGE_KEY);
    userIdentity.reset();
  });

  afterEach(() => {
    localStorage.removeItem(USER_IDENTITY_STORAGE_KEY);
    userIdentity.reset();
  });

  it("defaults to ui-anonymous", () => {
    expect(userIdentity.value).toBe(USER_IDENTITY_DEFAULT);
    expect(USER_IDENTITY_DEFAULT).toBe("ui-anonymous");
  });

  it("set persists to localStorage when non-default", () => {
    userIdentity.set("alice@example.com");
    expect(userIdentity.value).toBe("alice@example.com");
    expect(localStorage.getItem(USER_IDENTITY_STORAGE_KEY)).toBe(
      "alice@example.com",
    );
  });

  it("set with default value clears localStorage", () => {
    userIdentity.set("alice");
    expect(localStorage.getItem(USER_IDENTITY_STORAGE_KEY)).toBe("alice");
    userIdentity.set(USER_IDENTITY_DEFAULT);
    expect(localStorage.getItem(USER_IDENTITY_STORAGE_KEY)).toBeNull();
  });

  it("set trims whitespace", () => {
    userIdentity.set("  bob  ");
    expect(userIdentity.value).toBe("bob");
    expect(localStorage.getItem(USER_IDENTITY_STORAGE_KEY)).toBe("bob");
  });

  it("set with empty string resets to default", () => {
    userIdentity.set("alice");
    userIdentity.set("");
    expect(userIdentity.value).toBe(USER_IDENTITY_DEFAULT);
    expect(localStorage.getItem(USER_IDENTITY_STORAGE_KEY)).toBeNull();
  });

  it("set with whitespace-only string resets to default", () => {
    userIdentity.set("alice");
    userIdentity.set("   ");
    expect(userIdentity.value).toBe(USER_IDENTITY_DEFAULT);
  });

  it("reset clears stored identity and returns to default", () => {
    userIdentity.set("alice");
    userIdentity.reset();
    expect(userIdentity.value).toBe(USER_IDENTITY_DEFAULT);
    expect(localStorage.getItem(USER_IDENTITY_STORAGE_KEY)).toBeNull();
  });
});
