import { defineConfig, devices } from "@playwright/test";

/**
 * Playwright config for agentic journey tests.
 *
 * These tests exercise the full semteams agentic surface (agentic-dispatch,
 * agentic-loop, agentic-tools, agentic-governance) against a mock-llm server
 * configured via a YAML fixture. The stack is different from the default
 * Playwright stack (docker-compose.e2e.yml) — see docker-compose.agentic-e2e.yml
 * for the service list.
 *
 * Run with:
 *   npm run test:e2e:agentic
 *
 * Or from the repo root:
 *   task ui:test:e2e:agentic
 *
 * Each spec in `e2e/agentic/` is responsible for setting the `FIXTURE` env
 * var via a test fixture so mock-llm loads the right YAML. See
 * `e2e/agentic/helpers/agentic-fixtures.ts` for the shared fixture helper.
 */

const E2E_AGENTIC_UI_PORT = process.env.E2E_AGENTIC_UI_PORT || "3100";

export default defineConfig({
  testDir: "e2e/agentic",

  // Run tests in files in parallel
  fullyParallel: true,

  // Fail the build on CI if you accidentally left test.only in the source code
  forbidOnly: !!process.env.CI,

  // Retry on CI only
  retries: process.env.CI ? 2 : 0,

  // Single worker because the stack is single-tenant — sharing a mock-llm
  // across parallel tests would cause response-sequence interleaving.
  workers: 1,

  // Reporter to use
  reporter: process.env.CI ? "github" : "list",

  // Start Docker Compose stack automatically. Note: the stack uses a
  // different host port (3100 by default) than the non-agentic stack (3000)
  // so the two can coexist.
  webServer: {
    command: `E2E_AGENTIC_UI_PORT=${E2E_AGENTIC_UI_PORT} docker compose -f docker-compose.agentic-e2e.yml up --build`,
    url: `http://localhost:${E2E_AGENTIC_UI_PORT}/health`,
    timeout: 180000, // Longer than the default stack — mock-llm needs to build
    reuseExistingServer: !process.env.CI,
    stdout: "pipe",
    stderr: "pipe",
  },

  use: {
    baseURL: `http://localhost:${E2E_AGENTIC_UI_PORT}`,

    // Collect trace on failure for debugging
    trace: "on-first-retry",

    // Take screenshot on failure
    screenshot: "only-on-failure",

    // Capture video on failure
    video: "retain-on-failure",
  },

  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
});
