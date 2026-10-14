// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("$env/dynamic/private", async () => ({
  env: (await import("./testkit")).mockEnv,
}));

import { readWorkConfig, WorkConfigError } from "./config";
import { setEnv } from "./testkit";

afterEach(() => setEnv({}));

describe("readWorkConfig", () => {
  it("is unconfigured when WORK_SOURCE is unset, with documented defaults", () => {
    setEnv({});
    expect(readWorkConfig()).toEqual({
      source: "unconfigured",
      fixtureSet: "board-mvp",
      portfolioPath: undefined,
      githubToken: undefined,
      githubApiUrl: "https://api.github.com",
      backendHost: "localhost:8080",
    });
  });

  it("reads every variable at call time", () => {
    setEnv({ WORK_SOURCE: "fixture", WORK_FIXTURE_SET: "other", BACKEND_HOST: "backend:8080" });
    expect(readWorkConfig()).toMatchObject({ source: "fixture", fixtureSet: "other", backendHost: "backend:8080" });
    setEnv({
      WORK_SOURCE: "github",
      SEMTEAMS_PORTFOLIO_PATH: "/etc/portfolio.json",
      GITHUB_TOKEN: "ghp_test",
      GITHUB_API_URL: "https://gh.example.test/",
    });
    expect(readWorkConfig()).toMatchObject({
      source: "github",
      portfolioPath: "/etc/portfolio.json",
      githubToken: "ghp_test",
      githubApiUrl: "https://gh.example.test",
    });
  });

  it("treats blank values as unset", () => {
    setEnv({ WORK_SOURCE: "  ", WORK_FIXTURE_SET: "", BACKEND_HOST: " " });
    expect(readWorkConfig()).toMatchObject({ source: "unconfigured", fixtureSet: "board-mvp", backendHost: "localhost:8080" });
  });

  it("refuses an unrecognised WORK_SOURCE instead of guessing", () => {
    setEnv({ WORK_SOURCE: "gitlab" });
    expect(() => readWorkConfig()).toThrow(WorkConfigError);
  });
});
