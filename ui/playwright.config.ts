import { defineConfig, devices } from "@playwright/test";

const E2E_UI_PORT = process.env.E2E_UI_PORT || "3000";

export default defineConfig({
  testDir: "e2e",

  // Run tests in files in parallel
  fullyParallel: true,

  // Fail the build on CI if you accidentally left test.only in the source code
  forbidOnly: !!process.env.CI,

  // Retry on CI only
  retries: process.env.CI ? 2 : 0,

  // Opt out of parallel tests on CI
  workers: process.env.CI ? 1 : undefined,

  // Reporter to use
  reporter: process.env.CI ? "github" : "list",

  // Global setup to check port availability and detect conflicts
  globalSetup: "./e2e/global-setup.ts",

  // Global teardown to cleanup Docker after tests
  globalTeardown: "./playwright.teardown.ts",

  // Start Docker Compose stack automatically
  webServer: {
    command: `E2E_UI_PORT=${E2E_UI_PORT} docker compose -f docker-compose.e2e.yml up --build`,
    url: `http://localhost:${E2E_UI_PORT}/health`,
    timeout: 120000,
    reuseExistingServer: !process.env.CI,
    stdout: "pipe",
    stderr: "pipe",
  },

  use: {
    baseURL: `http://localhost:${E2E_UI_PORT}`,

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
