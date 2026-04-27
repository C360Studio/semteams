import { defineConfig, devices } from "@playwright/test";

/**
 * E2E journey tests against the agentic stack (mock-llm + dispatch +
 * loop + tools + governance). Each spec under `e2e/agentic/` pairs a
 * mock-llm fixture (test/fixtures/journeys/*.yaml) with a backend
 * config (configs/*.json), wired in via FIXTURE / AGENTIC_CONFIG env
 * vars by the per-scenario Taskfile entries.
 *
 * Stack lifecycle is owned by the Taskfile (task test:e2e:agentic:stack:up
 * → docker compose -f docker-compose.agentic-e2e.yml up). Playwright
 * does NOT auto-start the stack — env-var propagation through Vite's
 * Docker layer is unreliable when Playwright owns the lifecycle.
 *
 * Quick-start:
 *   task test:e2e:deep-research
 *   task test:e2e:tool-approval-gate
 *   task test:e2e:real-time-activity-stream
 *   task test:e2e:ops-agent
 */

const E2E_AGENTIC_UI_PORT = process.env.E2E_AGENTIC_UI_PORT || "3100";

export default defineConfig({
  testDir: "e2e/agentic",

  fullyParallel: true,

  forbidOnly: !!process.env.CI,

  retries: process.env.CI ? 2 : 0,

  // Single worker because the stack is single-tenant — sharing one
  // mock-llm across parallel tests would interleave fixture responses.
  workers: 1,

  reporter: process.env.CI ? "github" : "list",

  use: {
    baseURL: `http://localhost:${E2E_AGENTIC_UI_PORT}`,
    trace: "on-first-retry",
    screenshot: "only-on-failure",
    video: "retain-on-failure",
  },

  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
});
